//go:build windows

package runner

import "strings"

func shellQuote(value string) string {
	value = escapePercentExpansion(value)
	if value != "" && strings.IndexFunc(value, func(char rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%^+=:,./-\\", char)
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
