package definition

import (
	"apih/skeleton/internal/domain"
	"context"
)

type Decoder struct{}

func NewDecoder() *Decoder {
	return &Decoder{}
}

// DecodeFiles traverses directory structure from suite.Root,
// decoding *directory.DefaultsFile into directory.DefaultsDefinition and decoding *directory.StepsFiles
// into directory.StepsDefinitions. DecodeFiles mutates only
// domain.Directory.DefaultsDefinition and domain.Directory.StepsDefinitions
func (l *Decoder) DecodeFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// ValidateDefaultsDefinitions traverses directory structure from suite.Root,
// validating *directory.DefaultsDefinition. App exits on error.
func (l *Decoder) ValidateDefaultsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// ValidateStepsDefinitions traverses directory structure from suite.Root,
// iterating and validating *directory.StepsDefinitions. App exits on error.
func (l *Decoder) ValidateStepsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}
