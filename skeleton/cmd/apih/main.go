package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/divilla/apihydra/skeleton/internal/definition"
	"github.com/divilla/apihydra/skeleton/internal/domain"
	"github.com/divilla/apihydra/skeleton/internal/execution"
	"github.com/divilla/apihydra/skeleton/internal/reporting"
	"github.com/divilla/apihydra/skeleton/pkg/errs"

	"github.com/spf13/pflag"
)

// ErrInvalidPath classifies an invalid working-directory argument.
var ErrInvalidPath = errors.New("invalid path")

// ErrWorkingDirectory classifies failure to read the current working directory.
var ErrWorkingDirectory = errors.New("working directory error")

// ErrInvalidArguments classifies pflag parsing, positional-argument, and
// parallelism-range failures.
var ErrInvalidArguments = errors.New("invalid arguments")

// ErrUserCacheDirectory classifies failure to resolve or create the apih user
// cache directory.
var ErrUserCacheDirectory = errors.New("user cache directory error")

// ErrTempRunDirectory classifies failure to create the private directory for
// one application run.
var ErrTempRunDirectory = errors.New("temporary run directory error")

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	var help bytes.Buffer
	config, err := parseConfig(os.Args, &help)
	if errors.Is(err, pflag.ErrHelp) {
		if _, writeErr := os.Stdout.Write(help.Bytes()); writeErr != nil {
			log.Print(writeErr)
			os.Exit(errs.ExitInternal)
		}
		return
	}
	if err != nil {
		log.Print(err)
		os.Exit(errs.ExitConfiguration)
	}

	exitCode, err := run(
		context.Background(),
		config,
		reporting.NewReporter(os.Stdout, isTerminal(os.Stdout)),
	)
	if err != nil {
		log.Print(err)
	}
	os.Exit(exitCode)
}

// parseConfig uses native pflag parsing. It accepts attached, equals, repeated,
// and interspersed flag forms; the final repeated value wins, and -- terminates
// flag parsing. It accepts at most one positional directory. Help returns
// pflag.ErrHelp after pflag writes usage to output. All other failures return a
// configuration-coded ErrInvalidArguments.
func parseConfig(args []string, output io.Writer) (domain.Config, error) {
	config := domain.Config{Parallelism: 1}
	name := "apih"
	values := []string(nil)
	if len(args) > 0 {
		name = args[0]
		values = args[1:]
	}

	flags := pflag.NewFlagSet(name, pflag.ContinueOnError)
	flags.SetOutput(output)
	flags.IntVarP(&config.Parallelism, "parallelism", "p", 1, "execution parallelism mode: 0, 1, or 2")
	if err := flags.Parse(values); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return domain.Config{}, err
		}
		return domain.Config{}, errs.Build(errs.ExitConfiguration, ErrInvalidArguments, err)
	}
	if flags.NArg() > 1 {
		return domain.Config{}, errs.Build(errs.ExitConfiguration, ErrInvalidArguments, nil, "expected at most one directory")
	}
	if config.Parallelism < 0 || config.Parallelism > 2 {
		return domain.Config{}, errs.Build(errs.ExitConfiguration, ErrInvalidArguments, nil, "parallelism must be 0, 1, or 2")
	}
	if flags.NArg() == 1 {
		config.Directory = flags.Arg(0)
	}
	return config, nil
}

func run(ctx context.Context, config domain.Config, reporter *reporting.Reporter) (int, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrWorkingDirectory, err)
	}

	if config.Directory != "" {
		path := filepath.Join(workDir, config.Directory)
		info, err := os.Stat(path)
		if err != nil {
			return errs.ExitConfiguration, errs.Build(errs.ExitConfiguration, ErrInvalidPath, err, path)
		}
		if !info.IsDir() {
			return errs.ExitConfiguration, errs.Build(errs.ExitConfiguration, ErrInvalidPath, nil, path)
		}
		workDir = path
	}

	config.TempRunDir, err = createTempRunDirectory()
	if err != nil {
		return errs.ExitInternal, err
	}
	defer func() {
		_ = os.RemoveAll(config.TempRunDir)
	}()

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
	validator := execution.NewValidator(config)
	executor := execution.NewExecutor(binder, validator, reporter, config)
	exitCode, err := executor.ValidateDirectories(suite)
	if err != nil {
		return exitCode, err
	}
	executor.Prepare(suite)
	stagesPlan := executor.PlanStages(suite)

	return executor.Execute(ctx, stagesPlan)
}

// createTempRunDirectory creates one private run-* directory below
// os.UserCacheDir()/apih. The caller owns best-effort removal of the complete
// run directory. Cleanup failures are always suppressed.
func createTempRunDirectory() (string, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", errs.Build(errs.ExitInternal, ErrUserCacheDirectory, err)
	}
	cacheDir := filepath.Join(userCacheDir, "apih")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", errs.Build(errs.ExitInternal, ErrUserCacheDirectory, err, cacheDir)
	}
	if err := os.Chmod(cacheDir, 0o700); err != nil {
		return "", errs.Build(errs.ExitInternal, ErrUserCacheDirectory, err, cacheDir)
	}
	tempRunDir, err := os.MkdirTemp(cacheDir, "run-")
	if err != nil {
		return "", errs.Build(errs.ExitInternal, ErrTempRunDirectory, err, cacheDir)
	}
	return tempRunDir, nil
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
