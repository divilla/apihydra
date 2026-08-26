//go:build !windows

package runner

import (
	"context"
	"strings"
)

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(char rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", char)
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func curlShellCommand(command, body string) (string, error) {
	if body == "" {
		return command, nil
	}
	return "printf '%s' " + shellQuote(body) + " | " + command, nil
}

func executeCurlShellCommand(ctx context.Context, command string) (string, string, int, error) {
	return execute(ctx, "/bin/sh", command)
}
