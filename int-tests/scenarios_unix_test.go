//go:build integration && unix

package inttests

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runUnixSpecificScenarios(t *testing.T, ctx context.Context, binary, runRoot, coverageDir, tempRoot, successSuite, validationSuite string) {
	t.Helper()

	toolDir := filepath.Join(tempRoot, "tools-without-git")
	if err := os.Mkdir(toolDir, 0o755); err != nil {
		t.Fatalf("create restricted tool directory: %v", err)
	}
	for _, name := range []string{"curl", "jq"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("locate %s: %v", name, err)
		}
		if err := os.Symlink(path, filepath.Join(toolDir, name)); err != nil {
			t.Fatalf("link %s: %v", name, err)
		}
	}
	missingGit := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, validationSuite, "PATH="+toolDir)
	if missingGit.exitCode == 0 || missingGit.exitCode == 101 || missingGit.stderr == "" {
		t.Fatalf("missing-git result = code %d, stderr %q, want fatal diagnostic", missingGit.exitCode, missingGit.stderr)
	}
	for name, curlScript := range map[string]string{
		"missing status": "#!/bin/sh\nprintf 'response without status'\n",
		"invalid status": "#!/bin/sh\nprintf 'response'\nprintf 'http-code:not-a-number' >&2\n",
	} {
		malformedCurlTools := createToolDirectory(t, map[string]string{"curl": curlScript}, []string{"jq", "git"})
		malformedCurl := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "success-output"), "PATH="+malformedCurlTools)
		if malformedCurl.exitCode == 0 || malformedCurl.exitCode == 101 || malformedCurl.stderr == "" {
			t.Fatalf("curl %s result = code %d, stderr %q, want fatal diagnostic", name, malformedCurl.exitCode, malformedCurl.stderr)
		}
	}
	realCurl, err := exec.LookPath("curl")
	if err != nil {
		t.Fatalf("locate curl: %v", err)
	}
	for name, mutation := range map[string]string{
		"missing source":   `/bin/rm -f "$jar"`,
		"directory source": `/bin/rm -f "$jar"; /bin/mkdir "$jar"`,
	} {
		curlProxy := fmt.Sprintf(`#!/bin/sh
jar=
previous=
for argument do
  if [ "$previous" = jar ]; then jar=$argument; fi
  previous=
  if [ "$argument" = --cookie-jar ]; then previous=jar; fi
done
%q "$@"
status=$?
%s
exit "$status"
`, realCurl, mutation)
		tools := createToolDirectory(t, map[string]string{"curl": curlProxy}, []string{"jq", "git"})
		result := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "cookies-mode1"), "PATH="+tools)
		if result.exitCode != 103 || result.stderr == "" {
			t.Fatalf("cookie copy %s = code %d, stdout %q, stderr %q", name, result.exitCode, result.stdout, result.stderr)
		}
	}
	cookieCreateCacheRoot := filepath.Join(tempRoot, "cookie-create-denied")
	cookieCreateCache := userCacheDirectory(cookieCreateCacheRoot)
	curlProxy := fmt.Sprintf(`#!/bin/sh
jar=
previous=
for argument do
  if [ "$previous" = jar ]; then jar=$argument; fi
  previous=
  if [ "$argument" = --cookie-jar ]; then previous=jar; fi
done
%q "$@"
status=$?
/bin/chmod 500 "$(/usr/bin/dirname "$jar")"
exit "$status"
`, realCurl)
	tools := createToolDirectory(t, map[string]string{"curl": curlProxy}, []string{"jq", "git"})
	environment := append([]string{"PATH=" + tools}, userCacheEnvironment(cookieCreateCacheRoot)...)
	cookieCreateFailure := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "cookies-mode1"), environment...)
	if cookieCreateFailure.exitCode != 103 || cookieCreateFailure.stderr == "" {
		t.Fatalf("cookie create failure = code %d, stdout %q, stderr %q", cookieCreateFailure.exitCode, cookieCreateFailure.stdout, cookieCreateFailure.stderr)
	}
	restoreRunDirectoryPermissions(t, filepath.Join(cookieCreateCache, "apih"))

	unavailableTemp := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "curl-failure"), "TMPDIR="+filepath.Join(tempRoot, "missing-temp"))
	if unavailableTemp.exitCode != 0 || unavailableTemp.stderr != "" {
		t.Fatalf("unavailable-temp result = code %d, stderr %q, want curl success without temporary response storage", unavailableTemp.exitCode, unavailableTemp.stderr)
	}

	// Exercise command-result branches for coverage without asserting the
	// unspecified normalization behavior.
	for _, gitScript := range []string{
		"#!/bin/sh\nexit 0\n",
		"#!/bin/sh\nprintf 'diff without a hunk\\n'\nexit 1\n",
	} {
		fakeTools := createToolDirectory(t, map[string]string{"git": gitScript}, []string{"curl", "jq"})
		runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("test2", "body-only"), "PATH="+fakeTools)
	}

	jqCounter := filepath.Join(tempRoot, "jq-counter")
	jqFailureScript := fmt.Sprintf("#!/bin/sh\ncount=0\nif [ -f %q ]; then read count < %q; fi\ncount=$((count + 1))\nprintf '%%s' \"$count\" > %q\nif [ \"$count\" -eq 3 ]; then printf 'project failed' >&2; exit 4; fi\nprintf '{}'\n", jqCounter, jqCounter, jqCounter)
	jqFailureTools := createToolDirectory(t, map[string]string{"jq": jqFailureScript}, []string{"curl"})
	jqFailure := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("test2", "body-only"), "PATH="+jqFailureTools)
	if jqFailure.exitCode == 0 || jqFailure.exitCode == 101 || jqFailure.stderr == "" {
		t.Fatalf("jq-project failure result = code %d, stderr %q, want fatal diagnostic", jqFailure.exitCode, jqFailure.stderr)
	}

	jqPath, err := exec.LookPath("jq")
	if err != nil {
		t.Fatalf("locate jq: %v", err)
	}
	debugJQCounter := filepath.Join(tempRoot, "debug-jq-counter")
	debugJQScript := fmt.Sprintf("#!/bin/sh\ninput=$(/bin/cat)\ncount=0\nif [ -f %q ]; then read count < %q; fi\ncount=$((count + 1))\nprintf '%%s' \"$count\" > %q\nif [ \"$count\" -eq 4 ]; then printf 'debug pretty failed' >&2; exit 4; fi\nprintf '%%s' \"$input\" | %q \"$@\"\n", debugJQCounter, debugJQCounter, debugJQCounter, jqPath)
	debugJQTools := createToolDirectory(t, map[string]string{"jq": debugJQScript}, []string{"curl", "git"})
	debugJQFailure := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "debug-defaults"), "PATH="+debugJQTools)
	if debugJQFailure.exitCode == 0 || debugJQFailure.exitCode == 101 || debugJQFailure.stderr == "" {
		t.Fatalf("debug-jq failure result = code %d, stderr %q, want fatal diagnostic", debugJQFailure.exitCode, debugJQFailure.stderr)
	}

	operationCacheRoot := filepath.Join(tempRoot, "git-operation-cache")
	operationCache := userCacheDirectory(operationCacheRoot)
	if err := os.MkdirAll(operationCache, 0o700); err != nil {
		t.Fatalf("create Git operation cache: %v", err)
	}
	operationCounter := filepath.Join(tempRoot, "git-operation-jq-counter")
	operationScript := fmt.Sprintf("#!/bin/sh\ncount=0\nif [ -f %q ]; then read count < %q; fi\ncount=$((count + 1))\nprintf '%%s' \"$count\" > %q\nif [ \"$count\" -eq 1 ]; then for run in %q/apih/run-*; do /bin/mkdir \"$run/git-diff\"; /bin/chmod 500 \"$run/git-diff\"; done; exit 0; fi\nif [ \"$count\" -eq 2 ]; then printf '{\"value\":1}'; else printf '{\"value\":2}'; fi\n", operationCounter, operationCounter, operationCounter, operationCache)
	operationTools := createToolDirectory(t, map[string]string{"jq": operationScript}, []string{"curl", "git"})
	operationEnvironment := append([]string{"PATH=" + operationTools}, userCacheEnvironment(operationCacheRoot)...)
	operationFailure := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("test2", "body-only"), operationEnvironment...)
	if operationFailure.exitCode == 0 || operationFailure.exitCode == 101 || operationFailure.stderr == "" {
		t.Fatalf("Git operation-directory failure = code %d, stderr %q, want fatal diagnostic", operationFailure.exitCode, operationFailure.stderr)
	}

	t.Run("permission-denial scenarios", func(t *testing.T) {
		requirePermissionDenialScenarios(t)

		lockedCacheRoot := filepath.Join(tempRoot, "locked-cache")
		lockedAppCache := filepath.Join(userCacheDirectory(lockedCacheRoot), "apih")
		if err := os.MkdirAll(filepath.Dir(lockedAppCache), 0o700); err != nil {
			t.Fatalf("create application cache parent: %v", err)
		}
		if err := os.Symlink("/proc", lockedAppCache); err != nil {
			t.Fatalf("create unwritable application cache link: %v", err)
		}
		runDirectoryFailure := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "success-output"), userCacheEnvironment(lockedCacheRoot)...)
		if runDirectoryFailure.exitCode != 103 || runDirectoryFailure.stdout != "" || runDirectoryFailure.stderr == "" {
			t.Fatalf("run directory failure = code %d, stdout %q, stderr %q", runDirectoryFailure.exitCode, runDirectoryFailure.stdout, runDirectoryFailure.stderr)
		}

		gitCacheRoot := filepath.Join(tempRoot, "git-cache-denied")
		gitCache := userCacheDirectory(gitCacheRoot)
		if err := os.MkdirAll(gitCache, 0o700); err != nil {
			t.Fatalf("create Git cache directory: %v", err)
		}
		gitCounter := filepath.Join(tempRoot, "git-jq-counter")
		gitTempScript := fmt.Sprintf("#!/bin/sh\ncount=0\nif [ -f %q ]; then read count < %q; fi\ncount=$((count + 1))\nprintf '%%s' \"$count\" > %q\nif [ \"$count\" -eq 1 ]; then for run in %q/apih/run-*; do /bin/chmod 500 \"$run\"; done; exit 0; fi\nif [ \"$count\" -eq 2 ]; then printf '{\"value\":1}'; else printf '{\"value\":2}'; fi\n", gitCounter, gitCounter, gitCounter, gitCache)
		gitTempTools := createToolDirectory(t, map[string]string{"jq": gitTempScript}, []string{"curl", "git"})
		gitTempEnvironment := append([]string{"PATH=" + gitTempTools}, userCacheEnvironment(gitCacheRoot)...)
		gitTempFailure := runCLIWithEnv(t, ctx, binary, runRoot, coverageDir, filepath.Join("test2", "body-only"), gitTempEnvironment...)
		if gitTempFailure.exitCode == 0 || gitTempFailure.exitCode == 101 || gitTempFailure.stderr == "" {
			t.Fatalf("Git temp failure result = code %d, stderr %q, want fatal diagnostic", gitTempFailure.exitCode, gitTempFailure.stderr)
		}
		restoreRunDirectoryPermissions(t, filepath.Join(gitCache, "apih"))

		umaskCacheRoot := filepath.Join(tempRoot, "umask-cache")
		if err := os.Mkdir(umaskCacheRoot, 0o777); err != nil {
			t.Fatalf("create umask cache directory: %v", err)
		}
		gitExpectedFailure := runCLIWithUmask(t, ctx, binary, runRoot, coverageDir, filepath.Join("test2", "body-only"), userCacheEnvironment(umaskCacheRoot)...)
		if gitExpectedFailure.exitCode == 0 || gitExpectedFailure.exitCode == 101 || gitExpectedFailure.stderr == "" {
			t.Fatalf("Git expected-file failure result = code %d, stderr %q, want fatal diagnostic", gitExpectedFailure.exitCode, gitExpectedFailure.stderr)
		}

		gitActualCacheRoot := filepath.Join(tempRoot, "git-actual-cache")
		gitActualCache := userCacheDirectory(gitActualCacheRoot)
		if err := os.MkdirAll(gitActualCache, 0o700); err != nil {
			t.Fatalf("create actual-file cache directory: %v", err)
		}
		actualCounter := filepath.Join(tempRoot, "actual-jq-counter")
		actualScript := fmt.Sprintf("#!/bin/sh\ncount=0\nif [ -f %q ]; then read count < %q; fi\ncount=$((count + 1))\nprintf '%%s' \"$count\" > %q\nif [ \"$count\" -eq 1 ]; then exit 0; fi\nif [ \"$count\" -eq 2 ]; then /usr/bin/head -c 5000000 /dev/zero; else printf '{}'; fi\n", actualCounter, actualCounter, actualCounter)
		actualTools := createToolDirectory(t, map[string]string{"jq": actualScript}, []string{"curl", "git"})
		actualEnvironment := append([]string{"PATH=" + actualTools}, userCacheEnvironment(gitActualCacheRoot)...)
		gitActualFailure := runCLIWithFileSizeLimit(t, ctx, binary, runRoot, coverageDir, filepath.Join("test2", "body-only"), actualEnvironment...)
		if gitActualFailure.exitCode == 0 || gitActualFailure.exitCode == 101 || gitActualFailure.stderr == "" {
			t.Fatalf("Git actual-file failure result = code %d, stderr %q, want fatal diagnostic", gitActualFailure.exitCode, gitActualFailure.stderr)
		}

		for _, unreadable := range []string{
			filepath.Join("scenarios", "unreadable-directory"),
			filepath.Join("scenarios", "unreadable-child", "blocked"),
			filepath.Join("scenarios", "unreadable-file", "root.yaml"),
			filepath.Join("scenarios", "unreadable-child-file", "child", "steps.yaml"),
		} {
			path := filepath.Join(runRoot, unreadable)
			if err := os.Chmod(path, 0); err != nil {
				t.Fatalf("make %s unreadable: %v", unreadable, err)
			}
			selected := filepath.Dir(unreadable)
			if unreadable == filepath.Join("scenarios", "unreadable-child-file", "child", "steps.yaml") {
				selected = filepath.Join("scenarios", "unreadable-child-file")
			}
			result := runCLI(t, ctx, binary, runRoot, coverageDir, selected)
			if unreadable == filepath.Join("scenarios", "unreadable-directory") {
				result = runCLI(t, ctx, binary, runRoot, coverageDir, unreadable)
			}
			if err := os.Chmod(path, 0o700); err != nil {
				t.Fatalf("restore %s permissions: %v", unreadable, err)
			}
			if result.exitCode != 102 || result.stderr == "" {
				t.Fatalf("unreadable %s result = code %d, stderr %q, want configuration failure", unreadable, result.exitCode, result.stderr)
			}
		}
	})

	removedCWD := filepath.Join(tempRoot, "removed-cwd")
	if err := os.Mkdir(removedCWD, 0o700); err != nil {
		t.Fatalf("create removable working directory: %v", err)
	}
	missingCWD := runCLIWithoutWorkingDirectory(t, ctx, binary, removedCWD, coverageDir)
	if missingCWD.exitCode != 103 || missingCWD.stdout != "" || missingCWD.stderr == "" {
		t.Fatalf("missing-CWD result = code %d, stdout %q, stderr %q, want internal diagnostic", missingCWD.exitCode, missingCWD.stdout, missingCWD.stderr)
	}

	if full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0); err == nil {
		outputFailure := runCLIWithOutput(t, ctx, binary, runRoot, coverageDir, successSuite, full)
		_ = full.Close()
		if outputFailure.exitCode != 103 || outputFailure.stderr == "" {
			t.Fatalf("output-failure result = code %d, stderr %q, want code 103 and diagnostic", outputFailure.exitCode, outputFailure.stderr)
		}
	}
	readOnlyOutput, err := os.Open(filepath.Join(runRoot, successSuite, "root.yaml"))
	if err != nil {
		t.Fatalf("open read-only output target: %v", err)
	}
	readOnlyOutputFailure := runCLIWithOutput(t, ctx, binary, runRoot, coverageDir, successSuite, readOnlyOutput)
	_ = readOnlyOutput.Close()
	if readOnlyOutputFailure.exitCode != 103 || readOnlyOutputFailure.stderr == "" {
		t.Fatalf("read-only output-failure result = code %d, stderr %q, want code 103 and diagnostic", readOnlyOutputFailure.exitCode, readOnlyOutputFailure.stderr)
	}
}

