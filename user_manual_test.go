package apih_test

import (
	"apih/internal/domain"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/goccy/go-yaml"
)

const userManualPath = "docs/user-manual/apih.md"

func TestUserManualAcceptanceCriterion1AgentFirstSingleDocument(t *testing.T) {
	manual := readUserManual(t)
	manuals, err := filepath.Glob("docs/user-manual/*.md")
	if err != nil {
		t.Fatalf("Glob(): %v", err)
	}
	if want := []string{userManualPath}; !slices.Equal(manuals, want) {
		t.Fatalf("user manuals = %v, want %v", manuals, want)
	}
	if got := manualH1Count(manual); got != 1 || !strings.HasPrefix(manual, "# APIHydra (`apih`) user manual\n") {
		t.Fatalf("manual must have exactly one H1 at its beginning")
	}

	required := []string{
		"This manual is written for coding agents first and humans too.",
		"## Contents",
		"## Quick start",
		"## YAML document reference",
		"## Task recipes",
		"## Agent checklist",
	}
	assertContainsAll(t, manual, required)
	assertManualLinks(t, manual)
}

func TestUserManualAcceptanceCriterion2QuickStartCLIAndExitCodes(t *testing.T) {
	manual := readUserManual(t)
	required := []string{
		"go build -o ./bin/apih ./cmd/cli",
		"apih [flags] [directory]",
		"`-h`, `--help`",
		"`-p`, `--parallelism`",
		"`-p0`, `-p 0`",
		"`--parallelism=2`, `--parallelism 2`",
		"`--` ends flag parsing",
		"| `0` | Success",
		"| `101` | One or more response validations failed.",
		"| `102` | Invocation, selected-directory, YAML, definition, or other configuration failure.",
		"| `103` | Internal execution",
		"Some terminal external-command paths preserve that command's nonzero exit code",
		"curl",
		"jq",
		"git",
	}
	assertContainsAll(t, manual, required)
}

func TestUserManualAcceptanceCriterion3CompleteConfigurationReference(t *testing.T) {
	manual := readUserManual(t)
	requiredKeys := []string{
		"`app`",
		"`kind`",
		"`metadata.name`",
		"`metadata.labels`",
		"`spec.base_url`",
		"`spec.base_path`",
		"`spec.headers`",
		"`spec.disable_cookies`",
		"`spec.timeout`",
		"`spec.retries`",
		"`spec.defaults`",
		"`spec.steps`",
		"`spec.steps[].vars`",
		"`spec.steps[].request.path`",
		"`spec.steps[].request.method`",
		"`spec.steps[].request.query`",
		"`spec.steps[].request.body`",
		"`spec.steps[].request.defaults`",
		"`spec.steps[].response.expected_status`",
		"`spec.steps[].response.expected_body`",
		"`spec.steps[].response.expected_types`",
		"`spec.steps[].response.capture`",
		"`spec.steps[].debug`",
	}
	assertContainsAll(t, manual, requiredKeys)
	assertContainsAll(t, manual, []string{
		"### Runtime-owned fields",
		"`response.actual_status` and `response.actual_body`",
		"Do not set these as suite input",
	})
}

func TestUserManualAcceptanceCriterion4BindingBehavior(t *testing.T) {
	manual := readUserManual(t)
	required := []string{
		"parent directory\n  -> current directory root/defaults document\n  -> steps-file spec.defaults\n  -> step request.defaults",
		"timeout: 0",
		"absent inherits, `true` disables, and\n`false` explicitly re-enables",
		"resolved base_url + resolved base_path + request.path",
		"one concurrent, write-once string store",
		"`expected_status: 0` mean `<any>`",
		"Extra actual object fields are ignored.",
		"Exactly one run jar.",
		"Exactly one jar per directory.",
		"Exactly one jar per steps file.",
		"intentionally nondeterministic",
		"Debug output is complete and unredacted",
		"stage, directory, steps file, then step",
		"`os.UserCacheDir()/apih/run-*`",
	}
	assertContainsAll(t, manual, required)
}

