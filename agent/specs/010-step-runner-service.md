# `internal/execution` StepRunner

## Status and ownership

- Binding reference: `skeleton/internal/execution/steprun.go`
- Reference tests: `skeleton/internal/execution/steprun_test.go`
- Shared domain and exit codes: [`prd.md`](../prd.md)
- Reporter methods: [`09-reporter.md`](009-reporter-service.md)
- Status: skeleton-aligned specification

This specification is the single owner of preparation scope, per-step execution
order, directory-tree validation, and stage scheduling. Collaborator specs do
not restate the phase order.

## Public API

```go
var ErrInvalidDirectoryTree = errors.New("invalid directory tree")
var ErrExecutionCanceled = errors.New("execution canceled")

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

`Prepare` traverses the suite's directories and deep-copies each directory's
`ResolvedSteps` into `RuntimeSteps` so execution-time mutations cannot affect
the resolved steps. All mutable slices and maps are copied. Each copied
`Step.Definition` retains the original pointer as read-only provenance.

Variable loading and interpolation are not preparation phases; they run per
runtime step inside `Execute`. The skeleton does not define traversal order,
preparation transactionality, or cancellation behavior.

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

The first fatal directory error recorded for a stage retains its error and
supplied non-zero code and cancels the shared execution context. If that
directory supplied code `0`, StepRunner derives a code through `errs.Code` with
`ExitInternal` fallback and preserves the derived code, including `0`. Sibling
goroutines are still joined, and later stages do not start.

A directory may return `errs.ExitValidation` with a nil error after reporting
its validation failures. The scheduler retains the first non-zero status,
joins the stage, and continues through later stages. A later fatal error takes
precedence over an earlier validation status.

If the shared context is canceled between otherwise successful stages,
execution returns `ExitInternal` and a built error matching
`ErrExecutionCanceled` that preserves the context error.

No ordering between same-stage directory goroutines is promised.

## Directory and step processing

For each directory, StepRunner processes the runtime steps represented by
`Directory.RuntimeSteps`. The reference does not specify sorting beyond that
slice structure and does not define separate file or step goroutines.

For each executed step, the skeleton comment fixes this phase order:

1. `VariableProcessor.LoadVariables`
2. `VariableProcessor.InterpolateRequestBody`
3. `VariableProcessor.InterpolateResponseExpected`
4. `runner.Curl`
5. `Validator.ValidateTypes`
6. `Validator.ValidateExpected`
7. `VariableProcessor.CaptureResponseVariables`

The response body returned by `runner.Curl` becomes `Step.Response.Body` for
the validation and capture phases that follow it. Expected-response variables
are interpolated before the request is executed.

StepRunner sends detected validation failures to its Reporter. After traversal
finishes with at least one validation mismatch, `Execute` returns
`errs.ExitValidation, nil`. Validation failures therefore affect final process
status without becoming fatal diagnostics or canceling remaining work.

The skeleton does not define URL construction, HTTP-status treatment, the
mapping of individual validation errors to Reporter methods, success reporting,
debug selection/stopping, or precedence between multiple nonfatal failures.
Those details remain outside this spec.

## Boundaries

StepRunner coordinates its collaborators. It does not own variable syntax,
validation algorithms, command implementation, output layout, or contextual
error formatting. Those contracts remain with their owning specs.

## Acceptance criteria

1. Public names, signatures, constructor state, and static error text match the
   reference.
2. `Prepare` deep-copies `ResolvedSteps` into `RuntimeSteps`, including all
   mutable slices and maps, preserves the original `Step.Definition` pointers,
   does not mutate `ResolvedSteps`, and does not load or interpolate variables.
3. Every invalid tree shape covered by the reference returns configuration code
   and `ErrInvalidDirectoryTree` without panic.
4. Same-stage directories may overlap, all are joined, and later stages wait
   for the barrier.
5. The first recorded fatal stage error cancels shared work, prevents later
   stages, and returns the code derived by the reference implementation.
6. Per-step execution uses the seven phases in the reference order, assigns the
   Curl response body to `Step.Response.Body`, and interpolates expected values
   before Curl runs.
7. Completed validation mismatch traversal continues through remaining work
   and returns code `101` with a nil error.
8. Debug, presentation, sorting, and per-validator payload rules absent from
   the skeleton are not introduced here.
