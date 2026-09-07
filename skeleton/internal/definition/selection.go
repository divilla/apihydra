package definition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/divilla/apihydra/skeleton/internal/domain"
	"github.com/divilla/apihydra/skeleton/pkg/errs"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
)

// ErrInvalidSelection classifies an invalid target, selector, or combination
// of suite roots. It uses configuration exit code 102 and the invalid-arguments
// manual anchor. The diagnostic retains the offending selection.
var ErrInvalidSelection = errors.New("invalid selection")

// Select resolves targets relative to suite.WorkDir, defaults to that directory
// when targets is empty, and finds each target's nearest qualifying root.
// Step suffixes apply only to the final path component. Target paths resolve
// symlinks before parent components, root discovery, and scope matching.
// Paths use filesystem spelling before root and scope comparisons where parent
// listing is permitted; otherwise they retain the accessible component spelling.
// All targets must share that root. On success it commits the discovered root
// to WorkDir and normalized targets to Selections; on failure neither changes.
// It performs no recursive discovery, output, or run-cache creation. Full
// definition and source-index validation follow in decoding and resolution.
func (l *Loader) Select(ctx context.Context, suite *domain.Suite, targets []string) error {
	if len(targets) == 0 {
		targets = []string{"."}
	}
	selections := make([]domain.Selection, 0, len(targets))
	root := ""
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, target)
		}
		selection, err := parseSelection(suite.WorkDir, target)
		if err != nil {
			return err
		}
		start := selection.Path
		if !selection.Directory {
			start = filepath.Dir(start)
		}
		found, err := nearestRoot(ctx, start)
		if err != nil {
			return err
		}
		if root != "" && root != found {
			return selectionError(target, "selections must share one suite root", nil)
		}
		root = found
		selections = append(selections, selection)
	}
	suite.WorkDir, suite.Selections = root, selections
	return nil
}

func selectionError(target, reason string, cause error) error {
	return errs.Build(errs.ExitConfiguration, ErrInvalidSelection, cause, "selection "+strconv.Quote(target)+": "+reason)
}

func parseSelection(workDir, target string) (domain.Selection, error) {
	selection := domain.Selection{Last: -1}
	path := target
	// Only the final component can carry a step suffix; preserve parent and
	// Windows volume colons as part of the path.
	parent, _ := filepath.Split(target)
	if i := strings.LastIndex(target, ":"); i >= len(parent) && i >= len(filepath.VolumeName(target)) {
		path = target[:i]
		bounds := strings.Split(target[i+1:], "-")
		if len(bounds) > 2 {
			return selection, selectionError(target, "expected N or N-M", nil)
		}
		var err error
		selection.First, err = selectionIndex(bounds[0])
		if err != nil {
			return selection, selectionError(target, "expected a non-negative decimal index", err)
		}
		selection.Last = selection.First
		if len(bounds) == 2 {
			selection.Last, err = selectionIndex(bounds[1])
			if err != nil {
				return selection, selectionError(target, "expected a non-negative decimal index", err)
			}
		}
		if selection.Last < selection.First {
			return selection, selectionError(target, "range start exceeds end", nil)
		}
	}
	if path == "" {
		return selection, selectionError(target, "target path is empty", nil)
	}
	if !filepath.IsAbs(path) && workDir != "" {
		// Joining or making the path absolute would clean symlink/.. before
		// the filesystem can resolve the symlink's actual parent.
		path = workDir + string(filepath.Separator) + path
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return selection, selectionError(target, "cannot resolve target symlinks", err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return selection, selectionError(target, "cannot resolve path", err)
	}
	absolute, err = filesystemPath(absolute)
	if err != nil {
		return selection, selectionError(target, "cannot resolve filesystem spelling", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return selection, selectionError(target, "cannot inspect target", err)
	}
	selection.Path, selection.Directory = absolute, info.IsDir()
	if selection.Directory {
		if selection.Last >= 0 {
			return selection, selectionError(target, "step selectors require a steps file", nil)
		}
	} else if !info.Mode().IsRegular() || !isRootYAMLFile(absolute) {
		return selection, selectionError(target, "expected a directory or regular .yaml/.yml steps file", nil)
	}
	return selection, nil
}

// filesystemPath canonicalizes an absolute, symlink-resolved path one component
// at a time. Prefer exact names so distinct case-sensitive names and hard links
// remain distinct; otherwise let the filesystem determine identity.
func filesystemPath(path string) (string, error) {
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	parent, err := filesystemPath(parent)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(parent)
	if errors.Is(err, os.ErrPermission) {
		// Accessing a known path requires search permission on its ancestors,
		// not listing permission. Preserve its spelling when enumeration is denied.
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		return filepath.Join(parent, filepath.Base(path)), nil
	}
	if err != nil {
		return "", err
	}
	name, err := filesystemName(path, entries)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, name), nil
}

// filesystemName finds the stored name without conflating unrelated hard links.
func filesystemName(path string, entries []os.DirEntry) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	name := filepath.Base(path)
	for _, entry := range entries {
		if entry.Name() == name {
			entries = []os.DirEntry{entry}
			break
		}
	}
	for _, entry := range entries {
		// Identity alone could choose an unrelated hard link to the target.
		if !strings.EqualFold(entry.Name(), name) {
			continue
		}
		candidate, err := entry.Info()
		if err != nil {
			return "", err
		}
		if os.SameFile(info, candidate) {
			return entry.Name(), nil
		}
	}
	return "", &os.PathError{Op: "resolve", Path: path, Err: os.ErrNotExist}
}

