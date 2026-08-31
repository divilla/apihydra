package reporting

import (
	"apih/internal/domain"
	"apih/pkg/errs"
	"apih/pkg/runner"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type shortWriter struct{}

func (shortWriter) Write(payload []byte) (int, error) {
	return len(payload) - 1, nil
}

type descriptorWriter struct {
	bytes.Buffer
	fd uintptr
}

func (w *descriptorWriter) Fd() uintptr {
	return w.fd
}

type divergentStringWriter struct {
	output            bytes.Buffer
	writeCalled       bool
	writeStringCalled bool
	writeStringErr    error
}

func (w *divergentStringWriter) Write(payload []byte) (int, error) {
	w.writeCalled = true
	return w.output.Write(payload)
}

func (w *divergentStringWriter) WriteString(string) (int, error) {
	w.writeStringCalled = true
	return 0, w.writeStringErr
}

type yieldingBuffer struct {
	mu     sync.Mutex
	output bytes.Buffer
}

func (w *yieldingBuffer) Write(payload []byte) (int, error) {
	for _, value := range payload {
		w.mu.Lock()
		if err := w.output.WriteByte(value); err != nil {
			w.mu.Unlock()
			return 0, err
		}
		w.mu.Unlock()
		runtime.Gosched()
	}
	return len(payload), nil
}

func (w *yieldingBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.String()
}

type gateWriter struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	output  bytes.Buffer
}

func (w *gateWriter) Write(payload []byte) (int, error) {
	select {
	case w.started <- struct{}{}:
		<-w.release
	default:
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.Write(payload)
}

func (w *gateWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.String()
}

func TestReporterExportedContract(t *testing.T) {
	var _ func(io.Writer, bool) *Reporter = NewReporter
	var _ func(*Reporter, string) error = (*Reporter).WorkingDirectory
	var _ func(*Reporter, context.Context, []*domain.Directory) error = (*Reporter).BeginStage
	var _ func(*Reporter, context.Context) error = (*Reporter).EndStage
	var _ func(*Reporter, context.Context, *domain.StepsDefinition) error = (*Reporter).Success
	var _ func(*Reporter, context.Context, *domain.Step, string) error = (*Reporter).ValidationTypes
	var _ func(*Reporter, context.Context, *domain.Step, error) error = (*Reporter).ValidationStatus
	var _ func(*Reporter, context.Context, *domain.Step, string) error = (*Reporter).ValidationBody
	var _ func(*Reporter, context.Context, *domain.Step) error = (*Reporter).Debug

	labels := map[error]string{
		ErrReporter:         "reporting error",
		ErrTypeValidation:   "type validation failed for",
		ErrStatusValidation: "response status does not match expected",
		ErrBodyValidation:   "response body does not match expected",
	}
	for label, want := range labels {
		if got := label.Error(); got != want {
			t.Errorf("error label = %q, want %q", got, want)
		}
	}
}

func TestWorkingDirectoryUsesInjectedWriterByteExactly(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output, false)

	if err := report.WorkingDirectory("/work"); err != nil {
		t.Fatalf("WorkingDirectory() error = %v", err)
	}
	if got, want := output.String(), "Working Directory: /work\n\n"; got != want {
		t.Fatalf("WorkingDirectory() output = %q, want %q", got, want)
	}
}

func TestNewReporterUsesTerminalDimensionsWhenAvailable(t *testing.T) {
	previous := getTerminalSize
	getTerminalSize = func(fd int) (int, int, error) {
		if fd != 17 {
			t.Fatalf("terminal descriptor = %d, want 17", fd)
		}
		return 132, 43, nil
	}
	t.Cleanup(func() { getTerminalSize = previous })

	report := NewReporter(&descriptorWriter{fd: 17}, true)
	if report.terminalWidth != 132 || report.terminalHeight != 43 {
		t.Fatalf("terminal dimensions = %dx%d, want 132x43", report.terminalWidth, report.terminalHeight)
	}
}

func TestWorkingDirectoryPreservesReferenceWritePath(t *testing.T) {
	writer := &divergentStringWriter{writeStringErr: errors.New("WriteString must not be called")}

	if err := NewReporter(writer, false).WorkingDirectory("/work"); err != nil {
		t.Fatalf("WorkingDirectory() error = %v", err)
	}
	if !writer.writeCalled {
		t.Fatal("WorkingDirectory() did not call Write")
	}
	if writer.writeStringCalled {
		t.Fatal("WorkingDirectory() called WriteString, want reference Write path")
	}
	if got, want := writer.output.String(), "Working Directory: /work\n\n"; got != want {
		t.Fatalf("WorkingDirectory() output = %q, want %q", got, want)
	}
}

