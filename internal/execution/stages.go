package execution

import (
	"context"
	"errors"
	"sync"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/internal/reporting"
	"github.com/divilla/apihydra/pkg/errs"
)

type resultPublisher func(int, error)

type directoryProcessor func(context.Context, resultPublisher, *domain.Directory) (int, error)

type fileProcessor func(context.Context, *domain.Directory, int) (int, error)

type stagePreparer func([]*domain.Directory) error

type processResult struct {
	mu   sync.Mutex
	code int
	err  error
}

func (r *processResult) setResult(code int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if errors.Is(err, errDebugStop) {
		r.code = 0
		r.err = err
		return
	}
	if errors.Is(r.err, errDebugStop) {
		return
	}
	if code == 0 {
		return
	}
	if r.code == 0 {
		r.code = code
		r.err = err
	}
	if r.code == errs.ExitValidation && r.err == nil && err != nil {
		r.code = code
		r.err = err
	}
}

func executeStages(
	ctx context.Context,
	dirs [][]*domain.Directory,
	parallelism int,
	report *reporting.Reporter,
	process directoryProcessor,
) (int, error) {
	return executeStagesPrepared(ctx, dirs, parallelism, report, nil, process)
}

func executeStagesPrepared(
	ctx context.Context,
	dirs [][]*domain.Directory,
	parallelism int,
	report *reporting.Reporter,
	prepare stagePreparer,
	process directoryProcessor,
) (int, error) {
	if parallelism < 0 || parallelism > 2 {
		return errs.ExitConfiguration, errs.Build(errs.ExitConfiguration, ErrInvalidParallelism, nil, parallelism)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var firstStatusCode int

	for _, stage := range dirs {
		if err := ctx.Err(); err != nil {
			return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
		}
		if prepare != nil {
			if err := prepare(stage); err != nil {
				return errs.Code(err, errs.ExitInternal), err
			}
		}
		if report != nil {
			if err := report.BeginStage(ctx, stage); err != nil {
				return errs.Code(err, errs.ExitInternal), err
			}
		}

		exitCode, err := executeStage(ctx, cancel, stage, parallelism > 0, process)
		var endErr error
		if report != nil {
			endErr = report.EndStage(context.WithoutCancel(ctx))
		}
		if errors.Is(err, errDebugStop) {
			if endErr != nil {
				return errs.Code(endErr, errs.ExitInternal), endErr
			}
			return 0, nil
		}
		if err != nil {
			return exitCode, err
		}
		if endErr != nil {
			return errs.Code(endErr, errs.ExitInternal), endErr
		}
		if firstStatusCode == 0 && exitCode != 0 {
			firstStatusCode = exitCode
		}
		if err := ctx.Err(); err != nil {
			return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
		}
	}

	return firstStatusCode, nil
}

func executeStage(
	ctx context.Context,
	cancel context.CancelFunc,
	dirs []*domain.Directory,
	parallel bool,
	process directoryProcessor,
) (int, error) {
	var result processResult
	publish := func(code int, err error) {
		result.setResult(code, err)
		if err != nil {
			cancel()
		}
	}
	if !parallel {
		for _, dir := range dirs {
			exitCode, err := process(ctx, publish, dir)
			publish(exitCode, err)
			if err != nil {
				break
			}
		}
		return result.code, result.err
	}

	var wg sync.WaitGroup
	for _, dir := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exitCode, err := process(ctx, publish, dir)
			publish(exitCode, err)
		}()
	}

	wg.Wait()
	return result.code, result.err
}

func executeFiles(
	ctx context.Context,
	cancel context.CancelFunc,
	dir *domain.Directory,
	parallel bool,
	process fileProcessor,
) (int, error) {
	var result processResult
	if !parallel {
		for fileIndex := range dir.RuntimeSteps {
			exitCode, err := process(ctx, dir, fileIndex)
			result.setResult(exitCode, err)
			if err != nil {
				cancel()
				break
			}
		}
		return result.code, result.err
	}

	var wg sync.WaitGroup
	for fileIndex := range dir.RuntimeSteps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exitCode, err := process(ctx, dir, fileIndex)
			result.setResult(exitCode, err)
			if err != nil {
				cancel()
			}
		}()
	}
	wg.Wait()
	return result.code, result.err
}

func executeDirectoryFiles(
	ctx context.Context,
	publish resultPublisher,
	dir *domain.Directory,
	parallel bool,
	process fileProcessor,
) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return executeFiles(ctx, cancel, dir, parallel, func(ctx context.Context, dir *domain.Directory, fileIndex int) (int, error) {
		exitCode, err := process(ctx, dir, fileIndex)
		if err != nil && publish != nil {
			publish(exitCode, err)
		}
		return exitCode, err
	})
}
