package definition

import (
	"apih/skeleton/internal/domain"
	"context"
)

type Resolver struct{}

func NewResolver() *Resolver {
	return &Resolver{}
}

// ResolveDefaults traverses directory structure from suite.Root and
// populates directory.ResolvedDefaults with values merged from
// self directory.DefaultsDefinition and parent directory.DefaultsDefinition
func (l *Resolver) ResolveDefaults(
	ctx context.Context,
	suite *domain.Suite,
) error {
	return nil
}

// ResolveSteps traverses directory structure from suite.Root and
// populates directory.ResolvedSteps with values merged from
// self directory.StepsDefinition and directory.DefaultsDefinition
func (l *Resolver) ResolveSteps(
	ctx context.Context,
	suite *domain.Suite,
) error {
	return nil
}

// ValidateStepsDefinitions traverses directory structure from suite.Root,
// iterating and validating *directory.StepsDefinitions. App exits on error.
func (l *Resolver) ValidateStepsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	return nil
}
