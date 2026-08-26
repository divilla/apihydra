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
// CookieMode defaults to included, and CookieMode and CookieKeys inherit through
// the same root-to-directory chain as the other defaults.
func (l *Resolver) ResolveDefaults(
	ctx context.Context,
	suite *domain.Suite,
) error {
	// TODO: implement
	return nil
}

// ResolveSteps traverses suite.Root and populates each ResolvedSteps with values
// merged from the directory's StepsDefinitions and DefaultsDefinition. Request
// CookieMode and CookieKeys inherit in root, nested-directory, steps-file
// defaults, and step order; each narrower value overrides its inherited value.
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
