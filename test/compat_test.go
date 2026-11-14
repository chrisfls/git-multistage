package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func (r *repo) hook(name, script string) {
	r.t.Helper()
	p := r.path(".git/hooks/" + name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		r.t.Fatal(err)
	}
}

func TestHooksRunAgainstTheCommittedTree(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.hook("pre-commit", `if git diff --cached --name-only | grep -q forbidden; then echo "forbidden file" >&2; exit 1; fi
`)
	r.hook("commit-msg", `printf '\nHooked: yes\n' >> "$1"
`)
	r.hook("post-commit", `echo ran > "$(git rev-parse --git-dir)/post-commit-ran"
`)
	r.write("forbidden.txt", "x\n")
	r.mustMS("agent-a", "add", "forbidden.txt")
	res := r.expectCode(1, "agent-a", "commit", "-m", "bad")
	if !strings.Contains(res.err, "forbidden file") || !strings.Contains(res.err, "pre-commit hook failed") {
		t.Fatalf("hook failure:\n%s", res)
	}
	if got := r.stagedPaths("agent-a"); !eq(got, []string{"forbidden.txt"}) {
		t.Fatalf("staged %v after the hook failed", got)
	}
	if len(r.commits()) != 1 {
		t.Fatal("a failed hook still committed")
	}
	r.mustMS("agent-a", "commit", "--no-verify", "-m", "skip hooks")
	if strings.Contains(r.git("log", "-1", "--format=%B"), "Hooked") {
		t.Fatal("--no-verify still ran commit-msg")
	}

	r.write("ok.txt", "ok\n")
	r.mustMS("agent-a", "add", "ok.txt")
	r.mustMS("agent-a", "commit", "-m", "fine")
	if !strings.Contains(r.git("log", "-1", "--format=%B"), "Hooked: yes") {
		t.Fatal("commit-msg hook did not run")
	}
	if _, err := os.Stat(r.path(".git/post-commit-ran")); err != nil {
		t.Fatal("post-commit hook did not run")
	}
}

// TestHooksFromHooksPathAndSettings runs hooks that are not in .git/hooks:
// a relative core.hooksPath, read from the worktree top, and a hook.* setting.
func TestHooksFromHooksPathAndSettings(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("my-hooks/prepare-commit-msg", "#!/bin/sh\nprintf '\\nPrepared: yes\\n' >> \"$1\"\n")
	r.write("my-hooks/pre-commit", "#!/bin/sh\necho hooks-path-hook >&2\nexit 1\n")
	for _, f := range []string{"my-hooks/prepare-commit-msg", "my-hooks/pre-commit"} {
		if err := os.Chmod(r.path(f), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r.git("config", "core.hooksPath", "my-hooks")
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	res := r.expectCode(1, "agent-a", "commit", "-m", "a")
	if !strings.Contains(res.err, "hooks-path-hook") || !strings.Contains(res.err, "pre-commit hook failed") {
		t.Fatalf("core.hooksPath pre-commit:\n%s", res)
	}
	r.mustMS("agent-a", "commit", "--no-verify", "-m", "a")
	if !strings.Contains(r.git("log", "-1", "--format=%B"), "Prepared: yes") {
		t.Fatal("the prepare-commit-msg hook of core.hooksPath did not run")
	}

	r.git("config", "--unset", "core.hooksPath")
	r.git("config", "hook.lint.event", "pre-commit")
	r.git("config", "hook.lint.command", "echo setting-hook >&2; exit 1")
	r.write("b.txt", "b\n")
	r.mustMS("agent-a", "add", "b.txt")
	res = r.expectCode(1, "agent-a", "commit", "-m", "b")
	if !strings.Contains(res.err, "setting-hook") || !strings.Contains(res.err, "pre-commit hook failed") {
		t.Fatalf("hook.* pre-commit:\n%s", res)
	}
	r.git("config", "--unset", "hook.lint.command")
	r.git("config", "--unset", "hook.lint.event")
	r.mustMS("agent-a", "commit", "-m", "b")
	if got := r.git("log", "-1", "--format=%s"); strings.TrimSpace(got) != "b" {
		t.Fatalf("commit without hooks: subject %q", got)
	}
}

func TestPreCommitHookCannotAddUnstagedPaths(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.hook("pre-commit", `echo sneaky > sneaky.txt && git add sneaky.txt
`)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	res := r.expectCode(1, "agent-a", "commit", "-m", "a")
	if !strings.Contains(res.err, "pre-commit hook changed the staged content") {
		t.Fatalf("error:\n%s", res)
	}
	if len(r.commits()) != 1 {
		t.Fatal("committed anyway")
	}
}

func TestSignedCommits(t *testing.T) {
	t.Parallel()
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not available")
	}
	r := newRepo(t)
	key := filepath.Join(t.TempDir(), "key")
	if out, err := exec.CommandContext(context.Background(), keygen, "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Skipf("ssh-keygen: %v %s", err, out)
	}
	r.git("config", "gpg.format", "ssh")
	r.git("config", "user.signingkey", key+".pub")
	r.git("config", "commit.gpgSign", "true")
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "signed")
	if !strings.Contains(r.git("cat-file", "-p", "HEAD"), "gpgsig") {
		t.Fatal("commit is not signed")
	}
}

func TestLineEndingNormalization(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write(".gitattributes", "*.txt text eol=crlf\n")
	r.write("f.txt", "a\r\nb\r\nc\r\nd\r\ne\r\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "crlf")
	if r.git("show", "HEAD:f.txt") != "a\nb\nc\nd\ne\n" {
		t.Fatal("setup: blob not normalized")
	}
	r.write("f.txt", "a\r\nB\r\nc\r\nD\r\ne\r\n")
	r.mustMS("agent-a", "add", "f.txt", "--hunk", r.hunkID("agent-a", "f.txt", "B"))
	r.mustMS("agent-a", "commit", "-m", "b")
	if got := r.git("show", "HEAD:f.txt"); got != "a\nB\nc\nd\ne\n" {
		t.Fatalf("committed %q", got)
	}
}

func TestFilteredFilesStageWhole(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write(".gitattributes", "*.big filter=lfs diff=lfs merge=lfs -text\n")
	r.write("data.big", "one\ntwo\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "lfs")
	r.write("data.big", "one\nTWO\n")
	var fj filesJSON
	decode(t, r.mustMS("agent-a", "hunks", "--json", "data.big"), &fj)
	if len(fj.Files) != 1 || len(fj.Files[0].Hunks) != 0 || !strings.Contains(fj.Files[0].Note, "lfs") {
		t.Fatalf("listing: %+v", fj)
	}
	r.mustMS("agent-a", "add", "data.big")
	if got := r.stagedPaths("agent-a"); !eq(got, []string{"data.big"}) {
		t.Fatalf("staged %v", got)
	}
}

func TestDoctorAndManualSync(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.git("config", "multistage.syncIndex", "false")
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "a")
	if out := r.git("diff", "--cached", "--name-only"); out == "" {
		t.Fatal("setup: the default index should lag behind")
	}
	doc := r.ms("", "doctor")
	if !strings.Contains(doc.out, "behind refs/heads/main") || !strings.Contains(doc.out, "a.txt") {
		t.Fatalf("doctor:\n%s", doc)
	}
	r.mustMS("", "sync")
	r.checkClean()
	if doc := r.mustMS("", "doctor"); !strings.Contains(doc.out, "in step") {
		t.Fatalf("doctor after sync:\n%s", doc)
	}
}
