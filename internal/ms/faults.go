//go:build !testhooks

package ms

// Test-only fault injection; see faults_testhooks.go.
func fault(string)           {}
func testDelay()             {}
func testRace(*Repo, string) {}

// fakeAncestors replaces the process chain in tests; see faults_testhooks.go.
func fakeAncestors() ([]proc, bool, error) { return nil, false, nil }
