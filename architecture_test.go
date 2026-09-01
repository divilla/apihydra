package apih_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageBoundaries(t *testing.T) {
	t.Run("commands belong to runner", func(t *testing.T) {
		assertPatternsAbsentOutside(
			t,
			filepath.FromSlash("pkg/runner"),
			[]string{`"os/exec"`, "exec.Command(", "exec.CommandContext(", "os.StartProcess(", "syscall.Exec("},
		)
	})

	t.Run("contextual errors belong to errs", func(t *testing.T) {
		assertPatternsAbsentOutside(
			t,
			filepath.FromSlash("pkg/errs"),
			[]string{"fmt.Errorf(", "errors.Join("},
		)
	})

	t.Run("execution output belongs to reporting", func(t *testing.T) {
		assertPatternsAbsentOutside(
			t,
			filepath.FromSlash("internal/reporting"),
			[]string{"fmt.Fprint(", "fmt.Fprintf(", "fmt.Fprintln("},
		)
	})

	t.Run("fatal diagnostics belong to apih", func(t *testing.T) {
		assertPatternsAbsentOutside(
			t,
			filepath.FromSlash("cmd/apih"),
			[]string{"log.Print(", "log.Printf(", "log.Println(", "os.Stderr", "os.Exit("},
		)
	})

	t.Run("bat is not a command dependency", func(t *testing.T) {
		assertPatternsAbsentOutside(t, "", []string{"BatDiff", "exec.Command(\"bat\"", "exec.CommandContext(ctx, \"bat\""})
	})
}

func TestSharedDomainTypesHaveOneOwner(t *testing.T) {
	sharedTypes := map[string]struct{}{
		"Config":             {},
		"DocumentKind":       {},
		"Suite":              {},
		"Directory":          {},
		"File":               {},
		"BaseDefinition":     {},
		"DefaultsDefinition": {},
		"StepsDefinition":    {},
		"Metadata":           {},
		"Defaults":           {},
		"Step":               {},
		"YAMLString":         {},
	}

	err := walkProductionGoFiles(func(path string) error {
		if path == filepath.FromSlash("internal/domain/suite.go") || path == filepath.FromSlash("internal/domain/config.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range parsed.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				if _, shared := sharedTypes[typeSpec.Name.Name]; shared {
					t.Errorf("%s declares shared domain type %s outside internal/domain", path, typeSpec.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production source: %v", err)
	}
}

func TestSkeletonTODOConvention(t *testing.T) {
	err := filepath.WalkDir("skeleton", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(contents)
		for lineNumber, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "TODO") && strings.TrimSpace(line) != "// TODO: implement" {
				t.Errorf("%s:%d uses noncanonical TODO %q", path, lineNumber+1, strings.TrimSpace(line))
			}
		}
		if strings.Contains(text, "// TODO: implement") && strings.Contains(text, "panic(") {
			t.Errorf("%s mixes a TODO implementation marker with panic", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan skeleton TODOs: %v", err)
	}
}

func assertPatternsAbsentOutside(t *testing.T, allowedDir string, patterns []string) {
	t.Helper()

	err := walkProductionGoFiles(func(path string) error {
		if path == allowedDir || strings.HasPrefix(path, allowedDir+string(filepath.Separator)) {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, pattern := range patterns {
			if strings.Contains(string(contents), pattern) {
				t.Errorf("%s contains forbidden production pattern %q", path, pattern)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production source: %v", err)
	}
}

func walkProductionGoFiles(visit func(string) error) error {
	return filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == "skeleton" || path == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return visit(strings.TrimPrefix(path, "."+string(filepath.Separator)))
	})
}
