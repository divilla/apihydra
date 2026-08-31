package definition

import (
	"apih/skeleton/internal/domain"
	"context"
)

// Resolver combines decoded definitions into executable step values.
type Resolver struct{}

// NewResolver returns a stateless Resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// ResolveDefaults traverses suite.Root and populates each ResolvedDefaults with
// values merged from the directory's and parent directory's DefaultsDefinition.
// DisableCookies is presence-sensitive: nil inherits, true disables automatic
// cookie handling, and false explicitly enables it.
func (l *Resolver) ResolveDefaults(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// ResolveSteps traverses suite.Root and populates each ResolvedSteps with values
// merged from the directory's StepsDefinitions and DefaultsDefinition. It
// applies the same DisableCookies presence-sensitive overlay from directory to
// steps-file to individual-step defaults.
func (l *Resolver) ResolveSteps(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// ValidateStepsDefinitions traverses suite.Root and validates every entry in
// Directory.StepsDefinitions, returning an error on failure.
func (l *Resolver) ValidateStepsDefinitions(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}