func TestWorkingDirectoryPreservesReferenceShortWriteHandling(t *testing.T) {
	if err := NewReporter(shortWriter{}, false).WorkingDirectory("/work"); err != nil {
		t.Fatalf("WorkingDirectory() error = %v, want nil from reference fmt.Fprintf path", err)
	}
}

func TestReporterWritesChosenOutputBlocks(t *testing.T) {
	step := reporterStep("suite/steps.yaml", 3)
	step.Request.Method = "GET"
	step.Request.Body = `{"request":true}`
	step.Response.ExpectedStatus = 201
	step.Response.ActualStatus = 500
	step.Response.ExpectedBody = `{"message":"wanted"}`
	step.Response.ActualBody = `{"message":"failed"}`
	step.Response.ExpectedTypes = map[string][]string{".id": {"string"}}
	step.Debug = true
	step.RawCurl = "curl --header Authorization: Bearer complete-secret --header Cookie: session=visible"

	debugJSON, err := json.Marshal(debugStepValue(step))
	if err != nil {
		t.Fatalf("marshal debug step: %v", err)
	}
	prettyDebugJSON, _, err := runner.JQPretty(context.Background(), string(debugJSON))
	if err != nil {
		t.Fatalf("jq debug step: %v", err)
	}
	tests := map[string]struct {
		call func(*Reporter) error
		want string
	}{
		"success": {
			call: func(report *Reporter) error { return report.Success(context.Background(), step.Definition) },
			want: "[\x1b[38;5;10m✓\x1b[0m] /suite/steps\n",
		},
		"types": {
			call: func(report *Reporter) error {
				return report.ValidationTypes(finalValidationContext(report), step, `{"selector":".id","actual":"null"}`)
			},
			want: "[\x1b[38;5;210m✗\x1b[0m] /suite/steps\n[\x1b[38;5;210m✗\x1b[0m] / GET \x1b[36mstep-4\x1b[0m\n    expected_types:\n        \x1b[38;5;15m.id:\x1b[0m \x1b[38;5;210m[string]\x1b[0m\n\n",
		},
		"status": {
			call: func(report *Reporter) error {
				return report.ValidationStatus(finalValidationContext(report), step, errors.New("validation error"))
			},
			want: "[\x1b[38;5;210m✗\x1b[0m] /suite/steps\n[\x1b[38;5;210m✗\x1b[0m] / GET \x1b[36mstep-4\x1b[0m\n    actual_status: \x1b[38;5;210m500\x1b[0m\n    expected_status: \x1b[38;5;10m201\x1b[0m\n\n",
		},
		"body": {
			call: func(report *Reporter) error {
				return report.ValidationBody(finalValidationContext(report), step, "- actual\n+ expected")
			},
			want: "[\x1b[38;5;210m✗\x1b[0m] /suite/steps\n[\x1b[38;5;210m✗\x1b[0m] / GET \x1b[36mstep-4\x1b[0m\n    expected_body:\n        - actual\n        + expected\n\n",
		},
		"debug": {
			call: func(report *Reporter) error { return report.Debug(context.Background(), step) },
			want: fmt.Sprintf(
				"stage: 0\ndir-path: /suite\nfile-path: suite/steps.yaml\n\ncurl-command:\n%s\n\n%s\n",
				step.RawCurl,
				colorizeJQJSON(prettyDebugJSON),
			),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			if err := test.call(NewReporter(&output, false)); err != nil {
				t.Fatalf("reporting error = %v", err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDebugProjectsValidJSONBodiesWithoutTransformingOtherValues(t *testing.T) {
	step := reporterStep("private/steps.yaml", 2)
	step.Request.Body = `{"b":2,"a":1}`
	step.Request.Defaults = domain.Defaults{
		BaseURL:  "https://example.test",
		BasePath: "/api",
		Headers: map[string]string{
			"Authorization": "Bearer complete-secret",
			"Cookie":        "session=unredacted-cookie",
		},
		Timeout: 10,
		Retries: 3,
	}
	step.Response.ExpectedBody = `{"expected":true}`
	step.Response.ActualBody = `{"actual":true}`
	step.RawCurl = "curl --header Authorization: Bearer complete-secret --header Cookie: session=unredacted-cookie"
	var output bytes.Buffer

	if err := NewReporter(&output, false).Debug(context.Background(), step); err != nil {
		t.Fatalf("Debug() error = %v", err)
	}

	payload, err := json.Marshal(debugStepValue(step))
	if err != nil {
		t.Fatal(err)
	}
	pretty, _, err := runner.JQPretty(context.Background(), string(payload))
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(
		"stage: 0\ndir-path: /suite\nfile-path: private/steps.yaml\n\ncurl-command:\n%s\n\n%s\n",
		step.RawCurl,
		colorizeJQJSON(pretty),
	)
	if got := output.String(); got != want {
		t.Fatalf("Debug() output = %q, want exact projected Step dump %q", got, want)
	}
	var projected struct {
		Request struct {
			Body map[string]any `json:"body"`
		} `json:"request"`
		Response struct {
			ExpectedBody map[string]any `json:"expected_body"`
			ActualBody   map[string]any `json:"actual_body"`
		} `json:"response"`
	}
	if err := json.Unmarshal(payload, &projected); err != nil {
		t.Fatalf("decode projected Debug JSON: %v", err)
	}
	if projected.Request.Body["a"] != float64(1) || projected.Request.Body["b"] != float64(2) ||
		projected.Response.ExpectedBody["expected"] != true || projected.Response.ActualBody["actual"] != true {
		t.Fatalf("projected Debug bodies = request %#v, expected %#v, actual %#v", projected.Request.Body, projected.Response.ExpectedBody, projected.Response.ActualBody)
	}
	for _, complete := range []string{"Bearer complete-secret", "session=unredacted-cookie"} {
		if !strings.Contains(output.String(), complete) {
			t.Fatalf("Debug() output omitted sensitive value %q: %q", complete, output.String())
		}
	}
	if strings.Contains(output.String(), "raw_curl") || strings.Contains(output.String(), "definition") {
		t.Fatalf("Debug() Step JSON contains runtime-only field: %q", output.String())
	}
}

func TestDebugPreservesInvalidAndEmptyBodiesAsStrings(t *testing.T) {
	step := reporterStep("invalid/steps.yaml", 0)
	step.Request.Body = "not-json"
	step.Response.ExpectedBody = ""
	step.Response.ActualBody = "plain response"

	payload, err := json.Marshal(debugStepValue(step))
	if err != nil {
		t.Fatal(err)
	}
	var projected struct {
		Request struct {
			Body any `json:"body"`
		} `json:"request"`
		Response struct {
			ExpectedBody any `json:"expected_body"`
			ActualBody   any `json:"actual_body"`
		} `json:"response"`
	}
	if err := json.Unmarshal(payload, &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Request.Body != "not-json" || projected.Response.ExpectedBody != "" || projected.Response.ActualBody != "plain response" {
		t.Fatalf("projected invalid bodies = request %#v, expected %#v, actual %#v", projected.Request.Body, projected.Response.ExpectedBody, projected.Response.ActualBody)
	}
}

func TestDebugStepValuePreservesNil(t *testing.T) {
	if got := debugStepValue(nil); got != nil {
		t.Fatalf("debugStepValue(nil) = %#v, want nil", got)
	}
}

func TestReporterGroupsEveryValidationForOneStepUnderOneFailureHeader(t *testing.T) {
	step := reporterStep("change/create.yaml", 2)
	step.Request.Defaults.BasePath = "/api/v1"
	step.Request.Path = "/change/create"
	step.Request.Body = `{}`
	step.Response.ExpectedTypes = map[string][]string{
		".change_types": {"array"},
		".version":      {"number", "null"},
	}
	var output bytes.Buffer
	report := NewReporter(&output, false)

	failedTypes := "{\"selector\":\".version\",\"actual\":\"string\"}\n" +
		"{\"selector\":\".change_types\",\"actual\":\"object\"}"
	if err := report.ValidationTypes(context.Background(), step, failedTypes); err != nil {
		t.Fatalf("ValidationTypes() error = %v", err)
	}
	if err := report.ValidationBody(finalValidationContext(report), step, "- actual\n+ expected"); err != nil {
		t.Fatalf("ValidationBody() error = %v", err)
	}

	want := "[\x1b[38;5;210m✗\x1b[0m] /change/create\n" +
		"[\x1b[38;5;210m✗\x1b[0m] /api/v1/change/create POST \x1b[36mstep-3\x1b[0m\n" +
		"    expected_types:\n" +
		"        \x1b[38;5;15m.version:\x1b[0m \x1b[38;5;210m[number, null]\x1b[0m\n" +
		"        \x1b[38;5;15m.change_types:\x1b[0m \x1b[38;5;210m[array]\x1b[0m\n" +
		"    expected_body:\n" +
		"        - actual\n" +
		"        + expected\n\n"
	if got := output.String(); got != want {
		t.Fatalf("validation output = %q, want %q", got, want)
	}
}

func TestSuccessReportsOnlyDefinitionsWithoutValidationFailures(t *testing.T) {
	directory := reporterDirectory("change/create.yaml")
	create := directory.StepsDefinitions[0]
	updateFile := &domain.File{Path: "change/update.yaml", Directory: directory}
	update := &domain.StepsDefinition{File: updateFile}
	directory.StepsDefinitions = append(directory.StepsDefinitions, update)
	step := &domain.Step{Definition: create}
	step.Response.ExpectedTypes = map[string][]string{".version": {"int"}}
	var output bytes.Buffer
	report := NewReporter(&output, false)

	if err := report.ValidationTypes(finalValidationContext(report), step, `{"selector":".version","actual":"string"}`); err != nil {
		t.Fatalf("ValidationTypes() error = %v", err)
	}
	if err := report.Success(context.Background(), update); err != nil {
		t.Fatalf("Success() error = %v", err)
	}

	got := output.String()
	if strings.Contains(got, "[\x1b[38;5;10m✓\x1b[0m] /change/create\n") {
		t.Fatalf("output = %q, contains success for failed definition", got)
	}
	if !strings.Contains(got, "[\x1b[38;5;10m✓\x1b[0m] /change/update\n") {
		t.Fatalf("output = %q, want success for valid sibling definition", got)
	}
}

func TestValidationTypesFallsBackToEveryOriginalDeclaration(t *testing.T) {
	step := reporterStep("change/create.yaml", 0)
	step.Response.ExpectedTypes = map[string][]string{
		".version":      {"number", "null"},
		".change_types": {"array"},
	}
	var output bytes.Buffer

	if err := NewReporter(&output, false).ValidationTypes(context.Background(), step, "non-JSON failure"); err != nil {
		t.Fatalf("ValidationTypes() error = %v", err)
	}

	changeTypes := "        \x1b[38;5;15m.change_types:\x1b[0m \x1b[38;5;210m[array]\x1b[0m\n"
	version := "        \x1b[38;5;15m.version:\x1b[0m \x1b[38;5;210m[number, null]\x1b[0m\n"
	if got := output.String(); !strings.Contains(got, changeTypes) || !strings.Contains(got, version) {
		t.Fatalf("ValidationTypes() output = %q, want original declarations", got)
	} else if strings.Index(got, changeTypes) > strings.Index(got, version) {
		t.Fatalf("ValidationTypes() output = %q, want sorted declarations", got)
	}
}

func TestValidationBodyPreservesColoredDiff(t *testing.T) {
	const diff = "\x1b[38;5;210m-actual\x1b[m\n\x1b[92m+expected\x1b[m"
	var output bytes.Buffer

	if err := NewReporter(&output, false).ValidationBody(context.Background(), reporterStep("steps.yaml", 0), diff); err != nil {
		t.Fatalf("ValidationBody() error = %v", err)
	}
	wantIndentedDiff := "        \x1b[38;5;210m-actual\x1b[m\n" +
		"        \x1b[92m+expected\x1b[m\n"
	if !strings.Contains(output.String(), wantIndentedDiff) {
		t.Fatalf("ValidationBody() output = %q, want indented colored diff %q", output.String(), wantIndentedDiff)
	}
}

func TestReporterPreservesWriterFailureCauses(t *testing.T) {
	wantErr := errors.New("write failed")
	step := reporterStep("steps.yaml", 0)
	tests := map[string]func(*Reporter) error{
		"working directory": func(report *Reporter) error { return report.WorkingDirectory("/work") },
		"success": func(report *Reporter) error {
			return report.Success(context.Background(), step.Definition)
		},
		"types": func(report *Reporter) error {
			return report.ValidationTypes(context.Background(), step, "failed")
		},
		"status": func(report *Reporter) error {
			return report.ValidationStatus(context.Background(), step, errors.New("mismatch"))
		},
		"body":  func(report *Reporter) error { return report.ValidationBody(context.Background(), step, "diff") },
		"debug": func(report *Reporter) error { return report.Debug(context.Background(), step) },
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			err := call(NewReporter(failingWriter{err: wantErr}, false))
			if !errors.Is(err, ErrReporter) || !errors.Is(err, wantErr) {
				t.Fatalf("reporting error = %v, want ErrReporter and writer cause", err)
			}
			if got := errs.Code(err, 0); got != errs.ExitInternal {
				t.Fatalf("reporting exit code = %d, want %d", got, errs.ExitInternal)
			}
		})
	}
}

func TestReporterRejectsNilOutputWithoutPanicking(t *testing.T) {
	var nilReport *Reporter
	tests := map[string]func(*Reporter) error{
		"working directory": func(report *Reporter) error { return report.WorkingDirectory("/work") },
		"success":           func(report *Reporter) error { return report.Success(context.Background(), nil) },
		"types":             func(report *Reporter) error { return report.ValidationTypes(context.Background(), nil, "failed") },
		"status":            func(report *Reporter) error { return report.ValidationStatus(context.Background(), nil, nil) },
		"body":              func(report *Reporter) error { return report.ValidationBody(context.Background(), nil, "diff") },
		"debug":             func(report *Reporter) error { return report.Debug(context.Background(), nil) },
	}

	for name, call := range tests {
		t.Run(name+" nil receiver", func(t *testing.T) {
			if err := call(nilReport); !errors.Is(err, ErrReporter) {
				t.Fatalf("reporting error = %v, want ErrReporter", err)
			}
		})
		t.Run(name+" nil writer", func(t *testing.T) {
			if err := call(NewReporter(nil, false)); !errors.Is(err, ErrReporter) {
				t.Fatalf("reporting error = %v, want ErrReporter", err)
			}
		})
	}
}

func TestReporterCancellationPolicySkipsContextualWrites(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	step := reporterStep("steps.yaml", 0)
	tests := map[string]func(*Reporter) error{
		"success": func(report *Reporter) error { return report.Success(ctx, step.Definition) },
		"types":   func(report *Reporter) error { return report.ValidationTypes(ctx, step, "failed") },
		"status":  func(report *Reporter) error { return report.ValidationStatus(ctx, step, errors.New("mismatch")) },
		"body":    func(report *Reporter) error { return report.ValidationBody(ctx, step, "diff") },
		"debug":   func(report *Reporter) error { return report.Debug(ctx, step) },
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			err := call(NewReporter(&output, false))
			if !errors.Is(err, ErrReporter) || !errors.Is(err, context.Canceled) {
				t.Fatalf("reporting error = %v, want ErrReporter and context.Canceled", err)
			}
			if output.Len() != 0 {
				t.Fatalf("canceled reporting output = %q, want empty", output.String())
			}
		})
	}
}

func TestReporterRechecksCancellationAfterWaitingForWriter(t *testing.T) {
	writer := &gateWriter{started: make(chan struct{}, 1), release: make(chan struct{})}
	report := NewReporter(writer, false)
	firstDone := make(chan error, 1)
	first := reporterDirectory("first.yaml")
	go func() {
		firstDone <- report.Success(context.Background(), first.StepsDefinitions[0])
	}()
	<-writer.started

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	second := reporterDirectory("second.yaml")
	go func() {
		secondDone <- report.Success(ctx, second.StepsDefinitions[0])
	}()
	cancel()
	close(writer.release)

	if err := <-firstDone; err != nil {
		t.Fatalf("first Success() error = %v", err)
	}
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("second Success() error = %v, want context.Canceled", err)
	}
	if got, want := writer.String(), "[\x1b[38;5;10m✓\x1b[0m] /first\n"; got != want {
		t.Fatalf("serialized output = %q, want %q", got, want)
	}
}

func TestReporterSerializesConcurrentWrites(t *testing.T) {
	writer := &yieldingBuffer{}
	report := NewReporter(writer, false)
	const reports = 32
	var wait sync.WaitGroup
	for index := 0; index < reports; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			directory := reporterDirectory(fmt.Sprintf("dir-%02d.yaml", index))
			if err := report.Success(context.Background(), directory.StepsDefinitions[0]); err != nil {
				t.Errorf("Success() error = %v", err)
			}
		}()
	}
	wait.Wait()

	output := writer.String()
	for index := 0; index < reports; index++ {
		block := fmt.Sprintf("[\x1b[38;5;10m✓\x1b[0m] /dir-%02d\n", index)
		if got := strings.Count(output, block); got != 1 {
			t.Errorf("count of %q = %d, want 1; output = %q", block, got, output)
		}
	}
	if got := strings.Count(output, "[\x1b[38;5;10m✓\x1b[0m]"); got != reports {
		t.Fatalf("success block count = %d, want %d", got, reports)
	}
}

