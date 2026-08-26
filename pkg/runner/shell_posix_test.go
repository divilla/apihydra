//go:build !windows

package runner

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCurlCancellationTerminatesShellProcessGroup(t *testing.T) {
	installCommand(t, "curl", `
printf '%s\n' "$$" > "$APIH_TEST_CURL_PID"
/bin/sleep 30
`)
	pidPath := t.TempDir() + "/curl-pid"
	t.Setenv("APIH_TEST_CURL_PID", pidPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, _, err := Curl(ctx, "GET", "https://example.test", nil, 0, 0, "", "")
		result <- err
	}()

	pid := waitForProcessPID(t, pidPath)
	cancel()

	select {
	case err := <-result:
		assertCommandError(t, err, ErrCurl)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Curl() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Curl() did not return after context cancellation")
	}

	deadline := time.Now().Add(2 * time.Second)
	for processExists(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processExists(pid) {
		t.Fatalf("curl process %d still exists after context cancellation", pid)
	}
}

func waitForProcessPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(contents)))
			if parseErr != nil {
				t.Fatalf("parse curl process ID: %v", parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read curl process ID: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("curl process did not start")
	return 0
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
