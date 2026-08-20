package skeleton

import (
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

	t.Run("fatal diagnostics belong to cli", func(t *testing.T) {
		assertPatternsAbsentOutside(
			t,
			filepath.FromSlash("cmd/cli"),
			[]string{"log.Print(", "log.Printf(", "log.Println("},
		)
	})

	t.Run("bat is not a command dependency", func(t *testing.T) {
		assertPatternsAbsentOutside(t, "", []string{"BatDiff", "exec.Command(\"bat\"", "exec.CommandContext(ctx, \"bat\""})
	})
}

func assertPatternsAbsentOutside(t *testing.T, allowedDir string, patterns []string) {
	t.Helper()

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
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
		t.Fatalf("scan skeleton source: %v", err)
	}
}
