package definition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/divilla/apihydra/skeleton/internal/domain"
	"github.com/divilla/apihydra/skeleton/pkg/errs"
)

func TestDefinitionErrorClassifications(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"root missing": {ErrRootDefinitionMissing, "root definition missing"},
		"discovery":    {ErrDefinitionDiscovery, "definition discovery error"},
		"invalid":      {ErrInvalidDefinition, "invalid definition"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("error = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLoadDirectoryStructureRequiresTopLevelRootDefinition(t *testing.T) {
	tests := map[string]struct {
		files   map[string]string
		wantErr bool
	}{
		"yaml root with arbitrary name": {
			files: map[string]string{"suite.yaml": "app: apihydra\nkind: root\nspec: {}\n"},
		},
		"yml root": {
			files: map[string]string{"configuration.yml": "app: apihydra\nkind: root\nspec: {}\n"},
		},
		"root alongside malformed yaml": {
			files: map[string]string{
				"broken.yaml": "[",
				"root.yaml":   "app: apihydra\nkind: root\nspec: {}\n",
			},
		},
		"no yaml": {wantErr: true},
		"malformed": {
			files:   map[string]string{"root.yaml": "["},
			wantErr: true,
		},
		"non-string app": {
			files:   map[string]string{"root.yaml": "app: []\nkind: root\nspec: {}\n"},
			wantErr: true,
		},
		"non-string kind": {
			files:   map[string]string{"root.yaml": "app: apihydra\nkind: []\nspec: {}\n"},
			wantErr: true,
		},
		"wrong app": {
			files:   map[string]string{"root.yaml": "app: another\nkind: root\nspec: {}\n"},
			wantErr: true,
		},
		"wrong kind": {
			files:   map[string]string{"root.yaml": "app: apihydra\nkind: defaults\nspec: {}\n"},
			wantErr: true,
		},
		"wrong extension": {
			files:   map[string]string{"root.json": "app: apihydra\nkind: root\nspec: {}\n"},
			wantErr: true,
		},
		"nested root": {
			files:   map[string]string{"nested/root.yaml": "app: apihydra\nkind: root\nspec: {}\n"},
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			workDir := t.TempDir()
			for path, contents := range test.files {
				absolutePath := filepath.Join(workDir, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolutePath), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolutePath, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			suite := &domain.Suite{WorkDir: workDir}

			err := NewLoader().LoadDirectoryStructure(context.Background(), suite)
			if test.wantErr {
				if !errors.Is(err, ErrRootDefinitionMissing) {
					t.Fatalf("LoadDirectoryStructure() error = %v, want ErrRootDefinitionMissing", err)
				}
				if got := errs.Code(err, errs.ExitInternal); got != errs.ExitConfiguration {
					t.Fatalf("error code = %d, want %d", got, errs.ExitConfiguration)
				}
				if suite.Root != nil {
					t.Fatalf("suite.Root = %#v, want nil", suite.Root)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadDirectoryStructure() error = %v", err)
			}
			if suite.Root == nil || suite.Root.Path != "/" {
				t.Fatalf("suite.Root = %#v, want root path /", suite.Root)
			}
		})
	}
}

func TestLoadDirectoryStructurePreservesDiscoveryFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	suite := &domain.Suite{WorkDir: missing}

	err := NewLoader().LoadDirectoryStructure(context.Background(), suite)
	if !errors.Is(err, ErrDefinitionDiscovery) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadDirectoryStructure() error = %v, want discovery and not-exist errors", err)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("LoadDirectoryStructure() error = %q, want path %q", err, missing)
	}
	if got := errs.Code(err, errs.ExitInternal); got != errs.ExitConfiguration {
		t.Fatalf("error code = %d, want %d", got, errs.ExitConfiguration)
	}
}

func TestLoadDirectoryStructurePreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	suite := &domain.Suite{WorkDir: t.TempDir()}

	err := NewLoader().LoadDirectoryStructure(ctx, suite)
	if !errors.Is(err, ErrDefinitionDiscovery) || !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadDirectoryStructure() error = %v, want discovery and cancellation errors", err)
	}
	if suite.Root != nil {
		t.Fatalf("suite.Root = %#v, want nil", suite.Root)
	}
}
