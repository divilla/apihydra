package definition

import (
	"apih/skeleton/internal/domain"
	"context"
)

type Loader struct{}

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
	// TODO: implement
	return nil
}

// LoadDirectoryFiles traverses directory structure from suite.Root
// populating each domain.Directory.Files with all directory *.yaml and *.yml files.
// LoadDirectoryFiles mutates only *domain.Directory.Files slice
func (l *Loader) LoadDirectoryFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// DecodeBaseDefinitions traverses directory structure from suite.Root,
// trying to decode each directory's Files into domain.BaseDefinition.
// On success it updates File.Kind and populates each directory's DefaultsFile
// and StepsFiles.
func (l *Loader) DecodeBaseDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}
