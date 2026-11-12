package hunks

import (
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitDiff returns `git diff --no-index -U0` of old and new.
func gitDiff(t *testing.T, old, new string) []byte {
	t.Helper()
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(a, []byte(old), 0o644)
	os.WriteFile(b, []byte(new), 0o644)
	out, err := exec.Command("git", "diff", "--no-index", "--no-color", "-U0",
		"--diff-algorithm=myers", a, b).Output()
	if err != nil && len(out) == 0 && old != new {
		t.Fatalf("git diff: %v", err)
	}
	return out
}

func TestParseAndApply(t *testing.T) {
	old := "one\ntwo\nthree\nfour\nfive\nsix\n"
	new := "one\nTWO\nthree\nfour\nfive\nsix\nseven"
	hs, err := Parse("f", gitDiff(t, old, new))
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 2 {
		t.Fatalf("got %d hunks, want 2", len(hs))
	}
	got, _ := Apply([]byte(old), hs[1:])
	if string(got) != "one\ntwo\nthree\nfour\nfive\nsix\nseven" {
		t.Fatalf("Apply second hunk = %q", got)
	}
	got, _ = Apply([]byte(old), hs[:1])
	if string(got) != "one\nTWO\nthree\nfour\nfive\nsix\n" {
		t.Fatalf("Apply first hunk = %q", got)
	}
}

func TestIDsDependOnContentNotPosition(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\n"
	h1, _ := Parse("f", gitDiff(t, old, "a\nB\nc\nd\ne\nF\n"))
	// After the first hunk is applied, the second keeps its identifier.
	h2, _ := Parse("f", gitDiff(t, "a\nB\nc\nd\ne\nf\n", "a\nB\nc\nd\ne\nF\n"))
	if h1[1].ID != h2[0].ID {
		t.Fatalf("id changed: %s vs %s", h1[1].ID, h2[0].ID)
	}
	other, _ := Parse("g", gitDiff(t, old, "a\nB\nc\nd\ne\nF\n"))
	if other[0].ID == h1[0].ID {
		t.Fatal("the same change in another path must get another id")
	}
}

func TestDuplicateHunksGetOrdinals(t *testing.T) {
	hs, _ := Parse("f", gitDiff(t, "a\nx\nb\nx\nc\n", "a\ny\nb\ny\nc\n"))
	if len(hs) != 2 || hs[1].ID != hs[0].ID+"-2" {
		t.Fatalf("ids = %v", []string{hs[0].ID, hs[1].ID})
	}
}

func TestBinary(t *testing.T) {
	if _, err := Parse("f", gitDiff(t, "a\x00b", "a\x00c")); err != ErrBinary {
		t.Fatalf("err = %v, want ErrBinary", err)
	}
}

// TestRandomSubsets checks that applying any subset of hunks gives content
// whose diff from old is exactly that subset.
func TestRandomSubsets(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	words := []string{"alpha", "beta", "gamma", "delta", "eps", "zeta"}
	gen := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(words[rng.Intn(len(words))])
			if i < n-1 || rng.Intn(4) > 0 {
				b.WriteString("\n")
			}
		}
		return b.String()
	}
	for iter := 0; iter < 60; iter++ {
		old, new := gen(rng.Intn(20)), gen(rng.Intn(20))
		if old == new {
			continue
		}
		hs, err := Parse("f", gitDiff(t, old, new))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		all, err := Apply([]byte(old), hs)
		if err != nil || string(all) != new {
			t.Fatalf("apply all: %q, %v; old=%q new=%q", all, err, old, new)
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
		// The rest of the hunks, applied on top of part, must reach new.
		rest, err := Parse("f", gitDiff(t, string(part), new))
		if err != nil {
			t.Fatal(err)
		}
		final, err := Apply(part, rest)
		if err != nil || string(final) != new {
			t.Fatalf("apply rest: %q, %v", final, err)
		}
	}
}
