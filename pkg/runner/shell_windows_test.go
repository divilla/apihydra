//go:build windows

package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestShellCommandEscapesCmdPercentExpansion(t *testing.T) {
	command := shellCommand(
		"curl",
		"--url", "https://example.test/%PATH%",
		"--header", "X-Value: %USERPROFILE%",
		"--data-binary", `{"value":"%TEMP%"}`,
	)

	for _, expanded := range []string{"%PATH%", "%USERPROFILE%", "%TEMP%"} {
		if strings.Contains(command, expanded) {
			t.Fatalf("shellCommand() = %q, contains expandable token %q", command, expanded)
		}
	}
	for _, escaped := range []string{"%PATH^%", "%USERPROFILE^%", "%TEMP^%"} {
		if !strings.Contains(command, escaped) {
			t.Fatalf("shellCommand() = %q, want escaped token %q", command, escaped)
		}
	}
}

func TestShellCommandPreservesPercentArgumentsThroughCmd(t *testing.T) {
	if os.Getenv("APIH_PERCENT_HELPER") == "1" {
		fmt.Fprintf(os.Stdout, "%s\n%s", os.Args[2], os.Args[3])
		os.Exit(0)
	}

	command := shellCommand(
		os.Args[0],
		"-test.run=TestShellCommandPreservesPercentArgumentsThroughCmd",
		"%PATH%",
		"X-Value: %USERPROFILE%",
	)
	cmd := exec.Command("cmd.exe", "/d", "/s", "/c", command)
	cmd.Env = append(os.Environ(), "APIH_PERCENT_HELPER=1")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("copy-pasted command error = %v; output = %q; command = %q", err, output.String(), command)
	}
	if got, want := output.String(), "%PATH%\nX-Value: %USERPROFILE%"; got != want {
		t.Fatalf("copy-pasted arguments = %q, want %q; command = %q", got, want, command)
	}
}

func TestShellCommandPreservesLiteralCaretsThroughCmd(t *testing.T) {
	if os.Getenv("APIH_CARET_HELPER") == "1" {
		fmt.Fprint(os.Stdout, os.Args[2])
		os.Exit(0)
	}

	command := shellCommand(
		os.Args[0],
		"-test.run=TestShellCommandPreservesLiteralCaretsThroughCmd",
		"https://example.test/a^b",
	)
	if !strings.Contains(command, `"https://example.test/a^b"`) {
		t.Fatalf("shellCommand() = %q, want literal caret argument quoted", command)
	}
	cmd := exec.Command("cmd.exe", "/d", "/s", "/c", command)
	cmd.Env = append(os.Environ(), "APIH_CARET_HELPER=1")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("copy-pasted command error = %v; output = %q; command = %q", err, output.String(), command)
	}
	if got, want := output.String(), "https://example.test/a^b"; got != want {
		t.Fatalf("copy-pasted argument = %q, want %q; command = %q", got, want, command)
	}
}

func TestEscapePercentExpansionPreservesUnpairedPercent(t *testing.T) {
	if got, want := escapePercentExpansion("100% complete"), "100% complete"; got != want {
		t.Fatalf("escapePercentExpansion() = %q, want %q", got, want)
	}
}
