// Package definition loads, decodes, and resolves APIHydra definitions.
package definition

import (
	"apih/internal/domain"
	"context"
)

// Resolver resolves inherited defaults and request steps.
type Resolver struct{}

// NewResolver constructs a stateless Resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// ResolveDefaults traverses directory structure from suite.Root and
// populates directory.ResolvedDefaults with values merged from
// self directory.DefaultsDefinition and parent directory.DefaultsDefinition.
func (r *Resolver) ResolveDefaults(
	ctx context.Context,
	suite *domain.Suite,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if suite == nil || suite.Root == nil {
		return nil
	}

	var resolve func(*domain.Directory, domain.Defaults) error
	resolve = func(directory *domain.Directory, inherited domain.Defaults) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		resolved := mergeDefaults(inherited, defaultsDefinition(directory))
		directory.ResolvedDefaults = resolved

		for _, child := range directory.Children {
			if err := resolve(child, resolved); err != nil {
				return err
			}
		}
		return nil
	}

	return resolve(suite.Root, domain.Defaults{})
}

// ResolveSteps traverses directory structure from suite.Root and
// populates directory.ResolvedSteps with values merged from
// self directory.StepsDefinition and directory.DefaultsDefinition.
func (r *Resolver) ResolveSteps(
	ctx context.Context,
	suite *domain.Suite,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if suite == nil || suite.Root == nil {
		return nil
	}

	var resolve func(*domain.Directory, domain.Defaults) error
	resolve = func(directory *domain.Directory, inherited domain.Defaults) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		defaults := mergeDefaults(inherited, defaultsDefinition(directory))
		resolved := make([][]domain.Step, len(directory.StepsDefinitions))
		for definitionIndex, definition := range directory.StepsDefinitions {
			if err := ctx.Err(); err != nil {
				return err
			}
			if definition == nil {
				resolved[definitionIndex] = []domain.Step{}
				continue
			}

			steps := make([]domain.Step, len(definition.Spec.Steps))
			for stepIndex, step := range definition.Spec.Steps {
				if err := ctx.Err(); err != nil {
					return err
				}
				steps[stepIndex] = mergeStepDefaults(step, defaults)
			}
			resolved[definitionIndex] = steps
		}
		directory.ResolvedSteps = resolved

		for _, child := range directory.Children {
			if err := resolve(child, defaults); err != nil {
				return err
			}
		}
		return nil
	}

	return resolve(suite.Root, domain.Defaults{})
}

// ValidateStepsDefinitions traverses directory structure from suite.Root,
// iterating and validating *directory.StepsDefinitions. App exits on error.
func (r *Resolver) ValidateStepsDefinitions(
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
		if err := ctx.Err(); err != nil {
			return err
		}
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

func defaultsDefinition(directory *domain.Directory) domain.Defaults {
	if directory.DefaultsDefinition == nil {
		return domain.Defaults{}
	}
	return directory.DefaultsDefinition.Spec
}

func mergeDefaults(inherited, local domain.Defaults) domain.Defaults {
	resolved := inherited
	if local.BaseURL != "" {
		resolved.BaseURL = local.BaseURL
	}
	if local.BasePath != "" {
		resolved.BasePath = local.BasePath
	}
	if local.Timeout != 0 {
		resolved.Timeout = local.Timeout
	}
	if local.Retries != 0 {
		resolved.Retries = local.Retries
	}
	resolved.Headers = mergeHeaders(inherited.Headers, local.Headers)
	return resolved
}

func mergeStepDefaults(step domain.Step, defaults domain.Defaults) domain.Step {
	if step.Request.BaseURL == "" {
		step.Request.BaseURL = defaults.BaseURL
	}
	if step.Request.BasePath == "" {
		step.Request.BasePath = defaults.BasePath
	}
	if step.Request.Timeout == 0 {
		step.Request.Timeout = defaults.Timeout
	}
	if step.Request.Retries == 0 {
		step.Request.Retries = defaults.Retries
	}
	step.Request.Headers = mergeHeaders(defaults.Headers, step.Request.Headers)
	return step
}

func mergeHeaders(inherited, local map[string]string) map[string]string {
	if inherited == nil && local == nil {
		return nil
	}

	merged := make(map[string]string, len(inherited)+len(local))
	for name, value := range inherited {
		merged[name] = value
	}
	for name, value := range local {
		merged[name] = value
	}
	return merged
}