func TestReporterClassifiesContextualShortWrites(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		directory := reporterDirectory("work.yaml")
		err := NewReporter(shortWriter{}, false).Success(context.Background(), directory.StepsDefinitions[0])
		if !errors.Is(err, ErrReporter) || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("Success() error = %v, want ErrReporter and io.ErrShortWrite", err)
		}
	})
	t.Run("debug", func(t *testing.T) {
		err := NewReporter(shortWriter{}, false).Debug(context.Background(), reporterStep("steps.yaml", 0))
		if !errors.Is(err, ErrReporter) || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("Debug() error = %v, want ErrReporter and io.ErrShortWrite", err)
		}
	})
}

func TestDebugSuppressesEveryLaterReport(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output, false)
	step := reporterStep("steps.yaml", 0)
	step.RawCurl = "curl --url https://example.test"
	if err := report.Debug(context.Background(), step); err != nil {
		t.Fatalf("Debug() error = %v", err)
	}
	want := output.String()
	step.Response.ExpectedTypes = map[string][]string{".value": {"string"}}
	if err := report.ValidationTypes(finalValidationContext(report), step, `{"selector":".value","actual":"number"}`); err != nil {
		t.Fatalf("ValidationTypes() after Debug error = %v", err)
	}
	later := reporterDirectory("later.yaml")
	if err := report.Success(context.Background(), later.StepsDefinitions[0]); err != nil {
		t.Fatalf("Success() after Debug error = %v", err)
	}
	if err := report.WorkingDirectory("/later"); err != nil {
		t.Fatalf("WorkingDirectory() after Debug error = %v", err)
	}
	if err := report.Debug(context.Background(), reporterStep("other.yaml", 99)); err != nil {
		t.Fatalf("Debug() after Debug error = %v", err)
	}
	if got := output.String(); got != want {
		t.Fatalf("output after Debug = %q, want unchanged %q", got, want)
	}
}

