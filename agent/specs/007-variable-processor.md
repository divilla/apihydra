# `internal/execution` VariableProcessor

## Status and ownership

- Binding reference: `skeleton/internal/execution/varproc.go`
- Shared step model: [`prd.md`](../prd.md#defaults-and-steps)
- Phase orchestration: [`10-step-runner.md`](010-step-runner.md)
- Status: skeleton-aligned specification

This specification owns the VariableProcessor API and the step field associated
with each phase. StepRunner owns when the phases are called.

## Public API

```go
type VariableProcessor struct {
    kvs *KeyValueStore
}

func NewVariableProcessor(kvs *KeyValueStore) *VariableProcessor
func (p *VariableProcessor) LoadVariables(ctx context.Context, step *domain.Step) (int, error)
func (p *VariableProcessor) InterpolateRequestBody(ctx context.Context, step *domain.Step) (int, error)
func (p *VariableProcessor) InterpolateResponseExpected(ctx context.Context, step *domain.Step) (int, error)
func (p *VariableProcessor) CaptureResponseVariables(ctx context.Context, step *domain.Step) (int, error)
```

The names and signatures are exact. The skeleton method bodies panic with an
explicit not-implemented message; those placeholders define no runtime result.

## Construction and phases

`NewVariableProcessor` retains the supplied `KeyValueStore`. All four phases
read from or write to that same store.

The phases have these responsibilities:

| Method | Responsibility |
| --- | --- |
| `LoadVariables` | Store every `Step.Vars` entry in the processor's `KeyValueStore`. |
| `InterpolateRequestBody` | Replace `$var` and `${var}` placeholders in `Step.Request.Body` with values from the store. |
| `InterpolateResponseExpected` | Replace `$var` and `${var}` placeholders in `Step.Response.Expected` with values from the store. |
| `CaptureResponseVariables` | Evaluate every `Step.Response.Capture` selector against `Step.Response.Body` with `runner.JQExtract`, then store the extracted value under its capture name. |

The call order is owned by the StepRunner specification and is not repeated
here.

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

Duplicate storage behavior is inherited from `KeyValueStore.Set`; this spec
does not redefine it.

## Acceptance criteria

1. Names, signatures, injected store, and explicit not-implemented placeholders
   match the reference.
2. Each method remains limited to its documented variable phase, shared Step
   data, and the injected `KeyValueStore`.
3. Phase ordering is not duplicated from StepRunner.
4. Capture delegates value extraction to `runner.JQExtract`; VariableProcessor
   does not execute external commands directly.
5. No variable grammar or error policy absent from the skeleton is introduced.
