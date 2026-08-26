//go:build windows

package runner

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
)

func shellQuote(value string) string {
	value = escapePercentExpansion(value)
	if value != "" && strings.IndexFunc(value, func(char rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-\\", char)
	}) < 0 {
		return value
	}

	var quoted strings.Builder
	quoted.WriteByte('"')
	backslashes := 0
	for _, char := range value {
		if char == '\\' {
			backslashes++
			continue
		}
		if char == '"' {
			quoted.WriteString(strings.Repeat("\\", backslashes*2+1))
			quoted.WriteRune(char)
			backslashes = 0
			continue
		}
		quoted.WriteString(strings.Repeat("\\", backslashes))
		backslashes = 0
		quoted.WriteRune(char)
	}
	quoted.WriteString(strings.Repeat("\\", backslashes*2))
	quoted.WriteByte('"')
	return quoted.String()
}

func curlShellCommand(command, body string) (string, error) {
	if body == "" {
		return command + "\r\nexit /b %errorlevel%", nil
	}

	bodyFile, err := os.CreateTemp("", "apih-curl-body-")
	if err != nil {
		return "", err
	}
	bodyPath := bodyFile.Name()
	if err := bodyFile.Close(); err != nil {
		os.Remove(bodyPath)
		return "", err
	}
	if err := os.Remove(bodyPath); err != nil {
		return "", err
	}
	encodedPath := bodyPath + ".b64"
	encoded := base64.StdEncoding.EncodeToString([]byte(body))

	lines := make([]string, 0, len(encoded)/4096+8)
	for start := 0; start < len(encoded); start += 4096 {
		end := min(start+4096, len(encoded))
		redirect := ">"
		if start > 0 {
			redirect = ">>"
		}
		lines = append(lines, redirect+shellQuote(encodedPath)+" echo "+encoded[start:end])
	}
	lines = append(lines,
		"certutil -f -decode "+shellQuote(encodedPath)+" "+shellQuote(bodyPath)+" >nul",
		"if errorlevel 1 exit /b %errorlevel%",
		"del /q "+shellQuote(encodedPath),
		"type "+shellQuote(bodyPath)+" | "+command,
		`set "apih_curl_exit=%errorlevel%"`,
		"del /q "+shellQuote(bodyPath),
		"exit /b %apih_curl_exit%",
	)
	return strings.Join(lines, "\r\n"), nil
}

func executeCurlShellCommand(ctx context.Context, command string) (string, string, int, error) {
	return execute(ctx, "cmd.exe", command, "/d", "/q")
}

func escapePercentExpansion(value string) string {
	var escaped strings.Builder
	start := 0
	for {
		opening := strings.IndexByte(value[start:], '%')
		if opening < 0 {
			escaped.WriteString(value[start:])
			return escaped.String()
		}
		opening += start
		closing := strings.IndexByte(value[opening+1:], '%')
		if closing < 0 {
			escaped.WriteString(value[start:])
			return escaped.String()
		}
		closing += opening + 1
		escaped.WriteString(value[start:closing])
		escaped.WriteByte('^')
		escaped.WriteByte('%')
		start = closing + 1
	}
}
