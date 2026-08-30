package reporting

import (
	"apih/internal/domain"
	"apih/pkg/errs"
	"apih/pkg/runner"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// ErrReporter classifies a failure to write execution output.
var ErrReporter = errors.New("reporting error")

// ErrTypeValidation labels a reported response-type mismatch.
var ErrTypeValidation = errors.New("type validation failed for")

// ErrStatusValidation labels a reported response-status mismatch.
var ErrStatusValidation = errors.New("response status does not match expected")

// ErrBodyValidation labels a reported response-body mismatch.
var ErrBodyValidation = errors.New("response body does not match expected")

// Reporter owns all human-readable execution output. The writer is normally
// os.Stdout and may be replaced by a buffer or another writer in tests.
// Reporter never writes fatal diagnostics to standard error; reporting
// failures are returned to the caller.
type Reporter struct {
	output            io.Writer
	mu                sync.Mutex
	failedDefinitions map[*domain.StepsDefinition]struct{}
	failedSteps       map[failedStepKey]struct{}
	stopped           bool
}

type failedStepKey struct {
	definition *domain.StepsDefinition
	index      int
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

// NewReporter returns a Reporter that serializes writes to output.
func NewReporter(output io.Writer) *Reporter {
	return &Reporter{
		output:            output,
		failedDefinitions: make(map[*domain.StepsDefinition]struct{}),
		failedSteps:       make(map[failedStepKey]struct{}),
	}
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

// Success reports every definition file in a directory whose execution
// completed without validation failures. It returns a reporting error without
// terminating execution.
func (r *Reporter) Success(ctx context.Context, directory *domain.Directory) error {
	return r.writeGenerated(ctx, func() string {
		var block strings.Builder
		if directory != nil {
			for _, definition := range directory.StepsDefinitions {
				if _, failed := r.failedDefinitions[definition]; failed {
					continue
				}
				fmt.Fprintf(&block, "[\x1b[38;5;10m✓\x1b[0m] %s\n", definitionReference(definition))
			}
		}
		return block.String()
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

// Debug reports the latest runtime state of a selected debug step to the
// injected standard-output writer with exactly these fields and blank lines:
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
// remains encoded as a string. It neither redacts nor omits data. Debug
// atomically suppresses all later reporting calls after successfully writing
// the complete block. It returns a reporting error without terminating
// execution.
func (r *Reporter) Debug(ctx context.Context, step *domain.Step) error {
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
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
	written, err := io.WriteString(r.output, block)
	if err == nil && written != len(block) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
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

func (r *Reporter) writeGenerated(ctx context.Context, generate func() string) error {
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}

	block := generate()
	written, err := io.WriteString(r.output, block)
	if err == nil && written != len(block) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
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
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}

	var definition *domain.StepsDefinition
	if step != nil {
		definition = step.Definition
	}
	key := failedStepKey{definition: definition}
	if step != nil {
		key.index = step.Index
	}

	var block strings.Builder
	if _, reported := r.failedDefinitions[definition]; !reported {
		fmt.Fprintf(&block, "[\x1b[38;5;210m✗\x1b[0m] %s\n", definitionReference(definition))
		r.failedDefinitions[definition] = struct{}{}
	}
	if _, reported := r.failedSteps[key]; !reported {
		fmt.Fprintf(
			&block,
			"[\x1b[38;5;210m✗\x1b[0m] %s %s \x1b[36mstep-%d\x1b[0m\n",
			calculatedPath(step),
			effectiveMethod(step),
			key.index+1,
		)
		r.failedSteps[key] = struct{}{}
	}
	block.WriteString(validation)
	if final, _ := ctx.Value(r).(bool); final {
		block.WriteByte('\n')
	}

	written, err := io.WriteString(r.output, block.String())
	if err == nil && written != block.Len() {
		err = io.ErrShortWrite
	}
	if err != nil {
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
