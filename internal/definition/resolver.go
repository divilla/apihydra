package definition

import (
	"apih/internal/domain"
	"context"
)

const (
	defaultTimeoutSeconds = 10
	defaultRetries        = 3
)

// Resolver combines decoded definitions into executable step values.
type Resolver struct{}

// NewResolver returns a stateless Resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// ResolveDefaults traverses suite.Root and populates each ResolvedDefaults with
// values merged from the directory's and parent directory's DefaultsDefinition.
func (l *Resolver) ResolveDefaults(
	ctx context.Context,
	suite *domain.Suite,
) error {
	resolved := make(map[*domain.Directory]domain.Defaults)
	if err := walkDirectories(ctx, suite.Root, func(directory *domain.Directory) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		defaults := resolved[directory.Parent]
		if directory.Parent == nil {
			defaults.Timeout = defaultTimeoutSeconds
			defaults.Retries = defaultRetries
		}
		if directory.DefaultsDefinition != nil {
			defaults = mergeDefaults(defaults, directory.DefaultsDefinition.Spec)
		} else {
			defaults.Headers = cloneMap(defaults.Headers)
		}
		resolved[directory] = defaults
		return nil
	}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for directory, defaults := range resolved {
		directory.ResolvedDefaults = defaults
	}
	return nil
}

// ResolveSteps traverses suite.Root and populates each ResolvedSteps with values
// merged from the directory's StepsDefinitions and DefaultsDefinition.
func (l *Resolver) ResolveSteps(
	ctx context.Context,
	suite *domain.Suite,
) error {
	resolved := make(map[*domain.Directory][][]domain.Step)
	if err := walkDirectories(ctx, suite.Root, func(directory *domain.Directory) error {
		groups := make([][]domain.Step, len(directory.StepsDefinitions))
		for definitionIndex, definition := range directory.StepsDefinitions {
			if err := ctx.Err(); err != nil {
				return err
			}
			if definition == nil {
				continue
			}

			steps := make([]domain.Step, len(definition.Spec.Steps))
			for stepIndex, step := range definition.Spec.Steps {
				if err := ctx.Err(); err != nil {
					return err
				}
				steps[stepIndex] = resolveStep(directory.ResolvedDefaults, step)
			}
			groups[definitionIndex] = steps
		}
		resolved[directory] = groups
		return nil
	}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for directory, steps := range resolved {
		directory.ResolvedSteps = steps
	}
	return nil
}

// ValidateStepsDefinitions traverses suite.Root and validates every entry in
// Directory.StepsDefinitions, returning an error on failure.
func (l *Resolver) ValidateStepsDefinitions(
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

func mergeDefaults(parent, local domain.Defaults) domain.Defaults {
	merged := parent
	if local.BaseURL != "" {
		merged.BaseURL = local.BaseURL
	}
	if local.BasePath != "" {
		merged.BasePath = local.BasePath
	}
	if local.Timeout != 0 {
		merged.Timeout = local.Timeout
	}
	if local.Retries != 0 {
		merged.Retries = local.Retries
	}
	merged.Headers = mergeMaps(parent.Headers, local.Headers)
	return merged
}

func resolveStep(defaults domain.Defaults, step domain.Step) domain.Step {
	resolved := step
	if resolved.Request.BaseURL == "" {
		resolved.Request.BaseURL = defaults.BaseURL
	}
	if resolved.Request.BasePath == "" {
		resolved.Request.BasePath = defaults.BasePath
	}
	if resolved.Request.Timeout == 0 {
		resolved.Request.Timeout = defaults.Timeout
	}
	if resolved.Request.Retries == 0 {
		resolved.Request.Retries = defaults.Retries
	}

	resolved.Vars = cloneMap(step.Vars)
	resolved.Request.Headers = mergeMaps(defaults.Headers, step.Request.Headers)
	resolved.Response.ExpectedTypes = cloneStringSlices(step.Response.ExpectedTypes)
	resolved.Response.Capture = cloneMap(step.Response.Capture)
	return resolved
}

func mergeMaps[K comparable, V any](base, overlay map[K]V) map[K]V {
	if base == nil && overlay == nil {
		return nil
	}
	merged := make(map[K]V, len(base)+len(overlay))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	return mergeMaps(source, nil)
}

func cloneStringSlices(source map[string][]string) map[string][]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string][]string, len(source))
	for key, values := range source {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}
