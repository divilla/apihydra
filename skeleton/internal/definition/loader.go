package definition

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/divilla/apihydra/skeleton/internal/domain"
	"github.com/divilla/apihydra/skeleton/pkg/errs"

	"github.com/goccy/go-yaml"
)

// ErrRootDefinitionMissing classifies a selected suite directory without a
// qualifying top-level root definition.
var ErrRootDefinitionMissing = errors.New("root defaults file missing")

// ErrDefinitionDiscovery classifies a failure to inspect or read definition
// inputs from the selected suite directory tree.
var ErrDefinitionDiscovery = errors.New("definition discovery error")

// ErrInvalidDefinition classifies malformed YAML or an invalid definition
// field. Definition decoding preserves the file, YAML path, and original cause.
var ErrInvalidDefinition = errors.New("invalid definition")

// ErrInvalidKind classifies an apihydra document with an absent, empty,
// non-string, or unsupported kind. Its CLI diagnostic uses this exact message.
var ErrInvalidKind = errors.New("kind: must be one of: <root|defaults|steps>")

// Loader discovers directories and definition files.
type Loader struct{}

// NewLoader returns a stateless Loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadDirectoryStructure first requires a regular .yaml or .yml file directly
// in suite.WorkDir whose string envelope values are app: apihydra and
// kind: root. All parseable top-level apihydra documents must first have a
// string kind equal to root, defaults, or steps; otherwise ErrInvalidKind takes
// priority over a missing root, even when another file already qualifies.
// Other app values do not receive kind validation. Malformed files and roots
// in descendants do not satisfy the requirement. A missing qualifying file returns
// ErrRootDefinitionMissing before recursive traversal. It then traverses
// suite.WorkDir and builds suite.Root. Directory paths are relative to
// suite.WorkDir, and the root path is "/".
func (l *Loader) LoadDirectoryStructure(
	ctx context.Context,
	suite *domain.Suite,
) error {
	if err := validateRootDefinition(ctx, suite.WorkDir); err != nil {
		return err
	}
	// TODO: implement
	suite.Root = &domain.Directory{
		Path: "/",
	}
	return nil
}

// LoadDirectoryFiles traverses suite.Root and populates only each Directory.Files
// slice with that directory's .yaml and .yml files. A traversal or file-read
// failure returns an ErrDefinitionDiscovery configuration error with the
// affected path and original cause.
func (l *Loader) LoadDirectoryFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// DecodeBaseDefinitions traverses suite.Root and attempts to decode each File
// as a BaseDefinition. Successful decodes set File.Kind and populate the owning
// Directory's DefaultsFile and StepsFiles fields. Before typed decoding, every
// parseable file with string app: apihydra must have string kind root, defaults,
// or steps. Any other, missing, empty, null, or non-string kind returns a
// configuration-coded ErrInvalidKind with its exact message, without partially
// committing classifications. Other app values retain existing decoding behavior
// and never produce ErrInvalidKind. A malformed or type-invalid
// file returns an ErrInvalidDefinition configuration error with file provenance
// and the original YAML cause.
func (l *Loader) DecodeBaseDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

func validateRootDefinition(ctx context.Context, workDir string) error {
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, workDir)
	}

	entries, err := os.ReadDir(workDir)
	if err != nil {
		return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, workDir)
	}
	foundRoot := false
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, workDir)
		}
		if !entry.Type().IsRegular() || !isRootYAMLFile(entry.Name()) {
			continue
		}

		path := filepath.Join(workDir, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, path)
		}
		isRoot, err := checkDefinitionKind(ctx, contents)
		if errors.Is(err, ErrInvalidKind) {
			return err
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, ctxErr, path)
			}
			continue
		}
		foundRoot = foundRoot || isRoot
	}

	if foundRoot {
		return nil
	}
	return errs.Build(errs.ExitConfiguration, ErrRootDefinitionMissing, nil)
}

// checkDefinitionKind validates only the common envelope, before typed decoding.
// The bool identifies a qualifying root; YAML errors retain their original cause.
func checkDefinitionKind(ctx context.Context, contents []byte) (bool, error) {
	var envelope struct {
		App  any `yaml:"app"`
		Kind any `yaml:"kind"`
	}
	if err := yaml.UnmarshalContext(ctx, contents, &envelope); err != nil {
		return false, err
	}
	app, _ := envelope.App.(string)
	if app != "apihydra" {
		return false, nil
	}
	kind, _ := envelope.Kind.(string)
	switch domain.DocumentKind(kind) {
	case domain.KindRoot:
		return true, nil
	case domain.KindDefaults, domain.KindSteps:
		return false, nil
	default:
		return false, errs.Build(errs.ExitConfiguration, ErrInvalidKind, nil)
	}
}

func isRootYAMLFile(name string) bool {
	extension := filepath.Ext(name)
	return extension == ".yaml" || extension == ".yml"
}