func TestUserManualAcceptanceCriterion5CompleteExamplesDecodeAndCoverFields(t *testing.T) {
	manual := readUserManual(t)
	for index, block := range extractYAMLBlocks(t, manual) {
		var decoded any
		if err := yaml.Unmarshal([]byte(block), &decoded); err != nil {
			t.Errorf("YAML example %d does not decode: %v", index+1, err)
		}
	}
	examples := extractCompleteYAML(t, manual)
	wantPaths := []string{
		"manual-suite/root.yaml",
		"manual-suite/session-steps.yaml",
		"manual-suite/health-steps.yml",
		"manual-suite/items/defaults.yaml",
		"manual-suite/items/item-steps.yaml",
	}
	gotPaths := make([]string, 0, len(examples))
	for _, example := range examples {
		gotPaths = append(gotPaths, example.path)
		assertDefinitionDecodes(t, example)
	}
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("complete example paths = %v, want %v", gotPaths, wantPaths)
	}

	root := exampleMapping(t, examples[0])
	assertMapPaths(t, root, [][]string{
		{"app"}, {"kind"}, {"metadata", "name"}, {"metadata", "labels"},
		{"spec", "base_url"}, {"spec", "base_path"}, {"spec", "headers"},
		{"spec", "disable_cookies"}, {"spec", "timeout"}, {"spec", "retries"},
	})

	session := exampleMapping(t, examples[1])
	assertMapPaths(t, session, [][]string{
		{"app"}, {"kind"}, {"metadata", "name"}, {"metadata", "labels"},
		{"spec", "defaults"}, {"spec", "steps"},
	})
	spec := mapValue(t, session, "spec")
	assertMapPaths(t, mapValue(t, spec, "defaults"), defaultsPaths())
	steps, ok := spec["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("session spec.steps = %#v, want at least two steps", spec["steps"])
	}
	step, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("first step = %T, want mapping", steps[0])
	}
	assertMapPaths(t, step, [][]string{{"vars"}, {"request"}, {"response"}, {"debug"}})
	request := mapValue(t, step, "request")
	assertMapPaths(t, request, [][]string{
		{"path"}, {"method"}, {"query"}, {"body"}, {"defaults"},
	})
	assertMapPaths(t, mapValue(t, request, "defaults"), defaultsPaths())
	response := mapValue(t, step, "response")
	assertMapPaths(t, response, [][]string{
		{"expected_status"}, {"expected_body"}, {"expected_types"}, {"capture"},
	})

	assertContainsAll(t, manual, []string{
		"fixture is listening on `127.0.0.1:18080` with this contract",
		"# Type mismatch",
		"# Status mismatch",
		"# Body mismatch",
		"Cookie: manual_session=example",
		"debug: true",
	})
}

func TestUserManualAcceptanceCriterion6TroubleshootingAndUnspecifiedBoundaries(t *testing.T) {
	manual := readUserManual(t)
	required := []string{
		"## Troubleshooting",
		"Missing variable",
		"Duplicate variable or capture",
		"Invalid jq selector",
		"Invalid expected or actual JSON",
		"Exit `101`",
		"Exit `102`",
		"Exit `103`",
		"cross-file value",
		"unexpected cookie in mode `2`",
		"Debug exposed a secret",
		"not a published contract",
		"not stable product\ncontracts",
	}
	assertContainsAll(t, manual, required)
}

func TestUserManualAcceptanceCriterion7CompactStructure(t *testing.T) {
	manual := readUserManual(t)
	wordCount := len(strings.Fields(manual))
	if wordCount < 3_000 || wordCount > 5_500 {
		t.Fatalf("manual word count = %d, want exhaustive but compact range 3000..5500", wordCount)
	}
	if strings.Contains(manual, "\n\n\n") {
		t.Fatal("manual contains redundant consecutive blank lines")
	}

	headings := manualHeadings(manual)
	seen := make(map[string]struct{}, len(headings))
	for _, heading := range headings {
		anchor := markdownAnchor(heading)
		if _, duplicate := seen[anchor]; duplicate {
			t.Fatalf("duplicate heading anchor %q", anchor)
		}
		seen[anchor] = struct{}{}
	}
}

type manualExample struct {
	path string
	yaml string
}

func readUserManual(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(userManualPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", userManualPath, err)
	}
	return string(contents)
}

func assertContainsAll(t *testing.T, document string, required []string) {
	t.Helper()
	for _, text := range required {
		if !strings.Contains(document, text) {
			t.Errorf("manual is missing %q", text)
		}
	}
}

func assertManualLinks(t *testing.T, manual string) {
	t.Helper()
	headings := make(map[string]struct{})
	for _, heading := range manualHeadings(manual) {
		headings[markdownAnchor(heading)] = struct{}{}
	}

	linkPattern := regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	for _, match := range linkPattern.FindAllStringSubmatch(manual, -1) {
		target := match[1]
		if strings.HasPrefix(target, "#") {
			if _, ok := headings[strings.TrimPrefix(target, "#")]; !ok {
				t.Errorf("broken manual anchor %q", target)
			}
			continue
		}
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			continue
		}
		path := filepath.Join(filepath.Dir(userManualPath), target)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("broken manual-relative link %q: %v", target, err)
		}
	}
}

