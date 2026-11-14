//go:build !windows

package gitx

import (
	"os"
	"path/filepath"
	"strings"
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
