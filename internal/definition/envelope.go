package definition

import (
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
)

// inheritedDefinition recognizes defaults even when their body is malformed.
// On a syntax error, independently readable top-level envelope fields still
// identify required block or flow documents, regardless of their order around spec.
// Recover each field with its continuation lines to preserve multiline scalars.
func inheritedDefinition(contents []byte) bool {
	type envelope struct {
		App  any `yaml:"app"`
		Kind any `yaml:"kind"`
	}
	var decoded envelope
	if err := yaml.Unmarshal(contents, &decoded); err != nil {
		decoded = envelope{}
		fields := flowMappingFields(string(contents))
		// YAML accepts CR, LF, and CRLF; preserve one break per logical line
		// when recovering fields, including multiline scalar continuations.
		normalized := strings.ReplaceAll(string(contents), "\r\n", "\n")
		lines := strings.SplitAfter(strings.ReplaceAll(normalized, "\r", "\n"), "\n")
		indent := -1
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Document markers may carry a comment after separating whitespace.
			marker := trimmed
			if len(marker) > 3 && (marker[3] == ' ' || marker[3] == '\t') && strings.HasPrefix(strings.TrimSpace(marker[3:]), "#") {
				marker = marker[:3]
			}
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || marker == "---" || marker == "..." || strings.HasPrefix(trimmed, "%") {
				continue
			}
			// The first content line establishes the envelope indentation;
			// a malformed, dedented body must not replace it.
			indent = len(line) - len(strings.TrimLeft(line, " "))
			break
		}
		for start := 0; start < len(lines); {
			line := lines[start]
			if len(line)-len(strings.TrimLeft(line, " ")) != indent {
				start++
				continue
			}
			end := start + 1
			for end < len(lines) {
				trimmed := strings.TrimSpace(lines[end])
				if trimmed != "" && !strings.HasPrefix(trimmed, "#") && len(lines[end])-len(strings.TrimLeft(lines[end], " ")) <= indent {
					break
				}
				end++
			}
			fields = append(fields, strings.Join(lines[start:end], ""))
			start = end
		}
		for _, contents := range fields {
			var field envelope
			if yaml.Unmarshal([]byte(contents), &field) != nil {
				continue
			}
			if field.App != nil {
				decoded.App = field.App
			}
			if field.Kind != nil {
				decoded.Kind = field.Kind
			}
		}
	}
	return decoded.App == "apihydra" && (decoded.Kind == "root" || decoded.Kind == "defaults")
}

// flowMappingFields isolates top-level flow entries so a malformed body does
// not hide readable envelope fields. Tokens preserve quoting and nested values.
func flowMappingFields(contents string) []string {
	var fields []string
	var field strings.Builder
	depth := 0
	directiveLine := 0
	for _, tk := range lexer.Tokenize(contents) {
		if depth == 0 {
			if tk.Type == token.DirectiveType {
				// Directive names and arguments are separate tokens on this line.
				directiveLine = tk.Position.Line
			}
			if tk.Position.Line == directiveLine {
				continue
			}
			if tk.Type == token.CommentType || tk.Type == token.DocumentHeaderType {
				continue
			}
			if tk.Type != token.MappingStartType {
				return nil
			}
			depth = 1
			continue
		}
		switch tk.Type {
		case token.MappingStartType, token.SequenceStartType:
			depth++
		case token.MappingEndType, token.SequenceEndType:
			depth--
		}
		if (tk.Type == token.CollectEntryType && depth == 1) || depth == 0 {
			fields = append(fields, "{"+field.String()+"\n}")
			field.Reset()
			if depth == 0 {
				return fields
			}
			continue
		}
		field.WriteString(tk.Origin)
	}
	if field.Len() > 0 {
		fields = append(fields, "{"+field.String()+"\n}")
	}
	return fields
}
