# `internal/execution` StepRunner

## Status and ownership

- Binding reference: `skeleton/internal/execution/steprun.go`
- Reference tests: `skeleton/internal/execution/steprun_test.go`
- Shared domain and exit codes: [`prd.md`](../prd.md)
- Reporter methods: [`10-reporter.md`](10-reporter.md)
- Status: skeleton-aligned specification

This specification is the single owner of preparation order, execution phase
order, directory-tree validation, and stage scheduling. Collaborator specs do
not restate those rules.

## Public API

```go
var ErrInvalidDirectoryTree = errors.New("invalid directory tree")
var ExecutionCanceledError = errors.New("execution canceled")

func NewStepRunner(
    variableProcessor *VariableProcessor,
    validator *Validator,
    report *reporter.Reporter,
) *StepRunner

func (s *StepRunner) Prepare(ctx context.Context, suite *domain.Suite) error
func (s *StepRunner) Execute(ctx context.Context, suite *domain.Suite) (int, error)
```

`NewStepRunner` retains the three supplied collaborators without performing
work.

## Preparation

`Prepare` traverses directories through `Children`, starting at `suite.Root`.
For each step encountered in `Directory.ResolvedSteps`, it invokes:

1. `VariableProcessor.Load`
2. `VariableProcessor.ParseRequestBody`

The skeleton does not define runtime-copy policy, preparation transactionality,
or conversion of collaborators' integer codes into Prepare's error-only
result.

## Directory-tree validation

Before processing directories, `Execute` collects the reachable tree by stage.
The implemented reference rejects, with a built configuration error matching
`ErrInvalidDirectoryTree`:

- a nil Suite or nil root;
- a root whose stage is not `0`;
- a nil child;
- a repeated directory pointer, including a cycle;
- a child whose `Parent` is not the containing directory;
- a child whose stage is not its parent's stage plus one.

The collector supports arbitrary valid depth and preserves each directory
pointer in the group indexed by its stage.

## Stage scheduling

Stages run in ascending order. For one active stage, StepRunner starts one
goroutine per directory and waits for every started goroutine before returning
or advancing to the next stage.

The first directory error recorded for a stage retains its error and non-zero
code and cancels the shared execution context. If that directory supplied code
`0`, StepRunner derives a code through `errs.Code` with `ExitInternal` fallback.
Sibling goroutines are still joined, and later stages do not start.

If the shared context is canceled between otherwise successful stages,
execution returns `ExitInternal` and a built error matching
`ExecutionCanceledError` that preserves the context error.

No ordering between same-stage directory goroutines is promised.

## Directory and step processing

For each directory, StepRunner iterates the groups and steps already represented
by `Directory.ResolvedSteps`. The reference does not specify sorting beyond
that slice structure and does not define separate file or step goroutines.

For each executed step, the skeleton comment fixes this phase order:

1. `runner.Curl`
2. `VariableProcessor.ParseResponseExpected`
3. `Validator.ValidateTypes`
4. `Validator.ValidateExpected`
5. `VariableProcessor.Capture`

StepRunner sends detected validation failures to its Reporter. After traversal
finishes with at least one validation mismatch, `Execute` returns
`errs.ExitValidation` and an error matching `ValidationError`.

The skeleton does not define phase argument construction, response assignment,
the mapping of individual validation errors to Reporter methods, success
reporting, debug selection/stopping, or precedence between multiple nonfatal
failures. Those details remain outside this spec.

## Boundaries

StepRunner coordinates its collaborators. It does not own variable syntax,
validation algorithms, command implementation, output layout, or contextual
error formatting. Those contracts remain with their owning specs.

## Acceptance criteria

1. Public names, signatures, constructor state, and static error text match the
   reference.
2. Preparation uses the exact two phases in order.
3. Every invalid tree shape covered by the reference returns configuration code
   and `ErrInvalidDirectoryTree` without panic.
4. Same-stage directories may overlap, all are joined, and later stages wait
   for the barrier.
5. The first recorded fatal stage error cancels shared work, prevents later
   stages, and cannot be returned with success code.
6. Per-step execution uses the five phases in the reference order.
7. Completed validation mismatch traversal returns code `101` and
   `ValidationError`.
8. Debug, presentation, sorting, and per-validator payload rules absent from
   the skeleton are not introduced here.
