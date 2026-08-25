//go:build integration && !linux

package inttests

import (
	"context"
	"testing"
)

func runPlatformSpecificScenarios(t *testing.T, _ context.Context, _, _, _ string) {
	t.Helper()
	t.Run("closed PTY output failures", func(t *testing.T) {
		t.Skip("closed-PTY output-failure scenarios require Linux PTY ioctls")
	})
	t.Run("directory permission transition", func(t *testing.T) {
		t.Skip("directory permission-transition scenario requires Linux inotify")
	})
}
