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
	var _ func(io.Writer) *Reporter = NewReporter
	var _ func(*Reporter, string) error = (*Reporter).WorkingDirectory
	var _ func(*Reporter, context.Context, *domain.Directory) error = (*Reporter).Success
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
	report := NewReporter(&output)

	if err := report.WorkingDirectory("/work"); err != nil {
		t.Fatalf("WorkingDirectory() error = %v", err)
	}
	if got, want := output.String(), "Working Directory: /work\n\n"; got != want {
		t.Fatalf("WorkingDirectory() output = %q, want %q", got, want)
	}
}

func TestWorkingDirectoryPreservesReferenceWritePath(t *testing.T) {
	writer := &divergentStringWriter{writeStringErr: errors.New("WriteString must not be called")}

	if err := NewReporter(writer).WorkingDirectory("/work"); err != nil {
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
	if err := NewReporter(shortWriter{}).WorkingDirectory("/work"); err != nil {
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
	directory := step.Definition.File.Directory

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
			call: func(report *Reporter) error { return report.Success(context.Background(), directory) },
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
			if err := test.call(NewReporter(&output)); err != nil {
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

	if err := NewReporter(&output).Debug(context.Background(), step); err != nil {
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
	report := NewReporter(&output)

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
	report := NewReporter(&output)

	if err := report.ValidationTypes(finalValidationContext(report), step, `{"selector":".version","actual":"string"}`); err != nil {
		t.Fatalf("ValidationTypes() error = %v", err)
	}
	if err := report.Success(context.Background(), directory); err != nil {
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

	if err := NewReporter(&output).ValidationTypes(context.Background(), step, "non-JSON failure"); err != nil {
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

	if err := NewReporter(&output).ValidationBody(context.Background(), reporterStep("steps.yaml", 0), diff); err != nil {
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
			return report.Success(context.Background(), step.Definition.File.Directory)
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
			err := call(NewReporter(failingWriter{err: wantErr}))
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
			if err := call(NewReporter(nil)); !errors.Is(err, ErrReporter) {
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
		"success": func(report *Reporter) error { return report.Success(ctx, step.Definition.File.Directory) },
		"types":   func(report *Reporter) error { return report.ValidationTypes(ctx, step, "failed") },
		"status":  func(report *Reporter) error { return report.ValidationStatus(ctx, step, errors.New("mismatch")) },
		"body":    func(report *Reporter) error { return report.ValidationBody(ctx, step, "diff") },
		"debug":   func(report *Reporter) error { return report.Debug(ctx, step) },
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			err := call(NewReporter(&output))
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
	report := NewReporter(writer)
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- report.Success(context.Background(), reporterDirectory("first.yaml"))
	}()
	<-writer.started

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- report.Success(ctx, reporterDirectory("second.yaml"))
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
	report := NewReporter(writer)
	const reports = 32
	var wait sync.WaitGroup
	for index := 0; index < reports; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := report.Success(context.Background(), reporterDirectory(fmt.Sprintf("dir-%02d.yaml", index))); err != nil {
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
		err := NewReporter(shortWriter{}).Success(context.Background(), reporterDirectory("work.yaml"))
		if !errors.Is(err, ErrReporter) || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("Success() error = %v, want ErrReporter and io.ErrShortWrite", err)
		}
	})
	t.Run("debug", func(t *testing.T) {
		err := NewReporter(shortWriter{}).Debug(context.Background(), reporterStep("steps.yaml", 0))
		if !errors.Is(err, ErrReporter) || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("Debug() error = %v, want ErrReporter and io.ErrShortWrite", err)
		}
	})
}

func TestDebugSuppressesEveryLaterReport(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output)
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
	if err := report.Success(context.Background(), reporterDirectory("later.yaml")); err != nil {
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
	err := NewReporter(&bytes.Buffer{}).Debug(context.Background(), &domain.Step{})
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
