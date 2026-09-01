package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/errs"
	"github.com/divilla/apihydra/pkg/runner"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// ErrReporter classifies a failure to write execution output.
var ErrReporter = errors.New("reporting error")

// ErrTypeValidation labels a reported response-type mismatch.
var ErrTypeValidation = errors.New("type validation failed for")

// ErrStatusValidation labels a reported response-status mismatch.
var ErrStatusValidation = errors.New("response status does not match expected")

// ErrBodyValidation labels a reported response-body mismatch.
var ErrBodyValidation = errors.New("response body does not match expected")

var getTerminalSize = term.GetSize

var ansiSequencePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

// Reporter owns all human-readable execution output. The writer is normally
// os.Stdout and may be replaced by a buffer or another writer in tests.
// Reporter never writes fatal diagnostics to standard error; reporting
// failures are returned to the caller.
type Reporter struct {
	output         io.Writer
	terminal       bool
	terminalWidth  int
	terminalHeight int
	mu             sync.Mutex
	stage          *stageOutput
	stopped        bool
}

type stageOutput struct {
	order           []*domain.StepsDefinition
	files           map[*domain.StepsDefinition]*fileOutput
	debug           string
	renderedContent string
	renderedRows    int
	renderedWidth   int
	renderedHeight  int
	rendered        bool
	implicit        bool
}

type fileOutput struct {
	block       strings.Builder
	failed      bool
	failedSteps map[int]struct{}
}

type debugStep struct {
	Index    int                          `json:"index"`
	Vars     map[string]domain.YAMLString `json:"vars"`
	Request  debugRequest                 `json:"request"`
	Response debugResponse                `json:"response"`
	Debug    bool                         `json:"debug"`
}

type debugRequest struct {
	Path     string          `json:"path"`
	Method   string          `json:"method"`
	Query    string          `json:"query"`
	Body     any             `json:"body"`
	Defaults domain.Defaults `json:"defaults"`
}

type debugResponse struct {
	ExpectedStatus int                          `json:"expected_status"`
	ActualStatus   int                          `json:"actual_status"`
	ExpectedBody   any                          `json:"expected_body"`
	ActualBody     any                          `json:"actual_body"`
	ExpectedTypes  map[string][]string          `json:"expected_types"`
	Capture        map[string]domain.YAMLString `json:"capture"`
}

// NewReporter returns a Reporter that serializes writes to output. terminal
// selects live ANSI redraws; non-terminal output is buffered by stage and
// written once at the stage barrier.
func NewReporter(output io.Writer, terminal bool) *Reporter {
	reporter := &Reporter{
		output:         output,
		terminal:       terminal,
		terminalWidth:  80,
		terminalHeight: 24,
	}
	if terminal {
		reporter.refreshTerminalDimensionsLocked()
	}
	return reporter
}

// WorkingDirectory writes the selected working directory to the injected
// writer.
func (r *Reporter) WorkingDirectory(workDir string) error {
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if _, err := fmt.Fprintf(r.output, "Working Directory: %s\n\n", workDir); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
}

// BeginStage starts one ordered reporting transaction. directories are in
// PlanStages order, and each directory's StepsDefinitions order defines file
// order. Reporter retains one buffer per steps definition. On a terminal, each
// later reporting event clears and redraws only the active stage region in
// directory/file/step order; the working-directory heading and completed
// stages remain fixed. BeginStage itself produces no stage output.
func (r *Reporter) BeginStage(ctx context.Context, directories []*domain.Directory) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if err := r.contextError(ctx); err != nil {
		return err
	}

	stage := &stageOutput{files: make(map[*domain.StepsDefinition]*fileOutput)}
	for _, directory := range directories {
		if directory == nil {
			continue
		}
		for _, definition := range directory.StepsDefinitions {
			stage.order = append(stage.order, definition)
			stage.files[definition] = newFileOutput()
		}
	}
	r.stage = stage
	return nil
}

// EndStage commits the active stage. On non-terminal output it writes the
// complete stage exactly once in directory/file/step order. On a terminal it
// leaves the final redraw in place without duplicating it. After EndStage,
// Reporter never rewrites output belonging to that completed stage.
func (r *Reporter) EndStage(ctx context.Context) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.contextError(ctx); err != nil {
		return err
	}
	if r.stage == nil {
		return nil
	}
	if !r.terminal && !r.stage.implicit {
		if err := r.writeLocked(r.stage.render()); err != nil {
			return err
		}
	}
	r.stage = nil
	return nil
}

// Success records one steps definition whose execution completed without
// validation failures. Its file buffer is redrawn immediately on terminals or
// retained until EndStage on non-terminals. It returns a reporting error
// without terminating execution.
func (r *Reporter) Success(ctx context.Context, definition *domain.StepsDefinition) error {
	return r.updateFile(ctx, definition, func(file *fileOutput) {
		if file.failed {
			return
		}
		fmt.Fprintf(&file.block, "[\x1b[38;5;10m✓\x1b[0m] %s\n", definitionReference(definition))
	})
}

