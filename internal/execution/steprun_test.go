package execution

import (
	"apih/internal/domain"
	"apih/internal/reporter"
	"apih/pkg/errs"
	"context"
	"errors"
	"io"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestStepRunnerPublicContract(t *testing.T) {
	var constructor func(*VariableProcessor, *Validator, *reporter.Reporter) *StepRunner = NewStepRunner
	var prepare func(*StepRunner, context.Context, *domain.Suite) error = (*StepRunner).Prepare
	var execute func(*StepRunner, context.Context, *domain.Suite) (int, error) = (*StepRunner).Execute

	variableProcessor := NewVariableProcessor()
	validator := &Validator{}
	report := reporter.NewReporter(io.Discard)
	stepRunner := NewStepRunner(variableProcessor, validator, report)
	var process directoryProcessor = stepRunner.processDir
	_, _, _, _ = constructor, prepare, execute, process
	if stepRunner.varProc != variableProcessor || stepRunner.val != validator || stepRunner.report != report {
		t.Fatal("NewStepRunner() did not retain its collaborators")
	}
	if got, want := ErrInvalidDirectoryTree.Error(), "invalid directory tree"; got != want {
		t.Fatalf("ErrInvalidDirectoryTree = %q, want %q", got, want)
	}
	if got, want := ErrExecutionCanceled.Error(), "execution canceled"; got != want {
		t.Fatalf("ErrExecutionCanceled = %q, want %q", got, want)
	}
}

func TestPrepareUsesLoadThenParseRequestBodyForEveryResolvedStep(t *testing.T) {
	root := &domain.Directory{
		Stage:         0,
		Path:          "/",
		ResolvedSteps: [][]domain.Step{{{Index: 0}, {Index: 1}}, {{Index: 2}}},
	}
	child := &domain.Directory{
		Stage:         1,
		Path:          "/child",
		Parent:        root,
		ResolvedSteps: [][]domain.Step{{{Index: 3}}},
	}
	root.Children = []*domain.Directory{child}

	var phases []string
	load := func(_ context.Context, step *domain.Step) (int, error) {
		phases = append(phases, "load:"+strconv.Itoa(step.Index))
		return 0, nil
	}
	parse := func(_ context.Context, step *domain.Step) (int, error) {
		phases = append(phases, "parse:"+strconv.Itoa(step.Index))
		return 0, nil
	}

	dirs, err := collectDirs(&domain.Suite{Root: root})
	if err != nil {
		t.Fatalf("collectDirs() error = %v", err)
	}
	if err := prepareDirs(context.Background(), dirs, load, parse); err != nil {
		t.Fatalf("prepareDirs() error = %v", err)
	}
	want := []string{
		"load:0", "parse:0",
		"load:1", "parse:1",
		"load:2", "parse:2",
		"load:3", "parse:3",
	}
	if !reflect.DeepEqual(phases, want) {
		t.Fatalf("preparation phases = %v, want %v", phases, want)
	}

	stepRunner := NewStepRunner(NewVariableProcessor(), &Validator{}, reporter.NewReporter(io.Discard))
	if err := stepRunner.Prepare(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
}

func TestPrepareStopsOnPhaseErrorAndRejectsInvalidTree(t *testing.T) {
	wantLoadErr := errors.New("load failed")
	wantParseErr := errors.New("parse failed")
	dirs := [][]*domain.Directory{{{
		ResolvedSteps: [][]domain.Step{{{Index: 1}}},
	}}}

	tests := map[string]struct {
		load  preparePhase
		parse preparePhase
		want  error
	}{
		"load": {
			load: func(context.Context, *domain.Step) (int, error) { return 17, wantLoadErr },
			parse: func(context.Context, *domain.Step) (int, error) {
				t.Fatal("parse ran after load failure")
				return 0, nil
			},
			want: wantLoadErr,
		},
		"parse": {
			load:  func(context.Context, *domain.Step) (int, error) { return 0, nil },
			parse: func(context.Context, *domain.Step) (int, error) { return 18, wantParseErr },
			want:  wantParseErr,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := prepareDirs(context.Background(), dirs, test.load, test.parse)
			if !errors.Is(err, test.want) {
				t.Fatalf("prepareDirs() error = %v, want %v", err, test.want)
			}
		})
	}

	stepRunner := NewStepRunner(NewVariableProcessor(), &Validator{}, reporter.NewReporter(io.Discard))
	err := stepRunner.Prepare(context.Background(), &domain.Suite{})
	if !errors.Is(err, ErrInvalidDirectoryTree) {
		t.Fatalf("Prepare() error = %v, want ErrInvalidDirectoryTree", err)
	}
}

func TestCollectDirsSupportsArbitraryValidDepth(t *testing.T) {
	root := &domain.Directory{Stage: 0, Path: "/"}
	parent := root
	for stage := 1; stage <= 255; stage++ {
		child := &domain.Directory{
			Stage:  stage,
			Path:   "/" + strconv.Itoa(stage),
			Parent: parent,
		}
		parent.Children = []*domain.Directory{child}
		parent = child
	}

	dirs, err := collectDirs(&domain.Suite{Root: root})
	if err != nil {
		t.Fatalf("collectDirs() error = %v", err)
	}
	if got, want := len(dirs), 256; got != want {
		t.Fatalf("len(dirs) = %d, want %d", got, want)
	}
	if got := dirs[255][0]; got != parent {
		t.Fatalf("dirs[255][0] = %p, want %p", got, parent)
	}
}

func TestCollectDirsRejectsInvalidTrees(t *testing.T) {
	validRoot := func() *domain.Directory {
		return &domain.Directory{Stage: 0, Path: "/"}
	}

	tests := map[string]func() *domain.Suite{
		"nil suite": func() *domain.Suite {
			return nil
		},
		"nil root": func() *domain.Suite {
			return &domain.Suite{}
		},
		"invalid root parent": func() *domain.Suite {
			return &domain.Suite{
				Root: &domain.Directory{
					Stage:  0,
					Path:   "/",
					Parent: &domain.Directory{},
				},
			}
		},
		"invalid root stage": func() *domain.Suite {
			return &domain.Suite{Root: &domain.Directory{Stage: -1, Path: "/"}}
		},
		"nil child": func() *domain.Suite {
			root := validRoot()
			root.Children = []*domain.Directory{nil}
			return &domain.Suite{Root: root}
		},
		"invalid parent": func() *domain.Suite {
			root := validRoot()
			root.Children = []*domain.Directory{{Stage: 1, Path: "/child"}}
			return &domain.Suite{Root: root}
		},
		"invalid child stage": func() *domain.Suite {
			root := validRoot()
			root.Children = []*domain.Directory{{Stage: 2, Path: "/child", Parent: root}}
			return &domain.Suite{Root: root}
		},
		"repeated directory": func() *domain.Suite {
			root := validRoot()
			child := &domain.Directory{Stage: 1, Path: "/child", Parent: root}
			root.Children = []*domain.Directory{child, child}
			return &domain.Suite{Root: root}
		},
		"cycle": func() *domain.Suite {
			root := validRoot()
			child := &domain.Directory{Stage: 1, Path: "/child", Parent: root}
			root.Children = []*domain.Directory{child}
			child.Children = []*domain.Directory{root}
			return &domain.Suite{Root: root}
		},
	}

	for name, suite := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := collectDirs(suite())
			if !errors.Is(err, ErrInvalidDirectoryTree) {
				t.Fatalf("collectDirs() error = %v, want ErrInvalidDirectoryTree", err)
			}
			if got := errs.Code(err, errs.ExitInternal); got != errs.ExitConfiguration {
				t.Fatalf("collectDirs() code = %d, want %d", got, errs.ExitConfiguration)
			}
		})
	}
}

