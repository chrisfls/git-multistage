package ms

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chrisfls/git-multistage/internal/hunks"
)

func diff(t *testing.T, a, b string) []hunks.Hunk {
	t.Helper()
	dir := t.TempDir()
	pa, pb := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(pa, []byte(a), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pb, []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.CommandContext(context.Background(), "git", "diff", "--no-index", "--no-color", "-U0", "--inter-hunk-context=0",
		"--diff-algorithm=myers", pa, pb).Output()
	hs, err := hunks.Parse("f", out, "")
	if err != nil {
		t.Fatal(err)
	}
	return hs
}

func TestReplayLines(t *testing.T) {
	base := "1\n2\n3\n4\n5\n6\n"
	for _, c := range []struct {
		name, mine, theirs, want string
		ok                       bool
	}{
		{"adjacent lines", "1\n2\nA\n4\n5\n6\n", "1\n2\n3\nB\n5\n6\n", "1\n2\nA\nB\n5\n6\n", true},
		{"same line", "1\n2\nA\n4\n5\n6\n", "1\n2\nB\n4\n5\n6\n", "", false},
		{"identical change", "1\n2\nA\n4\n5\n6\n", "1\n2\nA\n4\n5\n6\n", "1\n2\nA\n4\n5\n6\n", true},
		{"insert after their block", "1\n2\n3\nnew\n4\n5\n6\n", "1\n2\nB\n4\n5\n6\n", "", false},
		{"insert where their replacement starts", "1\n2\n3\nB\n4\n5\n6\n", "1\n2\n3\nB\nC\n5\n6\n", "", false},
		{"their replacement ends where mine inserts", "1\n2\nB\n3\n4\n5\n6\n", "1\nX\nB\n3\n4\n5\n6\n", "", false},
		{"their insertion next to my replacement", "1\n2\nA\n4\n5\n6\n", "1\n2\nnew\n3\n4\n5\n6\n", "", false},
		{"insert at their insertion point", "1\n2\n3\nmine\n4\n5\n6\n", "1\n2\n3\ntheirs\n4\n5\n6\n", "", false},
		{"theirs inserted lines above", "1\n2\n3\n4\nA\n6\n", "0\n1\n2\n3\n4\n5\n6\n", "0\n1\n2\n3\n4\nA\n6\n", true},
		{"theirs deleted a neighbour", "1\n2\nA\n4\n5\n6\n", "1\n2\n3\n5\n6\n", "1\n2\nA\n5\n6\n", true},
		{"insertion inside their replaced block", "1\n2\n3\nmine\n4\n5\n6\n", "1\n2\nX\nY\n5\n6\n", "", false},
	} {
		got, ok := replayLines([]byte(base), []byte(c.theirs), diff(t, base, c.mine), diff(t, base, c.theirs))
		if ok != c.ok || ok && string(got) != c.want {
			t.Errorf("%s: got %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestReplayRefusesToJoinLines(t *testing.T) {
	base := "a\nx\n"
	theirs := "a\ny"
	got, ok := replayLines([]byte(base), []byte(theirs), diff(t, base, "a\nx\nnew\n"), diff(t, base, theirs))
	if ok {
		t.Fatalf("joined lines: %q", got)
	}
}
