//go:build integration && !unix

package inttests

import "testing"

func effectiveUserID() int {
	return 0
}

func runApplicationScenariosAsUnprivilegedUser(*testing.T) bool {
	return false
}
