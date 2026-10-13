package reporting

import (
	"encoding/json"
	"strings"

	"github.com/divilla/apihydra/internal/domain"
)

type debugStep struct {
	Index    int                          `json:"index"`
	Vars     map[string]domain.YAMLString `json:"vars"`
	Request  debugRequest                 `json:"request"`
	Response debugResponse                `json:"response"`
	Debug    bool                         `json:"debug"`
}

type debugRequest struct {
	Path     string          `json:"path"`
	Method   string          `json:"method"`
	Query    string          `json:"query"`
	Body     any             `json:"body"`
	Defaults domain.Defaults `json:"defaults"`
}

type debugResponse struct {
	ExpectedStatus int                          `json:"expected_status"`
	ActualStatus   int                          `json:"actual_status"`
	ExpectedBody   any                          `json:"expected_body"`
	ActualBody     any                          `json:"actual_body"`
	ExpectedTypes  map[string][]string          `json:"expected_types"`
	Capture        map[string]domain.YAMLString `json:"capture"`
}

func debugStepValue(step *domain.Step) any {
	if step == nil {
		return nil
	}
	return debugStep{
		Index: step.Index,
		Vars:  step.Vars,
		Request: debugRequest{
			Path:     step.Request.Path,
			Method:   step.Request.Method,
			Query:    step.Request.Query,
			Body:     debugBodyValue(step.Request.Body),
			Defaults: step.Request.Defaults,
		},
		Response: debugResponse{
			ExpectedStatus: step.Response.ExpectedStatus,
			ActualStatus:   step.Response.ActualStatus,
			ExpectedBody:   debugBodyValue(step.Response.ExpectedBody),
			ActualBody:     debugBodyValue(step.Response.ActualBody),
			ExpectedTypes:  step.Response.ExpectedTypes,
			Capture:        step.Response.Capture,
		},
		Debug: step.Debug,
	}
}

func debugBodyValue(body domain.YAMLString) any {
	if json.Valid([]byte(body)) {
		return json.RawMessage(body)
	}
	return body
}

func colorizeJQJSON(input string) string {
	const (
		reset       = "\x1b[0m"
		punctuation = "\x1b[1;39m"
		key         = "\x1b[1;34m"
		stringValue = "\x1b[0;32m"
		scalar      = "\x1b[0;39m"
		nullValue   = "\x1b[0;90m"
	)

	var colored strings.Builder
	for index := 0; index < len(input); {
		switch input[index] {
		case ' ', '\t', '\r', '\n':
			colored.WriteByte(input[index])
			index++
		case '"':
			end := index + 1
			for end < len(input) {
				if input[end] == '\\' {
					end += 2
					continue
				}
				end++
				if input[end-1] == '"' {
					break
				}
			}
			next := end
			for next < len(input) && (input[next] == ' ' || input[next] == '\t') {
				next++
			}
			color := stringValue
			if next < len(input) && input[next] == ':' {
				color = key
			}
			colored.WriteString(color)
			colored.WriteString(input[index:end])
			colored.WriteString(reset)
			index = end
		case '{', '}', '[', ']', ',', ':':
			colored.WriteString(punctuation)
			colored.WriteByte(input[index])
			colored.WriteString(reset)
			index++
		default:
			end := index
			for end < len(input) && !strings.ContainsRune(" \t\r\n,]}:", rune(input[end])) {
				end++
			}
			color := scalar
			if input[index:end] == "null" {
				color = nullValue
			}
			colored.WriteString(color)
			colored.WriteString(input[index:end])
			colored.WriteString(reset)
			index = end
		}
	}
	return colored.String()
}
