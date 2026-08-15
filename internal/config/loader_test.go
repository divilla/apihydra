package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestLoaderFindsYAMLFilesInWorkDir(t *testing.T) {
	workDir := t.TempDir()
	yamlPath := filepath.Join(workDir, "config.yaml")
	ymlPath := filepath.Join(workDir, "settings.yml")
	writeTestFile(t, yamlPath)
	writeTestFile(t, ymlPath)

	got := NewLoader(workDir).Files()
	want := []string{yamlPath, ymlPath}
	sort.Strings(got)
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Files() = %v, want %v", got, want)
	}
}

func TestLoaderFindsYAMLFilesRecursively(t *testing.T) {
	workDir := t.TempDir()
	firstPath := filepath.Join(workDir, "one", "config.yaml")
	secondPath := filepath.Join(workDir, "one", "two", "settings.yml")
	writeTestFile(t, firstPath)
	writeTestFile(t, secondPath)

	got := NewLoader(workDir).Files()
	want := []string{firstPath, secondPath}
	sort.Strings(got)
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Files() = %v, want %v", got, want)
	}
}

func TestLoaderExcludesNonYAMLEntries(t *testing.T) {
	workDir := t.TempDir()
	want := filepath.Join(workDir, "config.yaml")
	writeTestFile(t, want)
	writeTestFile(t, filepath.Join(workDir, "config.json"))
	writeTestFile(t, filepath.Join(workDir, "config.YAML"))
	if err := os.Mkdir(filepath.Join(workDir, "directory.yml"), 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}

	got := NewLoader(workDir).Files()
	if !reflect.DeepEqual(got, []string{want}) {
		t.Fatalf("Files() = %v, want [%s]", got, want)
	}
}

func TestLoaderHandlesWorkDirWithoutYAMLFiles(t *testing.T) {
	workDir := t.TempDir()

	got := NewLoader(workDir).Files()
	if len(got) != 0 {
		t.Fatalf("Files() = %v, want no paths", got)
	}
}

func TestLoaderHandlesInvalidWorkDir(t *testing.T) {
	parentDir := t.TempDir()
	nonDirectory := filepath.Join(parentDir, "config.yaml")
	writeTestFile(t, nonDirectory)

	for _, workDir := range []string{
		filepath.Join(parentDir, "missing"),
		nonDirectory,
	} {
		if got := NewLoader(workDir).Files(); len(got) != 0 {
			t.Errorf("Files() for %q = %v, want no paths", workDir, got)
		}
	}
}

func TestLoaderPreservesPublicAPI(t *testing.T) {
	var constructor func(string) *Loader = NewLoader
	var filesMethod func(*Loader) []string = (*Loader).Files

	loader := constructor(t.TempDir())
	filesMethod(loader)
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("test: true\n"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
