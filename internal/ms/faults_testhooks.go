//go:build testhooks

package ms

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
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

// testDelay sleeps for a random time up to GIT_MULTISTAGE_TEST_DELAY_MS.
func testDelay() {
	if n, _ := strconv.Atoi(os.Getenv("GIT_MULTISTAGE_TEST_DELAY_MS")); n > 0 {
		time.Sleep(time.Duration(rand.Intn(n)) * time.Millisecond) //nolint:gosec // a test delay needs no cryptographic randomness
	}
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
