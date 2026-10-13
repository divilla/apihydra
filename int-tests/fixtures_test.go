//go:build integration

package inttests

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func createToolDirectory(t *testing.T, scripts map[string]string, links []string) string {
	t.Helper()
	directory := t.TempDir()
	for name, contents := range scripts {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o700); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	for _, name := range links {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("locate %s: %v", name, err)
		}
		if err := os.Symlink(path, filepath.Join(directory, name)); err != nil {
			t.Fatalf("link %s: %v", name, err)
		}
	}
	return directory
}

func copyFixture(t *testing.T, source, destination, serverURL string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		contents = []byte(strings.ReplaceAll(string(contents), serverMarker, serverURL))
		return os.WriteFile(target, contents, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture %s: %v", filepath.Base(source), err)
	}
}

func TestStaticFixturesAreYAML(t *testing.T) {
	inputRoot := filepath.Join(repositoryRoot(t), "int-tests", "input")
	files := 0
	err := filepath.WalkDir(inputRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
			return nil
		}
		files++
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var document map[string]any
		if err := yaml.Unmarshal(contents, &document); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		for _, key := range []string{"app", "kind", "spec"} {
			if _, ok := document[key]; !ok {
				return fmt.Errorf("%s has no %q field", path, key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no integration YAML fixtures found")
	}
}
