package ms

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// traced returns a repository that traces every Git process it starts into
// the returned file, and a function that returns the subcommands of the
// processes started since its last call.
func traced(t *testing.T, refFormat string) (*Repo, func() []string) {
	t.Helper()
	dir := t.TempDir()
	trace := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE2_EVENT", trace)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, nil, "init", "-q", "--ref-format="+refFormat)
	git(t, dir, nil, "add", ".")
	git(t, dir, nil, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init")
	r, err := OpenRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r, func() []string {
		t.Helper()
		data, err := os.ReadFile(trace)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(trace, 0); err != nil {
			t.Fatal(err)
		}
		var subs []string
		for _, line := range strings.Split(string(data), "\n") {
			var e struct {
				Event string
				Argv  []string
			}
			if json.Unmarshal([]byte(line), &e) != nil || e.Event != "start" {
				continue
			}
			for i := 1; i < len(e.Argv); i++ {
				if e.Argv[i] == "-c" {
					i++
				} else if !strings.HasPrefix(e.Argv[i], "-") {
					subs = append(subs, e.Argv[i])
					break
				}
			}
		}
		return subs
	}
}

// git runs git in dir with extra environment entries and returns its output.
func git(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func countOf(subs []string, name string) (n int) {
	for _, s := range subs {
		if s == name {
			n++
		}
	}
	return n
}

// stageWithFile opens stage "k" and stages a new file in it.
func stageWithFile(t *testing.T, r *Repo) *Stage {
	t.Helper()
	l, err := r.OpenStage("k", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	if err := os.WriteFile(filepath.Join(r.Top, "new"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r.Top, []string{"GIT_INDEX_FILE=" + l.IndexPath()}, "add", "new")
	return l
}

func keepRefFile(l *Stage) string {
	return filepath.Join(l.r.GitDir, "refs", "worktree", "multistage", l.ID)
}

func mustProtect(t *testing.T, l *Stage) {
	t.Helper()
	if err := l.protect(); err != nil {
		t.Fatal(err)
	}
}

func TestKeepRecordSavesTheShow(t *testing.T) {
	r, started := traced(t, "files")
	l := stageWithFile(t, r)
	mustProtect(t, l)
	ref := git(t, r.Top, nil, "rev-parse", l.keepRef())
	rec, ok := l.readKeepRecord()
	if !ok || rec.commit != ref || rec.parents != l.M.Base || !strings.HasPrefix(rec.digest, keepDigestPrefix) {
		t.Fatalf("keep record %+v, %v; keep ref %s, base %s", rec, ok, ref, l.M.Base)
	}

	started()
	mustProtect(t, l)
	if subs := started(); len(subs) != 0 {
		t.Fatalf("protect of an unchanged stage started %v", subs)
	}
}

func TestKeepRefMissingFile(t *testing.T) {
	r, started := traced(t, "files")
	l := stageWithFile(t, r)
	mustProtect(t, l)
	git(t, r.Top, nil, "update-ref", "-d", l.keepRef())
	if _, err := os.Stat(keepRefFile(l)); !os.IsNotExist(err) {
		t.Fatalf("keep ref file after update-ref -d: %v", err)
	}
	started()
	mustProtect(t, l)
	subs := started()
	if countOf(subs, "show") != 0 || countOf(subs, "update-ref") != 1 {
		t.Fatalf("protect over a missing keep ref started %v, want no show and one update-ref", subs)
	}
	if got := git(t, r.Top, nil, "rev-parse", l.keepRef()+"^{tree}"); got == "" {
		t.Fatal("protect did not write a keep ref")
	}
	rec, _ := l.readKeepRecord()
	if now := git(t, r.Top, nil, "rev-parse", l.keepRef()); rec.commit != now {
		t.Fatalf("keep record names %s, keep ref names %s", rec.commit, now)
	}
}

func TestKeepRefNamingAnotherCommit(t *testing.T) {
	r, started := traced(t, "files")
	l := stageWithFile(t, r)
	mustProtect(t, l)
	tree := git(t, r.Top, nil, "rev-parse", l.keepRef()+"^{tree}")
	other := git(t, r.Top, []string{"GIT_AUTHOR_NAME=o", "GIT_AUTHOR_EMAIL=o@o", "GIT_COMMITTER_NAME=o", "GIT_COMMITTER_EMAIL=o@o"},
		"commit-tree", tree, "-p", l.M.Base, "-m", "someone else's keep commit")
	git(t, r.Top, nil, "update-ref", l.keepRef(), other)

	started()
	mustProtect(t, l)
	subs := started()
	if countOf(subs, "show") != 1 {
		t.Fatalf("protect over a changed keep ref started %v, want one show", subs)
	}
	// The other commit has no digest, so protect replaced it.
	now := git(t, r.Top, nil, "rev-parse", l.keepRef())
	if now == other {
		t.Fatal("protect kept a keep commit without the digest")
	}
	if rec, ok := l.readKeepRecord(); !ok || rec.commit != now {
		t.Fatalf("keep record %+v, %v; keep ref names %s", rec, ok, now)
	}
	started()
	mustProtect(t, l)
	if subs := started(); len(subs) != 0 {
		t.Fatalf("protect after the rewrite started %v", subs)
	}
}

func TestKeepCommitOfAnOlderVersionGetsARecord(t *testing.T) {
	r, started := traced(t, "files")
	l := stageWithFile(t, r)
	mustProtect(t, l)
	if err := os.Remove(l.keepRecordPath()); err != nil {
		t.Fatal(err)
	}
	ref := git(t, r.Top, nil, "rev-parse", l.keepRef())

	started()
	mustProtect(t, l)
	if subs := started(); countOf(subs, "show") != 1 || countOf(subs, "mktree") != 0 || countOf(subs, "update-ref") != 0 {
		t.Fatalf("protect over a keep commit without a record started %v, want one show", subs)
	}
	if rec, ok := l.readKeepRecord(); !ok || rec.commit != ref {
		t.Fatalf("keep record %+v, %v; keep ref names %s", rec, ok, ref)
	}
	mustProtect(t, l)
	if subs := started(); len(subs) != 0 {
		t.Fatalf("protect after the record started %v", subs)
	}
	// A damaged record runs show, too.
	if err := os.WriteFile(l.keepRecordPath(), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustProtect(t, l)
	if subs := started(); countOf(subs, "show") != 1 {
		t.Fatalf("protect over a damaged record started %v, want one show", subs)
	}
}

func TestKeepRefStaysLooseAfterPackRefs(t *testing.T) {
	r, started := traced(t, "files")
	l := stageWithFile(t, r)
	mustProtect(t, l)
	git(t, r.Top, nil, "pack-refs", "--all")
	if _, err := os.Stat(keepRefFile(l)); err != nil {
		t.Fatalf("the keep ref is not a loose file after pack-refs --all: %v", err)
	}
	started()
	mustProtect(t, l)
	if subs := started(); len(subs) != 0 {
		t.Fatalf("protect after pack-refs --all started %v", subs)
	}
}

func TestKeepRefOfAStageWithoutChangesIsDeletedWithItsRecord(t *testing.T) {
	r, _ := traced(t, "files")
	l := stageWithFile(t, r)
	mustProtect(t, l)
	git(t, r.Top, []string{"GIT_INDEX_FILE=" + l.IndexPath()}, "reset", "-q")
	mustProtect(t, l)
	if out, err := exec.CommandContext(context.Background(), "git", "-C", r.Top, "rev-parse", "-q", "--verify", l.keepRef()).Output(); err == nil {
		t.Fatalf("keep ref still names %s", out)
	}
	if _, err := os.Stat(l.keepRecordPath()); !os.IsNotExist(err) {
		t.Fatalf("keep record after the delete: %v", err)
	}
}

func TestKeepRecordWithReftable(t *testing.T) {
	r, started := traced(t, "reftable")
	l := stageWithFile(t, r)
	mustProtect(t, l)
	started()
	mustProtect(t, l)
	if subs := started(); countOf(subs, "show") != 1 || countOf(subs, "update-ref") != 0 {
		t.Fatalf("protect with reftable started %v, want one show", subs)
	}
}
