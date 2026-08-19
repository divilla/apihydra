package definition

import (
	"apih/internal/domain"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderPublicContract(t *testing.T) {
	var constructor func() *Loader = NewLoader
	var loadStructure func(*Loader, context.Context, *domain.Suite) error = (*Loader).LoadDirectoryStructure
	var loadFiles func(*Loader, context.Context, *domain.Suite) error = (*Loader).LoadDirectoryFiles
	var decodeBase func(*Loader, context.Context, *domain.Suite) error = (*Loader).DecodeBaseDefinitions
	_, _, _, _ = constructor, loadStructure, loadFiles, decodeBase

	if got := *NewLoader(); got != (Loader{}) {
		t.Fatalf("NewLoader() = %#v, want empty Loader", got)
	}
}

func TestLoadDirectoryStructureBuildsRelativeDirectoryTreeOnly(t *testing.T) {
	workDir := t.TempDir()
	writeLoaderFile(t, filepath.Join(workDir, "root.yaml"), []byte("kind: root\n"))
	if err := os.MkdirAll(filepath.Join(workDir, "alpha", "beta"), 0o755); err != nil {
		t.Fatalf("create directory tree: %v", err)
	}
	if err := os.Mkdir(filepath.Join(workDir, "gamma"), 0o755); err != nil {
		t.Fatalf("create sibling directory: %v", err)
	}

	oldRoot := &domain.Directory{Path: "/old", Files: []*domain.File{{Path: "/old/file.yaml"}}}
	suite := &domain.Suite{WorkDir: workDir, Root: oldRoot}
	if err := NewLoader().LoadDirectoryStructure(context.Background(), suite); err != nil {
		t.Fatalf("LoadDirectoryStructure() error = %v", err)
	}

	if suite.Root == oldRoot {
		t.Fatal("LoadDirectoryStructure() retained the previous root")
	}
	assertDirectory(t, suite.Root, 0, "/", nil)
	alpha := findChild(t, suite.Root, "/alpha")
	assertDirectory(t, alpha, 1, "/alpha", suite.Root)
	beta := findChild(t, alpha, "/alpha/beta")
	assertDirectory(t, beta, 2, "/alpha/beta", alpha)
	gamma := findChild(t, suite.Root, "/gamma")
	assertDirectory(t, gamma, 1, "/gamma", suite.Root)

	for _, directory := range []*domain.Directory{suite.Root, alpha, beta, gamma} {
		if directory.Files != nil || directory.DefaultsFile != nil || directory.StepsFiles != nil ||
			directory.DefaultsDefinition != nil || directory.StepsDefinitions != nil {
			t.Fatalf("directory %q contains non-structure definition state: %#v", directory.Path, directory)
		}
	}
}

func TestLoadDirectoryStructureReturnsTraversalAndContextErrors(t *testing.T) {
	loader := NewLoader()

	t.Run("missing work directory", func(t *testing.T) {
		suite := &domain.Suite{WorkDir: filepath.Join(t.TempDir(), "missing")}
		if err := loader.LoadDirectoryStructure(context.Background(), suite); err == nil {
			t.Fatal("LoadDirectoryStructure() error = nil, want traversal error")
		}
		if suite.Root != nil {
			t.Fatalf("LoadDirectoryStructure() root = %#v, want nil", suite.Root)
		}
	})

	t.Run("work directory is a file", func(t *testing.T) {
		workDir := filepath.Join(t.TempDir(), "file")
		writeLoaderFile(t, workDir, []byte("content"))
		if err := loader.LoadDirectoryStructure(context.Background(), &domain.Suite{WorkDir: workDir}); !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("LoadDirectoryStructure() error = %v, want os.ErrInvalid", err)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := loader.LoadDirectoryStructure(ctx, &domain.Suite{WorkDir: t.TempDir()}); !errors.Is(err, context.Canceled) {
			t.Fatalf("LoadDirectoryStructure() error = %v, want context.Canceled", err)
		}
	})

	t.Run("context canceled during traversal", func(t *testing.T) {
		ctx := newCancelDuringTraversalContext()
		suite := &domain.Suite{WorkDir: t.TempDir()}
		if err := loader.LoadDirectoryStructure(ctx, suite); !errors.Is(err, context.Canceled) {
			t.Fatalf("LoadDirectoryStructure() error = %v, want context.Canceled", err)
		}
		if suite.Root != nil {
			t.Fatalf("LoadDirectoryStructure() root = %#v, want nil after cancellation", suite.Root)
		}
	})
}

func TestLoadDirectoryFilesPopulatesYAMLFilesOnly(t *testing.T) {
	workDir := t.TempDir()
	writeLoaderFile(t, filepath.Join(workDir, "defaults.yaml"), []byte("kind: defaults\n"))
	writeLoaderFile(t, filepath.Join(workDir, "notes.txt"), []byte("ignored"))
	writeLoaderFile(t, filepath.Join(workDir, "upper.YAML"), []byte("ignored"))
	writeLoaderFile(t, filepath.Join(workDir, "child", "steps.yml"), []byte("kind: steps\n"))
	if err := os.Mkdir(filepath.Join(workDir, "fake.yaml"), 0o755); err != nil {
		t.Fatalf("create YAML-named directory: %v", err)
	}

	suite := &domain.Suite{WorkDir: workDir}
	loader := NewLoader()
	if err := loader.LoadDirectoryStructure(context.Background(), suite); err != nil {
		t.Fatalf("LoadDirectoryStructure() error = %v", err)
	}
	child := findChild(t, suite.Root, "/child")
	defaultsDefinition := &domain.DefaultsDefinition{}
	stepsDefinition := &domain.StepsDefinition{}
	suite.Root.DefaultsFile = &domain.File{Path: "preserved-defaults"}
	suite.Root.StepsFiles = []*domain.File{{Path: "preserved-steps"}}
	suite.Root.DefaultsDefinition = defaultsDefinition
	suite.Root.StepsDefinitions = []*domain.StepsDefinition{stepsDefinition}

	if err := loader.LoadDirectoryFiles(context.Background(), suite); err != nil {
		t.Fatalf("LoadDirectoryFiles() error = %v", err)
	}

	assertLoadedFiles(t, suite.Root, []fileExpectation{{
		path:    "/defaults.yaml",
		content: "kind: defaults\n",
	}})
	assertLoadedFiles(t, child, []fileExpectation{{
		path:    "/child/steps.yml",
		content: "kind: steps\n",
	}})
	if suite.Root.DefaultsFile.Path != "preserved-defaults" ||
		suite.Root.StepsFiles[0].Path != "preserved-steps" ||
		suite.Root.DefaultsDefinition != defaultsDefinition ||
		suite.Root.StepsDefinitions[0] != stepsDefinition {
		t.Fatal("LoadDirectoryFiles() mutated definition classification or decoding fields")
	}
}

func TestLoadDirectoryFilesReturnsTraversalAndContextErrors(t *testing.T) {
	loader := NewLoader()

	t.Run("directory disappeared", func(t *testing.T) {
		workDir := t.TempDir()
		childPath := filepath.Join(workDir, "child")
		if err := os.Mkdir(childPath, 0o755); err != nil {
			t.Fatalf("create child directory: %v", err)
		}
		suite := &domain.Suite{WorkDir: workDir}
		if err := loader.LoadDirectoryStructure(context.Background(), suite); err != nil {
			t.Fatalf("LoadDirectoryStructure() error = %v", err)
		}
		if err := os.Remove(childPath); err != nil {
			t.Fatalf("remove child directory: %v", err)
		}
		if err := loader.LoadDirectoryFiles(context.Background(), suite); err == nil {
			t.Fatal("LoadDirectoryFiles() error = nil, want traversal error")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		suite := &domain.Suite{WorkDir: t.TempDir(), Root: &domain.Directory{Path: "/"}}
		if err := loader.LoadDirectoryFiles(ctx, suite); !errors.Is(err, context.Canceled) {
			t.Fatalf("LoadDirectoryFiles() error = %v, want context.Canceled", err)
		}
	})
}

func TestDecodeBaseDefinitionsClassifiesFilesOnly(t *testing.T) {
	defaultsFile := definitionFile("/defaults.yaml", `
app: apihydra
kind: defaults
spec:
  baseUrl: https://example.test
`)
	stepsFile := definitionFile("/child/steps.yaml", `
app: apihydra
kind: steps
spec:
  steps: []
`)
	rootFile := definitionFile("/root.yaml", "app: apihydra\nkind: root\nspec: {}\n")
	otherFile := definitionFile("/other.yaml", "app: apihydra\nkind: other\nspec: value\n")

	root := &domain.Directory{Stage: 0, Path: "/", Files: []*domain.File{rootFile, defaultsFile, otherFile}}
	child := &domain.Directory{Stage: 1, Path: "/child", Parent: root, Files: []*domain.File{stepsFile}}
	root.Children = []*domain.Directory{child}
	for _, file := range []*domain.File{rootFile, defaultsFile, otherFile} {
		file.Directory = root
	}
	stepsFile.Directory = child
	resolvedDefaults := domain.Defaults{BaseURL: "preserved"}
	root.ResolvedDefaults = resolvedDefaults
	root.DefaultsDefinition = &domain.DefaultsDefinition{}
	child.StepsDefinitions = []*domain.StepsDefinition{{}}

	if err := NewLoader().DecodeBaseDefinitions(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("DecodeBaseDefinitions() error = %v", err)
	}

	if rootFile.Kind != domain.KindRoot || defaultsFile.Kind != domain.KindDefaults ||
		stepsFile.Kind != domain.KindSteps || otherFile.Kind != domain.DocumentKind("other") {
		t.Fatalf("decoded kinds = (%q, %q, %q, %q), want root, defaults, steps, other",
			rootFile.Kind, defaultsFile.Kind, stepsFile.Kind, otherFile.Kind)
	}
	if root.DefaultsFile != defaultsFile {
		t.Fatalf("root DefaultsFile = %p, want %p", root.DefaultsFile, defaultsFile)
	}
	if len(root.StepsFiles) != 0 {
		t.Fatalf("root StepsFiles = %v, want none", root.StepsFiles)
	}
	if len(child.StepsFiles) != 1 || child.StepsFiles[0] != stepsFile {
		t.Fatalf("child StepsFiles = %v, want steps file", child.StepsFiles)
	}
	if root.DefaultsDefinition == nil || len(child.StepsDefinitions) != 1 ||
		root.ResolvedDefaults.BaseURL != resolvedDefaults.BaseURL {
		t.Fatal("DecodeBaseDefinitions() mutated complete decoding or resolution fields")
	}
}

func TestDecodeBaseDefinitionsReturnsDecodeAndContextErrors(t *testing.T) {
	loader := NewLoader()

	t.Run("invalid YAML", func(t *testing.T) {
		file := definitionFile("/invalid.yaml", "kind: [\n")
		root := &domain.Directory{Path: "/"}
		child := &domain.Directory{Stage: 1, Path: "/child", Parent: root, Files: []*domain.File{file}}
		root.Children = []*domain.Directory{child}
		file.Directory = child
		if err := loader.DecodeBaseDefinitions(context.Background(), &domain.Suite{Root: root}); err == nil {
			t.Fatal("DecodeBaseDefinitions() error = nil, want YAML decode error")
		}
		if file.Kind != "" || child.DefaultsFile != nil || child.StepsFiles != nil {
			t.Fatal("DecodeBaseDefinitions() classified an invalid document")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		suite := &domain.Suite{Root: &domain.Directory{Path: "/"}}
		if err := loader.DecodeBaseDefinitions(ctx, suite); !errors.Is(err, context.Canceled) {
			t.Fatalf("DecodeBaseDefinitions() error = %v, want context.Canceled", err)
		}
	})
}

type fileExpectation struct {
	path    string
	content string
}

type cancelDuringTraversalContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func newCancelDuringTraversalContext() *cancelDuringTraversalContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &cancelDuringTraversalContext{Context: ctx, cancel: cancel}
}

func (c *cancelDuringTraversalContext) Err() error {
	c.checks++
	if c.checks == 2 {
		c.cancel()
	}
	return c.Context.Err()
}

func assertDirectory(t *testing.T, got *domain.Directory, stage int, path string, parent *domain.Directory) {
	t.Helper()
	if got == nil || got.Stage != stage || got.Path != path || got.Parent != parent {
		t.Fatalf("directory = %#v, want stage %d, path %q, parent %p", got, stage, path, parent)
	}
}

func findChild(t *testing.T, parent *domain.Directory, path string) *domain.Directory {
	t.Helper()
	for _, child := range parent.Children {
		if child.Path == path {
			return child
		}
	}
	t.Fatalf("directory %q has no child %q", parent.Path, path)
	return nil
}

func assertLoadedFiles(t *testing.T, directory *domain.Directory, want []fileExpectation) {
	t.Helper()
	if len(directory.Files) != len(want) {
		t.Fatalf("directory %q files = %v, want %d files", directory.Path, directory.Files, len(want))
	}
	for index, expectation := range want {
		file := directory.Files[index]
		if file.Stage != directory.Stage || file.Path != expectation.path ||
			string(file.Bytes) != expectation.content || file.Directory != directory || file.Kind != "" {
			t.Fatalf("directory %q file = %#v, want stage %d, path %q, content %q, owner %p, empty kind",
				directory.Path, file, directory.Stage, expectation.path, expectation.content, directory)
		}
	}
}

func definitionFile(path, contents string) *domain.File {
	return &domain.File{Path: path, Bytes: []byte(contents)}
}

func writeLoaderFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent directory: %v", err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