func TestExecuteStagesOverlapsDirectoriesAndUsesStageBarrier(t *testing.T) {
	root := &domain.Directory{Stage: 0, Path: "/"}
	first := &domain.Directory{Stage: 1, Path: "/first"}
	second := &domain.Directory{Stage: 1, Path: "/second"}
	later := &domain.Directory{Stage: 2, Path: "/later"}
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var laterStarted atomic.Bool

	process := func(_ context.Context, dir *domain.Directory) (int, error) {
		switch dir {
		case first, second:
			started <- struct{}{}
			<-release
		case later:
			laterStarted.Store(true)
		}
		return errs.ExitSuccess, nil
	}

	done := make(chan struct{})
	var exitCode int
	var executionErr error
	go func() {
		exitCode, executionErr = executeStages(
			context.Background(),
			[][]*domain.Directory{{root}, {first, second}, {later}},
			process,
		)
		close(done)
	}()

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("same-stage directories did not overlap")
		}
	}
	if laterStarted.Load() {
		t.Fatal("later stage started before active-stage directories completed")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("executeStages() did not finish")
	}
	if exitCode != errs.ExitSuccess || executionErr != nil {
		t.Fatalf("executeStages() = (%d, %v), want success", exitCode, executionErr)
	}
	if !laterStarted.Load() {
		t.Fatal("later stage did not start after the barrier")
	}
}

