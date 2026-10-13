package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/divilla/apihydra/internal/domain"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

func formatExpectedTypes(step *domain.Step, failed string) string {
	expectedTypes := map[string][]string(nil)
	if step != nil {
		expectedTypes = step.Response.ExpectedTypes
	}

	selectors := failedTypeSelectors(failed)
	if len(selectors) == 0 {
		selectors = make([]string, 0, len(expectedTypes))
		for selector := range expectedTypes {
			selectors = append(selectors, selector)
		}
		slices.Sort(selectors)
	}

	var block strings.Builder
	block.WriteString("    expected_types:\n")
	for _, selector := range selectors {
		expected, ok := expectedTypes[selector]
		if !ok {
			continue
		}
		fmt.Fprintf(
			&block,
			"        \x1b[38;5;15m%s:\x1b[0m \x1b[38;5;210m[%s]\x1b[0m\n",
			selector,
			strings.Join(expected, ", "),
		)
	}
	return block.String()
}

func failedTypeSelectors(failed string) []string {
	decoder := json.NewDecoder(strings.NewReader(failed))
	selectors := make([]string, 0)
	for {
		var declaration struct {
			Selector string `json:"selector"`
		}
		err := decoder.Decode(&declaration)
		if errors.Is(err, io.EOF) {
			return selectors
		}
		if err != nil {
			return nil
		}
		if declaration.Selector != "" {
			selectors = append(selectors, declaration.Selector)
		}
	}
}

func formatExpectedBody(diff string) string {
	var block strings.Builder
	block.WriteString("    expected_body:\n")
	trimmed := strings.TrimRight(diff, "\r\n")
	if trimmed == "" {
		return block.String()
	}
	for _, line := range strings.Split(trimmed, "\n") {
		block.WriteString("        ")
		block.WriteString(line)
		block.WriteByte('\n')
	}
	return block.String()
}

func (r *Reporter) writeValidation(ctx context.Context, step *domain.Step, field, validation string) error {
	var definition *domain.StepsDefinition
	if step != nil {
		definition = step.Definition
	}
	stepIndex := 0
	if step != nil {
		stepIndex = step.Index
	}
	return r.updateFile(ctx, definition, func(file *fileOutput) {
		if !file.failed {
			fmt.Fprintf(&file.block, "[\x1b[38;5;210m✗\x1b[0m] %s\n", definitionReference(definition))
			file.failed = true
		}
		if _, reported := file.failedSteps[stepIndex]; !reported {
			fmt.Fprintf(
				&file.block,
				"[\x1b[38;5;210m✗\x1b[0m] %s %s \x1b[38;5;117mline:%s\x1b[0m\n",
				calculatedPath(step),
				effectiveMethod(step),
				validationLine(step, field),
			)
			file.failedSteps[stepIndex] = struct{}{}
		}
		file.block.WriteString(validation)
		if final, _ := ctx.Value(r).(bool); final {
			file.block.WriteByte('\n')
		}
	})
}

func definitionReference(definition *domain.StepsDefinition) string {
	reference := "<unknown definition>"
	if definition != nil && definition.File != nil && definition.File.Path != "" {
		reference = filepath.ToSlash(definition.File.Path)
		reference = strings.TrimSuffix(reference, filepath.Ext(reference))
		if !strings.HasPrefix(reference, "/") {
			reference = "/" + reference
		}
	}
	return reference
}

func calculatedPath(step *domain.Step) string {
	path := "<unknown path>"
	if step != nil {
		path = step.Request.Defaults.BasePath + step.Request.Path
		if path == "" {
			path = "/"
		}
	}
	return path
}

func effectiveMethod(step *domain.Step) string {
	method := "<unknown method>"
	if step != nil {
		method = step.Request.Method
		if method == "" {
			method = "GET"
			if step.Request.Body != "" {
				method = "POST"
			}
		}
	}
	return method
}

// validationLine resolves the expectation key against the original file bytes.
// Missing keys fall back to the step position; unavailable sources stay explicit.
func validationLine(step *domain.Step, field string) string {
	if step == nil || step.Index < 0 || step.Definition == nil || step.Definition.File == nil {
		return "unknown"
	}
	stepPath := fmt.Sprintf("$.spec.steps[%d]", step.Index)
	// The nonnegative integer index makes this generated YAML path valid.
	path, _ := yaml.PathString(stepPath)
	node, err := path.ReadNode(bytes.NewReader(step.Definition.File.Bytes))
	if err != nil || node == nil {
		return "unknown"
	}
	for _, candidate := range ast.Filter(ast.MappingValueType, node) {
		mapping := candidate.(*ast.MappingValueNode)
		if mapping.Key.GetPath() == stepPath+".response."+field {
			return fmt.Sprint(mapping.Key.GetToken().Position.Line)
		}
	}
	return fmt.Sprint(node.GetToken().Position.Line)
}
