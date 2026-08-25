//go:build integration && unix && !linux

package inttests

// Linux-only PTY and inotify scenarios account for production error branches
// that cannot be driven portably by the black-box harness.
const minimumCoverage = 89.0
