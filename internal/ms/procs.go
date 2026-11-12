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

// ancestors returns this process's ancestors, nearest first.
func ancestors() []proc {
	table, err := processTable()
	if err != nil {
		return nil
	}
	var chain []proc
	seen := map[int]bool{}
	pid := os.Getpid()
	for i := 0; i < 64; i++ {
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
	return chain
}

// innermostTool reports which agent tool is the nearer ancestor: "claude",
// "codex", or "" when neither is found. A Claude Code process is known by
// CLAUDE_PID, which each Claude Code process sets for its own commands, or
// by its executable name.
func innermostTool() string {
	claudePID, _ := strconv.Atoi(os.Getenv("CLAUDE_PID"))
	for _, p := range ancestors() {
		if p.pid == claudePID || p.name == "claude" {
			return "claude"
		}
		if p.name == "codex" {
			return "codex"
		}
	}
	return ""
}

func procName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}
	return strings.TrimSuffix(strings.ToLower(path), ".exe")
}