func TestDebugClassifiesJQFailureAsReportingFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := NewReporter(&bytes.Buffer{}, false).Debug(context.Background(), &domain.Step{})
	if !errors.Is(err, ErrReporter) || !errors.Is(err, runner.ErrJQPretty) {
		t.Fatalf("Debug() error = %v, want ErrReporter and ErrJQPretty", err)
	}
}

func TestColorizeJQJSONUsesJQTerminalPalette(t *testing.T) {
	input := "{\n  \"key\": [null, true, 10, \"value\"]\n}"
	want := "\x1b[1;39m{\x1b[0m\n" +
		"  \x1b[1;34m\"key\"\x1b[0m\x1b[1;39m:\x1b[0m \x1b[1;39m[\x1b[0m" +
		"\x1b[0;90mnull\x1b[0m\x1b[1;39m,\x1b[0m " +
		"\x1b[0;39mtrue\x1b[0m\x1b[1;39m,\x1b[0m " +
		"\x1b[0;39m10\x1b[0m\x1b[1;39m,\x1b[0m " +
		"\x1b[0;32m\"value\"\x1b[0m\x1b[1;39m]\x1b[0m\n" +
		"\x1b[1;39m}\x1b[0m"
	if got := colorizeJQJSON(input); got != want {
		t.Fatalf("colorizeJQJSON() = %q, want %q", got, want)
	}
}

