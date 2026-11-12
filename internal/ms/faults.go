//go:build !testhooks

package ms

import "time"

// Test-only fault injection; see faults_testhooks.go.
func fault(string)           {}
func testDelay()             {}
func testRace(*Repo, string) {}

// testLockTimeout returns d; tests shorten it, see faults_testhooks.go.
func testLockTimeout(d time.Duration) time.Duration { return d }

// fakeAncestors replaces the process chain in tests; see faults_testhooks.go.
func fakeAncestors() ([]proc, bool, error) { return nil, false, nil }
