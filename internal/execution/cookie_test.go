package execution

import (
	"apih/internal/domain"
	"apih/internal/reporting"
	"apih/pkg/errs"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCookieJarsModeZeroUsesOneRunJar(t *testing.T) {
	runDir := t.TempDir()
	cookies := newCookieJars(domain.Config{Parallelism: 0, TempRunDir: runDir})
	root := &domain.Directory{Path: "/", RuntimeSteps: make([][]domain.Step, 2)}
	child := &domain.Directory{Path: "/child", Parent: root, RuntimeSteps: make([][]domain.Step, 1)}

	if err := cookies.prepareDirectory(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cookies.runJar, []byte("root-state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cookies.prepareDirectory(child); err != nil {
		t.Fatal(err)
	}

	want := cookies.runJar
	for _, request := range []struct {
		dir       *domain.Directory
		fileIndex int
	}{{root, 0}, {root, 1}, {child, 0}} {
		got := cookies.jarFor(request.dir, request.fileIndex)
		if got != want {
			t.Fatalf("jarFor(%s, %d) = %q, want %q", request.dir.Path, request.fileIndex, got, want)
		}
	}
	assertCookieJarFiles(t, runDir, []string{want})
	assertFileContentsEqual(t, want, "root-state")
}

func TestCookieJarsModeOneCopiesParentStateIntoIndependentDirectoryJars(t *testing.T) {
	runDir := t.TempDir()
	cookies := newCookieJars(domain.Config{Parallelism: 1, TempRunDir: runDir})
	root := &domain.Directory{Path: "/"}
	left := &domain.Directory{Path: "/left", Parent: root}
	right := &domain.Directory{Path: "/right", Parent: root}
	grandchild := &domain.Directory{Path: "/left/grandchild", Parent: left}

	if err := cookies.prepareDirectory(root); err != nil {
		t.Fatal(err)
	}
	rootJar := cookies.jarFor(root, 0)
	if err := os.WriteFile(rootJar, []byte("parent-state"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []*domain.Directory{left, right} {
		if err := cookies.prepareDirectory(dir); err != nil {
			t.Fatal(err)
		}
	}
	leftJar := cookies.jarFor(left, 0)
	rightJar := cookies.jarFor(right, 0)
	if rootJar == leftJar || rootJar == rightJar || leftJar == rightJar {
		t.Fatalf("directory jars are not independent: %q %q %q", rootJar, leftJar, rightJar)
	}
	assertFileContentsEqual(t, leftJar, "parent-state")
	assertFileContentsEqual(t, rightJar, "parent-state")
	if err := os.WriteFile(leftJar, []byte("left-state"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertFileContentsEqual(t, rootJar, "parent-state")
	assertFileContentsEqual(t, rightJar, "parent-state")

	if err := cookies.prepareDirectory(grandchild); err != nil {
		t.Fatal(err)
	}
	grandchildJar := cookies.jarFor(grandchild, 0)
	assertFileContentsEqual(t, grandchildJar, "left-state")
	assertCookieJarFiles(t, runDir, []string{rootJar, leftJar, rightJar, grandchildJar})
}

func TestCookieJarsModeTwoUsesLatestCompletionAndPreservesEmptyLineage(t *testing.T) {
	t.Run("latest file completion selects child source", func(t *testing.T) {
		runDir := t.TempDir()
		cookies := newCookieJars(domain.Config{Parallelism: 2, TempRunDir: runDir})
		root := &domain.Directory{Path: "/", RuntimeSteps: make([][]domain.Step, 2)}
		child := &domain.Directory{Path: "/child", Parent: root, RuntimeSteps: make([][]domain.Step, 2)}
		if err := cookies.prepareDirectory(root); err != nil {
			t.Fatal(err)
		}
		first := cookies.jarFor(root, 0)
		second := cookies.jarFor(root, 1)
		if first == second {
			t.Fatalf("root files share jar %q", first)
		}
		if err := os.WriteFile(first, []byte("first"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(second, []byte("second"), 0o600); err != nil {
			t.Fatal(err)
		}
		cookies.recordCompletion(root, 1)
		cookies.recordCompletion(root, 0)
		if err := cookies.prepareDirectory(child); err != nil {
			t.Fatal(err)
		}
		childFirst := cookies.jarFor(child, 0)
		childSecond := cookies.jarFor(child, 1)
		if childFirst == childSecond || childFirst == first || childFirst == second {
			t.Fatalf("child file jars are not independent copies: %q %q", childFirst, childSecond)
		}
		assertFileContentsEqual(t, childFirst, "first")
		assertFileContentsEqual(t, childSecond, "first")
	})

	t.Run("empty directory chain retains one root inheritance source", func(t *testing.T) {
		runDir := t.TempDir()
		cookies := newCookieJars(domain.Config{Parallelism: 2, TempRunDir: runDir})
		root := &domain.Directory{Path: "/"}
		emptyChild := &domain.Directory{Path: "/empty", Parent: root}
		grandchild := &domain.Directory{Path: "/empty/grandchild", Parent: emptyChild, RuntimeSteps: make([][]domain.Step, 1)}
		for _, dir := range []*domain.Directory{root, emptyChild, grandchild} {
			if err := cookies.prepareDirectory(dir); err != nil {
				t.Fatal(err)
			}
		}
		grandchildJar := cookies.jarFor(grandchild, 0)
		if grandchildJar == cookies.rootInheritance {
			t.Fatal("grandchild shares the root inheritance jar")
		}
		assertCookieJarFiles(t, runDir, []string{cookies.rootInheritance, grandchildJar})
	})

	t.Run("empty steps files retain incoming source without extra jar", func(t *testing.T) {
		runDir := t.TempDir()
		cookies := newCookieJars(domain.Config{Parallelism: 2, TempRunDir: runDir})
		root := &domain.Directory{Path: "/", RuntimeSteps: make([][]domain.Step, 2)}
		child := &domain.Directory{Path: "/child", Parent: root, RuntimeSteps: make([][]domain.Step, 1)}
		if err := cookies.prepareDirectory(root); err != nil {
			t.Fatal(err)
		}
		rootFirst := cookies.jarFor(root, 0)
		if err := os.WriteFile(rootFirst, []byte("incoming"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := cookies.prepareDirectory(child); err != nil {
			t.Fatal(err)
		}
		childJar := cookies.jarFor(child, 0)
		assertFileContentsEqual(t, childJar, "incoming")
		if cookies.rootInheritance != "" {
			t.Fatalf("root inheritance jar = %q, want none when root has file jars", cookies.rootInheritance)
		}
	})
}

func TestExecutorCookieSelectionDisablesAndReenablesOwningJar(t *testing.T) {
	commandDir := newExecutorCommandDir(t)
	logPath := filepath.Join(commandDir, "cookie-arguments")
	t.Setenv("APIH_COOKIE_LOG", logPath)
	installExecutorCommand(t, commandDir, "curl", `
url=
jar=
previous=
for argument do
  case "$previous" in
    url) url=$argument ;;
    jar) jar=$argument ;;
  esac
  previous=
  case "$argument" in
    --url) previous=url ;;
    --cookie-jar) previous=jar ;;
  esac
done
printf '%s|%s|%s\n' "$url" "$jar" "$*" >> "$APIH_COOKIE_LOG"
case "$url" in
  */enabled) printf 'retained-state' > "$jar" ;;
  */reenabled) printf 'state:%s\n' "$(/bin/cat "$jar")" >> "$APIH_COOKIE_LOG" ;;
esac
/bin/cat >/dev/null
printf 'same'
printf 'http-code:200' >&2
`)
	installPassthroughExecutorJQ(t, commandDir)

	dir := cookieTestDirectory("/", 0, "steps.yaml", 3)
	steps := dir.RuntimeSteps[0]
	for index := range steps {
		steps[index].Request.Defaults.BaseURL = "https://example.test"
		steps[index].Response.ExpectedBody = "same"
	}
	steps[0].Request.Path = "/enabled"
	steps[1].Request.Path = "/disabled"
	steps[1].Request.Defaults.DisableCookies = boolPointer(true)
	steps[1].Request.Defaults.Headers = map[string]string{"Cookie": "manual=preserved"}
	steps[2].Request.Path = "/reenabled"
	steps[2].Request.Defaults.DisableCookies = boolPointer(false)
	dir.RuntimeSteps[0] = steps

	config := domain.Config{Parallelism: 0, TempRunDir: t.TempDir()}
	executor := NewExecutor(NewBinder(NewKeyValueStore()), NewValidator(config), reporting.NewReporter(&strings.Builder{}, false), config)
	if err := executor.cookies.prepareStage([]*domain.Directory{dir}); err != nil {
		t.Fatal(err)
	}
	if exitCode, err := executor.processDir(context.Background(), discardResult, dir); exitCode != 0 || err != nil {
		t.Fatalf("processDir() = (%d, %v), want success", exitCode, err)
	}
	log := readExecutorFile(t, logPath)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 4 {
		t.Fatalf("cookie log = %q, want three requests and resumed state", log)
	}
	enabled := strings.SplitN(lines[0], "|", 3)
	disabled := strings.SplitN(lines[1], "|", 3)
	reenabled := strings.SplitN(lines[2], "|", 3)
	if enabled[1] == "" || reenabled[1] != enabled[1] {
		t.Fatalf("enabled jar paths = %q/%q, want same non-empty path", enabled[1], reenabled[1])
	}
	if disabled[1] != "" || strings.Contains(disabled[2], "--cookie-jar") || strings.Contains(disabled[2], "--cookie ") {
		t.Fatalf("disabled arguments = %q, want no automatic cookie options", disabled[2])
	}
	if !strings.Contains(disabled[2], "Cookie: manual=preserved") {
		t.Fatalf("disabled arguments = %q, want explicit Cookie header", disabled[2])
	}
	if lines[3] != "state:retained-state" {
		t.Fatalf("reenabled state = %q, want retained-state", lines[3])
	}
	assertCookieJarFiles(t, config.TempRunDir, []string{enabled[1]})
}

func TestExecutorModeTwoUsesActualLastCompletionIncludingDisabledStep(t *testing.T) {
	commandDir := newExecutorCommandDir(t)
	logPath := filepath.Join(commandDir, "mode-two")
	t.Setenv("APIH_COOKIE_LOG", logPath)
	installExecutorCommand(t, commandDir, "curl", `
url=
jar=
previous=
for argument do
  case "$previous" in
    url) url=$argument ;;
    jar) jar=$argument ;;
  esac
  previous=
  case "$argument" in
    --url) previous=url ;;
    --cookie-jar) previous=jar ;;
  esac
done
case "$url" in
  */fast) printf 'fast-state' > "$jar" ;;
  */slow-disabled) /bin/sleep 0.2 ;;
  */child) printf 'child-state:%s\n' "$(/bin/cat "$jar")" >> "$APIH_COOKIE_LOG" ;;
esac
/bin/cat >/dev/null
printf 'same'
printf 'http-code:200' >&2
`)
	installPassthroughExecutorJQ(t, commandDir)

	root := cookieTestDirectory("/", 0, "fast.yaml", 1)
	secondDefinition := cookieDefinition(root, "slow.yaml", 1)
	root.StepsDefinitions = append(root.StepsDefinitions, secondDefinition)
	root.RuntimeSteps = append(root.RuntimeSteps, secondDefinition.Spec.Steps)
	root.RuntimeSteps[0][0].Request.Defaults.BaseURL = "https://example.test"
	root.RuntimeSteps[0][0].Request.Path = "/fast"
	root.RuntimeSteps[0][0].Response.ExpectedBody = "same"
	root.RuntimeSteps[1][0].Request.Defaults.BaseURL = "https://example.test"
	root.RuntimeSteps[1][0].Request.Path = "/slow-disabled"
	root.RuntimeSteps[1][0].Request.Defaults.DisableCookies = boolPointer(true)
	root.RuntimeSteps[1][0].Response.ExpectedBody = "same"

	child := cookieTestDirectory("/child", 1, "child.yaml", 1)
	child.Parent = root
	root.Children = []*domain.Directory{child}
	child.RuntimeSteps[0][0].Request.Defaults.BaseURL = "https://example.test"
	child.RuntimeSteps[0][0].Request.Path = "/child"
	child.RuntimeSteps[0][0].Response.ExpectedBody = "same"

	config := domain.Config{Parallelism: 2, TempRunDir: t.TempDir()}
	executor := NewExecutor(NewBinder(NewKeyValueStore()), NewValidator(config), reporting.NewReporter(&strings.Builder{}, false), config)
	exitCode, err := executor.Execute(context.Background(), [][]*domain.Directory{{root}, {child}})
	if exitCode != 0 || err != nil {
		t.Fatalf("Execute() = (%d, %v), want success", exitCode, err)
	}
	if got := strings.TrimSpace(readExecutorFile(t, logPath)); got != "child-state:" {
		t.Fatalf("child inherited state = %q, want empty jar selected by last disabled completion", got)
	}
	files, err := filepath.Glob(filepath.Join(config.TempRunDir, "cookies", "*.cookie.jar"))
	if err != nil || len(files) != 3 {
		t.Fatalf("mode-two jar files = %v (error %v), want one per steps file", files, err)
	}
}

func TestCookieStagePreparationFinishesBeforeWorkersStart(t *testing.T) {
	runDir := t.TempDir()
	cookies := newCookieJars(domain.Config{Parallelism: 1, TempRunDir: runDir})
	root := &domain.Directory{Path: "/"}
	first := &domain.Directory{Path: "/first", Parent: root}
	second := &domain.Directory{Path: "/second", Parent: root}

	if err := cookies.prepareDirectory(root); err != nil {
		t.Fatal(err)
	}
	if err := cookies.prepareDirectory(first); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cookies.jarFor(root, 0)); err != nil {
		t.Fatal(err)
	}

	var workersStarted atomic.Int32
	exitCode, err := executeStagesPrepared(
		context.Background(),
		[][]*domain.Directory{{first, second}},
		1,
		nil,
		cookies.prepareStage,
		func(context.Context, resultPublisher, *domain.Directory) (int, error) {
			workersStarted.Add(1)
			return 0, nil
		},
	)
	if exitCode != errs.ExitInternal || !errors.Is(err, errCookieJar) {
		t.Fatalf("executeStagesPrepared() = (%d, %v), want internal cookie jar failure", exitCode, err)
	}
	if got := workersStarted.Load(); got != 0 {
		t.Fatalf("workers started = %d, want none before complete stage preparation", got)
	}
}

func TestCookieJarStorageFailuresAreInternalAndRunsAreIsolated(t *testing.T) {
	t.Run("executor stops before work when storage is unavailable", func(t *testing.T) {
		dir := cookieTestDirectory("/", 0, "steps.yaml", 1)
		executor := NewExecutor(nil, nil, nil, domain.Config{Parallelism: 0, TempRunDir: filepath.Join(t.TempDir(), "missing")})
		exitCode, err := executor.Execute(context.Background(), [][]*domain.Directory{{dir}})
		if exitCode != errs.ExitInternal || !errors.Is(err, errCookieJar) {
			t.Fatalf("Execute() = (%d, %v), want internal cookie jar failure", exitCode, err)
		}
	})

	t.Run("missing run directory", func(t *testing.T) {
		cookies := newCookieJars(domain.Config{Parallelism: 0, TempRunDir: filepath.Join(t.TempDir(), "missing")})
		err := cookies.prepareDirectory(&domain.Directory{Path: "/"})
		if errs.Code(err, 0) != errs.ExitInternal || !errors.Is(err, errCookieJar) {
			t.Fatalf("prepareDirectory() error = %v, want internal cookie jar error", err)
		}
	})

	t.Run("empty run directory", func(t *testing.T) {
		err := newCookieJars(domain.Config{Parallelism: 0}).prepareDirectory(&domain.Directory{Path: "/"})
		if errs.Code(err, 0) != errs.ExitInternal || !errors.Is(err, errCookieJar) {
			t.Fatalf("prepareDirectory() error = %v, want internal cookie jar error", err)
		}
	})

	t.Run("curl-ambiguous run directory", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "cache=one")
		if err := os.Mkdir(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
		err := newCookieJars(domain.Config{Parallelism: 0, TempRunDir: runDir}).prepareDirectory(&domain.Directory{Path: "/"})
		if errs.Code(err, 0) != errs.ExitInternal || !errors.Is(err, errCookieJar) {
			t.Fatalf("prepareDirectory() error = %v, want internal cookie jar error", err)
		}
	})

	t.Run("run path and namespace must be directories", func(t *testing.T) {
		for name, setup := range map[string]func(string) string{
			"run path is file": func(base string) string {
				path := filepath.Join(base, "run-file")
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
			"namespace is file": func(base string) string {
				if err := os.WriteFile(filepath.Join(base, "cookies"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return base
			},
		} {
			t.Run(name, func(t *testing.T) {
				runDir := setup(t.TempDir())
				err := newCookieJars(domain.Config{Parallelism: 0, TempRunDir: runDir}).prepareDirectory(&domain.Directory{Path: "/"})
				if errs.Code(err, 0) != errs.ExitInternal || !errors.Is(err, errCookieJar) {
					t.Fatalf("prepareDirectory() error = %v, want internal cookie jar error", err)
				}
			})
		}
	})

	t.Run("missing parent copy source", func(t *testing.T) {
		cookies := newCookieJars(domain.Config{Parallelism: 1, TempRunDir: t.TempDir()})
		root := &domain.Directory{Path: "/"}
		child := &domain.Directory{Path: "/child", Parent: root}
		if err := cookies.prepareDirectory(root); err != nil {
			t.Fatal(err)
		}
		rootJar := cookies.jarFor(root, 0)
		if err := os.Remove(rootJar); err != nil {
			t.Fatal(err)
		}
		err := cookies.prepareDirectory(child)
		if errs.Code(err, 0) != errs.ExitInternal || !errors.Is(err, errCookieJar) {
			t.Fatalf("prepareDirectory(child) error = %v, want internal cookie jar error", err)
		}
	})

	t.Run("repeated preparation is idempotent", func(t *testing.T) {
		for _, mode := range []int{1, 2} {
			runDir := t.TempDir()
			dir := &domain.Directory{Path: "/", RuntimeSteps: make([][]domain.Step, 1)}
			cookies := newCookieJars(domain.Config{Parallelism: mode, TempRunDir: runDir})
			if err := cookies.prepareDirectory(dir); err != nil {
				t.Fatal(err)
			}
			before, _ := filepath.Glob(filepath.Join(runDir, "cookies", "*.cookie.jar"))
			if err := cookies.prepareDirectory(dir); err != nil {
				t.Fatal(err)
			}
			after, _ := filepath.Glob(filepath.Join(runDir, "cookies", "*.cookie.jar"))
			if len(before) != 1 || len(after) != 1 || before[0] != after[0] {
				t.Fatalf("mode %d repeated preparation changed jars: %v -> %v", mode, before, after)
			}
		}
	})

	t.Run("separate runs use separate namespaces", func(t *testing.T) {
		firstRun := t.TempDir()
		secondRun := t.TempDir()
		first := newCookieJars(domain.Config{Parallelism: 0, TempRunDir: firstRun})
		second := newCookieJars(domain.Config{Parallelism: 0, TempRunDir: secondRun})
		root := &domain.Directory{Path: "/"}
		if err := first.prepareDirectory(root); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(first.runJar, []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := second.prepareDirectory(root); err != nil {
			t.Fatal(err)
		}
		if first.runJar == second.runJar || !strings.HasPrefix(first.runJar, filepath.Join(firstRun, "cookies")+string(os.PathSeparator)) || !strings.HasPrefix(second.runJar, filepath.Join(secondRun, "cookies")+string(os.PathSeparator)) {
			t.Fatalf("run jars are not isolated: %q %q", first.runJar, second.runJar)
		}
		assertFileContentsEqual(t, second.runJar, "")
	})
}

func cookieTestDirectory(path string, stage int, filePath string, steps int) *domain.Directory {
	dir := &domain.Directory{Path: path, Stage: stage}
	definition := cookieDefinition(dir, filePath, steps)
	dir.StepsDefinitions = []*domain.StepsDefinition{definition}
	dir.RuntimeSteps = [][]domain.Step{definition.Spec.Steps}
	return dir
}

func cookieDefinition(dir *domain.Directory, path string, steps int) *domain.StepsDefinition {
	definition := &domain.StepsDefinition{File: &domain.File{Path: path, Directory: dir}}
	definition.Spec.Steps = make([]domain.Step, steps)
	for index := range definition.Spec.Steps {
		definition.Spec.Steps[index].Definition = definition
		definition.Spec.Steps[index].Index = index
	}
	return definition
}

func boolPointer(value bool) *bool {
	return &value
}

func assertCookieJarFiles(t *testing.T, runDir string, want []string) {
	t.Helper()
	got, err := filepath.Glob(filepath.Join(runDir, "cookies", "*.cookie.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("cookie jar files = %v, want %v", got, want)
	}
	wantSet := make(map[string]bool, len(want))
	for _, path := range want {
		wantSet[path] = true
		if !strings.HasSuffix(path, ".cookie.jar") {
			t.Fatalf("cookie jar name = %q, want .cookie.jar suffix", path)
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("cookie jar %q info = (%v, %v), want existing regular file", path, info, err)
		}
	}
	for _, path := range got {
		if !wantSet[path] {
			t.Fatalf("unexpected cookie jar %q", path)
		}
	}
}

func assertFileContentsEqual(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(contents); got != want {
		t.Fatalf("%s contents = %q, want %q", path, got, want)
	}
}
