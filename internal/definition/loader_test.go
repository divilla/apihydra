package definition

import (
	"apih/internal/domain"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestLoaderContractAndConstructor(t *testing.T) {
	var _ func() *Loader = NewLoader
	var _ func(*Loader, context.Context, *domain.Suite) error = (*Loader).LoadDirectoryStructure
	var _ func(*Loader, context.Context, *domain.Suite) error = (*Loader).LoadDirectoryFiles
	var _ func(*Loader, context.Context, *domain.Suite) error = (*Loader).DecodeBaseDefinitions

	if loader := NewLoader(); loader == nil {
		t.Fatal("NewLoader() = nil")
	}
	if got := reflect.TypeOf(Loader{}).NumField(); got != 0 {
		t.Fatalf("Loader fields = %d, want stateless Loader", got)
	}
}

func TestLoadDirectoryStructureBuildsRelativeTree(t *testing.T) {
	workDir := t.TempDir()
	for _, path := range []string{
		filepath.Join(workDir, "alpha", "grandchild"),
		filepath.Join(workDir, "zeta"),
		filepath.Join(workDir, ".hidden"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(workDir, "not-a-directory.yaml"), []byte("kind: root"), 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	if err := os.Symlink(filepath.Join(workDir, "alpha"), filepath.Join(workDir, "linked-directory")); err != nil {
		t.Fatalf("Symlink(): %v", err)
	}

	oldRoot := &domain.Directory{Path: "/old"}
	suite := &domain.Suite{WorkDir: workDir, Root: oldRoot}
	if err := NewLoader().LoadDirectoryStructure(context.Background(), suite); err != nil {
		t.Fatalf("LoadDirectoryStructure() error = %v", err)
	}

	root := suite.Root
	if root == oldRoot {
		t.Fatal("LoadDirectoryStructure() retained the old root")
	}
	assertDirectory(t, root, 0, "/", nil)
	if got, want := childPaths(root), []string{"/.hidden", "/alpha", "/zeta"}; !slices.Equal(got, want) {
		t.Fatalf("root child paths = %v, want %v", got, want)
	}
	alpha := root.Children[1]
	assertDirectory(t, alpha, 1, "/alpha", root)
	if got, want := len(alpha.Children), 1; got != want {
		t.Fatalf("len(alpha.Children) = %d, want %d", got, want)
	}
	assertDirectory(t, alpha.Children[0], 2, "/alpha/grandchild", alpha)
	if suite.WorkDir != workDir {
		t.Fatalf("suite.WorkDir = %q, want %q", suite.WorkDir, workDir)
	}
}

func TestLoadDirectoryStructureReturnsErrorsWithoutReplacingRoot(t *testing.T) {
	oldRoot := &domain.Directory{Path: "/old"}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	descendingWorkDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(descendingWorkDir, "child"), 0o755); err != nil {
		t.Fatalf("Mkdir(): %v", err)
	}

	tests := map[string]struct {
		ctx     context.Context
		workDir string
		wantErr error
	}{
		"canceled context": {
			ctx:     canceled,
			workDir: t.TempDir(),
			wantErr: context.Canceled,
		},
		"missing work directory": {
			ctx:     context.Background(),
			workDir: filepath.Join(t.TempDir(), "missing"),
			wantErr: os.ErrNotExist,
		},
		"canceled while descending": {
			ctx:     &checkingContext{cancelAt: 2},
			workDir: descendingWorkDir,
			wantErr: context.Canceled,
		},
		"canceled after final directory read": {
			ctx:     &checkingContext{cancelAt: 2},
			workDir: t.TempDir(),
			wantErr: context.Canceled,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			suite := &domain.Suite{WorkDir: test.workDir, Root: oldRoot}
			err := NewLoader().LoadDirectoryStructure(test.ctx, suite)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("LoadDirectoryStructure() error = %v, want %v", err, test.wantErr)
			}
			if suite.Root != oldRoot {
				t.Fatal("LoadDirectoryStructure() replaced Root after an error")
			}
		})
	}
}

