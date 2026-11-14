//go:build !windows

package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// countingGit is a fake git that appends each call to a log and prints out,
// which can hold NUL bytes.
func countingGit(t *testing.T, out string) (r *Repo, calls func() []string) {
	t.Helper()
	dir := t.TempDir()
	log, outFile := filepath.Join(dir, "calls"), filepath.Join(dir, "out")
	if err := os.WriteFile(outFile, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	r = fakeGit(t, `echo "$*" >> '`+log+`'
cat '`+outFile+`'
`)
	return r, func() []string {
		data, _ := os.ReadFile(log)
		return strings.Fields(strings.ReplaceAll(string(data), " ", "_"))
	}
}

func TestTreeOfReadsEachCommitOnce(t *testing.T) {
	r, calls := countingGit(t, "tree1\n")
	for range 3 {
		tree, err := r.TreeOf("c1")
		if err != nil || tree != "tree1" {
			t.Fatalf("TreeOf = %q, %v", tree, err)
		}
	}
	if _, err := r.TreeOf("c2"); err != nil {
		t.Fatal(err)
	}
	if tree, _ := r.TreeOf(""); tree != r.EmptyTree {
		t.Fatalf("TreeOf of no commit = %q", tree)
	}
	if got := calls(); len(got) != 2 {
		t.Fatalf("git calls %v, want one for each commit", got)
	}
}

func TestTreeOfSharesConcurrentReads(t *testing.T) {
	r, calls := countingGit(t, "tree1\n")
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tree, err := r.TreeOf("c1"); err != nil || tree != "tree1" {
				t.Errorf("TreeOf = %q, %v", tree, err)
			}
		}()
	}
	wg.Wait()
	if got := calls(); len(got) != 1 {
		t.Fatalf("git calls %v, want one", got)
	}
}

func TestTreeOfDoesNotKeepAFailure(t *testing.T) {
	r := fakeGit(t, "exit 1\n")
	for range 2 {
		if _, err := r.TreeOf("c1"); err == nil {
			t.Fatal("TreeOf of a missing commit succeeded")
		}
	}
	if len(r.trees) != 0 {
		t.Fatalf("kept %d failed reads", len(r.trees))
	}
}

func TestRememberTree(t *testing.T) {
	r, calls := countingGit(t, "read\n")
	r.RememberTree("c1", "t1")
	r.RememberTree("c1", "other") // the first tree of a commit stays
	r.RememberTree("", "t2")
	r.RememberTree("c3", "")
	if tree, err := r.TreeOf("c1"); err != nil || tree != "t1" {
		t.Fatalf("TreeOf of a remembered commit = %q, %v", tree, err)
	}
	if got := calls(); len(got) != 0 {
		t.Fatalf("git calls %v, want none", got)
	}
	if r.KnownTree("c1") != "t1" || r.KnownTree("c2") != "" || r.KnownTree("c3") != "" {
		t.Fatal("KnownTree must return only remembered and read trees")
	}
	if tree, _ := r.TreeOf("c2"); tree != "read" || r.KnownTree("c2") != "read" {
		t.Fatalf("KnownTree after a read = %q", r.KnownTree("c2"))
	}
}

func TestConfigBool(t *testing.T) {
	list := strings.Join([]string{
		"core.filemode\nfalse", "core.bare\nfalse", "a.yes\nYes", "a.on\non", "a.off\nOFF",
		"a.empty\n", "a.novalue", "a.int\n2", "a.zero\n0", "a.unit\n1k", "a.hex\n0x0",
		"a.bad\nmaybe", "a.big\n1G", "a.neg\n-1", "a.plus\n+0", "a.space\n 1", "a.underscore\n1_0", "a.twounits\n1kk",
		"a.repeated\ntrue", "a.repeated\nfalse", "sec.Sub.var\ntrue",
	}, "\x00") + "\x00"
	r, calls := countingGit(t, list)
	tests := []struct {
		key       string
		def, want bool
	}{
		{"core.fileMode", true, false},
		{"CORE.BARE", true, false},
		{"a.yes", false, true},
		{"a.on", false, true},
		{"a.off", true, false},
		{"a.empty", true, false},
		{"a.novalue", false, true},
		{"a.int", false, true},
		{"a.zero", true, false},
		{"a.unit", false, true},
		{"a.hex", true, false},
		{"a.big", false, true},
		{"a.neg", false, true},
		{"a.plus", true, false},
		{"a.space", false, true},
		{"a.bad", true, true},
		{"a.bad", false, false},
		{"a.underscore", true, true},
		{"a.twounits", true, true},
		{"a.repeated", true, false},
		{"SEC.Sub.VAR", false, true},
		{"sec.sub.var", false, false},
		{"a.missing", true, true},
		{"a.missing", false, false},
	}
	for _, tt := range tests {
		if got := r.ConfigBool(tt.key, tt.def); got != tt.want {
			t.Errorf("ConfigBool(%q, %v) = %v, want %v", tt.key, tt.def, got, tt.want)
		}
	}
	if got := calls(); len(got) != 1 {
		t.Fatalf("git calls %v, want one", got)
	}
}

