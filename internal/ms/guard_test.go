package ms

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestGitCalls(t *testing.T) {
	t.Parallel()
	cwd := filepath.FromSlash("/w/repo")
	at := filepath.FromSlash
	for _, tc := range []struct {
		command string
		want    []gitCall
	}{
		{"git stash", []gitCall{{cwd, "stash", []string{}}}},
		{"git status && git stash list", []gitCall{{cwd, "status", []string{}}, {cwd, "stash", []string{"list"}}}},
		{"FOO=1 env git -c core.x=y --no-pager commit -m 'a; b'", []gitCall{{cwd, "commit", []string{"-m", "a; b"}}}},
		{"cd ../other && git add x.go", []gitCall{{at("/w/other"), "add", []string{"x.go"}}}},
		{"git -C /tmp/scratch commit -m x", []gitCall{{at("/tmp/scratch"), "commit", []string{"-m", "x"}}}},
		{"(git switch main) | cat", []gitCall{{cwd, "switch", []string{"main"}}}},
		{"echo \"git stash\"; /usr/bin/git restore f.txt", []gitCall{{cwd, "restore", []string{"f.txt"}}}},
		{"git multistage --stage a add --edits", []gitCall{{cwd, "multistage", []string{"--stage", "a", "add", "--edits"}}}},
		{"ls -la", nil},
	} {
		got := gitCalls(tc.command, cwd)
		for i := range got {
			if got[i].args == nil {
				got[i].args = []string{}
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("gitCalls(%q) = %+v, want %+v", tc.command, got, tc.want)
		}
	}
}