func TestLoadDirectoryFilesLoadsSupportedRegularFilesWithSourceLinks(t *testing.T) {
	workDir := t.TempDir()
	childPath := filepath.Join(workDir, "child")
	if err := os.Mkdir(childPath, 0o755); err != nil {
		t.Fatalf("Mkdir(): %v", err)
	}
	writeTestFile(t, filepath.Join(workDir, "a.yaml"), "kind: root\n")
	writeTestFile(t, filepath.Join(workDir, "b.yml"), "kind: defaults\n")
	writeTestFile(t, filepath.Join(workDir, "ignored.YAML"), "kind: steps\n")
	writeTestFile(t, filepath.Join(workDir, "ignored.txt"), "kind: steps\n")
	writeTestFile(t, filepath.Join(childPath, "steps.yaml"), "kind: steps\n")
	if err := os.Symlink(filepath.Join(workDir, "a.yaml"), filepath.Join(workDir, "linked.yaml")); err != nil {
		t.Fatalf("Symlink(): %v", err)
	}

	suite := &domain.Suite{WorkDir: workDir}
	loader := NewLoader()
	if err := loader.LoadDirectoryStructure(context.Background(), suite); err != nil {
		t.Fatalf("LoadDirectoryStructure() error = %v", err)
	}
	root := suite.Root
	child := root.Children[0]
	rootDefault := &domain.File{Path: "keep-default"}
	rootSteps := []*domain.File{{Path: "keep-steps"}}
	root.DefaultsFile = rootDefault
	root.StepsFiles = rootSteps
	root.ResolvedDefaults.BaseURL = "keep-resolved"

	if err := loader.LoadDirectoryFiles(context.Background(), suite); err != nil {
		t.Fatalf("LoadDirectoryFiles() error = %v", err)
	}
	assertFiles(t, root, []expectedFile{
		{stage: 0, path: "a.yaml", contents: "kind: root\n"},
		{stage: 0, path: "b.yml", contents: "kind: defaults\n"},
	})
	assertFiles(t, child, []expectedFile{
		{stage: 1, path: "child/steps.yaml", contents: "kind: steps\n"},
	})
	if root.DefaultsFile != rootDefault || !slices.Equal(root.StepsFiles, rootSteps) {
		t.Fatal("LoadDirectoryFiles() mutated base-classification fields")
	}
	if root.ResolvedDefaults.BaseURL != "keep-resolved" {
		t.Fatal("LoadDirectoryFiles() mutated a later-phase field")
	}

	firstRootFiles := root.Files
	if err := loader.LoadDirectoryFiles(context.Background(), suite); err != nil {
		t.Fatalf("second LoadDirectoryFiles() error = %v", err)
	}
	if len(root.Files) != 2 || root.Files[0] == firstRootFiles[0] {
		t.Fatal("second LoadDirectoryFiles() did not replace Files without duplicates")
	}
}

func TestLoadDirectoryFilesReturnsErrorsWithoutMutatingFiles(t *testing.T) {
	oldFiles := []*domain.File{{Path: "keep.yaml"}}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := map[string]struct {
		ctx     context.Context
		workDir string
		root    *domain.Directory
		wantErr error
	}{
		"canceled context": {
			ctx:     canceled,
			workDir: t.TempDir(),
			root:    &domain.Directory{Path: "/", Files: oldFiles},
			wantErr: context.Canceled,
		},
		"missing tree directory": {
			ctx:     context.Background(),
			workDir: t.TempDir(),
			root:    &domain.Directory{Path: "/missing", Files: oldFiles},
			wantErr: os.ErrNotExist,
		},
		"canceled while scanning entries": {
			ctx:     &checkingContext{cancelAt: 2},
			workDir: t.TempDir(),
			root:    &domain.Directory{Path: "/", Files: oldFiles},
			wantErr: context.Canceled,
		},
		"canceled after final file read": {
			ctx:     &checkingContext{cancelAt: 3},
			workDir: t.TempDir(),
			root:    &domain.Directory{Path: "/", Files: oldFiles},
			wantErr: context.Canceled,
		},
	}
	writeTestFile(t, filepath.Join(tests["canceled while scanning entries"].workDir, "file.yaml"), "kind: root\n")
	writeTestFile(t, filepath.Join(tests["canceled after final file read"].workDir, "file.yaml"), "kind: root\n")

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			suite := &domain.Suite{WorkDir: test.workDir, Root: test.root}
			err := NewLoader().LoadDirectoryFiles(test.ctx, suite)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("LoadDirectoryFiles() error = %v, want %v", err, test.wantErr)
			}
			if !slices.Equal(test.root.Files, oldFiles) {
				t.Fatal("LoadDirectoryFiles() mutated Files after an error")
			}
		})
	}
}

