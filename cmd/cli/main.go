package main

import (
    "apih/internal/definition"
    "apih/internal/domain"
    "apih/internal/execution"
    "apih/internal/reporting"
    "apih/pkg/errs"
    "context"
    "errors"
    "log"
    "os"
    "path/filepath"
)

// ErrInvalidPath classifies an invalid working-directory argument.
var ErrInvalidPath = errors.New("invalid path")

// ErrWorkingDirectory classifies failure to read the current working directory.
var ErrWorkingDirectory = errors.New("working directory error")

func main() {
    log.SetFlags(0)
    log.SetOutput(os.Stderr)

    exitCode, err := run(context.Background(), os.Args, reporting.NewReporter(os.Stdout))
    if err != nil {
        log.Print(err)
    }
    os.Exit(exitCode)
}

func run(ctx context.Context, args []string, reporter *reporting.Reporter) (int, error) {
    workDir, err := os.Getwd()
    if err != nil {
        return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrWorkingDirectory, err)
    }

    if len(args) > 1 {
        subdir := args[1]
        path := filepath.Join(workDir, subdir)
        info, err := os.Stat(path)
        if err != nil {
            return errs.ExitConfiguration, errs.Build(errs.ExitConfiguration, ErrInvalidPath, err, path)
        }
        if !info.IsDir() {
            return errs.ExitConfiguration, errs.Build(errs.ExitConfiguration, ErrInvalidPath, nil, path)
        }
        workDir = path
    }
    if err := reporter.WorkingDirectory(workDir); err != nil {
        return errs.ExitInternal, err
    }

    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    suite := &domain.Suite{WorkDir: workDir}

    loader := definition.NewLoader()
    if err = loader.LoadDirectoryStructure(ctx, suite); err != nil {
        return errs.ExitConfiguration, err
    }
    if err = loader.LoadDirectoryFiles(ctx, suite); err != nil {
        return errs.ExitConfiguration, err
    }
    if err = loader.DecodeBaseDefinitions(ctx, suite); err != nil {
        return errs.ExitConfiguration, err
    }

    decoder := definition.NewDecoder()
    if err = decoder.DecodeFiles(ctx, suite); err != nil {
        return errs.ExitConfiguration, err
    }
    if err = decoder.ValidateDefaultsDefinitions(ctx, suite); err != nil {
        return errs.ExitConfiguration, err
    }
    if err = decoder.ValidateStepsDefinitions(ctx, suite); err != nil {
        return errs.ExitConfiguration, err
    }

    resolver := definition.NewResolver()
    if err = resolver.ResolveDefaults(ctx, suite); err != nil {
        return errs.ExitConfiguration, err
    }
    if err = resolver.ResolveSteps(ctx, suite); err != nil {
        return errs.ExitConfiguration, err
    }

    kvs := execution.NewKeyValueStore()
    binder := execution.NewBinder(kvs)
    validator := execution.NewValidator()
    executor := execution.NewExecutor(binder, validator, reporter)
    exitCode, err := executor.ValidateDirectories(suite)
    if err != nil {
        return exitCode, err
    }
    executor.Prepare(suite)
    stagesPlan := executor.PlanStages(suite)

    return executor.Execute(ctx, stagesPlan)
}