func manualHeadings(manual string) []string {
	var headings []string
	for _, line := range strings.Split(manual, "\n") {
		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		if level >= 2 && level < len(line) && line[level] == ' ' {
			headings = append(headings, strings.TrimSpace(line[level+1:]))
		}
	}
	return headings
}

func manualH1Count(manual string) int {
	count := 0
	inFence := false
	for _, line := range strings.Split(manual, "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(line, "# ") {
			count++
		}
	}
	return count
}

func markdownAnchor(heading string) string {
	var anchor strings.Builder
	for _, character := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(character), unicode.IsDigit(character), character == '-':
			anchor.WriteRune(character)
		case unicode.IsSpace(character):
			anchor.WriteByte('-')
		}
	}
	return anchor.String()
}

func extractCompleteYAML(t *testing.T, manual string) []manualExample {
	t.Helper()
	lines := strings.Split(manual, "\n")
	var examples []manualExample
	for index, line := range lines {
		const prefix = "<!-- complete-yaml: "
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, " -->") {
			continue
		}
		path := strings.TrimSuffix(strings.TrimPrefix(line, prefix), " -->")
		fence := index + 1
		for fence < len(lines) && lines[fence] != "```yaml" {
			fence++
		}
		if fence == len(lines) {
			t.Fatalf("complete YAML %q has no yaml fence", path)
		}
		end := fence + 1
		for end < len(lines) && lines[end] != "```" {
			end++
		}
		if end == len(lines) {
			t.Fatalf("complete YAML %q has no closing fence", path)
		}
		examples = append(examples, manualExample{
			path: path,
			yaml: strings.Join(lines[fence+1:end], "\n") + "\n",
		})
	}
	return examples
}

func extractYAMLBlocks(t *testing.T, manual string) []string {
	t.Helper()
	lines := strings.Split(manual, "\n")
	var blocks []string
	for index := 0; index < len(lines); index++ {
		if lines[index] != "```yaml" {
			continue
		}
		end := index + 1
		for end < len(lines) && lines[end] != "```" {
			end++
		}
		if end == len(lines) {
			t.Fatalf("YAML block at line %d has no closing fence", index+1)
		}
		blocks = append(blocks, strings.Join(lines[index+1:end], "\n")+"\n")
		index = end
	}
	return blocks
}

func assertDefinitionDecodes(t *testing.T, example manualExample) {
	t.Helper()
	var envelope struct {
		App  string `yaml:"app"`
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal([]byte(example.yaml), &envelope); err != nil {
		t.Fatalf("decode envelope %q: %v", example.path, err)
	}
	if envelope.App != "apihydra" {
		t.Errorf("%s app = %q, want apihydra", example.path, envelope.App)
	}
	switch domain.DocumentKind(envelope.Kind) {
	case domain.KindRoot, domain.KindDefaults:
		var definition domain.DefaultsDefinition
		if err := yaml.Unmarshal([]byte(example.yaml), &definition); err != nil {
			t.Fatalf("decode defaults definition %q: %v", example.path, err)
		}
	case domain.KindSteps:
		var definition domain.StepsDefinition
		if err := yaml.Unmarshal([]byte(example.yaml), &definition); err != nil {
			t.Fatalf("decode steps definition %q: %v", example.path, err)
		}
	default:
		t.Fatalf("%s kind = %q, want a supported document kind", example.path, envelope.Kind)
	}
}

func exampleMapping(t *testing.T, example manualExample) map[string]any {
	t.Helper()
	var mapping map[string]any
	if err := yaml.Unmarshal([]byte(example.yaml), &mapping); err != nil {
		t.Fatalf("decode mapping %q: %v", example.path, err)
	}
	return mapping
}

func mapValue(t *testing.T, mapping map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := mapping[key].(map[string]any)
	if !ok {
		t.Fatalf("%q = %T, want mapping", key, mapping[key])
	}
	return value
}

func assertMapPaths(t *testing.T, mapping map[string]any, paths [][]string) {
	t.Helper()
	for _, path := range paths {
		current := mapping
		for index, key := range path {
			value, ok := current[key]
			if !ok {
				t.Errorf("complete example is missing %s", strings.Join(path, "."))
				break
			}
			if index == len(path)-1 {
				break
			}
			current, ok = value.(map[string]any)
			if !ok {
				t.Errorf("complete example path %s is %T, want mapping", strings.Join(path[:index+1], "."), value)
				break
			}
		}
	}
}

func defaultsPaths() [][]string {
	return [][]string{
		{"base_url"},
		{"base_path"},
		{"headers"},
		{"disable_cookies"},
		{"timeout"},
		{"retries"},
	}
}
