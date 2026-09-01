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

// ErrRootDefinitionMissing classifies a selected suite directory without a
// qualifying top-level root definition.
var ErrRootDefinitionMissing = errors.New("root definition missing")

// ErrDefinitionDiscovery classifies a failure to inspect or read definition
// inputs from the selected suite directory tree.
var ErrDefinitionDiscovery = errors.New("definition discovery error")

// ErrInvalidDefinition classifies malformed YAML or an invalid definition
// field. Definition decoding preserves the file, YAML path, and original cause.
var ErrInvalidDefinition = errors.New("invalid definition")

// Loader discovers directories and definition files.
type Loader struct{}

// NewLoader returns a stateless Loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadDirectoryStructure first requires a regular .yaml or .yml file directly
// in suite.WorkDir whose string envelope values are app: apihydra and
// kind: root. The filename is otherwise unrestricted. Malformed files, files
// with non-string or different envelope values, and definitions in descendants
// do not satisfy the requirement. A missing qualifying file returns
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
	root, err := loadDirectory(ctx, suite.WorkDir, "", nil, 0)
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
// slice with that directory's .yaml and .yml files. A traversal or file-read
// failure returns an ErrDefinitionDiscovery configuration error with the
// affected path and original cause.
func (l *Loader) LoadDirectoryFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	files := make(map[*domain.Directory][]*domain.File)
	if err := collectDirectoryFiles(ctx, suite.WorkDir, suite.Root, files); err != nil {
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
// Directory's DefaultsFile and StepsFiles fields. A malformed or type-invalid
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

func loadDirectory(
	ctx context.Context,
	workDir string,
	relativePath string,
	parent *domain.Directory,
	stage int,
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
		if !entry.IsDir() {
			continue
		}
		childRelativePath := filepath.Join(relativePath, entry.Name())
		child, err := loadDirectory(ctx, workDir, childRelativePath, directory, stage+1)
		if err != nil {
			return nil, err
		}
		directory.Children = append(directory.Children, child)
	}
	return directory, nil
}

func collectDirectoryFiles(
	ctx context.Context,
	workDir string,
	directory *domain.Directory,
	files map[*domain.Directory][]*domain.File,
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
		if !entry.Type().IsRegular() || !isYAMLFile(entry.Name()) {
			continue
		}

		absoluteFilePath := filepath.Join(absoluteDirectoryPath, entry.Name())
		contents, err := os.ReadFile(absoluteFilePath)
		if err != nil {
			return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, absoluteFilePath)
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
		if err := collectDirectoryFiles(ctx, workDir, child, files); err != nil {
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

func isYAMLFile(name string) bool {
	extension := filepath.Ext(name)
	return extension == ".yaml" || extension == ".yml"
}

func validateRootDefinition(ctx context.Context, workDir string) error {
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, workDir)
	}

	entries, err := os.ReadDir(workDir)
	if err != nil {
		return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, err, workDir)
	}
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
		var envelope struct {
			App  any `yaml:"app"`
			Kind any `yaml:"kind"`
		}
		if err := yaml.UnmarshalContext(ctx, contents, &envelope); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return errs.Build(errs.ExitConfiguration, ErrDefinitionDiscovery, ctxErr, path)
			}
			continue
		}
		app, appIsString := envelope.App.(string)
		kind, kindIsString := envelope.Kind.(string)
		if appIsString && kindIsString && app == "apihydra" && kind == string(domain.KindRoot) {
			return nil
		}
	}

	return errs.Build(errs.ExitConfiguration, ErrRootDefinitionMissing, nil)
}

func isRootYAMLFile(name string) bool {
	extension := filepath.Ext(name)
	return extension == ".yaml" || extension == ".yml"
}