// ValidationTypes writes the failed output from nonfatal response-type
// validation to the injected standard-output writer. It returns only reporting
// failures; the validation failure itself does not terminate execution.
func (r *Reporter) ValidationTypes(ctx context.Context, step *domain.Step, failed string) error {
	return r.writeValidation(ctx, step, formatExpectedTypes(step, failed))
}

// ValidationStatus writes one nonfatal response-status validation failure to
// the injected standard-output writer. It returns only reporting failures; the
// validation failure itself does not terminate execution.
func (r *Reporter) ValidationStatus(ctx context.Context, step *domain.Step, failure error) error {
	actualStatus := 0
	expectedStatus := 0
	if step != nil {
		actualStatus = step.Response.ActualStatus
		expectedStatus = step.Response.ExpectedStatus
	}
	return r.writeValidation(ctx, step, fmt.Sprintf(
		"    actual_status: \x1b[38;5;210m%d\x1b[0m\n    expected_status: \x1b[38;5;10m%d\x1b[0m\n",
		actualStatus,
		expectedStatus,
	))
}

// ValidationBody writes one nonfatal response-body validation diff to the
// injected standard-output writer. Any command colors carried by diff are
// preserved when the output block is rendered. It returns only reporting
// failures; the validation failure itself does not terminate execution.
func (r *Reporter) ValidationBody(ctx context.Context, step *domain.Step, diff string) error {
	return r.writeValidation(ctx, step, formatExpectedBody(diff))
}

func formatExpectedTypes(step *domain.Step, failed string) string {
	expectedTypes := map[string][]string(nil)
	if step != nil {
		expectedTypes = step.Response.ExpectedTypes
	}

	selectors := failedTypeSelectors(failed)
	if len(selectors) == 0 {
		selectors = make([]string, 0, len(expectedTypes))
		for selector := range expectedTypes {
			selectors = append(selectors, selector)
		}
		slices.Sort(selectors)
	}

	var block strings.Builder
	block.WriteString("    expected_types:\n")
	for _, selector := range selectors {
		expected, ok := expectedTypes[selector]
		if !ok {
			continue
		}
		fmt.Fprintf(
			&block,
			"        \x1b[38;5;15m%s:\x1b[0m \x1b[38;5;210m[%s]\x1b[0m\n",
			selector,
			strings.Join(expected, ", "),
		)
	}
	return block.String()
}

func failedTypeSelectors(failed string) []string {
	decoder := json.NewDecoder(strings.NewReader(failed))
	selectors := make([]string, 0)
	for {
		var declaration struct {
			Selector string `json:"selector"`
		}
		err := decoder.Decode(&declaration)
		if errors.Is(err, io.EOF) {
			return selectors
		}
		if err != nil {
			return nil
		}
		if declaration.Selector != "" {
			selectors = append(selectors, declaration.Selector)
		}
	}
}

func formatExpectedBody(diff string) string {
	var block strings.Builder
	block.WriteString("    expected_body:\n")
	trimmed := strings.TrimRight(diff, "\r\n")
	if trimmed == "" {
		return block.String()
	}
	for _, line := range strings.Split(trimmed, "\n") {
		block.WriteString("        ")
		block.WriteString(line)
		block.WriteByte('\n')
	}
	return block.String()
}

// Debug records the latest runtime state of a selected debug step with exactly
// these fields and blank lines:
//
//	stage: <Step.DirectoryStage()>
//	dir-path: <Step.DirectoryPath()>
//	file-path: <Step.FilePath()>
//
//	curl-command:
//	<Step.RawCurl>
//
//	<prettified-and-ANSI-colored-Step-JSON>
//
// RawCurl and Definition remain absent from the Step JSON according to their
// JSON tags. Debug preserves every other Step member and value, projecting only
// Request.Body, Response.ExpectedBody, and Response.ActualBody for display:
// valid JSON strings are embedded as JSON values, while empty or invalid JSON
// remains encoded as a string. It neither redacts nor omits data. Debug is kept
// outside the per-file buffers and rendered after every previously accumulated
// file block, so it is the final stdout block even when its file is not last in
// plan order. It atomically suppresses all later reporting calls after
// successfully recording and, on a terminal, redrawing the complete block. On
// non-terminals EndStage performs the single final write. It returns a
// reporting error without terminating execution.
func (r *Reporter) Debug(ctx context.Context, step *domain.Step) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if err := r.contextError(ctx); err != nil {
		return err
	}

	payload, err := json.Marshal(debugStepValue(step))
	if err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	pretty, _, err := runner.JQPretty(ctx, string(payload))
	if err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	block := fmt.Sprintf(
		"stage: %d\ndir-path: %s\nfile-path: %s\n\ncurl-command:\n%s\n\n%s\n",
		step.DirectoryStage(),
		step.DirectoryPath(),
		step.FilePath(),
		step.RawCurl,
		colorizeJQJSON(pretty),
	)
	stage := r.ensureStageLocked()
	previous := stage.debug
	stage.debug = block
	if r.terminal {
		if err := r.redrawLocked(); err != nil {
			stage.debug = previous
			return err
		}
	} else if stage.implicit {
		if err := r.writeLocked(block); err != nil {
			stage.debug = previous
			return err
		}
	}
	r.stopped = true
	return nil
}

