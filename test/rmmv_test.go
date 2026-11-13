package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// committedRepo returns a repository whose last commit holds files.
func committedRepo(tb testing.TB, files map[string]string) *repo {
	tb.Helper()
	r := newRepo(tb)
	for p, content := range files {
		r.write(p, content)
		r.git("add", p)
	}
	r.git("commit", "-q", "-m", "files")
	return r
}

func gone(tb testing.TB, p string) bool {
	tb.Helper()
	_, err := os.Lstat(p)
	return os.IsNotExist(err)
}

// stagedStatus returns the staged changes of agent, as "<status> <path>".
func (r *repo) stagedStatus(agent string) []string {
	r.t.Helper()
	var st statusJSON
	decode(r.t, r.mustMS(agent, "status", "--json"), &st)
	out := []string{}
	for _, s := range st.Status.Staged {
		out = append(out, s.Status+" "+s.Path)
	}
	return out
}

// stageMoves returns the moves that the metadata of stage id holds.
func (r *repo) stageMoves(id string) []string {
	r.t.Helper()
	data, err := os.ReadFile(r.path(".git/multistage/stages/" + id + "/meta.json"))
	if err != nil {
		r.t.Fatal(err)
	}
	var m struct {
		Moves []struct{ Old, New string } `json:"moves"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		r.t.Fatal(err)
	}
	out := []string{}
	for _, mv := range m.Moves {
		out = append(out, mv.Old+" -> "+mv.New)
	}
	return out
}

func TestAddRefusesAMissingFile(t *testing.T) {
	r := committedRepo(t, map[string]string{"old.go": "old\n"})
	if err := os.Remove(r.path("old.go")); err != nil {
		t.Fatal(err)
	}
	r.write("new.go", "old\n")
	for _, args := range [][]string{{"add", "old.go"}, {"add", "old.go", "new.go"}, {"add", "."}} {
		res := r.expectCode(1, "a", args...)
		if !strings.Contains(res.err, "git multistage rm") || !strings.Contains(res.err, "git multistage mv") {
			t.Fatalf("%v gives no hint at rm and mv:\n%s", args, res)
		}
		if got := r.stagedPaths("a"); len(got) != 0 {
			t.Fatalf("%v staged %v", args, got)
		}
	}
	var out struct {
		Error struct{ Kind string } `json:"error"`
	}
	decode(t, r.expectCode(1, "a", "add", "old.go", "--json"), &out)
	if out.Error.Kind != "missing-path" {
		t.Fatalf("kind %q", out.Error.Kind)
	}
}

func TestRmDeletesAFile(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		r := committedRepo(t, map[string]string{"old.go": "old\n", "keep.go": "keep\n"})
		r.write("keep.go", "unstaged\n")
		r.mustMS("b", "status")
		args := []string{"rm", "old.go"}
		if asJSON {
			args = append(args, "--json")
		}
		res := r.mustMS("a", args...)
		if asJSON {
			var out struct {
				Deleted []string `json:"deleted_paths"`
			}
			decode(t, res, &out)
			if !eq(out.Deleted, []string{"old.go"}) {
				t.Fatalf("deleted_paths %v", out.Deleted)
			}
		}
		if !gone(t, r.path("old.go")) {
			t.Fatal("old.go is still on disk")
		}
		if got := r.stagedStatus("a"); !eq(got, []string{"D old.go"}) {
			t.Fatalf("stage a holds %v", got)
		}
		if got := r.stagedPaths("b"); len(got) != 0 {
			t.Fatalf("stage b holds %v", got)
		}
		if r.read("keep.go") != "unstaged\n" {
			t.Fatal("rm changed another file")
		}
	}
}

func TestRmOfAFileAlreadyDeletedOnDisk(t *testing.T) {
	r := committedRepo(t, map[string]string{"old.go": "old\n"})
	if err := os.Remove(r.path("old.go")); err != nil {
		t.Fatal(err)
	}
	r.mustMS("a", "rm", "old.go", "--json")
	if got := r.stagedStatus("a"); !eq(got, []string{"D old.go"}) {
		t.Fatalf("stage a holds %v", got)
	}
	r.mustMS("a", "commit", "-m", "rm")
	if _, err := r.gitTry("cat-file", "-e", "main:old.go"); err == nil {
		t.Fatal("old.go is still on main")
	}
	r.checkClean()
}

func TestRmNeedsForceForAnUnstagedChange(t *testing.T) {
	r := committedRepo(t, map[string]string{"old.go": "old\n"})
	r.write("old.go", "other agent\n")
	for _, args := range [][]string{{"rm", "old.go"}, {"rm", "old.go", "--json"}} {
		res := r.expectCode(5, "a", args...)
		if !strings.Contains(res.out+res.err, "unstaged-changes") && !strings.Contains(res.err, "-f") {
			t.Fatalf("%v:\n%s", args, res)
		}
	}
	if r.read("old.go") != "other agent\n" {
		t.Fatal("the file changed")
	}
	if got := r.stagedPaths("a"); len(got) != 0 {
		t.Fatalf("stage a holds %v", got)
	}
	r.mustMS("a", "rm", "-f", "old.go")
	if !gone(t, r.path("old.go")) {
		t.Fatal("rm -f left the file")
	}
	r.expectCode(1, "a", "rm", "never.go")
}

func TestRmFinishesAfterACrash(t *testing.T) {
	r := committedRepo(t, map[string]string{"old.go": "old\n"})
	if res := run(r.dir, "a", []string{"GIT_MULTISTAGE_FAULT=before-disk-write"}, "rm", "old.go"); res.code != 86 {
		t.Fatalf("fault did not fire:\n%s", res)
	}
	if gone(t, r.path("old.go")) {
		t.Fatal("the disk changed before the stage")
	}
	r.mustMS("a", "rm", "old.go")
	if !gone(t, r.path("old.go")) {
		t.Fatal("the second run left the file")
	}
	if got := r.stagedStatus("a"); !eq(got, []string{"D old.go"}) {
		t.Fatalf("stage a holds %v", got)
	}
}

func TestMvMovesAFile(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n"})
	var out struct {
		Moved struct{ Old, New string } `json:"moved"`
	}
	decode(t, r.mustMS("a", "mv", "x.go", "sub/y.go", "--json"), &out)
	if out.Moved.Old != "x.go" || out.Moved.New != "sub/y.go" {
		t.Fatalf("moved %+v", out.Moved)
	}
	if !gone(t, r.path("x.go")) || r.read("sub/y.go") != "x\n" {
		t.Fatal("the disk does not show the move")
	}
	if got := r.stagedStatus("a"); !eq(got, []string{"R sub/y.go"}) {
		t.Fatalf("stage a holds %v", got)
	}
	if got := r.stageMoves("a"); !eq(got, []string{"x.go -> sub/y.go"}) {
		t.Fatalf("moves %v", got)
	}
	if got := r.stagedPaths("b"); len(got) != 0 {
		t.Fatalf("stage b holds %v", got)
	}
}

func TestMvOfAMoveAlreadyMadeOnDisk(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n"})
	if err := os.Rename(r.path("x.go"), r.path("y.go")); err != nil {
		t.Fatal(err)
	}
	r.write("y.go", "x\nunstaged\n")
	r.mustMS("a", "mv", "x.go", "y.go")
	if !gone(t, r.path("x.go")) || r.read("y.go") != "x\nunstaged\n" {
		t.Fatal("the disk changed")
	}
	if got := r.stageMoves("a"); !eq(got, []string{"x.go -> y.go"}) {
		t.Fatalf("moves %v", got)
	}
	if d := r.mustMS("a", "diff", "--cached", "-M").out; !strings.Contains(d, "rename to y.go") || strings.Contains(d, "unstaged") {
		t.Fatalf("staged diff:\n%s", d)
	}
}

func TestMvRefusals(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n", "y.go": "y\n"})
	r.expectCode(5, "a", "mv", "x.go", "y.go")
	r.expectCode(1, "a", "mv", "never.go", "z.go")
	if r.read("x.go") != "x\n" || r.read("y.go") != "y\n" || len(r.stagedPaths("a")) != 0 {
		t.Fatal("a refused mv changed something")
	}
}

func TestMvRecordedEditsFollow(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "1\n2\n"})
	r.hookedEdit("s", "", "x.go", "1\nTWO\n")
	r.mustMS("", "--stage", "claude-s", "mv", "x.go", "y.go")
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "move")
	if got := r.git("show", "main:y.go"); got != "1\nTWO\n" {
		t.Fatalf("y.go = %q", got)
	}
	if _, err := r.gitTry("cat-file", "-e", "main:x.go"); err == nil {
		t.Fatal("x.go is still on main")
	}
	r.checkClean()
}

func TestMvWritesOnlyItsPaths(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n", "other.go": "o\n", "dir/f.go": "f\n"})
	r.write("other.go", "o\nunstaged\n")
	r.write("dir/f.go", "f\nunstaged\n")
	r.write("untracked.go", "u\n")
	before := r.git("status", "--porcelain", "--untracked-files=all")
	r.mustMS("a", "mv", "x.go", "y.go")
	if r.read("other.go") != "o\nunstaged\n" || r.read("dir/f.go") != "f\nunstaged\n" || r.read("untracked.go") != "u\n" {
		t.Fatal("mv changed another file")
	}
	after := r.git("status", "--porcelain", "--untracked-files=all")
	want := strings.Split(strings.TrimSpace(before+" D x.go\n?? y.go"), "\n")
	got := strings.Split(strings.TrimSpace(after), "\n")
	sort.Strings(want)
	sort.Strings(got)
	if !eq(got, want) {
		t.Fatalf("git status:\n%s\nwant:\n%s", after, strings.Join(want, "\n"))
	}
}

func TestMvFinishesAfterACrash(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n"})
	if res := run(r.dir, "a", []string{"GIT_MULTISTAGE_FAULT=before-disk-write"}, "mv", "x.go", "y.go"); res.code != 86 {
		t.Fatalf("fault did not fire:\n%s", res)
	}
	r.mustMS("a", "mv", "x.go", "y.go")
	if !gone(t, r.path("x.go")) || r.read("y.go") != "x\n" {
		t.Fatal("the second run did not move the file")
	}
	if got := r.stageMoves("a"); !eq(got, []string{"x.go -> y.go"}) {
		t.Fatalf("moves %v", got)
	}
}

func TestRestoreForgetsTheMove(t *testing.T) {
	for _, p := range []string{"x.go", "y.go"} {
		r := committedRepo(t, map[string]string{"x.go": "x\n"})
		r.mustMS("a", "mv", "x.go", "y.go")
		r.mustMS("a", "restore", "--staged", p)
		if got := r.stageMoves("a"); len(got) != 0 {
			t.Fatalf("restore %s kept the moves %v", p, got)
		}
	}
	// A second move of the moved file extends the move; a move back cancels it.
	r := committedRepo(t, map[string]string{"x.go": "x\n"})
	r.mustMS("a", "mv", "x.go", "y.go")
	r.mustMS("a", "mv", "y.go", filepath.ToSlash(filepath.Join("d", "z.go")))
	if got := r.stageMoves("a"); !eq(got, []string{"x.go -> d/z.go"}) {
		t.Fatalf("moves %v", got)
	}
	r.mustMS("a", "mv", "d/z.go", "x.go")
	if got := r.stageMoves("a"); len(got) != 0 {
		t.Fatalf("moves %v", got)
	}
}

func TestRmOfAPathTheStageDoesNotHold(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n"})
	r.mustMS("a", "mv", "x.go", "y.go")
	r.write("x.go", "another agent's new file\n")
	r.expectCode(1, "a", "rm", "-f", "x.go")
	if r.read("x.go") != "another agent's new file\n" {
		t.Fatal("rm deleted the new x.go")
	}
}

func TestRmForgetsTheRecordedEdits(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty, "other.go": "o\n"})
	r.hookedEdit("a", "", "x.go", "FIRST\n"+twenty)
	r.hookedEdit("a", "", "other.go", "o\nedited\n")
	r.mustMS("", "--stage", "claude-a", "rm", "-f", "x.go")
	res := r.mustMS("", "--stage", "claude-a", "add", "--edits")
	if strings.Contains(res.out+res.err, "x.go") {
		t.Fatalf("add --edits:\n%s", res)
	}
	if got := r.stagedStatus("claude-a"); !eq(got, []string{"M other.go", "D x.go"}) {
		t.Fatalf("stage claude-a holds %v", got)
	}
}

func TestMvFinishesAChainedMoveAfterACrash(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n"})
	r.mustMS("a", "mv", "x.go", "y.go")
	if res := run(r.dir, "a", []string{"GIT_MULTISTAGE_FAULT=before-disk-write"}, "mv", "y.go", "z.go"); res.code != 86 {
		t.Fatalf("fault did not fire:\n%s", res)
	}
	r.mustMS("a", "mv", "y.go", "z.go")
	if !gone(t, r.path("y.go")) || r.read("z.go") != "x\n" {
		t.Fatal("the second run did not move the file")
	}
	if got := r.stageMoves("a"); !eq(got, []string{"x.go -> z.go"}) {
		t.Fatalf("moves %v", got)
	}
}

func TestRmFinishesAfterACrashOfAStagedFile(t *testing.T) {
	for _, tc := range []struct{ name, path string }{{"new file", "n.go"}, {"edited file", "x.go"}} {
		t.Run(tc.name, func(t *testing.T) {
			r := committedRepo(t, map[string]string{"x.go": "x\n"})
			r.write(tc.path, "staged\n")
			r.mustMS("a", "add", tc.path)
			if res := run(r.dir, "a", []string{"GIT_MULTISTAGE_FAULT=before-disk-write"}, "rm", tc.path); res.code != 86 {
				t.Fatalf("fault did not fire:\n%s", res)
			}
			r.mustMS("a", "rm", tc.path)
			if !gone(t, r.path(tc.path)) {
				t.Fatal("the second run left the file")
			}
		})
	}
}

func TestRmRerunKeepsAFileThatAnotherWriterChanged(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n"})
	if res := run(r.dir, "a", []string{"GIT_MULTISTAGE_FAULT=before-disk-write"}, "rm", "x.go"); res.code != 86 {
		t.Fatalf("fault did not fire:\n%s", res)
	}
	r.write("x.go", "another writer\n")
	r.ms("a", "rm", "x.go")
	if r.read("x.go") != "another writer\n" {
		t.Fatal("the rerun deleted another writer's file")
	}
}

func TestMvRefusesATargetOnDisk(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n"})
	r.write("y.go", "untracked\n")
	var out struct {
		Error struct{ Kind string } `json:"error"`
	}
	decode(t, r.expectCode(5, "a", "mv", "x.go", "y.go", "--json"), &out)
	if out.Error.Kind != "path-exists" || r.read("x.go") != "x\n" || r.read("y.go") != "untracked\n" {
		t.Fatalf("kind %q, or a file changed", out.Error.Kind)
	}
	// A target that appears after the stage changed is refused by the rename.
	if err := os.Remove(r.path("y.go")); err != nil {
		t.Fatal(err)
	}
	if res := run(r.dir, "a", []string{"GIT_MULTISTAGE_FAULT=before-disk-write"}, "mv", "x.go", "y.go"); res.code != 86 {
		t.Fatalf("fault did not fire:\n%s", res)
	}
	r.write("y.go", "another writer\n")
	decode(t, r.expectCode(5, "a", "mv", "x.go", "y.go", "--json"), &out)
	if out.Error.Kind != "path-exists" || r.read("x.go") != "x\n" || r.read("y.go") != "another writer\n" {
		t.Fatalf("kind %q, or a file changed", out.Error.Kind)
	}
}

func TestMvCaseOnlyRename(t *testing.T) {
	r := committedRepo(t, map[string]string{"Readme.md": "doc\n"})
	if _, err := os.Stat(r.path("README.md")); err != nil {
		t.Skip("the file system is case-sensitive")
	}
	if strings.TrimSpace(r.git("config", "--bool", "core.ignoreCase")) != "true" {
		t.Skip("core.ignoreCase is not set")
	}
	r.mustMS("a", "mv", "Readme.md", "README.md")
	names, err := os.ReadDir(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, n := range names {
		if strings.EqualFold(n.Name(), "readme.md") {
			found = append(found, n.Name())
		}
	}
	if !eq(found, []string{"README.md"}) || r.read("README.md") != "doc\n" {
		t.Fatalf("the disk holds %v", found)
	}
	if got := r.stageMoves("a"); !eq(got, []string{"Readme.md -> README.md"}) {
		t.Fatalf("moves %v", got)
	}
}
