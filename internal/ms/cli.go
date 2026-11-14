// Package ms implements git multistage: each stage is a private index that
// one agent stages into and commits from, beside the default index.
package ms

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// Version is set at build time with -ldflags "-X ...ms.Version=v1.2.3".
var Version = "dev"

//nolint:dupword // the list command's description starts with the command's name
const usage = `usage: git multistage [--stage <id> | --agentic] <command> [<args>]

Each stage is a private index with its own staged changes. Several stages
can stage and commit to the current branch at the same time, from one
checkout, without waiting for each other.

Stage ID: --stage <id>, or the GIT_MULTISTAGE_ID environment variable.
--agentic takes the ID of the calling agent from its environment when no
stage ID is given: CLAUDE_CODE_SESSION_ID or CODEX_THREAD_ID (the innermost
tool wins), else ORCA_AGENT_SESSION_ID or PASEO_AGENT_ID.

Commands:
  init [<id>]                  create a stage; without <id>, pick a unique ID
  status                       show this stage's staged, unstaged, and untracked files
  diff [--cached] [<args>]     git diff against this stage (--cached: staged changes)
  hunks [--cached] [<path>...] list hunks with IDs (default: unstaged changes)
  add <path>...                stage whole files
  add <path>... --hunk <id>... stage only the listed hunks
  add --edits [<path>...] [--recreate <path>]...
                               stage the edits that the editor hook recorded
  rm [-f] <path>...            delete files and stage their deletions
  mv <old> <new>               move a file and stage the move
  apply [-R] <patch>|-         apply a patch to this stage's index
  restore --staged <path>... [--hunk <id>...]
                               unstage files or hunks
  commit -m <msg> [-F <file>] [--no-verify] [--recreate <path>]...
                               commit this stage's staged changes to the branch
  log [<git log args>]         show commits made by this stage
  list                         list the stages of this worktree
  doctor                       check the repository, default index, and stages
  sync [--from <commit>]       bring the default index up to the branch tip
  drop [--force]               delete this stage
  drop --edits [<path>...]     forget recorded edits
  hook [claude|codex]          editor hook that records file edits (Claude Code, Codex)

Most commands take --json. Exit codes: 0 ok, 1 error or nothing to commit,
2 usage, 3 conflict, 4 stale hunk ID, 5 refused, 6 lock timeout.
`

type flagSpec struct {
	names []string // first name is canonical
	value bool
}

type parsed struct {
	pos  []string
	vals map[string][]string
}

func (p *parsed) has(name string) bool { _, ok := p.vals[name]; return ok }
func (p *parsed) get(name string) string {
	v := p.vals[name]
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}

var globalFlags = []flagSpec{
	{[]string{flagStage}, true},
	{[]string{flagAgentic}, false},
	{[]string{flagJSON}, false},
	{[]string{"--help", "-h"}, false},
}

func parseArgs(argv []string, specs []flagSpec) (*parsed, error) {
	all := flagSpecs(append(append([]flagSpec{}, specs...), globalFlags...))
	p := &parsed{vals: map[string][]string{}}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if a == "--" {
			p.pos = append(p.pos, argv[i+1:]...)
			break
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			p.pos = append(p.pos, a)
			continue
		}
		key, val, used, err := all.option(argv[i:])
		if err != nil {
			return nil, err
		}
		i += used
		p.vals[key] = append(p.vals[key], val)
	}
	return p, nil
}

type flagSpecs []flagSpec

func (fs flagSpecs) find(name string) *flagSpec {
	for i := range fs {
		if slices.Contains(fs[i].names, name) {
			return &fs[i]
		}
	}
	return nil
}

// option parses the option args[0], and returns its first name, its value,
// and how many of the following arguments it used as its value.
func (fs flagSpecs) option(args []string) (key, val string, used int, err error) {
	a := args[0]
	name, val, hasVal := strings.Cut(a, "=")
	if !strings.HasPrefix(a, "--") && len(a) > 2 {
		if s := fs.find(a[:2]); s != nil && s.value {
			name, val, hasVal = a[:2], a[2:], true // -mMessage
		}
	}
	s := fs.find(name)
	switch {
	case s == nil:
		return "", "", 0, usageErr("unknown option %s", name)
	case s.value && !hasVal:
		if len(args) < 2 {
			return "", "", 0, usageErr("option %s needs a value", name)
		}
		return s.names[0], args[1], 1, nil
	case !s.value && hasVal:
		return "", "", 0, usageErr("option %s takes no value", name)
	}
	return s.names[0], val, 0, nil
}

