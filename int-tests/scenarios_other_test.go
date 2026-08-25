//go:build integration && !unix

package inttests

import (
	"context"
	"testing"
)

func runUnixSpecificScenarios(t *testing.T, _ context.Context, _, _, _, _, _, _ string) {
	t.Helper()
	t.Run("POSIX shell and filesystem scenarios", func(t *testing.T) {
		t.Skip("fake POSIX tools, permission failures, and deleted working directories require Unix")
	})
}