func TestDecodeBaseDefinitionsClassifiesFilesOnly(t *testing.T) {
	root := &domain.Directory{Stage: 0, Path: "/"}
	child := &domain.Directory{Stage: 1, Path: "/child", Parent: root}
	root.Children = []*domain.Directory{child}
	root.Files = []*domain.File{
		definitionFile(root, "root.yaml", "app: apihydra\nkind: root\nspec:\n  base_url: https://example.test\n"),
		definitionFile(root, "defaults.yaml", "app: apihydra\nkind: defaults\nspec: defaults\n"),
		definitionFile(root, "steps-a.yaml", "app: apihydra\nkind: steps\nspec: steps\n"),
		definitionFile(root, "steps-b.yml", "app: apihydra\nkind: steps\nspec: steps\n"),
		definitionFile(root, "unknown.yaml", "app: apihydra\nkind: custom\nspec: custom\n"),
	}
	child.Files = []*domain.File{
		definitionFile(child, "child/steps.yaml", "app: apihydra\nkind: steps\nspec: child\n"),
	}
	oldDefinition := &domain.DefaultsDefinition{}
	root.DefaultsDefinition = oldDefinition
	root.ResolvedDefaults.BasePath = "/keep"
	suite := &domain.Suite{WorkDir: "/unused", Root: root}

	loader := NewLoader()
	if err := loader.DecodeBaseDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("DecodeBaseDefinitions() error = %v", err)
	}
	if got, want := root.Files[0].Kind, domain.KindRoot; got != want {
		t.Fatalf("root file kind = %q, want %q", got, want)
	}
	if got, want := root.DefaultsFile, root.Files[1]; got != want {
		t.Fatalf("root.DefaultsFile = %p, want %p", got, want)
	}
	if got, want := root.StepsFiles, []*domain.File{root.Files[2], root.Files[3]}; !slices.Equal(got, want) {
		t.Fatalf("root.StepsFiles = %v, want %v", got, want)
	}
	if got, want := root.Files[4].Kind, domain.DocumentKind("custom"); got != want {
		t.Fatalf("unknown file kind = %q, want %q", got, want)
	}
	if child.DefaultsFile != nil || len(child.StepsFiles) != 1 || child.StepsFiles[0] != child.Files[0] {
		t.Fatal("child base classification is incorrect")
	}
	if root.DefaultsDefinition != oldDefinition || root.ResolvedDefaults.BasePath != "/keep" {
		t.Fatal("DecodeBaseDefinitions() mutated complete decoding or resolution fields")
	}

	if err := loader.DecodeBaseDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("second DecodeBaseDefinitions() error = %v", err)
	}
	if len(root.StepsFiles) != 2 {
		t.Fatalf("second DecodeBaseDefinitions() steps = %d, want 2", len(root.StepsFiles))
	}
}

func TestDecodeBaseDefinitionsClassifiesRootFileAsDefaults(t *testing.T) {
	root := &domain.Directory{Stage: 0, Path: "/"}
	rootFile := definitionFile(
		root,
		"root.yaml",
		"app: apihydra\nkind: root\nspec:\n  base_url: https://example.test\n",
	)
	root.Files = []*domain.File{rootFile}

	err := NewLoader().DecodeBaseDefinitions(context.Background(), &domain.Suite{Root: root})
	if err != nil {
		t.Fatalf("DecodeBaseDefinitions() error = %v", err)
	}
	if got, want := rootFile.Kind, domain.KindRoot; got != want {
		t.Fatalf("root file kind = %q, want %q", got, want)
	}
	if got := root.DefaultsFile; got != rootFile {
		t.Fatalf("root.DefaultsFile = %p, want %p", got, rootFile)
	}
	if len(root.StepsFiles) != 0 {
		t.Fatalf("len(root.StepsFiles) = %d, want 0", len(root.StepsFiles))
	}
}

