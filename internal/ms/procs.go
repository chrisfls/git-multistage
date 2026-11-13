package ms

import (
	"os"
	"strconv"
	"strings"
)

// proc is one entry of the process table.
type proc struct {
	pid, ppid int
	name      string // executable base name, lowercase, without .exe
}

// ancestors returns this process's ancestors, nearest first. It fails when
// the process table cannot be read.
func ancestors() ([]proc, error) {
	if chain, ok, err := fakeAncestors(); ok {
		return chain, err
	}
	table, err := processTable()
	if err != nil {
		return nil, err
	}
	var chain []proc
	seen := map[int]bool{}
	pid := os.Getpid()
	for i := range 64 {
		p, ok := table[pid]
		if !ok || seen[pid] {
			break
		}
		seen[pid] = true
		if i > 0 {
			chain = append(chain, p)
		}
		if p.ppid <= 0 || p.ppid == pid {
			break
		}
		pid = p.ppid
	}
	return chain, nil
}

// toolAncestors returns the positions in chain of the nearest Claude Code
// and Codex processes, or -1. A Claude Code process is known by CLAUDE_PID,
// which each Claude Code process sets for its own commands, or by its
// executable name.
func toolAncestors(chain []proc) (claude, codex int) {
	claude, codex = -1, -1
	claudePID, _ := strconv.Atoi(os.Getenv("CLAUDE_PID"))
	for i, p := range chain {
		if claude < 0 && (p.pid == claudePID || p.name == agentClaude) {
			claude = i
		}
		if codex < 0 && p.name == agentCodex {
			codex = i
		}
	}
	return claude, codex
}

func procName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}
	return strings.TrimSuffix(strings.ToLower(path), ".exe")
}