func TestExecuteStagesCancelsAndJoinsActiveStageOnError(t *testing.T) {
	wantErr := errors.New("fatal execution error")
	wantExitCode := 23
	failing := &domain.Directory{Stage: 0, Path: "/failing"}
	sibling := &domain.Directory{Stage: 0, Path: "/sibling"}
	later := &domain.Directory{Stage: 1, Path: "/later"}

	siblingReturned := make(chan struct{})
	var laterStarted atomic.Bool
	process := func(ctx context.Context, dir *domain.Directory) (int, error) {
		switch dir {
		case failing:
			return wantExitCode, wantErr
		case sibling:
			<-ctx.Done()
			close(siblingReturned)
			return errs.ExitInternal, ctx.Err()
		case later:
			laterStarted.Store(true)
		}
		return errs.ExitSuccess, nil
	}

	exitCode, err := executeStages(
		context.Background(),
		[][]*domain.Directory{{failing, sibling}, {later}},
		process,
	)
	if exitCode != wantExitCode {
		t.Fatalf("executeStages() exit code = %d, want %d", exitCode, wantExitCode)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("executeStages() error = %v, want %v", err, wantErr)
	}
	select {
	case <-siblingReturned:
	default:
		t.Fatal("executeStages() returned before the active-stage sibling joined")
	}
	if laterStarted.Load() {
		t.Fatal("executeStages() started a later stage after a fatal error")
	}
}

func TestExecuteStagesHandlesCancellationAndErrorCodes(t *testing.T) {
	t.Run("canceled between stages", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		root := &domain.Directory{Stage: 0}
		later := &domain.Directory{Stage: 1}
		var laterStarted atomic.Bool
		exitCode, err := executeStages(ctx, [][]*domain.Directory{{root}, {later}}, func(_ context.Context, dir *domain.Directory) (int, error) {
			if dir == root {
				cancel()
			} else {
				laterStarted.Store(true)
			}
			return errs.ExitSuccess, nil
		})
		if exitCode != errs.ExitInternal || !errors.Is(err, ErrExecutionCanceled) || !errors.Is(err, context.Canceled) {
			t.Fatalf("executeStages() = (%d, %v), want internal execution-canceled error", exitCode, err)
		}
		if laterStarted.Load() {
			t.Fatal("later stage started after cancellation")
		}
	})

	t.Run("uncoded error cannot have success code", func(t *testing.T) {
		wantErr := errors.New("uncoded error")
		exitCode, err := executeStages(
			context.Background(),
			[][]*domain.Directory{{{Stage: 0}}},
			func(context.Context, *domain.Directory) (int, error) {
				return errs.ExitSuccess, wantErr
			},
		)
		if exitCode != errs.ExitInternal || !errors.Is(err, wantErr) {
			t.Fatalf("executeStages() = (%d, %v), want (%d, %v)", exitCode, err, errs.ExitInternal, wantErr)
		}
	})

	t.Run("success-coded error preserves success code", func(t *testing.T) {
		wantErr := errors.New("success-coded error")
		exitCode, err := executeStages(
			context.Background(),
			[][]*domain.Directory{{{Stage: 0}}},
			func(context.Context, *domain.Directory) (int, error) {
				return errs.ExitSuccess, errs.WithExitCode(errs.ExitSuccess, wantErr)
			},
		)
		if exitCode != errs.ExitSuccess || !errors.Is(err, wantErr) {
			t.Fatalf("executeStages() = (%d, %v), want (%d, %v)", exitCode, err, errs.ExitSuccess, wantErr)
		}
	})

	t.Run("validation stops later stages and cancels active-stage siblings", func(t *testing.T) {
		mismatch := &domain.Directory{Stage: 0}
		sibling := &domain.Directory{Stage: 0}
		later := &domain.Directory{Stage: 1}
		var laterProcessed atomic.Bool
		siblingCanceled := make(chan struct{})
		exitCode, err := executeStages(
			context.Background(),
			[][]*domain.Directory{{mismatch, sibling}, {later}},
			func(ctx context.Context, dir *domain.Directory) (int, error) {
				switch dir {
				case mismatch:
					return errs.ExitValidation, errs.Build(errs.ExitValidation, ErrValidation, nil)
				case sibling:
					<-ctx.Done()
					close(siblingCanceled)
					return errs.ExitInternal, ctx.Err()
				case later:
					laterProcessed.Store(true)
				}
				return errs.ExitSuccess, nil
			},
		)
		if exitCode != errs.ExitValidation || !errors.Is(err, ErrValidation) {
			t.Fatalf("executeStages() = (%d, %v), want validation result", exitCode, err)
		}
		select {
		case <-siblingCanceled:
		default:
			t.Fatal("executeStages() returned before the canceled active-stage sibling joined")
		}
		if laterProcessed.Load() {
			t.Fatal("later stage was processed after validation mismatch")
		}
	})
}