type app struct {
	out, errw io.Writer
	json      bool
	stageFlag string // --stage
	agentic   bool   // --agentic
	repo      *Repo
	warnings  []string
	stage     *Stage // the open stage, whose late warnings are collected before output
}

// Main runs the command line and returns the exit code.
func Main(argv []string, stdout, stderr io.Writer) int {
	if i := commandIndex(argv); i < len(argv) && argv[i] == "hook" {
		// hookMain parses the global options before the command too.
		return hookMain(append(argv[:i:i], argv[i+1:]...), stdout, stderr)
	}
	a := &app{out: stdout, errw: stderr}
	err := a.run(argv)
	if err == nil {
		a.flushWarnings()
		return ExitOK
	}
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: ExitError, Kind: jsonError, Message: err.Error()}
	}
	if a.json {
		a.flushWarnings()
		a.emit(map[string]any{jsonError: e, jsonWarnings: a.warnings})
		return e.Code
	}
	a.flushWarnings()
	fmt.Fprintf(stderr, "error: %s\n", e.Message)
	for _, p := range e.Paths {
		fmt.Fprintf(stderr, "  %s\n", p)
	}
	for _, d := range e.Details {
		fmt.Fprintf(stderr, "  %s\n", d)
	}
	if e.Hint != "" {
		fmt.Fprintf(stderr, "hint: %s\n", e.Hint)
	}
	return e.Code
}

// commandIndex returns the index of the command name, after the global
// options that may come before it.
func commandIndex(argv []string) int {
	i := 0
	for i < len(argv) {
		switch {
		case argv[i] == flagStage:
			i += 2
		case strings.HasPrefix(argv[i], "--stage="), argv[i] == flagAgentic, argv[i] == flagJSON:
			i++
		default:
			return i
		}
	}
	return len(argv)
}

// collectStageWarnings moves the warnings that the open stage raised since
// the last collection into a.warnings, so each prints once.
func (a *app) collectStageWarnings() {
	if a.stage == nil {
		return
	}
	a.warnings = append(a.warnings, a.stage.Warnings...)
	a.stage.Warnings = nil
}

func (a *app) flushWarnings() {
	a.collectStageWarnings()
	if a.json {
		return
	}
	for _, w := range a.warnings {
		fmt.Fprintf(a.errw, "warning: %s\n", w)
	}
	a.warnings = nil
}

