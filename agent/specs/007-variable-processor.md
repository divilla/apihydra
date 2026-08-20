# `internal/execution` VariableProcessor

## Status and ownership

- Binding reference: [`skeleton/internal/execution/varproc.go`](../../skeleton/internal/execution/varproc.go)
- Shared step model: [`prd.md`](../prd.md#defaults-and-steps)
- Phase orchestration: [`010-step-runner-service.md`](010-step-runner-service.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines VariableProcessor's API, collaborator, and phase
contracts. This guide does not reproduce them. The StepRunner skeleton contract
defines when the phases run, while KeyValueStore defines duplicate-storage
behavior and Runner defines capture extraction.

## Deliberately unspecified

The skeleton does not define:

- variable-name grammar within the two documented placeholder forms;
- escaping, replacement precedence, or behavior for a missing variable;
- variable scope or lifetime beyond the injected processor store;
- YAMLString serialization;
- capture iteration order or selector syntax beyond delegation to
  `runner.JQExtract`;
- nil processor, store, context, or step behavior;
- success/failure code selection for these methods;
- cancellation behavior beyond accepting a context.

Duplicate storage behavior is inherited from `KeyValueStore.Set`; this guide
does not redefine it.

## Acceptance criteria

1. Names, signatures, injected store, and explicit not-implemented placeholders
   match the reference.
2. Each method remains limited to its documented variable phase, shared Step
   data, and the injected `KeyValueStore`.
3. Phase ordering is not duplicated from the StepRunner skeleton contract.
4. Capture delegates value extraction to `runner.JQExtract`; VariableProcessor
   does not execute external commands directly.
5. No variable grammar or error policy absent from the skeleton is introduced.
