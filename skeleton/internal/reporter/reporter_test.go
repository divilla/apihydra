package reporter

import (
	"bytes"
	"errors"
	"testing"
)

func TestWorkingDirectoryUsesInjectedWriter(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output)

	if err := report.WorkingDirectory("/work"); err != nil {
		t.Fatalf("WorkingDirectory() error = %v", err)
	}
	if got, want := output.String(), "Working Directory: /work\n\n"; got != want {
		t.Fatalf("WorkingDirectory() output = %q, want %q", got, want)
	}
}

func TestErrorUsesUnifiedRedAndRemovesEmbeddedColors(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output)

	if err := report.Error(errors.New("before \x1b[32mgreen\x1b[0m after")); err != nil {
		t.Fatalf("Error() error = %v", err)
	}
	if got, want := output.String(), "\x1b[31mbefore green after\x1b[0m\n"; got != want {
		t.Fatalf("Error() output = %q, want %q", got, want)
	}
}