func restoreRunDirectoryPermissions(t *testing.T, appCache string) {
	t.Helper()
	runDirs, err := filepath.Glob(filepath.Join(appCache, "run-*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, runDir := range runDirs {
		if err := os.Chmod(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := filepath.WalkDir(runDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return os.Chmod(path, 0o700)
			}
			return os.Chmod(path, 0o600)
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func runCLIWithFileSizeLimit(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string, env ...string) cliResult {
	t.Helper()
	cmd := exec.CommandContext(ctx, "sh", "-c", `ulimit -f 2048; exec "$1" "$2"`, "sh", binary, suite)
	cmd.Dir = workDir
	cmd.Env = cliEnvironment(coverageDir, env...)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run with file-size limit: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return cliResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func runCLIWithoutWorkingDirectory(t *testing.T, ctx context.Context, binary, workDir, coverageDir string) cliResult {
	t.Helper()
	cmd := exec.CommandContext(ctx, "sh", "-c", `cd "$1" && rmdir "$1" && exec "$2"`, "sh", workDir, binary)
	cmd.Env = cliEnvironment(coverageDir)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run without working directory: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return cliResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func runCLIWithUmask(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string, env ...string) cliResult {
	t.Helper()
	cmd := exec.CommandContext(ctx, "sh", "-c", `umask 0777; exec "$1" "$2"`, "sh", binary, suite)
	cmd.Dir = workDir
	cmd.Env = cliEnvironment(coverageDir, env...)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if chmodErr := filepath.WalkDir(coverageDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return os.Chmod(path, 0o600)
	}); chmodErr != nil {
		t.Fatalf("restore coverage permissions: %v", chmodErr)
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run with umask: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return cliResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}