// emit prints v as JSON. A jsonWarnings entry of v gets the stage's warnings
// that are not collected yet.
func (a *app) emit(v any) {
	if m, ok := v.(map[string]any); ok {
		if _, ok := m[jsonWarnings]; ok {
			a.collectStageWarnings()
			m[jsonWarnings] = a.warnings
		}
	}
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (a *app) printf(format string, args ...any) { fmt.Fprintf(a.out, format, args...) }

// takeGlobal removes the global options from a pass-through argument list.
func (a *app) takeGlobal(argv []string) (rest []string, err error) {
	for i := 0; i < len(argv); i++ {
		switch x := argv[i]; {
		case x == "--":
			return append(rest, argv[i:]...), nil
		case x == flagStage:
			if i+1 >= len(argv) {
				return nil, usageErr("option --stage needs a value")
			}
			i++
			a.stageFlag = argv[i]
		case strings.HasPrefix(x, "--stage="):
			a.stageFlag = strings.TrimPrefix(x, "--stage=")
		case x == flagAgentic:
			a.agentic = true
		case x == flagJSON:
			a.json = true
		default:
			rest = append(rest, x)
		}
	}
	return rest, nil
}

// stageID resolves the stage ID: --stage, then GIT_MULTISTAGE_ID, then, with
// --agentic only, the calling agent's ID.
func (a *app) stageID() (string, error) {
	if a.stageFlag != "" {
		return a.stageFlag, nil
	}
	if id := os.Getenv("GIT_MULTISTAGE_ID"); id != "" {
		return id, nil
	}
	if !a.agentic {
		return "", nil
	}
	return agentStageID()
}

// agentStageID derives a stage ID from the calling agent's environment, the
// same ID that the agent's editor hook records into. Agent tools come first:
// Claude Code and Codex set their variable for each agent, including a
// nested one. A process started from a tool inherits the tool's variable,
// such as a daemon started from a Claude Code shell, so a tool's variable
// counts only when the tool is an ancestor process; otherwise the next set
// variable wins. A tool's variable that is the only one set is kept, and when
// the process table cannot be read, the priority order alone decides.
// Orchestrators (Orca, Paseo) set one ID per agent they start, which nested
// agents and subagents inherit, so they count only when no tool variable does.
func agentStageID() (string, error) {
	claude, codex := os.Getenv("CLAUDE_CODE_SESSION_ID"), os.Getenv("CODEX_THREAD_ID")
	orca, paseo := os.Getenv("ORCA_AGENT_SESSION_ID"), os.Getenv("PASEO_AGENT_ID")
	claude, codex, err := pickTool(claude, codex, orca != "" || paseo != "")
	if err != nil {
		return "", err
	}
	switch {
	case claude != "":
		return sanitizeID("claude-" + claude), nil
	case codex != "":
		return sanitizeID("codex-" + codex), nil
	}
	switch {
	case orca != "" && paseo != "":
		return "", &Error{
			Code: ExitUsage, Kind: "ambiguous-agent",
			Message: "--agentic found both ORCA_AGENT_SESSION_ID and PASEO_AGENT_ID, so it cannot tell which agent is calling",
			Hint:    "pass --stage <id>",
		}
	case orca != "":
		return sanitizeID("orca-" + orca), nil
	case paseo != "":
		return sanitizeID("paseo-" + paseo), nil
	}
	return "", &Error{
		Code: ExitUsage, Kind: "no-agent-id",
		Message: "--agentic found no agent ID: none of CLAUDE_CODE_SESSION_ID, CODEX_THREAD_ID, ORCA_AGENT_SESSION_ID, PASEO_AGENT_ID is set",
		Hint:    "pass --stage <id>",
	}
}

// pickTool keeps the tool variables that count, as agentStageID describes.
// It fails when both still count.
func pickTool(claude, codex string, orchestrator bool) (string, string, error) {
	if claude == "" && codex == "" || (claude == "" || codex == "") && !orchestrator {
		return claude, codex, nil
	}
	// Without a process table, the priority order alone decides.
	if chain, err := ancestors(); err == nil {
		claude, codex = keepAncestorTool(chain, claude, codex, orchestrator)
	}
	if claude != "" && codex != "" {
		return "", "", &Error{
			Code: ExitUsage, Kind: "ambiguous-agent",
			Message: "--agentic found both CLAUDE_CODE_SESSION_ID and CODEX_THREAD_ID, and neither tool is an ancestor process",
			Hint:    "pass --stage <id>; the editor hook's message names the stage",
		}
	}
	return claude, codex, nil
}

// keepAncestorTool keeps the variable of the nearer tool in chain. With no
// tool in chain, an orchestrator's variable wins over both.
func keepAncestorTool(chain []proc, claude, codex string, orchestrator bool) (string, string) {
	claudeAt, codexAt := toolAncestors(chain)
	switch {
	case claudeAt >= 0 && (codexAt < 0 || claudeAt < codexAt):
		codex = ""
	case codexAt >= 0:
		claude = ""
	case orchestrator:
		claude, codex = "", ""
	}
	return claude, codex
}

func (a *app) run(argv []string) error {
	argv = a.takeLeadingGlobals(argv)
	if a.printInfo(argv) {
		return nil
	}
	cmd, args := argv[0], argv[1:]
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if a.repo, err = OpenRepo(cwd); err != nil {
		return &Error{Code: ExitError, Kind: "repository", Message: err.Error()}
	}
	if cmd == cmdDiff || cmd == cmdLog {
		return a.passThrough(cmd, args, cwd)
	}
	spec, ok := commandSpecs[cmd]
	if !ok {
		return usageErr("unknown command %q; run `git multistage help`", cmd)
	}
	p, err := parseArgs(args, spec)
	if err != nil {
		return err
	}
	if p.has("--help") {
		fmt.Fprint(a.out, usage)
		return nil
	}
	a.takeGlobals(p)
	switch cmd {
	case "list":
		return a.list()
	case "doctor":
		return a.doctor()
	case "sync":
		return a.sync(p)
	}
	return a.runOnStage(cmd, p)
}

// printInfo prints the usage or the version when argv asks for it.
func (a *app) printInfo(argv []string) bool {
	switch {
	case len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h":
		fmt.Fprint(a.out, usage)
	case argv[0] == "version" || argv[0] == "--version":
		a.printf("git-multistage %s\n", Version)
	default:
		return false
	}
	return true
}

// takeLeadingGlobals takes the global options that come before the
// command, and returns the rest of argv.
func (a *app) takeLeadingGlobals(argv []string) []string {
	for len(argv) > 0 {
		switch {
		case argv[0] == flagStage && len(argv) > 1:
			a.stageFlag, argv = argv[1], argv[2:]
		case strings.HasPrefix(argv[0], "--stage="):
			a.stageFlag, argv = strings.TrimPrefix(argv[0], "--stage="), argv[1:]
		case argv[0] == flagAgentic:
			a.agentic, argv = true, argv[1:]
		case argv[0] == flagJSON:
			a.json, argv = true, argv[1:]
		default:
			return argv
		}
	}
	return argv
}

// takeGlobals takes the global options that follow the command.
func (a *app) takeGlobals(p *parsed) {
	if p.has(flagJSON) {
		a.json = true
	}
	if p.has(flagStage) {
		a.stageFlag = p.get(flagStage)
	}
	if p.has(flagAgentic) {
		a.agentic = true
	}
}

// passThrough runs diff or log, whose arguments go to Git unchanged.
func (a *app) passThrough(cmd string, args []string, cwd string) error {
	rest, err := a.takeGlobal(args)
	if err != nil {
		return err
	}
	if a.json {
		return usageErr("%s has no --json output", cmd)
	}
	id, err := a.stageID()
	if err != nil {
		return err
	}
	if cmd == cmdDiff {
		return a.diff(rest, cwd, id)
	}
	return a.log(rest, id)
}

// commandSpecs holds the options of each command that parseArgs parses.
var commandSpecs = map[string][]flagSpec{
	"init":   nil,
	"status": nil,
	"hunks":  {{[]string{flagCached, "--staged"}, false}},
	cmdAdd: {
		{[]string{"--hunk"}, true},
		{[]string{"--force", "-f"}, false},
		{[]string{"--allow-shared"}, false},
		{[]string{"--patch", "-p"}, false},
		{[]string{"--edits"}, false},
		{[]string{"--recreate"}, true},
	},
	"rm":      {{[]string{"--force", "-f"}, false}},
	"mv":      nil,
	"apply":   {{[]string{"--reverse", "-R"}, false}},
	"restore": {{[]string{"--staged", "-S"}, false}, {[]string{"--hunk"}, true}},
	cmdCommit: {
		{[]string{"--message", "-m"}, true},
		{[]string{"--file", "-F"}, true},
		{[]string{"--no-verify", "-n"}, false},
		{[]string{"--recreate"}, true},
	},
	"list":   nil,
	"doctor": nil,
	"sync":   {{[]string{"--from"}, true}},
	"drop":   {{[]string{"--force", "-f"}, false}, {[]string{"--edits"}, false}},
}

// runOnStage runs a command that works on one stage.
func (a *app) runOnStage(cmd string, p *parsed) error {
	id, err := a.stageID()
	if err != nil {
		return err
	}
	if cmd == "init" {
		return a.init(p, id)
	}
	stage, err := a.repo.OpenStage(id, false)
	if err != nil {
		return err
	}
	defer stage.Close()
	a.stage = stage
	if stage.Created {
		stage.warn("created stage %s on %s", stage.ID, stage.M.Branch)
	}
	// Take the stage's warnings now, before a command prints its JSON.
	a.warnings = append(stage.Warnings, a.warnings...)
	stage.Warnings = nil
	if cmd == "drop" {
		return a.drop(stage, p)
	}
	if stage.Damaged {
		return stage.errDamaged()
	}
	if cmd == cmdCommit {
		return a.commit(stage, p)
	}
	f, err := stage.refresh()
	if err != nil {
		return err
	}
	a.noteFreshness(f)
	return a.runOnFreshStage(cmd, stage, p, f)
}

// runOnFreshStage runs a command on a stage that refresh brought up to date.
func (a *app) runOnFreshStage(cmd string, stage *Stage, p *parsed, f Freshness) error {
	switch cmd {
	case "status":
		return a.status(stage, f)
	case "hunks":
		return a.hunks(stage, p)
	case cmdAdd:
		return a.add(stage, p)
	case "rm":
		return a.rm(stage, p)
	case "mv":
		return a.mv(stage, p)
	case "apply":
		return a.apply(stage, p)
	case "restore":
		return a.restore(stage, p)
	}
	return nil
}

func (a *app) noteFreshness(f Freshness) {
	if !f.Stale {
		if f.Reason != "" {
			a.warnings = append(a.warnings, f.Reason)
		}
		return
	}
	msg := fmt.Sprintf("this stage's base %s is behind %s (%s)", short(f.Base), short(f.Tip), f.Reason)
	if len(f.Conflicts) > 0 {
		msg += ": " + strings.Join(f.Conflicts, ", ")
	}
	a.warnings = append(a.warnings, msg)
}

func (a *app) init(p *parsed, id string) error {
	if len(p.pos) > 1 {
		return usageErr("usage: git multistage init [<id>]")
	}
	if len(p.pos) == 1 {
		id = p.pos[0]
	}
	if id == "" {
		id = NewStageID()
	}
	stage, err := a.repo.OpenStage(id, true)
	if err != nil {
		return err
	}
	defer stage.Close()
	a.warnings = append(a.warnings, stage.Warnings...)
	if a.json {
		a.emit(map[string]any{"stage": id, "branch": stage.M.Branch, "base": stage.M.Base, "stage_dir": stage.Dir})
		return nil
	}
	a.printf("Created stage %s on %s at %s\n", id, strings.TrimPrefix(stage.M.Branch, "refs/heads/"), short(stage.M.Base))
	a.printf("Use it with --stage %s, or set GIT_MULTISTAGE_ID=%s\n", id, id)
	return nil
}

func (a *app) status(l *Stage, f Freshness) error {
	st, err := l.Status(f)
	if err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"status": st, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	state := "up to date"
	switch {
	case f.Stale:
		state = "behind " + short(f.Tip) + ", see warning"
	case f.Base == "":
		state = "no commits yet"
	}
	a.printf("stage %s on %s (base %s, %s)\n", st.ID, strings.TrimPrefix(f.Branch, "refs/heads/"), short(f.Base), state)
	section := func(title string, cs []Change) {
		if len(cs) == 0 {
			return
		}
		a.printf("%s:\n", title)
		for _, c := range cs {
			if c.OldPath != "" {
				a.printf("  %s  %s -> %s\n", c.Status, c.OldPath, c.Path)
			} else {
				a.printf("  %s  %s\n", c.Status, c.Path)
			}
		}
	}
	section("staged", st.Staged)
	section("not staged", st.Unstaged)
	a.printStatusLists(st)
	if len(st.Staged)+len(st.Unstaged)+len(st.Untracked) == 0 {
		a.printf("nothing to commit, working tree clean\n")
	}
	return nil
}

// printStatusLists prints the untracked files, the recorded edits, and the
// files staged in other stages.
func (a *app) printStatusLists(st *StatusInfo) {
	if len(st.Untracked) > 0 {
		a.printf("untracked:\n")
		for _, u := range st.Untracked {
			a.printf("  ?? %s\n", u)
		}
	}
	if len(st.Recorded) > 0 {
		a.printf("recorded edits (stage them with `add --edits`):\n")
		for _, p := range st.Recorded {
			a.printf("  %s\n", p)
		}
	}
	if len(st.Shared) > 0 {
		a.printf("also staged in other stages:\n")
		for _, s := range st.Shared {
			a.printf("  %s (%s)\n", s.Path, strings.Join(s.Stages, ", "))
		}
	}
}

func (a *app) hunks(l *Stage, p *parsed) error {
	paths, err := a.repo.repoPaths(p.pos)
	if err != nil {
		return err
	}
	files, err := l.Hunks(paths, p.has(flagCached))
	if err != nil {
		return err
	}
	if a.json {
		if files == nil {
			files = []FileHunks{}
		}
		a.emit(map[string]any{"files": files, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	for _, f := range files {
		a.printf("diff %s (%s)\n", f.Path, f.Status)
		if f.Note != "" {
			a.printf("# %s\n", f.Note)
		}
		for _, h := range f.Hunks {
			extra := ""
			if len(h.StagedBy) > 0 {
				extra = " staged-by=" + strings.Join(h.StagedBy, ",")
			}
			a.printf("%s id=%s%s\n", h.Header(), h.ID, extra)
			for _, line := range h.Lines {
				a.printf("%s\n", line)
			}
		}
	}
	return nil
}

func (a *app) add(l *Stage, p *parsed) error {
	if p.has("--edits") {
		return a.addEdits(l, p)
	}
	if p.has("--recreate") {
		return usageErr("--recreate needs --edits")
	}
	if len(p.pos) == 0 {
		return usageErr("usage: git multistage add <path>... [--hunk <id>...]")
	}
	paths, err := a.repo.repoPaths(p.pos)
	if err != nil {
		return err
	}
	if p.has("--patch") {
		if a.json || p.has("--hunk") {
			return usageErr("--patch is interactive; use --hunk for scripted staging")
		}
		_, err := a.repo.Run(gitx.Opt{
			Index: l.IndexPath(), Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
			Env: []string{"GIT_EDITOR=" + editor()},
		}, append([]string{cmdAdd, "-p", "--"}, paths...)...)
		return err
	}
	if ids := p.vals["--hunk"]; len(ids) > 0 {
		return a.addHunks(l, p, paths, ids)
	}
	return a.addPaths(l, p, paths)
}

// addEdits runs `add --edits`.
func (a *app) addEdits(l *Stage, p *parsed) error {
	paths, err := a.repo.repoPaths(p.pos)
	if err != nil {
		return err
	}
	recreate, err := a.repo.repoPaths(p.vals["--recreate"])
	if err != nil {
		return err
	}
	staged, err := l.AddEdits(paths, p.has("--force"), recreate)
	if err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"staged_paths": staged, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("staged recorded edits in %s\n", strings.Join(staged, ", "))
	return nil
}

// addHunks runs `add --hunk`.
func (a *app) addHunks(l *Stage, p *parsed, paths, ids []string) error {
	staged, err := l.AddHunks(paths, ids, p.has("--force"))
	if err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"staged_paths": staged, "hunks": ids, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("staged %d hunk(s) in %s\n", len(ids), strings.Join(staged, ", "))
	return nil
}

// addPaths runs `add` for whole files.
func (a *app) addPaths(l *Stage, p *parsed, paths []string) error {
	changed, err := l.AddPaths(paths, p.has("--force"), p.has("--allow-shared"))
	if err != nil {
		return err
	}
	if a.json {
		var cs []Change
		for _, c := range changed {
			cs = append(cs, Change{Status: string(c.Status), Path: c.Path})
		}
		if cs == nil {
			cs = []Change{}
		}
		a.emit(map[string]any{"staged": cs, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	if len(changed) == 0 {
		a.printf("no changes staged\n")
	}
	for _, c := range changed {
		a.printf("staged: %c  %s\n", c.Status, c.Path)
	}
	return nil
}

func editor() string {
	if e := os.Getenv("GIT_EDITOR"); e != "" {
		return e
	}
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

func (a *app) apply(l *Stage, p *parsed) error {
	if len(p.pos) != 1 {
		return usageErr("usage: git multistage apply [-R] <patch-file>|-")
	}
	var in io.Reader = os.Stdin
	if p.pos[0] != "-" {
		f, err := os.Open(p.pos[0])
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	if err := l.ApplyPatch(in, p.has("--reverse")); err != nil {
		return err
	}
	staged, err := l.stagedChanges()
	if err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"staged": staged, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("applied patch to stage %s\n", l.ID)
	return nil
}

func (a *app) rm(l *Stage, p *parsed) error {
	if len(p.pos) == 0 {
		return usageErr("usage: git multistage rm [-f] <path>...")
	}
	paths, err := a.repo.repoPaths(p.pos)
	if err != nil {
		return err
	}
	if err := l.RemovePaths(paths, p.has("--force")); err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"deleted_paths": paths, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("deleted and staged: %s\n", strings.Join(paths, ", "))
	return nil
}

func (a *app) mv(l *Stage, p *parsed) error {
	if len(p.pos) != 2 {
		return usageErr("usage: git multistage mv <old> <new>")
	}
	paths, err := a.repo.repoPaths(p.pos)
	if err != nil {
		return err
	}
	if err := l.MovePath(paths[0], paths[1]); err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"moved": Move{Old: paths[0], New: paths[1]}, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("moved and staged: %s -> %s\n", paths[0], paths[1])
	return nil
}

func (a *app) restore(l *Stage, p *parsed) error {
	if !p.has("--staged") {
		return usageErr("git multistage restore changes only the stage's index: pass --staged")
	}
	if len(p.pos) == 0 {
		return usageErr("usage: git multistage restore --staged <path>... [--hunk <id>...]")
	}
	paths, err := a.repo.repoPaths(p.pos)
	if err != nil {
		return err
	}
	restored := paths
	if ids := p.vals["--hunk"]; len(ids) > 0 {
		if restored, err = l.RestoreHunks(paths, ids); err != nil {
			return err
		}
	} else if err := l.RestorePaths(paths); err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"unstaged_paths": restored, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("unstaged: %s\n", strings.Join(restored, ", "))
	return nil
}

func (a *app) commit(l *Stage, p *parsed) error {
	if !p.has("--message") && !p.has("--file") {
		return usageErr("pass a commit message with -m <msg> or -F <file>; git multistage never opens an editor")
	}
	msg, err := commitMessage(p.vals["--message"], p.get("--file"))
	if err != nil {
		return err
	}
	recreate, err := a.repo.repoPaths(p.vals["--recreate"])
	if err != nil {
		return err
	}
	res, err := l.Commit(msg, p.has("--no-verify"), recreate)
	if err != nil {
		return err
	}
	a.warnings = append(a.warnings, res.Sync.Warnings...)
	if a.json {
		a.emit(map[string]any{"commit": res, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	branch := strings.TrimPrefix(res.Branch, "refs/heads/")
	if res.Result == "already-applied" {
		a.printf("nothing new to commit: %s already contains every staged change; the stage is now empty\n", branch)
		return nil
	}
	a.printf("[%s %s] %s\n", branch, short(res.Commit), res.Subject)
	retry := ""
	if res.Attempts > 1 {
		retry = fmt.Sprintf(", replayed onto a newer tip %d time(s)", res.Attempts-1)
	}
	a.printf(" %d file(s) changed%s\n", len(res.Files), retry)
	return nil
}

func (a *app) diff(args []string, cwd, id string) error {
	stage, err := a.repo.OpenStage(id, false)
	if err != nil {
		return err
	}
	defer stage.Close()
	if stage.Damaged {
		return stage.errDamaged()
	}
	f, err := stage.refresh()
	if err != nil {
		return err
	}
	a.noteFreshness(f)
	a.warnings = append(stage.Warnings, a.warnings...)
	a.flushWarnings()
	gitArgs := []string{cmdDiff}
	for _, x := range args {
		if x == "--" {
			break
		}
		if x == flagCached || x == "--staged" {
			base, err := a.repo.TreeOf(stage.M.Base)
			if err != nil {
				return err
			}
			gitArgs = append(gitArgs, base)
			break
		}
	}
	_, err = a.repo.Run(gitx.Opt{Index: stage.IndexPath(), Dir: cwd, User: true, Stdout: os.Stdout, Stderr: os.Stderr},
		append(gitArgs, args...)...)
	if err != nil && gitx.ExitCode(err) == 1 {
		return &Error{Code: ExitError, Kind: cmdDiff} // --exit-code reported differences
	}
	return err
}

func (a *app) log(args []string, id string) error {
	if err := ValidateStageID(id); err != nil {
		return err
	}
	m, err := readMeta(a.repo.stagesDir() + string(os.PathSeparator) + id)
	if err != nil {
		return errorf(ExitError, "no-stage", "stage %q does not exist", id)
	}
	var commits []string
	for i := len(m.Commits) - 1; i >= 0; i-- {
		if _, err := a.repo.Run(gitx.Opt{}, "cat-file", "-e", m.Commits[i]+"^{commit}"); err == nil {
			commits = append(commits, m.Commits[i])
		}
	}
	if len(commits) == 0 {
		a.printf("stage %s has made no commits\n", id)
		return nil
	}
	// The commits come first, so that the user's paths, with or without
	// `--`, filter them.
	gitArgs := append(append([]string{"log", "--no-walk=unsorted"}, commits...), args...)
	_, err = a.repo.Run(gitx.Opt{User: true, Stdout: os.Stdout, Stderr: os.Stderr}, gitArgs...)
	return err
}

func (a *app) list() error {
	stages, err := a.repo.Stages()
	if err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"stages": stages})
		return nil
	}
	if len(stages) == 0 {
		a.printf("no stages in this worktree\n")
	}
	for _, li := range stages {
		flags := ""
		if li.Busy {
			flags += " busy"
		}
		if li.Behind {
			flags += " behind"
		}
		if len(li.Problems) > 0 {
			flags += " problems"
		}
		a.printf("%-20s %-20s base %s  %d staged  %d commit(s)%s\n", li.ID,
			strings.TrimPrefix(li.Branch, "refs/heads/"), short(li.Base), len(li.Staged), li.Commits, flags)
	}
	return nil
}

func (a *app) doctor() error {
	rep, err := a.repo.Doctor()
	if err != nil {
		return err
	}
	failed := false
	for _, c := range rep.Checks {
		failed = failed || c.Level == jsonError
	}
	if a.json {
		a.emit(rep)
	} else {
		for _, c := range rep.Checks {
			a.printf("%-7s %s\n", c.Level+":", c.Message)
		}
	}
	if failed {
		return &Error{Code: ExitError, Kind: "doctor", Message: "doctor found errors"}
	}
	return nil
}

func (a *app) sync(p *parsed) error {
	from := ""
	if f := p.get("--from"); f != "" {
		oid, err := a.repo.ResolveRef(f)
		if err != nil || oid == "" {
			return usageErr("--from %s is not a commit", f)
		}
		from = oid
	}
	res, err := a.repo.SyncIndex(from, syncTimeoutCommit)
	if err != nil {
		return err
	}
	a.warnings = append(a.warnings, res.Warnings...)
	if a.json {
		a.emit(map[string]any{"index_sync": res, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("default index: %d path(s) updated, %d kept\n", len(res.Updated), len(res.Kept))
	return nil
}

func (a *app) drop(l *Stage, p *parsed) error {
	if p.has("--edits") {
		paths, err := a.repo.repoPaths(p.pos)
		if err != nil {
			return err
		}
		n, err := l.DropEdits(paths)
		if err != nil {
			return err
		}
		if a.json {
			a.emit(map[string]any{"dropped_edits": n, jsonWarnings: a.warnings})
			a.warnings = nil
			return nil
		}
		a.printf("forgot %d recorded edit(s)\n", n)
		return nil
	}
	tree, err := l.Drop(p.has("--force"))
	if err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"dropped": l.ID, "staged_tree": tree, jsonWarnings: a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("dropped stage %s\n", l.ID)
	if tree != "" {
		a.printf("its staged tree was %s (inspect it with `git diff %s %s`)\n", tree, short(l.M.Base), tree)
	}
	return nil
}

// hookMain runs `git multistage hook [claude|codex] [--stage <id>]`. Without
// a kind, the hook input tells: Codex reports apply_patch, Claude Code its
// Edit, MultiEdit, and Write tools. One hooks file then serves both tools.
func hookMain(argv []string, stdout, stderr io.Writer) int {
	p, err := parseArgs(argv, nil)
	kind := ""
	if err == nil && len(p.pos) == 1 {
		kind = p.pos[0]
	}
	if err != nil || len(p.pos) > 1 || kind != "" && kind != agentClaude && kind != agentCodex {
		fmt.Fprintln(stderr, "usage: git multistage hook [claude|codex] [--stage <id>]")
		return ExitUsage
	}
	return runEditHook(kind, os.Stdin, stdout, stderr, p.get(flagStage))
}