func TestNonTerminalStageCommitsOnceInCanonicalOrder(t *testing.T) {
	first := reporterDirectory("stage/first.yaml")
	second := reporterDirectory("stage/second.yaml")
	var output bytes.Buffer
	report := NewReporter(&output, false)

	if err := report.BeginStage(context.Background(), []*domain.Directory{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), second.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("partial non-terminal output = %q, want empty", output.String())
	}
	if err := report.Success(context.Background(), first.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("partial non-terminal output = %q, want empty", output.String())
	}
	if err := report.EndStage(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := "[\x1b[38;5;10m✓\x1b[0m] /stage/first\n" +
		"[\x1b[38;5;10m✓\x1b[0m] /stage/second\n"
	if got := output.String(); got != want {
		t.Fatalf("committed stage = %q, want canonical %q", got, want)
	}
	if err := report.EndStage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != want {
		t.Fatalf("second EndStage output = %q, want unchanged", got)
	}
}

func TestStageSnapshotIsRenderedOnlyForImplicitOutput(t *testing.T) {
	definition := reporterDirectory("stage/first.yaml").StepsDefinitions[0]
	file := newFileOutput()
	file.block.WriteString("accumulated output")
	stage := &stageOutput{
		order: []*domain.StepsDefinition{definition},
		files: map[*domain.StepsDefinition]*fileOutput{definition: file},
	}

	if got := stage.implicitSnapshot(); got != "" {
		t.Fatalf("explicit stage snapshot = %q, want empty", got)
	}
	stage.implicit = true
	if got, want := stage.implicitSnapshot(), "accumulated output"; got != want {
		t.Fatalf("implicit stage snapshot = %q, want %q", got, want)
	}
}

func TestTerminalRedrawChangesOnlyActiveStageRegion(t *testing.T) {
	first := reporterDirectory("stage/first.yaml")
	second := reporterDirectory("stage/second.yaml")
	later := reporterDirectory("later/only.yaml")
	var output bytes.Buffer
	report := NewReporter(&output, true)

	if err := report.WorkingDirectory("/work"); err != nil {
		t.Fatal(err)
	}
	if err := report.BeginStage(context.Background(), []*domain.Directory{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), second.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), first.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[1A\r\x1b[J[\x1b[38;5;10m✓\x1b[0m] /stage/first\n[\x1b[38;5;10m✓\x1b[0m] /stage/second\n") {
		t.Fatalf("terminal redraw = %q, want active-region clear and canonical redraw", output.String())
	}
	if got := strings.Count(output.String(), "Working Directory: /work\n\n"); got != 1 {
		t.Fatalf("working-directory heading count = %d, want 1", got)
	}
	if err := report.EndStage(context.Background()); err != nil {
		t.Fatal(err)
	}
	completed := output.Len()

	if err := report.BeginStage(context.Background(), []*domain.Directory{later}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), later.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	suffix := output.String()[completed:]
	if strings.Contains(suffix, "/stage/") || strings.HasPrefix(suffix, "\x1b[") {
		t.Fatalf("later-stage output rewrote completed stage: %q", suffix)
	}
}

func TestTerminalRedrawTracksWrappedVisualRowsAndViewportHeight(t *testing.T) {
	first := reporterDirectory("a.yaml")
	wrapped := reporterDirectory("a-very-long-name.yaml")
	var output bytes.Buffer
	report := NewReporter(&output, true)
	report.terminalWidth = 10
	report.terminalHeight = 20

	if err := report.BeginStage(context.Background(), []*domain.Directory{first, wrapped}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), wrapped.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), first.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[3A\r\x1b[J") {
		t.Fatalf("wrapped terminal redraw = %q, want three-row cursor movement", output.String())
	}

	output.Reset()
	report = NewReporter(&output, true)
	report.terminalWidth = 10
	report.terminalHeight = 2
	if err := report.BeginStage(context.Background(), []*domain.Directory{first, wrapped}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), wrapped.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), first.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[1A\r\x1b[J") {
		t.Fatalf("viewport-limited redraw = %q, want cursor movement capped to visible rows", output.String())
	}
}

func TestTerminalRedrawRefreshesDimensionsAndReflowsPreviousContent(t *testing.T) {
	previous := getTerminalSize
	dimensions := [][2]int{{20, 24}, {20, 24}, {10, 24}}
	calls := 0
	getTerminalSize = func(fd int) (int, int, error) {
		if fd != 17 {
			t.Fatalf("terminal descriptor = %d, want 17", fd)
		}
		index := min(calls, len(dimensions)-1)
		calls++
		return dimensions[index][0], dimensions[index][1], nil
	}
	t.Cleanup(func() { getTerminalSize = previous })

	first := reporterDirectory("1234567890.yaml")
	second := reporterDirectory("second.yaml")
	writer := &descriptorWriter{fd: 17}
	report := NewReporter(writer, true)
	if err := report.BeginStage(context.Background(), []*domain.Directory{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), first.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), second.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}

	if calls != 3 {
		t.Fatalf("terminal-size queries = %d, want constructor plus both redraws", calls)
	}
	if !strings.Contains(writer.String(), "\x1b[2A\r\x1b[J") {
		t.Fatalf("resized terminal redraw = %q, want two-row movement after reflow", writer.String())
	}
}

func TestVisualRowsToCursorIgnoresANSIAndUsesDisplayWidth(t *testing.T) {
	content := "\x1b[38;5;10m✓\x1b[0m 12345678\n界界\n"
	if got, want := visualRowsToCursor(content, 5), 3; got != want {
		t.Fatalf("visualRowsToCursor() = %d, want %d", got, want)
	}

	tests := map[string]struct {
		content string
		width   int
		want    int
	}{
		"empty":              {width: 5},
		"invalid width":      {content: "value"},
		"no final newline":   {content: "value", width: 5},
		"wrapped final line": {content: "values", width: 5, want: 1},
		"empty line":         {content: "\n", width: 5, want: 1},
		"tab stop":           {content: "\tvalue\n", width: 8, want: 2},
		"carriage return":    {content: "old\rnew\n", width: 5, want: 1},
		"wrapped carriage":   {content: "values\rnew\n", width: 5, want: 2},
		"grapheme cluster":   {content: "👨‍👩‍👧‍👦123\n", width: 5, want: 1},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := visualRowsToCursor(test.content, test.width); got != test.want {
				t.Fatalf("visualRowsToCursor(%q, %d) = %d, want %d", test.content, test.width, got, test.want)
			}
		})
	}
}

func TestBufferedDebugIsFinalAndSuppressesLaterEvents(t *testing.T) {
	first := reporterDirectory("stage/first.yaml")
	second := reporterDirectory("stage/second.yaml")
	step := &domain.Step{Definition: first.StepsDefinitions[0], RawCurl: "curl --url example.test"}
	var output bytes.Buffer
	report := NewReporter(&output, false)

	if err := report.BeginStage(context.Background(), []*domain.Directory{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), second.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if err := report.Debug(context.Background(), step); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), first.StepsDefinitions[0]); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("Debug wrote before non-terminal barrier: %q", output.String())
	}
	if err := report.EndStage(context.Background()); err != nil {
		t.Fatal(err)
	}

	got := output.String()
	if strings.Contains(got, "/stage/first\n") {
		t.Fatalf("post-Debug success was reported: %q", got)
	}
	if success, debug := strings.Index(got, "/stage/second\n"), strings.Index(got, "stage: 0\n"); success < 0 || debug < success {
		t.Fatalf("Debug was not final after canonical file output: %q", got)
	}
}

func TestStageMethodsPreserveContextAndWriterFailures(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewReporter(&bytes.Buffer{}, false).BeginStage(canceled, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("BeginStage(canceled) error = %v", err)
	}
	if err := NewReporter(&bytes.Buffer{}, false).EndStage(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("EndStage(canceled) error = %v", err)
	}
	if err := (*Reporter)(nil).BeginStage(context.Background(), nil); !errors.Is(err, ErrReporter) {
		t.Fatalf("nil BeginStage error = %v", err)
	}

	wantErr := errors.New("stage write failed")
	directory := reporterDirectory("stage/fail.yaml")
	report := NewReporter(failingWriter{err: wantErr}, false)
	if err := report.BeginStage(context.Background(), []*domain.Directory{nil, directory}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), directory.StepsDefinitions[0]); err != nil {
		t.Fatalf("buffered Success error = %v", err)
	}
	if err := report.EndStage(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("EndStage writer error = %v, want %v", err, wantErr)
	}
}

