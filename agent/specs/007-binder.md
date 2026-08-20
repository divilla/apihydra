# `internal/execution` Binder

## Status and ownership

- Binding reference: [`skeleton/internal/execution/binder.go`](../../skeleton/internal/execution/binder.go)
- Reference tests: [`skeleton/internal/execution/binder_test.go`](../../skeleton/internal/execution/binder_test.go)
- Shared step model: [`prd.md`](../prd.md#defaults-and-steps)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Phase orchestration: [`010-executor.md`](010-executor.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Binder's API, collaborator, and phase
contracts. This guide does not reproduce them. The Executor skeleton contract
defines when the phases run, while KeyValueStore defines duplicate-storage
behavior and Runner defines capture extraction.

## Deliberately unspecified

The skeleton does not define:

- variable-name grammar within the two documented placeholder forms;
- escaping, replacement precedence, or behavior for a missing variable;
- variable scope or lifetime beyond the injected Binder store;
- YAMLString serialization;
- capture iteration order or selector syntax beyond delegation to
  `runner.JQExtract`;
- nil binder, store, context, or step behavior;
- success/failure code selection for these methods;
- cancellation behavior beyond accepting a context.

Duplicate storage behavior is inherited from `KeyValueStore.Set`; this guide
does not redefine it.

## Required implementation and tests

- Production output: `internal/execution/binder.go` replaces all canonical
  zero-value TODO bodies with the four binding variable phases while retaining
  the exact constructor and method signatures.
- Test output: `internal/execution/binder_test.go` covers loading, both
  placeholder forms in request and expected-response bodies, missing and
  duplicate variables, capture extraction/storage, context/command failures,
  and mutation boundaries under the chosen unspecified policies.
- Each acceptance criterion is traced to at least one meaningful unit test, and
  Binder unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Names, signatures, injected store, and binding phase responsibilities match
   the reference; production methods do not retain zero-value TODO bodies.
2. Each method remains limited to its documented variable phase, shared Step
   data, and the injected `KeyValueStore`.
3. Phase ordering is not duplicated from the Executor skeleton contract.
4. Capture delegates value extraction to `runner.JQExtract`; Binder does not
   execute external commands directly.
5. No variable grammar or error policy absent from the skeleton is introduced.
6. Package tests, race tests, and `git diff --check` pass.
