# `internal/definition` Decoder

## Status and ownership

- Binding reference: `skeleton/internal/definition/decoder.go`
- Shared domain and pipeline: [`prd.md`](../prd.md)
- Contextual definition errors: [`02-errs-pkg.md`](02-errs-pkg.md)
- Status: skeleton-aligned specification

This specification owns decoding classified files and validating the resulting
definitions. It does not restate the shared model.

## Public API

```go
type Decoder struct{}

func NewDecoder() *Decoder
func (d *Decoder) DecodeFiles(ctx context.Context, suite *domain.Suite) error
func (d *Decoder) ValidateDefaultsDefinitions(ctx context.Context, suite *domain.Suite) error
func (d *Decoder) ValidateStepsDefinitions(ctx context.Context, suite *domain.Suite) error
```

The receiver name is not contractual; the type and method names are.
`NewDecoder` returns an empty Decoder with no retained Loader or Suite.

## `DecodeFiles`

The method traverses from `suite.Root`, decodes each directory's
`DefaultsFile` into `DefaultsDefinition`, and decodes `StepsFiles` into
`StepsDefinitions`.

As stated by the reference comment, it mutates only
`Directory.DefaultsDefinition` and `Directory.StepsDefinitions`. File
classification, resolution, runtime steps, and output remain untouched.

## Validation methods

`ValidateDefaultsDefinitions` traverses from `suite.Root` and validates each
decoded `DefaultsDefinition`.

`ValidateStepsDefinitions` traverses from `suite.Root` and validates each
decoded entry in `StepsDefinitions`.

The exact field rules are not present in the skeleton. In particular, this
spec does not define required fields, scalar presence rules, variable syntax,
response-type tokens, jq syntax, HTTP-status behavior, or a non-empty suite
rule.

When an implementation creates contextual definition errors, their
construction is governed by `02-errs-pkg.md`; this spec does not duplicate its
formatting rules. The skeleton currently declares no Decoder-specific static
errors.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. `DecodeFiles` consumes directory-owned classified files and mutates only
   the two decoded-definition fields named by the skeleton.
3. Both validation methods traverse the decoded definitions indicated by their
   names and comments.
4. Decoder does not classify files, resolve values, execute steps, or invent
   validation rules absent from the reference.