func TestTerminalDebugRedrawsAccumulatedFilesAndStopsNewStages(t *testing.T) {
	directory := reporterDirectory("stage/steps.yaml")
	step := &domain.Step{Definition: directory.StepsDefinitions[0], RawCurl: "curl"}
	var output bytes.Buffer
	report := NewReporter(&output, true)
	if err := report.BeginStage(context.Background(), []*domain.Directory{directory}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), step.Definition); err != nil {
		t.Fatal(err)
	}
	if err := report.Debug(context.Background(), step); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[1A\r\x1b[J") || !strings.Contains(output.String(), "\nstage: 0\n") {
		t.Fatalf("terminal Debug redraw = %q", output.String())
	}
	if err := report.BeginStage(context.Background(), nil); err != nil {
		t.Fatalf("BeginStage after Debug error = %v", err)
	}
	if err := report.EndStage(context.Background()); err != nil {
		t.Fatalf("terminal EndStage error = %v", err)
	}
}

func TestTerminalStageClassifiesRedrawFailure(t *testing.T) {
	wantErr := errors.New("redraw failed")
	directory := reporterDirectory("stage/steps.yaml")
	report := NewReporter(failingWriter{err: wantErr}, true)
	if err := report.BeginStage(context.Background(), []*domain.Directory{directory}); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), directory.StepsDefinitions[0]); !errors.Is(err, wantErr) {
		t.Fatalf("terminal Success error = %v, want %v", err, wantErr)
	}

	stage := &stageOutput{
		order: []*domain.StepsDefinition{nil},
		files: map[*domain.StepsDefinition]*fileOutput{nil: nil},
		debug: "debug-only",
	}
	if got := stage.render(); got != "debug-only" {
		t.Fatalf("stage render = %q, want debug-only", got)
	}
	if got := (*stageOutput)(nil).render(); got != "" {
		t.Fatalf("nil stage render = %q, want empty", got)
	}
}

