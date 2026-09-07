package definition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/divilla/apihydra/skeleton/internal/domain"
	"github.com/divilla/apihydra/skeleton/pkg/errs"
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

func TestRootQualificationIgnoresUnrelatedKinds(t *testing.T) {
	for _, rootName := range []string{"", "a-root.yaml", "z-root.yaml"} {
		t.Run(rootName, func(t *testing.T) {
			dir := t.TempDir()
			if rootName != "" {
				if err := os.WriteFile(filepath.Join(dir, rootName), []byte("app: apihydra\nkind: root\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "invalid.yml"), []byte("app: apihydra\n"), 0600); err != nil {
				t.Fatal(err)
			}
			suite := &domain.Suite{WorkDir: dir}
			err := NewLoader().LoadDirectoryStructure(context.Background(), suite)
			if rootName == "" {
				if !errors.Is(err, ErrRootDefinitionMissing) || suite.Root != nil {
					t.Fatalf("missing root: %v", err)
				}
			} else if err != nil || suite.Root == nil {
				t.Fatalf("qualified root: %v", err)
			}
		})
	}
}
