//go:build !testhooks

package ms

import "time"

// Test-only fault injection; see faults_testhooks.go.
func fault(string)           {}
func testDelay()             {}
func testPause(string)       {}
func testRace(*Repo, string) {}

// testTwoStepRename forces the two renames of a case-only move in tests;
// see faults_testhooks.go.
func testTwoStepRename() bool { return false }

// testLockTimeout returns d; tests shorten it, see faults_testhooks.go.
func testLockTimeout(d time.Duration) time.Duration { return d }

// fakeAncestors replaces the process chain in tests; see faults_testhooks.go.
func fakeAncestors() ([]proc, bool, error) { return nil, false, nil }
