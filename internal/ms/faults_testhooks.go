//go:build testhooks

package ms

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// fault exits abruptly at a named point, as a killed process would.
func fault(point string) {
	if os.Getenv("GIT_MULTISTAGE_FAULT") == point {
		os.Exit(86)
	}
}

// testPause stops at a named point while GIT_MULTISTAGE_TEST_PAUSE, a
// comma-separated list, names it: it creates the file named after the point
// in GIT_MULTISTAGE_TEST_PAUSE_DIR and waits, up to a minute, until the test
// deletes it.
func testPause(point string) {
	if !slices.Contains(strings.Split(os.Getenv("GIT_MULTISTAGE_TEST_PAUSE"), ","), point) {
		return
	}
	p := filepath.Join(os.Getenv("GIT_MULTISTAGE_TEST_PAUSE_DIR"), point)
	if os.WriteFile(p, nil, 0o666) != nil {
		return
	}
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			return
		}
	}
}

// testDelay sleeps for a random time up to GIT_MULTISTAGE_TEST_DELAY_MS.
func testDelay() {
	if n, _ := strconv.Atoi(os.Getenv("GIT_MULTISTAGE_TEST_DELAY_MS")); n > 0 {
		time.Sleep(time.Duration(rand.Intn(n)) * time.Millisecond) //nolint:gosec // a test delay needs no cryptographic randomness
	}
}

// testLockTimeout returns GIT_MULTISTAGE_TEST_LOCK_TIMEOUT_MS when it is set,
// so that a test of a busy stage need not wait for the full timeout.
func testLockTimeout(d time.Duration) time.Duration {
	if n, err := strconv.Atoi(os.Getenv("GIT_MULTISTAGE_TEST_LOCK_TIMEOUT_MS")); err == nil && n >= 0 {
		return time.Duration(n) * time.Millisecond
	}
	return d
}

// testLeaseTimeout returns GIT_MULTISTAGE_LEASE_TIMEOUT_MS when it is set,
// so that a test of a held lease need not wait for the full timeout.
func testLeaseTimeout(d time.Duration) time.Duration {
	if n, err := strconv.Atoi(os.Getenv("GIT_MULTISTAGE_LEASE_TIMEOUT_MS")); err == nil && n >= 0 {
		return time.Duration(n) * time.Millisecond
	}
	return d
}

var raced int

// testRace moves the branch with a foreign commit just before update-ref,
// for the first GIT_MULTISTAGE_TEST_RACE attempts, to force CAS failures.
func testRace(r *Repo, branch string) {
	testDelay()
	if n, _ := strconv.Atoi(os.Getenv("GIT_MULTISTAGE_TEST_RACE")); raced >= n {
		return
	}
	raced++
	must := func(s string, err error) string {
		if err != nil {
			panic(err)
		}
		return s
	}
	tip := must(r.ResolveRef(branch))
	name := fmt.Sprintf("race-%d-%d.txt", os.Getpid(), raced)
	blob := must(r.Line(gitx.Opt{Stdin: strings.NewReader(name + "\n")}, "hash-object", "-w", "--stdin"))
	idx := filepath.Join(r.Dir, "tmp", "race-index-"+strconv.Itoa(os.Getpid()))
	_ = os.MkdirAll(filepath.Dir(idx), 0o777)
	defer func() { _ = os.Remove(idx) }()
	must(r.Line(gitx.Opt{Index: idx}, "read-tree", tip))
	must(r.Line(gitx.Opt{Index: idx}, "update-index", "--add", "--cacheinfo", "100644", blob, name))
	tree := must(r.WriteTree(idx))
	c := must(r.Line(gitx.Opt{Stdin: strings.NewReader("race\n")}, "commit-tree", tree, "-p", tip))
	must(r.Line(gitx.Opt{}, "update-ref", branch, c, tip))
}

// fakeAncestors replaces the process chain with GIT_MULTISTAGE_TEST_ANCESTORS:
// executable names, nearest first and comma-separated, or jsonError for a
// process table that cannot be read. An empty value is a chain without tools.
func fakeAncestors() ([]proc, bool, error) {
	v, ok := os.LookupEnv("GIT_MULTISTAGE_TEST_ANCESTORS")
	if !ok {
		return nil, false, nil
	}
	if v == jsonError {
		return nil, true, errors.New("no process table")
	}
	var chain []proc
	for i, name := range strings.Split(v, ",") {
		if name != "" {
			chain = append(chain, proc{pid: 1000 + i, ppid: 1001 + i, name: name})
		}
	}
	return chain, true, nil
}
