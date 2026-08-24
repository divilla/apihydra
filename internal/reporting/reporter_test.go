package reporting

import (
	"apih/internal/domain"
	"apih/pkg/errs"
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
	step.Response.ExpectedStatus = 201
	step.Response.ActualStatus = 500
	step.Response.ActualBody = `{"message":"failed"}`
	step.Debug = true
	directory := step.Definition.File.Directory

	debugJSON, err := json.MarshalIndent(step, "", "  ")
	if err != nil {
		t.Fatalf("marshal debug step: %v", err)
	}
	tests := map[string]struct {
		call func(*Reporter) error
		want string
	}{
		"success": {
			call: func(report *Reporter) error { return report.Success(context.Background(), directory) },
			want: "Success: /suite\n\n",
		},
		"types": {
			call: func(report *Reporter) error {
				return report.ValidationTypes(context.Background(), step, `{"selector":".id","actual":"null"}`)
			},
			want: "type validation failed for suite/steps.yaml step 3:\n{\"selector\":\".id\",\"actual\":\"null\"}\n\n",
		},
		"status": {
			call: func(report *Reporter) error {
				return report.ValidationStatus(context.Background(), step, errors.New("validation error"))
			},
			want: "response status does not match expected for suite/steps.yaml step 3: validation error\n\n",
		},
		"body": {
			call: func(report *Reporter) error {
				return report.ValidationBody(context.Background(), step, "- expected\n+ actual")
			},
			want: "response body does not match expected for suite/steps.yaml step 3:\n- expected\n+ actual\n\n",
		},
		"debug": {
			call: func(report *Reporter) error { return report.Debug(context.Background(), step) },
			want: fmt.Sprintf("Debug suite/steps.yaml step 3:\n%s\n\n", debugJSON),
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

func TestValidationBodyPreservesColoredDiff(t *testing.T) {
	const diff = "\x1b[36m@@ -1 +1 @@\x1b[m\n\x1b[31m-old\x1b[m\n\x1b[32m+new\x1b[m"
	var output bytes.Buffer

	if err := NewReporter(&output).ValidationBody(context.Background(), reporterStep("steps.yaml", 0), diff); err != nil {
		t.Fatalf("ValidationBody() error = %v", err)
	}
	if !strings.Contains(output.String(), diff) {
		t.Fatalf("ValidationBody() output = %q, want unchanged colored diff %q", output.String(), diff)
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
		firstDone <- report.Success(context.Background(), &domain.Directory{Path: "/first"})
	}()
	<-writer.started

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- report.Success(ctx, &domain.Directory{Path: "/second"})
	}()
	cancel()
	close(writer.release)

	if err := <-firstDone; err != nil {
		t.Fatalf("first Success() error = %v", err)
	}
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("second Success() error = %v, want context.Canceled", err)
	}
	if got, want := writer.String(), "Success: /first\n\n"; got != want {
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
			if err := report.Success(context.Background(), &domain.Directory{Path: fmt.Sprintf("/dir-%02d", index)}); err != nil {
				t.Errorf("Success() error = %v", err)
			}
		}()
	}
	wait.Wait()

	output := writer.String()
	for index := 0; index < reports; index++ {
		block := fmt.Sprintf("Success: /dir-%02d\n\n", index)
		if got := strings.Count(output, block); got != 1 {
			t.Errorf("count of %q = %d, want 1; output = %q", block, got, output)
		}
	}
	if got := strings.Count(output, "Success:"); got != reports {
		t.Fatalf("success block count = %d, want %d", got, reports)
	}
}

func TestReporterClassifiesContextualShortWrites(t *testing.T) {
	err := NewReporter(shortWriter{}).Success(context.Background(), &domain.Directory{Path: "/work"})
	if !errors.Is(err, ErrReporter) || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Success() error = %v, want ErrReporter and io.ErrShortWrite", err)
	}
}

func TestReporterUsesSafeFallbackReferences(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output)
	if err := report.Success(context.Background(), nil); err != nil {
		t.Fatalf("Success() error = %v", err)
	}
	if err := report.Debug(context.Background(), nil); err != nil {
		t.Fatalf("Debug() error = %v", err)
	}
	if err := report.ValidationTypes(context.Background(), &domain.Step{Index: 7}, "failed"); err != nil {
		t.Fatalf("ValidationTypes() error = %v", err)
	}
	if got, want := output.String(), "Success: <unknown directory>\n\nDebug <unknown step>:\nnull\n\ntype validation failed for step 7:\nfailed\n\n"; got != want {
		t.Fatalf("fallback output = %q, want %q", got, want)
	}
}

func reporterStep(path string, index int) *domain.Step {
	directory := &domain.Directory{Path: "/suite"}
	file := &domain.File{Path: path, Directory: directory}
	definition := &domain.StepsDefinition{File: file}
	return &domain.Step{Definition: definition, Index: index}
}
