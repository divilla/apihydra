package definition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/errs"

	"github.com/goccy/go-yaml"
)

// ErrRootDefinitionMissing classifies a selection without a
// qualifying root definition in its directory or any ancestor.
var ErrRootDefinitionMissing = errors.New("kind: root - file missing")

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

// LoadDirectoryStructure requires a qualifying root directly in suite.WorkDir,
// as established by Select. It builds only selected subtrees and the ancestor
// chains needed by selected files or directories. Paths are relative to the
// discovered root, whose Path is "/" and Stage is 0. Root qualification ignores
// unrelated invalid kinds; full validation follows within the selected scope.
func (l *Loader) LoadDirectoryStructure(
	ctx context.Context,
	suite *domain.Suite,
) error {
	if err := validateRootDefinition(ctx, suite.WorkDir); err != nil {
		return err
	}
	root, err := loadSelectedDirectory(ctx, suite.WorkDir, "", nil, 0, suite.Selections)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, suite.WorkDir)
	}
	suite.Root = root
	return nil
}

// LoadDirectoryFiles traverses suite.Root and populates only each Directory.Files
// slice with selected .yaml and .yml files plus applicable root/defaults files.
// Ancestor-only directories exclude unselected steps and unrelated malformed
// files. Recognizable malformed defaults remain in scope for decoding errors.
// A traversal or file-read
// failure returns an ErrDefinitionDiscovery configuration error with the
// affected path and original cause.
func (l *Loader) LoadDirectoryFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	files := make(map[*domain.Directory][]*domain.File)
	if err := collectSelectedDirectoryFiles(ctx, suite.WorkDir, suite.Root, files, suite.Selections); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, suite.WorkDir)
	}
	for directory, directoryFiles := range files {
		directory.Files = directoryFiles
	}
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
	type classification struct {
		defaults *domain.File
		steps    []*domain.File
	}

	kinds := make(map[*domain.File]domain.DocumentKind)
	classifications := make(map[*domain.Directory]classification)
	err := walkDirectories(ctx, suite.Root, func(directory *domain.Directory) error {
		classified := classification{}
		for _, file := range directory.Files {
			if err := ctx.Err(); err != nil {
				return errs.Build(errs.ExitConfiguration, ErrInvalidDefinition, err, "file "+file.Path)
			}
			if _, err := checkDefinitionKind(ctx, file.Bytes); err != nil {
				if errors.Is(err, ErrInvalidKind) {
					return err
				}
				return errs.Build(errs.ExitConfiguration, ErrInvalidDefinition, err, "file "+file.Path)
			}
			var base domain.BaseDefinition
			if err := yaml.UnmarshalContext(
				ctx,
				file.Bytes,
				&base,
				yaml.CustomUnmarshalerContext[domain.YAMLString](func(ctx context.Context, spec *domain.YAMLString, raw []byte) error {
					if err := ctx.Err(); err != nil {
						return err
					}
					*spec = domain.YAMLString(raw)
					return nil
				}),
			); err != nil {
				return errs.Build(errs.ExitConfiguration, ErrInvalidDefinition, err, "file "+file.Path)
			}

			kind := domain.DocumentKind(base.Kind)
			kinds[file] = kind
			switch kind {
			case domain.KindRoot, domain.KindDefaults:
				classified.defaults = file
			case domain.KindSteps:
				classified.steps = append(classified.steps, file)
			}
		}
		classifications[directory] = classified
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) && !errors.Is(err, ErrInvalidDefinition) {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDefinition, err)
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitConfiguration, ErrInvalidDefinition, err)
	}

	for file, kind := range kinds {
		file.Kind = kind
	}
	for directory, classified := range classifications {
		directory.DefaultsFile = classified.defaults
		directory.StepsFiles = classified.steps
	}
	return nil
}

func loadSelectedDirectory(
	ctx context.Context,
	workDir string,
	relativePath string,
	parent *domain.Directory,
	stage int,
	selections []domain.Selection,
) (*domain.Directory, error) {
	absolutePath := filepath.Join(workDir, relativePath)
	if err := ctx.Err(); err != nil {
		return nil, errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, absolutePath)
	}

	entries, err := os.ReadDir(absolutePath)
	if err != nil {
		return nil, errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, absolutePath)
	}
	directory := &domain.Directory{
		Stage:  stage,
		Path:   directoryPath(relativePath),
		Parent: parent,
	}
	for _, entry := range entries {
		if !entry.IsDir() || !directoryNeeded(selections, filepath.Join(absolutePath, entry.Name())) {
			continue
		}
		childRelativePath := filepath.Join(relativePath, entry.Name())
		child, err := loadSelectedDirectory(ctx, workDir, childRelativePath, directory, stage+1, selections)
		if err != nil {
			return nil, err
		}
		directory.Children = append(directory.Children, child)
	}
	return directory, nil
}

func collectSelectedDirectoryFiles(
	ctx context.Context,
	workDir string,
	directory *domain.Directory,
	files map[*domain.Directory][]*domain.File,
	selections []domain.Selection,
) error {
	if directory == nil {
		return nil
	}
	relativeDirectoryPath := strings.TrimPrefix(directory.Path, "/")
	absoluteDirectoryPath := filepath.Join(workDir, filepath.FromSlash(relativeDirectoryPath))
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, absoluteDirectoryPath)
	}

	entries, err := os.ReadDir(absoluteDirectoryPath)
	if err != nil {
		return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, absoluteDirectoryPath)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, absoluteDirectoryPath)
		}
		if !entry.Type().IsRegular() || !isRootYAMLFile(entry.Name()) {
			continue
		}

		absoluteFilePath := filepath.Join(absoluteDirectoryPath, entry.Name())
		contents, err := os.ReadFile(absoluteFilePath)
		if err != nil {
			return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, absoluteFilePath)
		}
		if !fileSelected(selections, absoluteFilePath) && !inheritedDefinition(contents) {
			continue
		}
		files[directory] = append(files[directory], &domain.File{
			Stage:     directory.Stage,
			Path:      filePath(relativeDirectoryPath, entry.Name()),
			Bytes:     contents,
			Directory: directory,
		})
	}
	if _, ok := files[directory]; !ok {
		files[directory] = nil
	}

	for _, child := range directory.Children {
		if err := collectSelectedDirectoryFiles(ctx, workDir, child, files, selections); err != nil {
			return err
		}
	}
	return nil
}

func walkDirectories(
	ctx context.Context,
	directory *domain.Directory,
	visit func(*domain.Directory) error,
) error {
	if directory == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := visit(directory); err != nil {
		return err
	}
	for _, child := range directory.Children {
		if err := walkDirectories(ctx, child, visit); err != nil {
			return err
		}
	}
	return nil
}

func directoryPath(relativePath string) string {
	if relativePath == "" {
		return "/"
	}
	return "/" + filepath.ToSlash(relativePath)
}

func filePath(relativeDirectoryPath, name string) string {
	return filepath.ToSlash(filepath.Join(relativeDirectoryPath, name))
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
