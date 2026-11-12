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
	out, err := exec.CommandContext(context.Background(), "git", "diff", "--no-index", "--no-color", "-U0",
		"--diff-algorithm=myers", a, b).Output()
	if err != nil && len(out) == 0 && old != updated {
		t.Fatalf("git diff: %v", err)
	}
	return out
}

func TestParseAndApply(t *testing.T) {
	old := "one\ntwo\nthree\nfour\nfive\nsix\n"
	updated := "one\nTWO\nthree\nfour\nfive\nsix\nseven"
	hs, err := Parse("f", gitDiff(t, old, updated))
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

func TestDuplicateHunksGetDistinctIDs(t *testing.T) {
	hs, _ := Parse("f", gitDiff(t, "a\nx\nb\nx\nc\n", "a\ny\nb\ny\nc\n"))
	if len(hs) != 2 || hs[0].ID == hs[1].ID {
		t.Fatalf("ids = %v", []string{hs[0].ID, hs[1].ID})
	}
	single, _ := Parse("f", gitDiff(t, "a\nx\nb\n", "a\ny\nb\n"))
	if single[0].ID == hs[0].ID || single[0].ID == hs[1].ID {
		t.Fatal("a duplicate hunk must not keep the ID of the single hunk")
	}
}

func TestBinary(t *testing.T) {
	if _, err := Parse("f", gitDiff(t, "a\x00b", "a\x00c")); !errors.Is(err, ErrBinary) {
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
		hs, err := Parse("f", gitDiff(t, old, updated))
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
		rest, err := Parse("f", gitDiff(t, string(part), updated))
		if err != nil {
			t.Fatal(err)
		}
		final, err := Apply(part, rest)
		if err != nil || string(final) != updated {
			t.Fatalf("apply rest: %q, %v", final, err)
		}
	}
}
