// Package definition loads, decodes, and resolves APIHydra definitions.
package definition

import (
	"apih/internal/domain"
	"context"

	"github.com/goccy/go-yaml"
)

// Decoder decodes and validates classified definition files.
type Decoder struct{}

// NewDecoder constructs a stateless Decoder.
func NewDecoder() *Decoder {
	return &Decoder{}
}

// DecodeFiles traverses directory structure from suite.Root,
// decoding *directory.DefaultsFile into directory.DefaultsDefinition and decoding *directory.StepsFiles
// into directory.StepsDefinitions. DecodeFiles mutates only
// domain.Directory.DefaultsDefinition and domain.Directory.StepsDefinitions.
func (d *Decoder) DecodeFiles(
	ctx context.Context,
	suite *domain.Suite,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if suite == nil || suite.Root == nil {
		return nil
	}

	var decode func(*domain.Directory) error
	decode = func(directory *domain.Directory) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		var defaultsDefinition *domain.DefaultsDefinition
		if file := directory.DefaultsFile; file != nil {
			definition := &domain.DefaultsDefinition{}
			if err := yaml.UnmarshalContext(ctx, file.Bytes, definition); err != nil {
				return err
			}
			definition.File = file
			defaultsDefinition = definition
		}

		stepsDefinitions := make([]*domain.StepsDefinition, 0, len(directory.StepsFiles))
		for _, file := range directory.StepsFiles {
			definition := &domain.StepsDefinition{}
			if err := yaml.UnmarshalContext(ctx, file.Bytes, definition); err != nil {
				return err
			}
			definition.File = file
			for index := range definition.Spec.Steps {
				definition.Spec.Steps[index].Definition = definition
				definition.Spec.Steps[index].Index = index
			}
			stepsDefinitions = append(stepsDefinitions, definition)
		}

		directory.DefaultsDefinition = defaultsDefinition
		directory.StepsDefinitions = stepsDefinitions

		for _, child := range directory.Children {
			if err := decode(child); err != nil {
				return err
			}
		}
		return nil
	}

	return decode(suite.Root)
}

// ValidateDefaultsDefinitions traverses directory structure from suite.Root,
// validating *directory.DefaultsDefinition. App exits on error.
func (d *Decoder) ValidateDefaultsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if suite == nil || suite.Root == nil {
		return nil
	}

	var validate func(*domain.Directory) error
	validate = func(directory *domain.Directory) error {
		if directory.DefaultsDefinition != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		for _, child := range directory.Children {
			if err := validate(child); err != nil {
				return err
			}
		}
		return nil
	}

	return validate(suite.Root)
}

// ValidateStepsDefinitions traverses directory structure from suite.Root,
// iterating and validating *directory.StepsDefinitions. App exits on error.
func (d *Decoder) ValidateStepsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if suite == nil || suite.Root == nil {
		return nil
	}

	var validate func(*domain.Directory) error
	validate = func(directory *domain.Directory) error {
		for range directory.StepsDefinitions {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		for _, child := range directory.Children {
			if err := validate(child); err != nil {
				return err
			}
		}
		return nil
	}

	return validate(suite.Root)
}
