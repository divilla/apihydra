//go:build integration && !unix

package inttests

// Unix shell, permission, deleted-directory, PTY, and inotify scenarios cover
// production error branches that cannot be driven on non-Unix systems.
const minimumCoverage = 86.0
