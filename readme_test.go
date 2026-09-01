package apih_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const readmePath = "README.md"

const productDescription = "APIHydra is an ultra-fast, agent-first API integration tester."

func TestREADMEAcceptanceCriterion1CompactLandingPage(t *testing.T) {
	readme := readREADME(t)
	wantOpening := productDescription + " Its `apih` command discovers YAML suites, executes HTTP requests, and validates responses."

	if !strings.HasPrefix(readme, "# APIHydra (`apih`)\n\n"+wantOpening+"\n") {
		t.Fatal("README must start with the required H1 and product-and-audience paragraph")
	}
	if got := countH1s(readme); got != 1 {
		t.Fatalf("README H1 count = %d, want 1", got)
	}
	if lines := strings.Count(readme, "\n"); lines > 40 {
		t.Fatalf("README line count = %d, want at most 40", lines)
	}
}

func TestProductDescriptionIsUnifiedAcrossDocumentation(t *testing.T) {
	paths := []string{
		readmePath,
		"docs/user-manual/apih.md",
		"agent/prd.md",
		"agent/changes/018-readme-and-install.md",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			normalized := strings.Join(strings.Fields(string(contents)), " ")
			if !strings.Contains(normalized, productDescription) {
				t.Errorf("%s does not contain the standard product description", path)
			}
		})
	}
}

func TestREADMEAcceptanceCriterion2CanonicalManualLink(t *testing.T) {
	readme := readREADME(t)
	const link = "[APIHydra user manual](docs/user-manual/apih.md)"
	if !strings.Contains(readme, link) {
		t.Fatalf("README does not contain canonical repository-relative link %q", link)
	}
	if info, err := os.Stat(userManualPath); err != nil {
		t.Fatalf("canonical manual link does not resolve: %v", err)
	} else if info.IsDir() {
		t.Fatalf("canonical manual link resolves to a directory")
	}
	for _, duplicate := range []string{"user-manual.md", "USER-MANUAL.md", "docs/user-manual.md"} {
		if _, err := os.Stat(duplicate); err == nil {
			t.Errorf("duplicate user manual exists at %s", duplicate)
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect possible duplicate %s: %v", duplicate, err)
		}
	}
}

func TestREADMEAcceptanceCriterion3InstallationContract(t *testing.T) {
	readme := readREADME(t)
	module, goVersion := readModuleContract(t)
	required := []string{
		"## Installation",
		"go install " + module + "/cmd/apih@latest",
		"Go " + goVersion,
		"`GOBIN`",
		"Go workspace's `bin` directory",
		"`PATH`",
		"`curl` executes HTTP requests",
		"`jq` performs response and Debug JSON processing",
		"`git` renders a body diff when response-body validation fails",
		"apih --help",
	}
	for _, text := range required {
		if !strings.Contains(readme, text) {
			t.Errorf("README does not contain %q", text)
		}
	}
	if info, err := os.Stat(filepath.FromSlash("cmd/apih/main.go")); err != nil {
		t.Fatalf("documented command package does not resolve: %v", err)
	} else if info.IsDir() {
		t.Fatal("documented command package entry point is a directory")
	}
	for _, unsupported := range []string{"Homebrew", "release archive", "container image", "Docker image"} {
		if strings.Contains(readme, unsupported) {
			t.Errorf("README claims unsupported delivery channel %q", unsupported)
		}
	}
}

func TestREADMEAcceptanceCriterion4IsolatedSourceInstall(t *testing.T) {
	binDir := t.TempDir()
	install := exec.Command("go", "install", "./cmd/apih")
	install.Env = append(os.Environ(), "GOBIN="+binDir, "GOPROXY=off")
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install ./cmd/apih into isolated GOBIN: %v\n%s", err, output)
	}

	executable := filepath.Join(binDir, "apih")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatalf("stat installed apih command: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Fatalf("installed apih mode = %v, want executable", info.Mode())
	}

	help := exec.Command(executable, "--help")
	output, err := help.CombinedOutput()
	if err != nil {
		t.Fatalf("installed apih --help: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Usage of ") {
		t.Fatalf("installed apih --help output = %q, want usage", output)
	}
}

func TestREADMEAcceptanceCriterion5FocusedAutomatedChecks(t *testing.T) {
	readme := readREADME(t)
	if strings.Count(readme, "## ") != 1 {
		t.Fatal("README must remain a landing page with only the Installation section")
	}
	for _, detailedSection := range []string{"CLI reference", "YAML document reference", "Troubleshooting", "Roadmap", "Contributing"} {
		if strings.Contains(readme, "## "+detailedSection) {
			t.Errorf("README duplicates or adds out-of-scope section %q", detailedSection)
		}
	}
}

func readREADME(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %s: %v", readmePath, err)
	}
	return string(contents)
}

func countH1s(markdown string) int {
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(markdown))
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "# ") {
			count++
		}
	}
	return count
}

func readModuleContract(t *testing.T) (string, string) {
	t.Helper()
	contents, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	var module, goVersion string
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "module":
			module = fields[1]
		case "go":
			goVersion = fields[1]
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan go.mod: %v", err)
	}
	if module == "" || goVersion == "" {
		t.Fatalf("go.mod contract = module %q, Go %q", module, goVersion)
	}
	return module, goVersion
}
