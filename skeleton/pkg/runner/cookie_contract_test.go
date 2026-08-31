package runner

import (
	"context"
	"testing"
)

func TestCookieAwareCurlSignatures(t *testing.T) {
	t.Parallel()
	var _ func(context.Context, string, string, map[string]string, string, int, int, string, string) (string, int, error) = Curl
	var _ func(context.Context, string, string, map[string]string, string, int, int, string, string) (string, []string, error) = CurlBuild
}
