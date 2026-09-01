package definition

import (
	"context"

	"github.com/divilla/apihydra/skeleton/internal/domain"
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
	// TODO: implement
	suite.Root = &domain.Directory{
		Path: "/",
	}
	return nil
}

// LoadDirectoryFiles traverses suite.Root and populates only each Directory.Files
// slice with that directory's .yaml and .yml files.
func (l *Loader) LoadDirectoryFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// DecodeBaseDefinitions traverses suite.Root and attempts to decode each File
// as a BaseDefinition. Successful decodes set File.Kind and populate the owning
// Directory's DefaultsFile and StepsFiles fields.
func (l *Loader) DecodeBaseDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}
