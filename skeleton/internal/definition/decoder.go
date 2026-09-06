package definition

import (
	"context"

	"github.com/divilla/apihydra/skeleton/internal/domain"
)

// Decoder decodes and validates classified definition files.
type Decoder struct{}

// NewDecoder returns a stateless Decoder.
func NewDecoder() *Decoder {
	return &Decoder{}
}

// DecodeFiles traverses suite.Root, decoding each DefaultsFile and StepsFiles
// entry into the corresponding Directory definition fields. It mutates only
// Directory.DefaultsDefinition and Directory.StepsDefinitions. A malformed or
// type-invalid file returns an ErrInvalidDefinition configuration error that
// preserves the source file, the most specific available YAML path, and the
// original YAML cause.
func (l *Decoder) DecodeFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// ValidateDefaultsDefinitions traverses suite.Root and validates each
// Directory.DefaultsDefinition. A field validation failure returns an
// ErrInvalidDefinition configuration error with file and YAML-path provenance.
func (l *Decoder) ValidateDefaultsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// ValidateStepsDefinitions traverses suite.Root and validates every entry in
// Directory.StepsDefinitions. A field validation failure returns an
// ErrInvalidDefinition configuration error with file and YAML-path provenance.
func (l *Decoder) ValidateStepsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}
