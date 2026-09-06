package definition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/errs"
)

func TestCheckDefinitionKind(t *testing.T) {
	for _, kind := range []string{"", "kind:", "kind: null", "kind: ''", "kind: custom", "kind: ROOT", "kind: ' root '", "kind: 1", "kind: false", "kind: []", "kind: {}"} {
		t.Run(kind, func(t *testing.T) {
			root, err := checkDefinitionKind(context.Background(), []byte("app: apihydra\n"+kind+"\n"))
			var coder errs.ExitCoder
			if root || !errors.Is(err, ErrInvalidKind) || !errors.As(err, &coder) || coder.ExitCode() != 102 {
				t.Fatalf("checkDefinitionKind() = %v, %v, want configuration ErrInvalidKind", root, err)
			}
			if err.Error() != "kind: must be one of: <root|defaults|steps>" {
				t.Fatalf("kind diagnostic = %q", err.Error())
			}
		})
	}
	for _, kind := range []string{"root", "defaults", "steps"} {
		root, err := checkDefinitionKind(context.Background(), []byte("app: apihydra\nkind: "+kind+"\n"))
		if err != nil || root != (kind == "root") {
			t.Fatalf("kind %q = %v, %v", kind, root, err)
		}
	}
	for _, app := range []string{"", "app:", "app: apyhidra", "app: other", "app: APIHydra", "app: []", "app: 1"} {
		root, err := checkDefinitionKind(context.Background(), []byte(app+"\nkind: []\n"))
		if root || err != nil {
			t.Fatalf("app %q received kind validation: %v, %v", app, root, err)
		}
	}
	if _, err := checkDefinitionKind(context.Background(), []byte("app: apihydra\nkind: [")); err == nil || errors.Is(err, ErrInvalidKind) {
		t.Fatalf("malformed YAML error = %v, want original parser error", err)
	}
}

func TestTopLevelInvalidKindPrecedesRootResult(t *testing.T) {
	for _, rootName := range []string{"", "a-root.yaml", "z-root.yaml"} {
		t.Run(rootName, func(t *testing.T) {
			dir := t.TempDir()
			if rootName != "" {
				if err := os.WriteFile(filepath.Join(dir, rootName), []byte("app: apihydra\nkind: root\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "invalid.yml"), []byte("app: apihydra\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			suite := &domain.Suite{WorkDir: dir}
			err := NewLoader().LoadDirectoryStructure(context.Background(), suite)
			if !errors.Is(err, ErrInvalidKind) || suite.Root != nil {
				t.Fatalf("root check = %v, root %v, want ErrInvalidKind before discovery", err, suite.Root)
			}
		})
	}
}

func TestDecodeBaseDefinitionsRejectsInvalidKindWithoutPartialChanges(t *testing.T) {
	for _, kind := range []string{"", "kind:", "kind: null", "kind: ''", "kind: custom", "kind: ROOT", "kind: 1", "kind: false", "kind: []", "kind: {}"} {
		t.Run(kind, func(t *testing.T) {
			root := &domain.Directory{Path: "/"}
			rootFile := definitionFile(root, "root.yaml", "app: apihydra\nkind: root\n")
			root.Files = []*domain.File{rootFile}
			child := &domain.Directory{Path: "/child", Parent: root}
			root.Children = []*domain.Directory{child}
			child.Files = []*domain.File{definitionFile(child, "child/invalid.yaml", "app: apihydra\n"+kind+"\n")}
			oldDefault := &domain.File{Path: "old.yaml"}
			root.DefaultsFile = oldDefault
			err := NewLoader().DecodeBaseDefinitions(context.Background(), &domain.Suite{Root: root})
			if !errors.Is(err, ErrInvalidKind) || errs.Code(err, 0) != 102 {
				t.Fatalf("DecodeBaseDefinitions() = %v, want configuration ErrInvalidKind", err)
			}
			if rootFile.Kind != "" || child.Files[0].Kind != "" || root.DefaultsFile != oldDefault {
				t.Fatal("kind failure partially committed classifications")
			}
		})
	}
}

func TestDecodeBaseDefinitionsDoesNotValidateOtherAppsKinds(t *testing.T) {
	for _, app := range []string{"", "app: other", "app: apyhidra", "app: []"} {
		for _, kind := range []string{"", "kind: custom", "kind: []"} {
			root := &domain.Directory{Path: "/"}
			root.Files = []*domain.File{definitionFile(root, "other.yaml", app+"\n"+kind+"\n")}
			err := NewLoader().DecodeBaseDefinitions(context.Background(), &domain.Suite{Root: root})
			if errors.Is(err, ErrInvalidKind) {
				t.Fatalf("%q %q received kind validation: %v", app, kind, err)
			}
			// Other applications still receive the existing typed-decoding errors.
			wantDecodeError := app == "app: []" || kind == "kind: []"
			if (err != nil) != wantDecodeError {
				t.Fatalf("%q %q decode error = %v, want error %v", app, kind, err, wantDecodeError)
			}
		}
	}
}