func TestConfigBoolWithoutSettings(t *testing.T) {
	r := fakeGit(t, "exit 1\n")
	if !r.ConfigBool("core.fileMode", true) || r.ConfigBool("core.ignoreCase", false) {
		t.Fatal("a failed read must give the defaults")
	}
}

func TestHasHook(t *testing.T) {
	write := func(t *testing.T, path string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	abs, empty := filepath.Join(t.TempDir(), "hooks"), filepath.Join(t.TempDir(), "hooks")
	tests := []struct {
		name   string
		config []string
		files  map[string]os.FileMode // relative to the common dir ("hooks/...") or the top ("top/...")
		want   bool
	}{
		{"nothing", nil, nil, false},
		{"sample only", nil, map[string]os.FileMode{"hooks/pre-commit.sample": 0o755}, false},
		{"other hook", nil, map[string]os.FileMode{"hooks/post-commit": 0o755}, false},
		{"hook file", nil, map[string]os.FileMode{"hooks/pre-commit": 0o755}, true},
		{"not executable", nil, map[string]os.FileMode{"hooks/pre-commit": 0o644}, true},
		{"exe", nil, map[string]os.FileMode{"hooks/pre-commit.exe": 0o755}, true},
		{"hook setting", []string{"hook.lint.command\nmake lint"}, nil, true},
		{"mixed-case hook setting", []string{"Hook.lint.Event\npre-commit"}, nil, true},
		{"absolute hooksPath", []string{"core.hooksPath\n" + abs}, map[string]os.FileMode{"abs/pre-commit": 0o755}, true},
		{"absolute hooksPath without the hook", []string{"core.hooksPath\n" + empty}, map[string]os.FileMode{"hooks/pre-commit": 0o755}, false},
		{"relative hooksPath", []string{"core.hooksPath\nmy-hooks"}, map[string]os.FileMode{"top/my-hooks/pre-commit": 0o755}, true},
		{"relative hooksPath without the hook", []string{"core.hooksPath\nmy-hooks"}, nil, false},
		{"hooksPath of a file", []string{"core.hooksPath\n" + os.DevNull}, map[string]os.FileMode{"hooks/pre-commit": 0o755}, false},
		{"home hooksPath", []string{"core.hooksPath\n~/hooks"}, nil, true},
		{"prefix hooksPath", []string{"core.hooksPath\n%(prefix)/hooks"}, nil, true},
		{"empty hooksPath", []string{"core.hooksPath\n"}, nil, true},
		{"hooksPath without a value", []string{"core.hooksPath"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := strings.Join(tt.config, "\x00")
			if list != "" {
				list += "\x00"
			}
			r, _ := countingGit(t, list)
			r.CommonDir = t.TempDir()
			for f, mode := range tt.files {
				switch dir, rest, _ := strings.Cut(f, "/"); dir {
				case "hooks":
					write(t, filepath.Join(r.CommonDir, f), mode)
				case "top":
					write(t, filepath.Join(r.Top, rest), mode)
				case "abs":
					write(t, filepath.Join(abs, rest), mode)
				}
			}
			if got := r.HasHook("pre-commit"); got != tt.want {
				t.Fatalf("HasHook = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasHookWithoutSettings(t *testing.T) {
	r := fakeGit(t, "exit 1\n")
	r.CommonDir = t.TempDir()
	if !r.HasHook("pre-commit") {
		t.Fatal("a failed config read must report a hook")
	}
}
