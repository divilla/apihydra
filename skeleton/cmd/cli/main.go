package main

import (
	"apih/skeleton/internal/definition"
	"apih/skeleton/internal/domain"
	"apih/skeleton/internal/reporter"
	"apih/skeleton/pkg/errs"
	"context"
	"errors"
	"os"
	"path/filepath"
)

var InvalidPathError = errors.New("invalid path")
var WorkingDirectoryError = errors.New("working directory error")

func main() {
	outputReport := reporter.NewReporter(os.Stdout)
	errorReport := reporter.NewReporter(os.Stderr)
	exitCode, err := run(context.Background(), os.Args, outputReport)
	if err != nil {
		_ = errorReport.Error(err)
	}
	os.Exit(exitCode)
}

func run(ctx context.Context, args []string, report *reporter.Reporter) (int, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return errs.ExitInternal, errs.Build(errs.ExitInternal, WorkingDirectoryError, err)
	}

	if len(args) > 1 {
		subdir := args[1]
		path := filepath.Join(workDir, subdir)
		info, err := os.Stat(path)
		if err != nil {
			return errs.ExitConfiguration, errs.Build(errs.ExitConfiguration, InvalidPathError, err, path)
		}
		if !info.IsDir() {
			return errs.ExitConfiguration, errs.Build(errs.ExitConfiguration, InvalidPathError, nil, path)
		}
		workDir = path
	}
	if err := report.WorkingDirectory(workDir); err != nil {
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

	return errs.ExitSuccess, nil
}