func TestDecodeBaseDefinitionsReturnsErrorsWithoutPartialClassification(t *testing.T) {
	valid := &domain.File{Path: "valid.yaml", Bytes: []byte("kind: steps\nspec: valid\n")}
	invalid := &domain.File{Path: "invalid.yaml", Bytes: []byte("kind: [\n")}
	oldDefault := &domain.File{Path: "old-default.yaml"}
	oldSteps := []*domain.File{{Path: "old-steps.yaml"}}
	root := &domain.Directory{
		Path:         "/",
		Files:        []*domain.File{valid, invalid},
		DefaultsFile: oldDefault,
		StepsFiles:   oldSteps,
	}
	valid.Directory = root
	invalid.Directory = root

	err := NewLoader().DecodeBaseDefinitions(context.Background(), &domain.Suite{Root: root})
	if err == nil {
		t.Fatal("DecodeBaseDefinitions() error = nil for malformed YAML")
	}
	if valid.Kind != "" || invalid.Kind != "" {
		t.Fatal("DecodeBaseDefinitions() partially mutated File.Kind after an error")
	}
	if root.DefaultsFile != oldDefault || !slices.Equal(root.StepsFiles, oldSteps) {
		t.Fatal("DecodeBaseDefinitions() partially mutated directory classification after an error")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewLoader().DecodeBaseDefinitions(canceled, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
		t.Fatalf("DecodeBaseDefinitions() cancellation error = %v, want context.Canceled", err)
	}

	if err := NewLoader().DecodeBaseDefinitions(
		&checkingContext{cancelAt: 2},
		&domain.Suite{Root: &domain.Directory{Path: "/", Files: []*domain.File{valid}}},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("DecodeBaseDefinitions() in-file cancellation error = %v, want context.Canceled", err)
	}

	descendingRoot := &domain.Directory{Path: "/"}
	descendingRoot.Children = []*domain.Directory{{Path: "/child", Parent: descendingRoot}}
	if err := NewLoader().DecodeBaseDefinitions(
		&checkingContext{cancelAt: 2},
		&domain.Suite{Root: descendingRoot},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("DecodeBaseDefinitions() descending cancellation error = %v, want context.Canceled", err)
	}
}

func TestDecodeBaseDefinitionsHonorsCancellationDuringDecodeAndBeforeCommit(t *testing.T) {
	for name, cancelAt := range map[string]int{
		"during decode": 3,
		"before commit": 4,
	} {
		t.Run(name, func(t *testing.T) {
			oldDefault := &domain.File{Path: "old-default.yaml"}
			oldSteps := []*domain.File{{Path: "old-steps.yaml"}}
			root := &domain.Directory{
				Path:         "/",
				DefaultsFile: oldDefault,
				StepsFiles:   oldSteps,
			}
			file := definitionFile(root, "steps.yaml", "kind: steps\nspec: steps\n")
			file.Kind = domain.KindRoot
			root.Files = []*domain.File{file}

			err := NewLoader().DecodeBaseDefinitions(
				&checkingContext{cancelAt: cancelAt},
				&domain.Suite{Root: root},
			)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("DecodeBaseDefinitions() error = %v, want context.Canceled", err)
			}
			if file.Kind != domain.KindRoot {
				t.Fatalf("file.Kind = %q, want unchanged %q", file.Kind, domain.KindRoot)
			}
			if root.DefaultsFile != oldDefault || !slices.Equal(root.StepsFiles, oldSteps) {
				t.Fatal("DecodeBaseDefinitions() committed classification after cancellation")
			}
		})
	}
}

func TestLoaderPhasesAllowEmptyTree(t *testing.T) {
	suite := &domain.Suite{}
	loader := NewLoader()
	if err := loader.LoadDirectoryFiles(context.Background(), suite); err != nil {
		t.Fatalf("LoadDirectoryFiles() empty-tree error = %v", err)
	}
	if err := loader.DecodeBaseDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("DecodeBaseDefinitions() empty-tree error = %v", err)
	}
}

func assertDirectory(t *testing.T, directory *domain.Directory, stage int, path string, parent *domain.Directory) {
	t.Helper()
	if directory == nil {
		t.Fatal("directory = nil")
	}
	if directory.Stage != stage || directory.Path != path || directory.Parent != parent {
		t.Fatalf(
			"directory = {Stage:%d Path:%q Parent:%p}, want {Stage:%d Path:%q Parent:%p}",
			directory.Stage,
			directory.Path,
			directory.Parent,
			stage,
			path,
			parent,
		)
	}
}

func childPaths(directory *domain.Directory) []string {
	paths := make([]string, len(directory.Children))
	for i, child := range directory.Children {
		paths[i] = child.Path
	}
	return paths
}

type expectedFile struct {
	stage    int
	path     string
	contents string
}

func assertFiles(t *testing.T, directory *domain.Directory, expected []expectedFile) {
	t.Helper()
	if got, want := len(directory.Files), len(expected); got != want {
		t.Fatalf("len(%s.Files) = %d, want %d", directory.Path, got, want)
	}
	for i, want := range expected {
		got := directory.Files[i]
		if got.Stage != want.stage || got.Path != want.path || string(got.Bytes) != want.contents || got.Directory != directory {
			t.Fatalf(
				"%s.Files[%d] = {Stage:%d Path:%q Bytes:%q Directory:%p}, want {Stage:%d Path:%q Bytes:%q Directory:%p}",
				directory.Path,
				i,
				got.Stage,
				got.Path,
				got.Bytes,
				got.Directory,
				want.stage,
				want.path,
				want.contents,
				directory,
			)
		}
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func definitionFile(directory *domain.Directory, path, contents string) *domain.File {
	return &domain.File{
		Stage:     directory.Stage,
		Path:      path,
		Bytes:     []byte(contents),
		Directory: directory,
	}
}

type checkingContext struct {
	checks   int
	cancelAt int
}

func (c *checkingContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *checkingContext) Done() <-chan struct{} {
	return nil
}

func (c *checkingContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func (c *checkingContext) Value(any) any {
	return nil
}
