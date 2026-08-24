package definition

import (
	"apih/internal/domain"
	"apih/pkg/errs"
	"context"

	"github.com/goccy/go-yaml"
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
	type decodedDefinitions struct {
		defaults *domain.DefaultsDefinition
		steps    []*domain.StepsDefinition
	}

	decoded := make(map[*domain.Directory]decodedDefinitions)
	err := walkDirectories(ctx, suite.Root, func(directory *domain.Directory) error {
		definitions := decodedDefinitions{}
		if directory.DefaultsFile != nil {
			definition, err := decodeDefaultsDefinition(ctx, directory.DefaultsFile)
			if err != nil {
				return err
			}
			definitions.defaults = definition
		}

		definitions.steps = make([]*domain.StepsDefinition, 0, len(directory.StepsFiles))
		for _, file := range directory.StepsFiles {
			if err := ctx.Err(); err != nil {
				return err
			}
			definition, err := decodeStepsDefinition(ctx, file)
			if err != nil {
				return err
			}
			definitions.steps = append(definitions.steps, definition)
		}
		decoded[directory] = definitions
		return nil
	})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for directory, definitions := range decoded {
		directory.DefaultsDefinition = definitions.defaults
		directory.StepsDefinitions = definitions.steps
	}
	return nil
}

// ValidateDefaultsDefinitions traverses suite.Root and validates each
// Directory.DefaultsDefinition, returning an error on failure.
func (l *Decoder) ValidateDefaultsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	return walkDirectories(ctx, suite.Root, func(directory *domain.Directory) error {
		if directory.DefaultsDefinition == nil {
			return nil
		}
		return ctx.Err()
	})
}

// ValidateStepsDefinitions traverses suite.Root and validates every entry in
// Directory.StepsDefinitions, returning an error on failure.
func (l *Decoder) ValidateStepsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	return walkDirectories(ctx, suite.Root, func(directory *domain.Directory) error {
		for range directory.StepsDefinitions {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		return nil
	})
}

func decodeDefaultsDefinition(ctx context.Context, file *domain.File) (*domain.DefaultsDefinition, error) {
	definition := &domain.DefaultsDefinition{File: file}
	if err := yaml.UnmarshalContext(ctx, file.Bytes, definition); err != nil {
		return nil, errs.DefaultsDefinitionError(definition, "", err, nil)
	}
	return definition, nil
}

func decodeStepsDefinition(ctx context.Context, file *domain.File) (*domain.StepsDefinition, error) {
	definition := &domain.StepsDefinition{File: file}
	if err := yaml.UnmarshalContext(ctx, file.Bytes, definition); err != nil {
		return nil, errs.StepDefinitionError(definition, "", err, nil)
	}
	for index := range definition.Spec.Steps {
		definition.Spec.Steps[index].Definition = definition
		definition.Spec.Steps[index].Index = index
	}
	return definition, nil
}
