package ms

import (
	"fmt"
	"os"
	"strings"
)

// Exit codes. Every command uses them, so scripts and agents can branch on the status.
const (
	ExitOK       = 0
	ExitError    = 1 // a failure, or nothing to commit
	ExitUsage    = 2 // bad arguments or missing agent ID
	ExitConflict = 3 // staged changes conflict with the branch
	ExitStale    = 4 // a hunk ID does not match the current diff
	ExitRefused  = 5 // the repository is in a state that the command refuses to change
	ExitBusy     = 6 // a lock stayed held until the timeout
)

// Error is a failure with an exit code and details for JSON output.
type Error struct {
	Code    int      `json:"-"`
	Kind    string   `json:"kind"`
	Message string   `json:"message"`
	Paths   []string `json:"paths,omitempty"`
	Details []string `json:"details,omitempty"`
	Hint    string   `json:"hint,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func errorf(code int, kind, format string, args ...any) *Error {
	return &Error{Code: code, Kind: kind, Message: fmt.Sprintf(format, args...)}
}

func usageErr(format string, args ...any) *Error {
	return errorf(ExitUsage, "usage", format, args...)
}

// Hints for a coding agent leave alone the state that other agents share.
const (
	hintRetry = "retry after a few seconds; if it fails again, run `git multistage doctor` and report its output"
	hintStop  = "stop and report it; do not change branches, because other agents work in this checkout"
	// hintCodexSandbox answers a failed write to .git in the Codex sandbox.
	hintCodexSandbox = "the Codex sandbox makes .git read-only: run `git multistage` with escalated permissions, as you would `git commit`"
)

// underAgent reports whether a coding agent runs the command: it sets one
// of the variables that --agentic reads.
func underAgent() bool {
	for _, v := range []string{"CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID", "ORCA_AGENT_SESSION_ID", "PASEO_AGENT_ID"} {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}

// forAgent returns agent when a coding agent runs the command, and person
// otherwise.
func forAgent(person, agent string) string {
	if underAgent() {
		return agent
	}
	return person
}

// addCodexHint gives an error from a read-only .git in Codex its hint.
func addCodexHint(e *Error) {
	msg := strings.ToLower(e.Message)
	if e.Hint == "" && os.Getenv("CODEX_THREAD_ID") != "" &&
		(strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "read-only file system")) {
		e.Hint = hintCodexSandbox
	}
}
