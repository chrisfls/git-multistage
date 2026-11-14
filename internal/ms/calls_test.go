package ms

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// traceGitCalls makes every Git process of this test write its trace2 events,
// and returns a function that lists the Git commands started since the last
// call, such as "diff-index". A test that uses it cannot run in parallel.
func traceGitCalls(t *testing.T) func() []string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "trace2")
	t.Setenv("GIT_TRACE2_EVENT", file)
	return func() []string {
		f, err := os.Open(file)
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()
		var cmds []string
		sc := bufio.NewScanner(f)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			var e struct {
				Event string   `json:"event"`
				Argv  []string `json:"argv"`
			}
			if json.Unmarshal(sc.Bytes(), &e) != nil || e.Event != "start" || len(e.Argv) < 2 {
				continue
			}
			args := e.Argv[1:]
			for len(args) > 1 && args[0] == "-c" {
				args = args[2:]
			}
			if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
				cmds = append(cmds, args[0])
			}
		}
		_ = os.Truncate(file, 0)
		return cmds
	}
}

// count returns how many times cmd is in cmds.
func count(cmds []string, cmd string) int {
	n := 0
	for _, c := range cmds {
		if c == cmd {
			n++
		}
	}
	return n
}