func debugStepValue(step *domain.Step) any {
	if step == nil {
		return nil
	}
	return debugStep{
		Index: step.Index,
		Vars:  step.Vars,
		Request: debugRequest{
			Path:     step.Request.Path,
			Method:   step.Request.Method,
			Query:    step.Request.Query,
			Body:     debugBodyValue(step.Request.Body),
			Defaults: step.Request.Defaults,
		},
		Response: debugResponse{
			ExpectedStatus: step.Response.ExpectedStatus,
			ActualStatus:   step.Response.ActualStatus,
			ExpectedBody:   debugBodyValue(step.Response.ExpectedBody),
			ActualBody:     debugBodyValue(step.Response.ActualBody),
			ExpectedTypes:  step.Response.ExpectedTypes,
			Capture:        step.Response.Capture,
		},
		Debug: step.Debug,
	}
}

func debugBodyValue(body domain.YAMLString) any {
	if json.Valid([]byte(body)) {
		return json.RawMessage(body)
	}
	return body
}

func colorizeJQJSON(input string) string {
	const (
		reset       = "\x1b[0m"
		punctuation = "\x1b[1;39m"
		key         = "\x1b[1;34m"
		stringValue = "\x1b[0;32m"
		scalar      = "\x1b[0;39m"
		nullValue   = "\x1b[0;90m"
	)

	var colored strings.Builder
	for index := 0; index < len(input); {
		switch input[index] {
		case ' ', '\t', '\r', '\n':
			colored.WriteByte(input[index])
			index++
		case '"':
			end := index + 1
			for end < len(input) {
				if input[end] == '\\' {
					end += 2
					continue
				}
				end++
				if input[end-1] == '"' {
					break
				}
			}
			next := end
			for next < len(input) && (input[next] == ' ' || input[next] == '\t') {
				next++
			}
			color := stringValue
			if next < len(input) && input[next] == ':' {
				color = key
			}
			colored.WriteString(color)
			colored.WriteString(input[index:end])
			colored.WriteString(reset)
			index = end
		case '{', '}', '[', ']', ',', ':':
			colored.WriteString(punctuation)
			colored.WriteByte(input[index])
			colored.WriteString(reset)
			index++
		default:
			end := index
			for end < len(input) && !strings.ContainsRune(" \t\r\n,]}:", rune(input[end])) {
				end++
			}
			color := scalar
			if input[index:end] == "null" {
				color = nullValue
			}
			colored.WriteString(color)
			colored.WriteString(input[index:end])
			colored.WriteString(reset)
			index = end
		}
	}
	return colored.String()
}

func (r *Reporter) writeValidation(ctx context.Context, step *domain.Step, validation string) error {
	var definition *domain.StepsDefinition
	if step != nil {
		definition = step.Definition
	}
	stepIndex := 0
	if step != nil {
		stepIndex = step.Index
	}
	return r.updateFile(ctx, definition, func(file *fileOutput) {
		if !file.failed {
			fmt.Fprintf(&file.block, "[\x1b[38;5;210m✗\x1b[0m] %s\n", definitionReference(definition))
			file.failed = true
		}
		if _, reported := file.failedSteps[stepIndex]; !reported {
			fmt.Fprintf(
				&file.block,
				"[\x1b[38;5;210m✗\x1b[0m] %s %s \x1b[36mstep-%d\x1b[0m\n",
				calculatedPath(step),
				effectiveMethod(step),
				stepIndex+1,
			)
			file.failedSteps[stepIndex] = struct{}{}
		}
		file.block.WriteString(validation)
		if final, _ := ctx.Value(r).(bool); final {
			file.block.WriteByte('\n')
		}
	})
}

func newFileOutput() *fileOutput {
	return &fileOutput{failedSteps: make(map[int]struct{})}
}

func (s *stageOutput) render() string {
	if s == nil {
		return ""
	}
	var output strings.Builder
	for _, definition := range s.order {
		if file := s.files[definition]; file != nil {
			output.WriteString(file.block.String())
		}
	}
	output.WriteString(s.debug)
	return output.String()
}

func (s *stageOutput) implicitSnapshot() string {
	if s == nil || !s.implicit {
		return ""
	}
	return s.render()
}

