package hunks

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitDiff returns `git diff --no-index -U0` of old and updated.
func gitDiff(t *testing.T, old, updated string) []byte {
	t.Helper()
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	// The flags of production (internal/ms diffBlobs), and no user or
	// system configuration.
	cmd := exec.CommandContext(context.Background(), "git", "diff", "--no-index", "--no-color", "--no-ext-diff", "--no-textconv",
		"-U0", "--inter-hunk-context=0", "--diff-algorithm=myers", a, b)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_DIFF_OPTS=")
	out, err := cmd.Output()
	var ee *exec.ExitError
	if err != nil && (!errors.As(err, &ee) || ee.ExitCode() != 1) {
		t.Fatalf("git diff: %v", err)
	}
	return out
}

// mustParse parses the production diff of old and updated. The updated
// content stands for the versions that Parse takes: the stage's base does not
// change in these tests.
func mustParse(t *testing.T, path, old, updated string) []Hunk {
	t.Helper()
	hs, err := Parse(path, gitDiff(t, old, updated), updated, NewSide)
	if err != nil {
		t.Fatal(err)
	}
	return hs
}

func TestParseAndApply(t *testing.T) {
	old := "one\ntwo\nthree\nfour\nfive\nsix\n"
	updated := "one\nTWO\nthree\nfour\nfive\nsix\nseven"
	hs, err := Parse("f", gitDiff(t, old, updated), updated, NewSide)
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 2 {
		t.Fatalf("got %d hunks, want 2", len(hs))
	}
	got, err := Apply([]byte(old), hs[1:])
	if err != nil || string(got) != "one\ntwo\nthree\nfour\nfive\nsix\nseven" {
		t.Fatalf("Apply second hunk = %q, %v", got, err)
	}
	got, err = Apply([]byte(old), hs[:1])
	if err != nil || string(got) != "one\nTWO\nthree\nfour\nfive\nsix\n" {
		t.Fatalf("Apply first hunk = %q, %v", got, err)
	}
}

func TestApplyEmptiesFile(t *testing.T) {
	old := "one\ntwo\n"
	got, err := Apply([]byte(old), mustParse(t, "f", old, ""))
	if err != nil || len(got) != 0 {
		t.Fatalf("Apply = %q, %v; want no content", got, err)
	}
	if got, err := Apply(nil, nil); err != nil || len(got) != 0 {
		t.Fatalf("Apply of nothing = %q, %v; want no content", got, err)
	}
}

func TestIDsDependOnContentNotPosition(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\n"
	h1 := mustParse(t, "f", old, "a\nB\nc\nd\ne\nF\n")
	// After the first hunk is applied, the second keeps its identifier.
	h2 := mustParse(t, "f", "a\nB\nc\nd\ne\nf\n", "a\nB\nc\nd\ne\nF\n")
	if h1[1].ID != h2[0].ID {
		t.Fatalf("id changed: %s vs %s", h1[1].ID, h2[0].ID)
	}
	other := mustParse(t, "g", old, "a\nB\nc\nd\ne\nF\n")
	if other[0].ID == h1[0].ID {
		t.Fatal("the same change in another path must get another id")
	}
}

func TestDuplicateHunksGetDistinctIDs(t *testing.T) {
	hs := mustParse(t, "f", "a\nx\nb\nx\nc\n", "a\ny\nb\ny\nc\n")
	if len(hs) != 2 || hs[0].ID == hs[1].ID {
		t.Fatalf("ids = %v", []string{hs[0].ID, hs[1].ID})
	}
	single := mustParse(t, "f", "a\nx\nb\n", "a\ny\nb\n")
	if single[0].ID == hs[0].ID || single[0].ID == hs[1].ID {
		t.Fatal("a duplicate hunk must not keep the ID of the single hunk")
	}
}

func TestBinary(t *testing.T) {
	if _, err := Parse("f", gitDiff(t, "a\x00b", "a\x00c"), "", NewSide); !errors.Is(err, ErrBinary) {
		t.Fatalf("err = %v, want ErrBinary", err)
	}
}

// TestRandomSubsets checks that applying any subset of hunks gives content
// whose diff from old is exactly that subset.
func TestRandomSubsets(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // a seeded generator makes the test cases repeatable
	words := []string{"alpha", "beta", "gamma", "delta", "eps", "zeta"}
	gen := func(n int) string {
		var b strings.Builder
		for i := range n {
			b.WriteString(words[rng.Intn(len(words))])
			if i < n-1 || rng.Intn(4) > 0 {
				b.WriteString("\n")
			}
		}
		return b.String()
	}
	for range 60 {
		old, updated := gen(rng.Intn(20)), gen(rng.Intn(20))
		if old == updated {
			continue
		}
		checkSubset(t, rng, old, updated)
	}
}

