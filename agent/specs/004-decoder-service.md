# `internal/definition` Decoder

## Status and ownership

- Binding reference: [`skeleton/internal/definition/decoder.go`](../../skeleton/internal/definition/decoder.go)
- Shared domain and pipeline: [`prd.md`](../prd.md)
- Contextual definition errors: [`002-errs-pkg.md`](002-errs-pkg.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Decoder's stateless API, mutations, and validation
scope. This guide does not reproduce those declarations or method contracts.
Decoder consumes Loader's classified files and leaves resolution, runtime
steps, and output to their owning packages.

The exact field rules are not present in the skeleton. In particular, this
guide does not define required fields, scalar presence rules, variable syntax,
response-type tokens, jq syntax, or a non-empty suite rule. The shared
`ExpectedStatus` zero-value rule belongs to the PRD rather than Decoder.

When an implementation creates contextual definition errors, their
construction is governed by `002-errs-pkg.md`; this guide does not duplicate its
formatting rules. The skeleton currently declares no Decoder-specific static
errors.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. `DecodeFiles` consumes directory-owned classified files and mutates only
   the two decoded-definition fields named by the skeleton.
3. Both validation methods traverse the decoded definitions indicated by the
   reference contract.
4. Decoder does not classify files, resolve values, execute steps, or invent
   validation rules absent from the reference.
