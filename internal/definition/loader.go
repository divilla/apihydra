// Package definition loads, decodes, and resolves APIHydra definitions.
package definition

import (
	"apih/internal/domain"
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// Loader discovers directories and definition files.
type Loader struct{}

// NewLoader constructs a stateless Loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadDirectoryStructure traverses directory structure from suite.WorkDir
// building *domain.Directory structure. *domain.Directory.Path is relative
// to suite.WorkDir, so suite.Root.Path = "/".
func (l *Loader) LoadDirectoryStructure(
	ctx context.Context,
	suite *domain.Suite,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	workDir := filepath.Clean(suite.WorkDir)
	var root *domain.Directory
	directories := make(map[string]*domain.Directory)
	err := filepath.WalkDir(workDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			if filePath == workDir {
				return fs.ErrInvalid
			}
			return nil
		}

		relativePath, err := filepath.Rel(workDir, filePath)
		if err != nil {
			return err
		}
		directoryPath := "/"
		if relativePath != "." {
			directoryPath = "/" + filepath.ToSlash(relativePath)
		}

		directory := &domain.Directory{Path: directoryPath}
		if filePath == workDir {
			root = directory
		} else {
			parent := directories[filepath.Dir(filePath)]
			directory.Stage = parent.Stage + 1
			directory.Parent = parent
			parent.Children = append(parent.Children, directory)
		}
		directories[filePath] = directory
		return nil
	})
	if err != nil {
		return err
	}

	suite.Root = root
	return nil
}

// LoadDirectoryFiles traverses directory structure from suite.Root
// populating each domain.Directory.Files with all directory *.yaml and *.yml files.
// LoadDirectoryFiles mutates only *domain.Directory.Files slice.
func (l *Loader) LoadDirectoryFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	var load func(*domain.Directory) error
	load = func(directory *domain.Directory) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		relativePath := strings.TrimPrefix(directory.Path, "/")
		directoryPath := filepath.Join(suite.WorkDir, filepath.FromSlash(relativePath))
		entries, err := os.ReadDir(directoryPath)
		if err != nil {
			return err
		}

		files := make([]*domain.File, 0)
		for _, entry := range entries {
			if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") &&
				!strings.HasSuffix(entry.Name(), ".yml")) {
				continue
			}

			contents, err := os.ReadFile(filepath.Join(directoryPath, entry.Name()))
			if err != nil {
				return err
			}
			files = append(files, &domain.File{
				Stage:     directory.Stage,
				Path:      path.Join(directory.Path, entry.Name()),
				Bytes:     contents,
				Directory: directory,
			})
		}
		directory.Files = files

		for _, child := range directory.Children {
			if err := load(child); err != nil {
				return err
			}
		}
		return nil
	}

	return load(suite.Root)
}

// DecodeBaseDefinitions traverses directory structure from suite.Root,
// trying to decode each directory's Files into domain.BaseDefinition.
// On success it updates File.Kind and populates each directory's DefaultsFile
// and StepsFiles.
func (l *Loader) DecodeBaseDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	type baseDocument struct {
		App  string          `yaml:"app"`
		Kind string          `yaml:"kind"`
		Spec yaml.RawMessage `yaml:"spec"`
	}

	var decode func(*domain.Directory) error
	decode = func(directory *domain.Directory) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		var defaultsFile *domain.File
		stepsFiles := make([]*domain.File, 0)
		for _, file := range directory.Files {
			var document baseDocument
			if err := yaml.UnmarshalContext(ctx, file.Bytes, &document); err != nil {
				return err
			}
			base := domain.BaseDefinition{
				App:  document.App,
				Kind: document.Kind,
				Spec: domain.YAMLString(document.Spec),
			}
			file.Kind = domain.DocumentKind(base.Kind)

			switch file.Kind {
			case domain.KindDefaults:
				defaultsFile = file
			case domain.KindSteps:
				stepsFiles = append(stepsFiles, file)
			}
		}
		directory.DefaultsFile = defaultsFile
		directory.StepsFiles = stepsFiles

		for _, child := range directory.Children {
			if err := decode(child); err != nil {
				return err
			}
		}
		return nil
	}

	return decode(suite.Root)
}