// checkSubset applies all hunks from old to updated, then a random subset,
// then the rest on top of the subset.
func checkSubset(t *testing.T, rng *rand.Rand, old, updated string) {
	t.Helper()
	hs, err := Parse("f", gitDiff(t, old, updated), updated, NewSide)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	all, err := Apply([]byte(old), hs)
	if err != nil || string(all) != updated {
		t.Fatalf("apply all: %q, %v; old=%q new=%q", all, err, old, updated)
	}
	var chosen []Hunk
	for _, h := range hs {
		if rng.Intn(2) == 0 {
			chosen = append(chosen, h)
		}
	}
	part, err := Apply([]byte(old), chosen)
	if err != nil {
		t.Fatalf("apply subset: %v", err)
	}
	// The rest of the hunks, applied on top of part, must reach updated.
	rest, err := Parse("f", gitDiff(t, string(part), updated), updated, NewSide)
	if err != nil {
		t.Fatal(err)
	}
	final, err := Apply(part, rest)
	if err != nil || string(final) != updated {
		t.Fatalf("apply rest: %q, %v", final, err)
	}
}

// TestShiftedCopiesGetNewIDs lists identical hunks, then lists them again
// after lines above them were deleted and another copy was added below, so
// that each copy now sits where another copy sat. No old ID may name a copy.
func TestShiftedCopiesGetNewIDs(t *testing.T) {
	ids := func(old string) map[string]bool {
		updated := strings.ReplaceAll(old, "x\n", "y\n")
		hs, err := Parse("f", gitDiff(t, old, updated), updated, NewSide)
		if err != nil {
			t.Fatal(err)
		}
		set := map[string]bool{}
		for _, h := range hs {
			set[h.ID] = true
		}
		if len(set) != 3 {
			t.Fatalf("want 3 distinct IDs, got %v", hs)
		}
		return set
	}
	before := ids("p1\np2\nx\np4\np5\nx\np7\np8\nx\np10\n")
	after := ids("p4\np5\nx\np7\np8\nx\np10\np11\nx\np13\n")
	for id := range after {
		if before[id] {
			t.Fatalf("ID %s names another copy after the shift", id)
		}
	}
	// Without a shift, the IDs stay.
	again := ids("p1\np2\nx\np4\np5\nx\np7\np8\nx\np10\n")
	for id := range again {
		if !before[id] {
			t.Fatalf("ID %s changed without a change", id)
		}
	}
}

// TestCopyIDsSurviveStagingAnotherCopy stages one of several identical
// hunks, then lists the hunks again with the same versions. Every other copy
// keeps its ID, as its ID or as its copy ID when it is the last copy.
func TestCopyIDsSurviveStagingAnotherCopy(t *testing.T) {
	for _, tc := range []struct{ name, old, updated string }{
		{"three copies", "a\nx\nb\nx\nc\nx\nd\n", "a\ny\nb\ny\nc\ny\nd\n"},
		{"two copies of a longer change", "a\nx\nb\nx\nc\n", "a\ny\nY\nb\ny\nY\nc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs := mustParse(t, "f", tc.old, tc.updated)
			staged, err := Apply([]byte(tc.old), hs[:1])
			if err != nil {
				t.Fatal(err)
			}
			rest := mustParse(t, "f", string(staged), tc.updated)
			if len(rest) != len(hs)-1 {
				t.Fatalf("got %d hunks after staging one of %d", len(rest), len(hs))
			}
			for i, h := range rest {
				if want := hs[i+1].ID; h.ID != want && h.CopyID != want {
					t.Fatalf("copy %d: ID %s, copy ID %s, want %s", i+2, h.ID, h.CopyID, want)
				}
			}
		})
	}
}

// TestPeriodicShiftChangesCopyIDs lists identical hunks whose neighbors are
// also identical, before and after a shift by the spacing of the copies, so
// that each copy has the position and the neighbors that the next copy had.
// The new content differs, so no old ID may name a copy.
func TestPeriodicShiftChangesCopyIDs(t *testing.T) {
	block := "h\nx\nt\n"
	parse := func(old string) []Hunk {
		hs := mustParse(t, "f", old, strings.ReplaceAll(old, "x\n", "y\n"))
		if len(hs) != 3 {
			t.Fatalf("got %d hunks", len(hs))
		}
		return hs
	}
	before := parse("A\nB\nC\n" + block + block + block)
	after := parse(block + block + block)
	for _, a := range after {
		for _, b := range before {
			if a.ID == b.ID || a.CopyID == b.ID {
				t.Fatalf("ID %s of a copy names another copy after the shift", b.ID)
			}
		}
	}
}