func TestFailedFileDoesNotGainSuccessAndFormattingHandlesEmptyPayloads(t *testing.T) {
	directory := reporterDirectory("stage/failed.yaml")
	step := &domain.Step{Definition: directory.StepsDefinitions[0]}
	var output bytes.Buffer
	report := NewReporter(&output, false)
	if err := report.BeginStage(context.Background(), []*domain.Directory{directory}); err != nil {
		t.Fatal(err)
	}
	if err := report.ValidationStatus(finalValidationContext(report), step, ErrStatusValidation); err != nil {
		t.Fatal(err)
	}
	if err := report.Success(context.Background(), step.Definition); err != nil {
		t.Fatal(err)
	}
	if err := report.EndStage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "✓") {
		t.Fatalf("failed file gained success output: %q", output.String())
	}

	step.Response.ExpectedTypes = map[string][]string{".known": {"string"}}
	if got := formatExpectedTypes(step, `{"selector":".missing"}`); got != "    expected_types:\n" {
		t.Fatalf("unknown failed selector output = %q", got)
	}
	if got := formatExpectedBody(""); got != "    expected_body:\n" {
		t.Fatalf("empty body diff output = %q", got)
	}
}

func reporterStep(path string, index int) *domain.Step {
	directory := reporterDirectory(path)
	definition := directory.StepsDefinitions[0]
	return &domain.Step{Definition: definition, Index: index}
}

func finalValidationContext(report *Reporter) context.Context {
	return context.WithValue(context.Background(), report, true)
}

func reporterDirectory(path string) *domain.Directory {
	directory := &domain.Directory{Path: "/suite"}
	file := &domain.File{Path: path, Directory: directory}
	definition := &domain.StepsDefinition{File: file}
	directory.StepsDefinitions = []*domain.StepsDefinition{definition}
	return directory
}
