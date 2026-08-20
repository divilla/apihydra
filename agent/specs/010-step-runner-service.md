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
    report *reporting.Reporter,
) *StepRunner

func (s *StepRunner) ValidateDirectories(suite *domain.Suite) (int, error)
func (s *StepRunner) PlanStages(suite *domain.Suite) [][]*domain.Directory
func (s *StepRunner) Prepare(suite *domain.Suite)
func (s *StepRunner) Execute(ctx context.Context, stages [][]*domain.Directory) (int, error)
```

`NewStepRunner` retains the three supplied collaborators without performing
work.

## Preparation

`Prepare` traverses the suite's directories and deep-copies each directory's
`ResolvedSteps` into `RuntimeSteps` so execution-time mutations cannot affect
the resolved steps. All mutable slices and maps are copied. Each copied
`Step.Definition` retains the original pointer as read-only provenance.

Variable loading and interpolation are not preparation phases; they run per
runtime step inside `Execute`. The skeleton does not define traversal order or
preparation transactionality. `Prepare` does not accept a context or return an
error.

## Directory-tree validation

Before planning or execution, the CLI calls `ValidateDirectories`. The
implemented reference rejects, with configuration code and a built error
matching `ErrInvalidDirectoryTree`:

- a nil Suite or nil root;
- a root whose stage is not `0`;
- a nil child;
- a repeated directory pointer, including a cycle;
- a child whose `Parent` is not the containing directory;
- a child whose stage is not its parent's stage plus one.

Validation does not produce the execution plan. After successful validation,
`PlanStages` finds the maximum stage reachable from `suite.Root`, allocates one
group per stage, and preserves each directory pointer in the group indexed by
its stage.

## Stage scheduling

`Execute` receives the plan produced by `PlanStages`. Plan groups run in slice
order. For one active group, StepRunner starts one goroutine per directory and
waits for every started goroutine before returning or advancing to the next
group.

One mutex-protected process result ignores successful directory results and
retains a validation result provisionally. The first fatal directory result
replaces provisional validation and is retained with its associated code and
error; later results do not replace it. A directory error cancels the shared
execution context. Sibling goroutines are still joined, and later stages do not
start.

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
6. `Validator.ValidateStatus`
7. `Validator.ValidateBody`
8. `VariableProcessor.CaptureResponseVariables`

The response status and body returned by `runner.Curl` become
`Step.Response.ActualStatus` and `Step.Response.ActualBody` for the validation
and capture phases that follow. Expected-response variables are interpolated
before the request is executed. The PRD owns the `ExpectedStatus == 0`
wildcard rule; Validator owns the comparison.

StepRunner sends each type or status validation failure and each non-empty body
diff to its corresponding Reporter method. After traversal finishes with at
least one validation mismatch, `Execute` returns
`errs.ExitValidation, nil`. Validation failures therefore affect final process
status without becoming fatal diagnostics or canceling remaining work.

The skeleton does not define URL construction, status rules beyond the PRD's
expected/actual comparison, success reporting, debug selection/stopping, or
precedence between multiple nonfatal failures. Those details remain outside
this spec.

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
3. `ValidateDirectories` returns configuration code and
   `ErrInvalidDirectoryTree` without panic for every invalid tree shape covered
   by the reference.
4. `PlanStages` groups a successfully validated tree by stage, same-stage
   directories may overlap during `Execute`, all are joined, and later stages
   wait for the barrier.
5. A fatal stage error cancels shared work and prevents later stages. The first
   fatal result replaces provisional validation and retains its associated code
   and error without being replaced by later results.
6. Per-step execution uses the eight phases in the reference order, assigns the
   Curl response status and body to `Step.Response.ActualStatus` and
   `Step.Response.ActualBody`, and interpolates expected values before Curl
   runs.
7. Completed validation mismatch traversal continues through remaining work
   and returns code `101` with a nil error.
8. Debug, presentation, sorting, and per-validator payload rules absent from
   the skeleton are not introduced here.
