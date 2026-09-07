//go:build integration

package inttests

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func TestTotalCoverage(t *testing.T) {
	percentage, err := totalCoverage("github.com/divilla/apihydra/pkg\tcoverage: 91.2% of statements\ntotal:\t(statements)\t91.2%\n")
	if err != nil {
		t.Fatalf("totalCoverage() error = %v", err)
	}
	if percentage != 91.2 {
		t.Fatalf("totalCoverage() = %.1f, want 91.2", percentage)
	}
	if _, err := totalCoverage("no total"); err == nil {
		t.Fatal("totalCoverage() error = nil for malformed summary")
	}
}

func assertProductionCoverage(t *testing.T, ctx context.Context, repoRoot, coverageDir string, minimum float64) {
	t.Helper()
	profile := filepath.Join(t.TempDir(), "integration.cover")
	textfmt := exec.CommandContext(ctx, "go", "tool", "covdata", "textfmt", "-i="+coverageDir, "-o="+profile)
	textfmt.Dir = repoRoot
	if output, err := textfmt.CombinedOutput(); err != nil {
		t.Fatalf("format integration coverage: %v\n%s", err, output)
	}

	cover := exec.CommandContext(ctx, "go", "tool", "cover", "-func="+profile)
	cover.Dir = repoRoot
	output, err := cover.CombinedOutput()
	if err != nil {
		t.Fatalf("summarize integration coverage: %v\n%s", err, output)
	}
	percentage, err := totalCoverage(string(output))
	if err != nil {
		t.Fatal(err)
	}
	if percentage < minimum {
		t.Fatalf("integration production coverage = %.1f%%, want at least %.1f%%\n%s", percentage, minimum, output)
	}
}

func totalCoverage(output string) (float64, error) {
	pattern := regexp.MustCompile(`(?m)^total:\s+\(statements\)\s+([0-9]+(?:\.[0-9]+)?)%$`)
	match := pattern.FindStringSubmatch(output)
	if match == nil {
		return 0, fmt.Errorf("coverage summary has no total line: %q", output)
	}
	percentage, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parse total coverage %q: %w", match[1], err)
	}
	return percentage, nil
}
