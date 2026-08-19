# `internal/execution` VariableProcessor

## Status and ownership

- Binding reference: `skeleton/internal/execution/varproc.go`
- Shared step model: [`prd.md`](../prd.md#defaults-and-steps)
- Phase orchestration: [`09-step-runner.md`](10-step-runner.md)
- Status: skeleton-aligned specification

This specification owns the VariableProcessor API and the step field associated
with each phase. StepRunner owns when the phases are called.

## Public API

```go
type VariableProcessor struct{}

func NewVariableProcessor() *VariableProcessor
func (p *VariableProcessor) Load(ctx context.Context, step *domain.Step) (int, error)
func (p *VariableProcessor) ParseRequestBody(ctx context.Context, step *domain.Step) (int, error)
func (p *VariableProcessor) ParseResponseExpected(ctx context.Context, step *domain.Step) (int, error)
func (p *VariableProcessor) Capture(ctx context.Context, step *domain.Step) (int, error)
```

The names are exact. In particular, the reference has no `ParseRequest`,
`ParseResponse`, or constructor receiving a KeyValueStore.

## Construction and phases

`NewVariableProcessor` returns an empty processor. The type has no retained
store or other collaborator.

The phase names associate operations with the shared Step fields as follows:

| Method | Associated input |
| --- | --- |
| `Load` | `Step.Vars` |
| `ParseRequestBody` | `Step.Request.Body` |
| `ParseResponseExpected` | `Step.Response.Expected` |
| `Capture` | `Step.Response.Capture` and the runtime response |

This association does not define a variable language or mutation algorithm.
The call order is stated once in the StepRunner specification.

## Deliberately unspecified

The skeleton does not connect VariableProcessor to KeyValueStore or Runner and
does not define:

- variable name syntax, scope, lifetime, or duplicate behavior;
- YAMLString serialization;
- interpolation markers, escaping, or canonicalization;
- capture selector semantics or storage;
- success/failure code selection for these methods;
- cancellation behavior beyond accepting a context.

No such behavior is a requirement of this spec.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. Each method remains limited to its named variable phase and shared Step
   data.
3. Phase ordering is not duplicated from StepRunner.
4. The implementation does not add constructor state, direct command
   execution, output formatting, or a variable grammar absent from the
   skeleton.
