//go:build integration && linux

package inttests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func runPlatformSpecificScenarios(t *testing.T, ctx context.Context, binary, workDir, coverageDir string) {
	t.Helper()
	t.Run("closed PTY output failures", func(t *testing.T) {
		for _, suite := range []string{
			filepath.Join("scenarios", "success-output"),
			filepath.Join("scenarios", "debug-defaults"),
			"test1",
			filepath.Join("test2", "type-only"),
			filepath.Join("test2", "status-only"),
			filepath.Join("test2", "body-only"),
		} {
			outputFailure := runCLIWithClosedPTY(t, ctx, binary, workDir, coverageDir, suite)
			if outputFailure.exitCode != 103 || outputFailure.stderr == "" {
				t.Fatalf("closed-PTY %s result = code %d, stderr %q, want code 103 and diagnostic", suite, outputFailure.exitCode, outputFailure.stderr)
			}
		}
	})

	t.Run("directory permission transition", func(t *testing.T) {
		requirePermissionDenialScenarios(t)
		transitionDirectory := filepath.Join(workDir, "scenarios", "transition-directory")
		transitionSource := filepath.Join(transitionDirectory, "root.yaml")
		for index := range 10000 {
			if err := os.Link(transitionSource, filepath.Join(transitionDirectory, fmt.Sprintf("entry-%05d.txt", index))); err != nil {
				t.Fatalf("create transition entry: %v", err)
			}
		}
		transition := runCLIWhileRevokingDirectory(t, ctx, binary, workDir, coverageDir, filepath.Join("scenarios", "transition-directory"))
		if transition.exitCode != 102 || transition.stderr == "" {
			t.Fatalf("transition-directory result = code %d, stderr %q, want configuration failure", transition.exitCode, transition.stderr)
		}
	})
}

func runCLIWithClosedPTY(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string) cliResult {
	t.Helper()
	master, slave := openPTY(t)
	cmd := exec.CommandContext(ctx, binary, suite)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+coverageDir)
	var stderr strings.Builder
	cmd.Stdout = slave
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start closed-PTY %s: %v", suite, err)
	}
	_ = slave.Close()

	output := make([]byte, 0, 256)
	buffer := make([]byte, 1)
	for len(output) < 4096 {
		if _, err := master.Read(buffer); err != nil {
			t.Fatalf("read working-directory output for %s: %v", suite, err)
		}
		output = append(output, buffer[0])
		if strings.HasSuffix(string(output), "\n\n") || strings.HasSuffix(string(output), "\r\n\r\n") {
			break
		}
	}
	_ = master.Close()
	err := cmd.Wait()
	if err == nil {
		return cliResult{stderr: stderr.String()}
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run closed-PTY %s: %v", suite, err)
	}
	return cliResult{exitCode: exitErr.ExitCode(), stderr: stderr.String()}
}

func openPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("open PTY master: %v", err)
	}
	var unlock int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		_ = master.Close()
		t.Fatalf("unlock PTY: %v", errno)
	}
	var number uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&number))); errno != 0 {
		_ = master.Close()
		t.Fatalf("get PTY number: %v", errno)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		t.Fatalf("open PTY slave: %v", err)
	}
	return master, slave
}

func runCLIWhileRevokingDirectory(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string) cliResult {
	t.Helper()
	directory := filepath.Join(workDir, suite)
	watch, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		t.Fatalf("create directory watch: %v", err)
	}
	defer syscall.Close(watch)
	if _, err := syscall.InotifyAddWatch(watch, directory, syscall.IN_OPEN); err != nil {
		t.Fatalf("watch transition directory: %v", err)
	}

	cmd := exec.CommandContext(ctx, binary, suite)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+coverageDir)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start transition-directory CLI: %v", err)
	}
	events := make([]byte, 4096)
	directoryOpened := false
	for !directoryOpened {
		count, err := syscall.Read(watch, events)
		if err != nil {
			t.Fatalf("read directory event: %v", err)
		}
		for offset := 0; offset+syscall.SizeofInotifyEvent <= count; {
			event := (*syscall.InotifyEvent)(unsafe.Pointer(&events[offset]))
			if event.Len == 0 {
				directoryOpened = true
				break
			}
			offset += syscall.SizeofInotifyEvent + int(event.Len)
		}
	}
	if err := os.Chmod(directory, 0); err != nil {
		t.Fatalf("revoke transition directory: %v", err)
	}
	err = cmd.Wait()
	if chmodErr := os.Chmod(directory, 0o755); chmodErr != nil {
		t.Fatalf("restore transition directory: %v", chmodErr)
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run transition-directory CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return cliResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}
