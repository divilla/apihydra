package definition

import (
	"apih/internal/domain"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// Loader discovers directories and definition files.
type Loader struct{}

// NewLoader returns a stateless Loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadDirectoryStructure traverses suite.WorkDir and builds suite.Root.
// Directory paths are relative to suite.WorkDir, and the root path is "/".
func (l *Loader) LoadDirectoryStructure(
	ctx context.Context,
	suite *domain.Suite,
) error {
	root, err := loadDirectory(ctx, suite.WorkDir, "", nil, 0)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	suite.Root = root
	return nil
}

// LoadDirectoryFiles traverses suite.Root and populates only each Directory.Files
// slice with that directory's .yaml and .yml files.
func (l *Loader) LoadDirectoryFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	files := make(map[*domain.Directory][]*domain.File)
	if err := collectDirectoryFiles(ctx, suite.WorkDir, suite.Root, files); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for directory, directoryFiles := range files {
		directory.Files = directoryFiles
	}
	return nil
}

// DecodeBaseDefinitions traverses suite.Root and attempts to decode each File
// as a BaseDefinition. Successful decodes set File.Kind and populate the owning
// Directory's DefaultsFile and StepsFiles fields.
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
				return err
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
				return err
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
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	absolutePath := filepath.Join(workDir, relativePath)
	entries, err := os.ReadDir(absolutePath)
	if err != nil {
		return nil, err
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
	if err := ctx.Err(); err != nil {
		return err
	}

	relativeDirectoryPath := strings.TrimPrefix(directory.Path, "/")
	absoluteDirectoryPath := filepath.Join(workDir, filepath.FromSlash(relativeDirectoryPath))
	entries, err := os.ReadDir(absoluteDirectoryPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.Type().IsRegular() || !isYAMLFile(entry.Name()) {
			continue
		}

		absoluteFilePath := filepath.Join(absoluteDirectoryPath, entry.Name())
		contents, err := os.ReadFile(absoluteFilePath)
		if err != nil {
			return err
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