func selectionIndex(value string) (int, error) {
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, strconv.ErrSyntax
		}
	}
	return strconv.Atoi(value)
}

func nearestRoot(ctx context.Context, start string) (string, error) {
	for directory := start; ; directory = filepath.Dir(directory) {
		err := validateRootDefinition(ctx, directory)
		if err == nil {
			return directory, nil
		}
		if !errors.Is(err, ErrRootDefinitionMissing) {
			return "", err
		}
		if filepath.Dir(directory) == directory {
			return "", err
		}
	}
}

func withinDirectory(path, directory string) bool {
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func directorySelected(selections []domain.Selection, path string) bool {
	if len(selections) == 0 {
		return true
	}
	for _, selection := range selections {
		if selection.Directory && withinDirectory(path, selection.Path) {
			return true
		}
	}
	return false
}

func directoryNeeded(selections []domain.Selection, path string) bool {
	if directorySelected(selections, path) {
		return true
	}
	for _, selection := range selections {
		if withinDirectory(selection.Path, path) {
			return true
		}
	}
	return false
}

func fileSelected(selections []domain.Selection, path string) bool {
	if directorySelected(selections, filepath.Dir(path)) {
		return true
	}
	for _, selection := range selections {
		if !selection.Directory && selection.Path == path {
			return true
		}
	}
	return false
}

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
		lines := strings.SplitAfter(string(contents), "\n")
		indent := -1
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "---" || trimmed == "..." || strings.HasPrefix(trimmed, "%") {
				continue
			}
			width := len(line) - len(strings.TrimLeft(line, " "))
			if indent == -1 || width < indent {
				indent = width
			}
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
	for _, tk := range lexer.Tokenize(contents) {
		if depth == 0 {
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

func filterSelectedSteps(suite *domain.Suite, resolved map[*domain.Directory][][]domain.Step) error {
	if len(suite.Selections) == 0 {
		return nil
	}
	definitions := make(map[string]*domain.StepsDefinition)
	for directory := range resolved {
		for _, definition := range directory.StepsDefinitions {
			if definition != nil {
				definitions[filepath.Join(suite.WorkDir, filepath.FromSlash(definition.File.Path))] = definition
			}
		}
	}
	// Validate every explicit target before applying any overlapping union.
	for _, selection := range suite.Selections {
		if selection.Directory {
			continue
		}
		definition := definitions[selection.Path]
		if definition == nil || definition.App != "apihydra" || definition.Kind != domain.KindSteps {
			return selectionError(selection.Path, "expected app: apihydra and kind: steps", nil)
		}
		if selection.Last >= len(definition.Spec.Steps) {
			return selectionError(selection.Path+":"+strconv.Itoa(selection.First)+"-"+strconv.Itoa(selection.Last), "step index out of bounds", nil)
		}
	}
	for directory, groups := range resolved {
		for i, steps := range groups {
			definition := directory.StepsDefinitions[i]
			if definition == nil {
				continue
			}
			path := filepath.Join(suite.WorkDir, filepath.FromSlash(definition.File.Path))
			if directorySelected(suite.Selections, filepath.Dir(path)) {
				continue
			}
			selected := make([]domain.Step, 0, len(steps))
			for index, step := range steps {
				for _, selection := range suite.Selections {
					if !selection.Directory && selection.Path == path && (selection.Last == -1 || (index >= selection.First && index <= selection.Last)) {
						selected = append(selected, step)
						break
					}
				}
			}
			groups[i] = selected
		}
	}
	return nil
}
