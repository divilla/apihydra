//go:build windows

package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
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

func TestShellCommandPreservesEmbeddedQuotesWithoutCommandInjection(t *testing.T) {
	if os.Getenv("APIH_QUOTE_HELPER") == "1" {
		fmt.Fprint(os.Stdout, os.Args[2])
		os.Exit(0)
	}

	injectedPath := filepath.Join(t.TempDir(), "injected")
	want := `https://example.test/a"& echo injected > "` + injectedPath
	command := shellCommand(
		os.Args[0],
		"-test.run=TestShellCommandPreservesEmbeddedQuotesWithoutCommandInjection",
		want,
	)
	cmd := exec.Command("cmd.exe", "/d", "/q", "/v:off")
	cmd.Stdin = strings.NewReader(command)
	cmd.Env = append(os.Environ(), "APIH_QUOTE_HELPER=1")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("copy-pasted command error = %v; output = %q; command = %q", err, output.String(), command)
	}
	if got := output.String(); got != want {
		t.Fatalf("copy-pasted argument = %q, want %q; command = %q", got, want, command)
	}
	if _, err := os.Stat(injectedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("injected command created %q: %v", injectedPath, err)
	}
}

func TestCurlCancellationTerminatesWindowsProcessTree(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child-pid")
	powerShell := `$PID | Set-Content -NoNewline '` + strings.ReplaceAll(pidPath, `'`, `''`) + `'; Start-Sleep -Seconds 30`
	command := shellCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", powerShell)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, _, err := executeCurlShellCommand(ctx, command)
		result <- err
	}()

	pid := waitForWindowsProcessPID(t, pidPath)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("executeCurlShellCommand() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("executeCurlShellCommand() did not return after cancellation")
	}

	deadline := time.Now().Add(2 * time.Second)
	for windowsProcessExists(uint32(pid)) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if windowsProcessExists(uint32(pid)) {
		t.Fatalf("descendant process %d still exists after cancellation", pid)
	}
}

func TestCurlShellCommandReturnsEveryBodyArtifactForCleanup(t *testing.T) {
	command, temporaryPaths, err := curlShellCommand("curl --data-binary @-", "secret body")
	if err != nil {
		t.Fatalf("curlShellCommand() error = %v", err)
	}
	if len(temporaryPaths) != 2 {
		t.Fatalf("curlShellCommand() temporary paths = %#v, want encoded and decoded body paths", temporaryPaths)
	}
	for _, path := range temporaryPaths {
		if !strings.Contains(command, shellQuote(path)) {
			t.Fatalf("curlShellCommand() = %q, want artifact path %q", command, path)
		}
		if err := os.WriteFile(path, []byte("sensitive"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	removeFiles(temporaryPaths)
	for _, path := range temporaryPaths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("body artifact %q remains after cleanup: %v", path, err)
		}
	}
}

func TestCurlRemovesBodyArtifactsWhenCertutilFails(t *testing.T) {
	commandDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(commandDir, "certutil.cmd"), []byte("@exit /b 9\r\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(certutil.cmd) error = %v", err)
	}
	t.Setenv("PATH", commandDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	temporaryDir := t.TempDir()
	t.Setenv("TEMP", temporaryDir)
	t.Setenv("TMP", temporaryDir)

	_, _, err := Curl(context.Background(), "POST", "https://example.test", nil, 0, 0, "", "secret body")
	assertCommandError(t, err, ErrCurl)
	bodyArtifacts, globErr := filepath.Glob(filepath.Join(temporaryDir, "apih-curl-body-*"))
	if globErr != nil {
		t.Fatalf("Glob(body artifacts) error = %v", globErr)
	}
	if len(bodyArtifacts) != 0 {
		t.Fatalf("body artifacts remain after certutil failure: %#v", bodyArtifacts)
	}
}

func waitForWindowsProcessPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(contents)))
			if parseErr != nil {
				t.Fatalf("parse process ID: %v", parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read process ID: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("descendant process did not start")
	return 0
}

func windowsProcessExists(pid uint32) bool {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer windows.CloseHandle(process)
	status, err := windows.WaitForSingleObject(process, 0)
	return err != nil || status != windows.WAIT_OBJECT_0
}

func TestEscapePercentExpansionPreservesUnpairedPercent(t *testing.T) {
	if got, want := escapePercentExpansion("100% complete"), "100% complete"; got != want {
		t.Fatalf("escapePercentExpansion() = %q, want %q", got, want)
	}
}
