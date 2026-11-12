//go:build !testhooks

package ms

// Test-only fault injection; see faults_testhooks.go.
func fault(string)           {}
func testDelay()             {}
func testRace(*Repo, string) {}
