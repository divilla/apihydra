//go:build integration && unix

package inttests

import (
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

const unprivilegedIntegrationEnv = "APIH_TEST_UNPRIVILEGED_INTEGRATION"

func effectiveUserID() int {
	return os.Geteuid()
}

func runApplicationScenariosAsUnprivilegedUser(t *testing.T) bool {
	t.Helper()
	if effectiveUserID() != 0 || os.Getenv(unprivilegedIntegrationEnv) == "1" {
		return false
	}

	workDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory for delegated integration run: %v", err)
	}
	uid, gid, err := unprivilegedIdentity(workDir)
	if err != nil {
		t.Fatalf("resolve unprivileged integration user: %v", err)
		return false
	}
	credential := &syscall.Credential{Uid: uid, Gid: gid}
	probe := exec.Command("true")
	probe.Dir = workDir
	probe.SysProcAttr = &syscall.SysProcAttr{Credential: credential}
	if err := probe.Run(); err != nil {
		t.Fatalf("unprivileged integration user cannot access the workspace: %v", err)
		return false
	}

	directory := delegatedIntegrationDirectory(t)
	testBinary := filepath.Join(directory, "integration.test")
	copyExecutable(t, testBinary)
	cacheDirectory := filepath.Join(directory, "go-cache")
	if err := os.Mkdir(cacheDirectory, 0o777); err != nil {
		t.Fatalf("create delegated Go cache: %v", err)
	}
	if err := os.Chmod(cacheDirectory, 0o777); err != nil {
		t.Fatalf("make delegated Go cache writable: %v", err)
	}
	moduleCacheDirectory := filepath.Join(directory, "go-mod-cache")
	stageModuleCache(t, workDir, moduleCacheDirectory)

	cmd := exec.Command(testBinary, "-test.run=^TestApplicationScenariosAndCoverage$", "-test.count=1")
	cmd.Dir = workDir
	cmd.Env = append(
		os.Environ(),
		unprivilegedIntegrationEnv+"=1",
		"GOCACHE="+cacheDirectory,
		"GOMODCACHE="+moduleCacheDirectory,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: credential}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("delegated unprivileged integration run: %v\n%s", err, output)
	}
	return true
}

func stageModuleCache(t *testing.T, workDir, destination string) {
	t.Helper()

	sourceCache := goOutput(t, workDir, "env", "GOMODCACHE")
	moduleDirectories := goOutput(t, workDir, "list", "-m", "-f={{if not .Main}}{{.Dir}}{{end}}", "all")
	if err := os.Mkdir(destination, 0o777); err != nil {
		t.Fatalf("create delegated module cache: %v", err)
	}
	if err := os.Chmod(destination, 0o777); err != nil {
		t.Fatalf("make delegated module cache writable: %v", err)
	}

	for _, source := range strings.Split(moduleDirectories, "\n") {
		if source == "" {
			continue
		}
		relative, err := filepath.Rel(sourceCache, source)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		moduleDestination := filepath.Join(destination, relative)
		if err := os.CopyFS(moduleDestination, os.DirFS(source)); err != nil {
			t.Fatalf("stage module %s for delegated integration run: %v", source, err)
		}
		versionSeparator := strings.LastIndex(relative, "@")
		if versionSeparator < 0 {
			t.Fatalf("cached module directory %s has no version suffix", source)
		}
		downloadRelative := filepath.Join("cache", "download", relative[:versionSeparator], "@v")
		downloadSource := filepath.Join(sourceCache, downloadRelative)
		if err := os.CopyFS(filepath.Join(destination, downloadRelative), os.DirFS(downloadSource)); err != nil {
			t.Fatalf("stage module metadata %s for delegated integration run: %v", downloadSource, err)
		}
	}
	if err := filepath.WalkDir(destination, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		permission := info.Mode().Perm() | 0o444
		if entry.IsDir() {
			permission |= 0o111
		}
		return os.Chmod(path, permission)
	}); err != nil {
		t.Fatalf("make delegated module cache readable: %v", err)
	}
}

func goOutput(t *testing.T, workDir string, arguments ...string) string {
	t.Helper()
	cmd := exec.Command("go", arguments...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func delegatedIntegrationDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "apih-integration-")
	if err != nil {
		t.Fatalf("create delegated integration directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove delegated integration directory: %v", err)
		}
	})
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatalf("make delegated integration directory accessible: %v", err)
	}
	return directory
}

func TestDelegatedIntegrationDirectoryIsTraversable(t *testing.T) {
	directory := delegatedIntegrationDirectory(t)
	for path := directory; ; path = filepath.Dir(path) {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat delegated integration ancestor %s: %v", path, err)
		}
		if info.Mode().Perm()&0o001 == 0 {
			t.Fatalf("delegated integration ancestor %s mode = %04o, want traversal for other users", path, info.Mode().Perm())
		}
		if parent := filepath.Dir(path); parent == path {
			break
		}
	}
}

func TestStagedModuleCacheSupportsOfflineGoList(t *testing.T) {
	workDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(delegatedIntegrationDirectory(t), "go-mod-cache")
	stageModuleCache(t, workDir, destination)

	cmd := exec.Command("go", "list", "-mod=mod", "./...")
	cmd.Dir = filepath.Dir(workDir)
	cmd.Env = append(os.Environ(), "GOMODCACHE="+destination, "GOPROXY=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("use staged module cache offline: %v\n%s", err, output)
	}

	if err := filepath.WalkDir(destination, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		want := os.FileMode(0o004)
		if entry.IsDir() {
			want = 0o005
		}
		if info.Mode().Perm()&want != want {
			t.Errorf("staged module cache path %s mode = %04o, want other-user access %04o", path, info.Mode().Perm(), want)
		}
		return nil
	}); err != nil {
		t.Fatalf("inspect staged module cache: %v", err)
	}
}

func unprivilegedIdentity(workDir string) (uint32, uint32, error) {
	info, err := os.Stat(workDir)
	if err != nil {
		return 0, 0, err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != 0 {
		return stat.Uid, stat.Gid, nil
	}

	account, err := user.Lookup("nobody")
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.ParseUint(account.Gid, 10, 32)
	if err != nil {
		return 0, 0, err
	}
	return uint32(uid), uint32(gid), nil
}

func copyExecutable(t *testing.T, destination string) {
	t.Helper()
	sourcePath, err := os.Executable()
	if err != nil {
		t.Fatalf("locate integration test executable: %v", err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatalf("open integration test executable: %v", err)
	}
	defer source.Close()
	destinationFile, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o755)
	if err != nil {
		t.Fatalf("create delegated integration executable: %v", err)
	}
	if _, err := io.Copy(destinationFile, source); err != nil {
		_ = destinationFile.Close()
		t.Fatalf("copy integration test executable: %v", err)
	}
	if err := destinationFile.Close(); err != nil {
		t.Fatalf("close delegated integration executable: %v", err)
	}
}