func (r *Reporter) updateFile(
	ctx context.Context,
	definition *domain.StepsDefinition,
	update func(*fileOutput),
) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if err := r.contextError(ctx); err != nil {
		return err
	}

	stage := r.ensureStageLocked()
	before := stage.implicitSnapshot()
	file := stage.files[definition]
	if file == nil {
		file = newFileOutput()
		stage.files[definition] = file
		stage.order = append(stage.order, definition)
	}
	update(file)
	if r.terminal {
		return r.redrawLocked()
	}
	if stage.implicit {
		after := stage.render()
		return r.writeLocked(strings.TrimPrefix(after, before))
	}
	return nil
}

func (r *Reporter) ensureStageLocked() *stageOutput {
	if r.stage == nil {
		r.stage = &stageOutput{
			files:    make(map[*domain.StepsDefinition]*fileOutput),
			implicit: true,
		}
	}
	return r.stage
}

func (r *Reporter) redrawLocked() error {
	stage := r.ensureStageLocked()
	r.refreshTerminalDimensionsLocked()
	content := stage.render()
	var redraw strings.Builder
	if stage.rendered {
		previousRows := stage.renderedRows
		if stage.renderedWidth != r.terminalWidth || stage.renderedHeight != r.terminalHeight {
			previousRows = r.visibleRowsToCursor(stage.renderedContent)
		}
		if previousRows > 0 {
			fmt.Fprintf(&redraw, "\x1b[%dA", previousRows)
		}
		redraw.WriteString("\r\x1b[J")
	}
	redraw.WriteString(content)
	if err := r.writeLocked(redraw.String()); err != nil {
		return err
	}
	stage.renderedContent = content
	stage.renderedRows = r.visibleRowsToCursor(content)
	stage.renderedWidth = r.terminalWidth
	stage.renderedHeight = r.terminalHeight
	stage.rendered = true
	return nil
}

func (r *Reporter) refreshTerminalDimensionsLocked() {
	if descriptor, ok := r.output.(interface{ Fd() uintptr }); ok {
		width, height, err := getTerminalSize(int(descriptor.Fd()))
		if err == nil && width > 0 && height > 0 {
			r.terminalWidth = width
			r.terminalHeight = height
		}
	}
}

func (r *Reporter) visibleRowsToCursor(content string) int {
	rows := visualRowsToCursor(content, r.terminalWidth)
	if r.terminalHeight > 0 && rows >= r.terminalHeight {
		return r.terminalHeight - 1
	}
	return rows
}

func visualRowsToCursor(content string, width int) int {
	if content == "" || width <= 0 {
		return 0
	}

	content = ansiSequencePattern.ReplaceAllString(content, "")
	rows := 0
	columns := 0
	textStart := 0
	for index := 0; index < len(content); index++ {
		switch content[index] {
		case '\n':
			columns += runewidth.StringWidth(content[textStart:index])
			rows += wrappedRows(columns, width) + 1
			columns = 0
			textStart = index + 1
		case '\r':
			columns += runewidth.StringWidth(content[textStart:index])
			rows, columns = rows+wrappedRows(columns, width), 0
			textStart = index + 1
		case '\t':
			columns += runewidth.StringWidth(content[textStart:index])
			columns += 8 - columns%8
			textStart = index + 1
		}
	}
	columns += runewidth.StringWidth(content[textStart:])
	if !strings.HasSuffix(content, "\n") {
		rows += wrappedRows(columns, width)
	}
	return rows
}

func wrappedRows(columns, width int) int {
	return max(1, (columns+width-1)/width) - 1
}

func (r *Reporter) writeLocked(content string) error {
	written, err := io.WriteString(r.output, content)
	if err == nil && written != len(content) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
}

func (r *Reporter) checkContext(ctx context.Context) error {
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	return r.contextError(ctx)
}

func (r *Reporter) contextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
}

func definitionReference(definition *domain.StepsDefinition) string {
	reference := "<unknown definition>"
	if definition != nil && definition.File != nil && definition.File.Path != "" {
		reference = filepath.ToSlash(definition.File.Path)
		reference = strings.TrimSuffix(reference, filepath.Ext(reference))
		if !strings.HasPrefix(reference, "/") {
			reference = "/" + reference
		}
	}
	return reference
}

func calculatedPath(step *domain.Step) string {
	path := "<unknown path>"
	if step != nil {
		path = step.Request.Defaults.BasePath + step.Request.Path
		if path == "" {
			path = "/"
		}
	}
	return path
}

func effectiveMethod(step *domain.Step) string {
	method := "<unknown method>"
	if step != nil {
		method = step.Request.Method
		if method == "" {
			method = "GET"
			if step.Request.Body != "" {
				method = "POST"
			}
		}
	}
	return method
}
