//go:build !windows

package runner

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"syscall"
)

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(char rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", char)
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func curlShellCommand(command, body string) (string, []string, error) {
	if body == "" {
		return command, nil, nil
	}
	return "printf '%s' " + shellQuote(body) + " | " + command, nil, nil
}

func executeCurlShellCommand(ctx context.Context, command string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	var stdout bytes.Buffer
	stderr, exitCode, err := runPreparedCommand(ctx, cmd, command, &stdout)
	return stdout.String(), stderr, exitCode, err
}