func TestExecuteValidatesTreeAndHandlesEmptySuite(t *testing.T) {
	stepRunner := NewStepRunner(nil, nil, nil)

	exitCode, err := stepRunner.Execute(context.Background(), &domain.Suite{})
	if exitCode != errs.ExitConfiguration || !errors.Is(err, ErrInvalidDirectoryTree) {
		t.Fatalf("Execute(invalid) = (%d, %v), want configuration tree error", exitCode, err)
	}

	exitCode, err = stepRunner.Execute(context.Background(), &domain.Suite{Root: &domain.Directory{Stage: 0}})
	if exitCode != errs.ExitSuccess || err != nil {
		t.Fatalf("Execute(empty) = (%d, %v), want success", exitCode, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	exitCode, err = stepRunner.Execute(ctx, &domain.Suite{Root: &domain.Directory{Stage: 0}})
	if exitCode != errs.ExitInternal || !errors.Is(err, ErrExecutionCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute(canceled) = (%d, %v), want canceled internal error", exitCode, err)
	}

	exitCode, err = stepRunner.processDir(ctx, &domain.Directory{})
	if exitCode != errs.ExitInternal || !errors.Is(err, ErrExecutionCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("processDir(canceled) = (%d, %v), want canceled internal error", exitCode, err)
	}
}

func TestProcessStepUsesFiveExecutionPhasesInOrder(t *testing.T) {
	step := &domain.Step{}
	step.Request.Method = "POST"
	step.Request.BaseURL = "https://example.test"
	step.Request.BasePath = "/api"
	step.Request.Path = "/items"
	step.Request.Headers = map[string]string{"X-Test": "yes"}
	step.Request.Timeout = 9
	step.Request.Retries = 2
	step.Request.Query = "page=1"
	step.Request.Body = `{"request":true}`
	typeFailure := errors.New("type mismatch")
	expectedFailure := errors.New("expected mismatch")
	var phases []string

	operations := noOpStepOperations()
	operations.curl = func(_ context.Context, method, url string, headers map[string]string, timeout, retries int, query, body string) (string, int, error) {
		phases = append(phases, "curl")
		if method != "POST" || url != "https://example.test/api/items" || headers["X-Test"] != "yes" || timeout != 9 || retries != 2 || query != "page=1" || body != `{"request":true}` {
			t.Fatalf("curl arguments were not derived from the Step: method=%q url=%q headers=%v timeout=%d retries=%d query=%q body=%q", method, url, headers, timeout, retries, query, body)
		}
		return `{"response":true}`, 201, nil
	}
	operations.parseResponseExpected = func(_ context.Context, got *domain.Step) (int, error) {
		phases = append(phases, "parse expected")
		if got.Response.Body != `{"response":true}` {
			t.Fatalf("response body = %q, want curl response", got.Response.Body)
		}
		return 0, nil
	}
	operations.validateTypes = func(context.Context, *domain.Step) []error {
		phases = append(phases, "validate types")
		return []error{nil, typeFailure}
	}
	operations.reportTypes = func(_ context.Context, got *domain.Step, failure error) error {
		phases = append(phases, "report types")
		if got != step || failure != typeFailure {
			t.Fatal("type reporter received the wrong payload")
		}
		return nil
	}
	operations.validateExpected = func(context.Context, *domain.Step) error {
		phases = append(phases, "validate expected")
		return expectedFailure
	}
	operations.reportExpected = func(_ context.Context, got *domain.Step, failure error) error {
		phases = append(phases, "report expected")
		if got != step || failure != expectedFailure {
			t.Fatal("expected reporter received the wrong payload")
		}
		return nil
	}
	operations.capture = func(context.Context, *domain.Step) (int, error) {
		phases = append(phases, "capture")
		return 0, nil
	}

	failed, exitCode, err := processStep(context.Background(), step, operations)
	if !failed || exitCode != errs.ExitSuccess || err != nil {
		t.Fatalf("processStep() = (%t, %d, %v), want validation failure with successful phases", failed, exitCode, err)
	}
	want := []string{
		"curl",
		"parse expected",
		"validate types",
		"report types",
		"validate expected",
		"report expected",
		"capture",
	}
	if !reflect.DeepEqual(phases, want) {
		t.Fatalf("execution phases = %v, want %v", phases, want)
	}
}

func TestProcessStepPropagatesFatalPhaseErrors(t *testing.T) {
	tests := map[string]struct {
		configure func(*stepOperations, error)
		wantCode  int
	}{
		"curl": {
			configure: func(operations *stepOperations, wantErr error) {
				operations.curl = func(context.Context, string, string, map[string]string, int, int, string, string) (string, int, error) {
					return "", 17, wantErr
				}
			},
			wantCode: 17,
		},
		"parse expected": {
			configure: func(operations *stepOperations, wantErr error) {
				operations.parseResponseExpected = func(context.Context, *domain.Step) (int, error) { return 18, wantErr }
			},
			wantCode: 18,
		},
		"type reporter": {
			configure: func(operations *stepOperations, wantErr error) {
				operations.validateTypes = func(context.Context, *domain.Step) []error { return []error{errors.New("mismatch")} }
				operations.reportTypes = func(context.Context, *domain.Step, error) error {
					return errs.WithExitCode(19, wantErr)
				}
			},
			wantCode: 19,
		},
		"expected reporter": {
			configure: func(operations *stepOperations, wantErr error) {
				operations.validateExpected = func(context.Context, *domain.Step) error { return errors.New("mismatch") }
				operations.reportExpected = func(context.Context, *domain.Step, error) error { return wantErr }
			},
			wantCode: errs.ExitInternal,
		},
		"capture": {
			configure: func(operations *stepOperations, wantErr error) {
				operations.capture = func(context.Context, *domain.Step) (int, error) { return 20, wantErr }
			},
			wantCode: 20,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			wantErr := errors.New(name + " failed")
			operations := noOpStepOperations()
			test.configure(&operations, wantErr)
			_, exitCode, err := processStep(context.Background(), &domain.Step{}, operations)
			if exitCode != test.wantCode || !errors.Is(err, wantErr) {
				t.Fatalf("processStep() = (_, %d, %v), want (%d, %v)", exitCode, err, test.wantCode, wantErr)
			}
		})
	}
}

func TestProcessResolvedStepsReturnsValidationAfterDirectoryTraversal(t *testing.T) {
	dir := &domain.Directory{ResolvedSteps: [][]domain.Step{{{}, {}}, {{}}}}
	operations := noOpStepOperations()
	var calls atomic.Int32
	operations.validateExpected = func(context.Context, *domain.Step) error {
		if calls.Add(1) == 1 {
			return errors.New("mismatch")
		}
		return nil
	}

	exitCode, err := processResolvedSteps(context.Background(), dir, operations)
	if exitCode != errs.ExitValidation || !errors.Is(err, ErrValidation) {
		t.Fatalf("processResolvedSteps() = (%d, %v), want deferred validation result", exitCode, err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("validated steps = %d, want 3", got)
	}

	exitCode, err = processResolvedSteps(context.Background(), &domain.Directory{}, noOpStepOperations())
	if exitCode != errs.ExitSuccess || err != nil {
		t.Fatalf("processResolvedSteps(empty) = (%d, %v), want success", exitCode, err)
	}

	wantErr := errors.New("fatal step failure")
	operations = noOpStepOperations()
	operations.curl = func(context.Context, string, string, map[string]string, int, int, string, string) (string, int, error) {
		return "", 31, wantErr
	}
	exitCode, err = processResolvedSteps(
		context.Background(),
		&domain.Directory{ResolvedSteps: [][]domain.Step{{{}}}},
		operations,
	)
	if exitCode != 31 || !errors.Is(err, wantErr) {
		t.Fatalf("processResolvedSteps(fatal) = (%d, %v), want (31, %v)", exitCode, err, wantErr)
	}
}

func noOpStepOperations() stepOperations {
	return stepOperations{
		curl: func(context.Context, string, string, map[string]string, int, int, string, string) (string, int, error) {
			return "response", 200, nil
		},
		parseResponseExpected: func(context.Context, *domain.Step) (int, error) { return 0, nil },
		validateTypes:         func(context.Context, *domain.Step) []error { return nil },
		validateExpected:      func(context.Context, *domain.Step) error { return nil },
		capture:               func(context.Context, *domain.Step) (int, error) { return 0, nil },
		reportTypes:           func(context.Context, *domain.Step, error) error { return nil },
		reportExpected:        func(context.Context, *domain.Step, error) error { return nil },
	}
}
