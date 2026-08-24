package definition

import (
	"apih/skeleton/internal/domain"
	"context"
)

// Decoder decodes and validates classified definition files.
type Decoder struct{}

// NewDecoder returns a stateless Decoder.
func NewDecoder() *Decoder {
	return &Decoder{}
}

// DecodeFiles traverses suite.Root, decoding each DefaultsFile and StepsFiles
// entry into the corresponding Directory definition fields. It mutates only
// Directory.DefaultsDefinition and Directory.StepsDefinitions.
func (l *Decoder) DecodeFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// ValidateDefaultsDefinitions traverses suite.Root and validates each
// Directory.DefaultsDefinition, returning an error on failure.
func (l *Decoder) ValidateDefaultsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// ValidateStepsDefinitions traverses suite.Root and validates every entry in
// Directory.StepsDefinitions, returning an error on failure.
func (l *Decoder) ValidateStepsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}
