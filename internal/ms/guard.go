package ms

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The shell command guard runs as a PreToolUse hook on the agent's shell
// tool. In an opted-in checkout it denies the Git commands that change state
// that every agent shares, and says what to do instead. It reads the words of
// the command, not the shell's meaning: `sh -c`, aliases, and scripts pass,
// and the rules of the session start notice cover them.

// shellTools are the tool names of the agents' shell tools.
var shellTools = map[string]bool{"Bash": true}

// guardedGit maps each Git command that the guard denies to what to do
// instead.
var guardedGit = map[string]string{
	cmdAdd:     "Stage with `git multistage --stage <id> add --edits`, `add <path>`, or `add <path> --hunk <id>`.",
	cmdCommit:  "Commit with `git multistage --stage <id> commit -m <message>`.",
	cmdRm:      "Delete with `git multistage --stage <id> rm <path>`.",
	cmdMv:      "Move with `git multistage --stage <id> mv <old> <new>`.",
	"stash":    "It would hide the other agents' uncommitted changes. To work on a commit without them, use a separate worktree from `git worktree add`.",
	"restore":  "Unstage with `git multistage --stage <id> restore --staged <path>`, and undo your own edits by editing the files: a restored file loses the other agents' edits too.",
	"reset":    "Unstage with `git multistage --stage <id> restore --staged <path>`. To work on another commit, use a separate worktree from `git worktree add`.",
	"checkout": "Undo your own edits by editing the files, and use a separate worktree from `git worktree add` for another branch or commit. If the task needs this checkout on another branch, stop and report it.",
	"switch":   "Use a separate worktree from `git worktree add` for another branch. If the task needs this checkout on another branch, stop and report it.",
}

// gitCall is one Git command of a shell command line.
type gitCall struct {
	dir  string // the directory that the command runs in
	sub  string // the Git command, such as "stash"
	args []string
}

// guardShellCommand answers a PreToolUse hook of a shell tool. It denies the
// first guarded Git command that runs in an opted-in checkout.
func guardShellCommand(in hookInput, stdout io.Writer) {
	if !strings.Contains(in.Input.Command, "git") {
		return
	}
	shared := map[string]bool{}
	for _, c := range gitCalls(in.Input.Command, in.Cwd) {
		instead, ok := guardedGit[c.sub]
		if !ok || c.sub == "stash" && len(c.args) > 0 && (c.args[0] == "list" || c.args[0] == "show") {
			continue
		}
		on, seen := shared[c.dir]
		if !seen {
			r, err := OpenRepo(c.dir)
			on = err == nil && r.shared()
			shared[c.dir] = on
		}
		if !on {
			continue
		}
		reason := fmt.Sprintf("git-multistage blocked `git %s`: other agents share this checkout, and `git %s` changes "+
			"state that they share. %s", c.sub, c.sub, instead)
		out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": reason,
		}})
		fmt.Fprintln(stdout, string(out))
		return
	}
}

// gitCalls returns the Git commands of a shell command line that starts in
// cwd. It follows `cd <dir>` and `git -C <dir>`.
func gitCalls(command, cwd string) []gitCall {
	var calls []gitCall
	dir := cwd
	for _, words := range shellSegments(command) {
		words = skipPrefixes(words)
		if len(words) == 0 {
			continue
		}
		switch {
		case words[0] == "cd" && len(words) > 1 && words[1] != "-":
			dir = resolveDir(dir, words[1])
		case filepath.Base(words[0]) == "git":
			if c, ok := parseGit(words[1:], dir); ok {
				calls = append(calls, c)
			}
		}
	}
	return calls
}

// parseGit reads Git's global options and returns the command after them.
func parseGit(words []string, dir string) (gitCall, bool) {
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case w == "-C" && i+1 < len(words):
			dir = resolveDir(dir, words[i+1])
			i++
		case (w == "-c" || w == "--git-dir" || w == "--work-tree" || w == "--namespace" || w == "--config-env") && i+1 < len(words):
			i++
		case strings.HasPrefix(w, "-"):
		default:
			return gitCall{dir: dir, sub: w, args: words[i+1:]}, true
		}
	}
	return gitCall{}, false
}

// skipPrefixes drops the variable assignments and the wrapper commands that
// come before a command, such as `FOO=1 env sudo git`.
func skipPrefixes(words []string) []string {
	for len(words) > 0 {
		w := words[0]
		eq := strings.IndexByte(w, '=')
		switch {
		case eq > 0 && !strings.ContainsAny(w[:eq], "/-."):
		case w == "env" || w == "command" || w == "sudo" || w == "time" || w == "nohup" || w == "exec":
		default:
			return words
		}
		words = words[1:]
	}
	return words
}

// resolveDir resolves the directory p, as `cd p` would from dir.
func resolveDir(dir, p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return filepath.Clean(p)
}

// shellSegments splits a shell command line into its simple commands, each
// as its words. It splits at `;`, `&`, `|`, newlines, and parentheses outside
// quotes, and removes the quotes and backslashes of each word.
func shellSegments(command string) [][]string {
	var sc shellScanner
	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch {
		case sc.quote != 0:
			i = sc.quoted(command, i)
		case ch == '\'' || ch == '"':
			sc.quote, sc.inWord = ch, true
		case ch == '\\' && i+1 < len(command):
			i++
			if command[i] != '\n' {
				sc.add(command[i])
			}
		case strings.IndexByte(";&|\n()", ch) >= 0:
			sc.endSeg()
		case ch == ' ' || ch == '\t':
			sc.endWord()
		default:
			sc.add(ch)
		}
	}
	sc.endSeg()
	return sc.segs
}

// shellScanner holds the state of shellSegments.
type shellScanner struct {
	segs   [][]string
	words  []string
	word   strings.Builder
	inWord bool
	quote  byte
}

func (sc *shellScanner) add(ch byte) {
	sc.word.WriteByte(ch)
	sc.inWord = true
}

// quoted reads the byte at i inside a quote, and returns the index of the
// last byte that it read.
func (sc *shellScanner) quoted(command string, i int) int {
	switch ch := command[i]; {
	case ch == sc.quote:
		sc.quote = 0
	case ch == '\\' && sc.quote == '"' && i+1 < len(command):
		i++
		sc.word.WriteByte(command[i])
	default:
		sc.word.WriteByte(ch)
	}
	return i
}

func (sc *shellScanner) endWord() {
	if sc.inWord {
		sc.words = append(sc.words, sc.word.String())
		sc.word.Reset()
		sc.inWord = false
	}
}

func (sc *shellScanner) endSeg() {
	sc.endWord()
	if len(sc.words) > 0 {
		sc.segs = append(sc.segs, sc.words)
		sc.words = nil
	}
}
