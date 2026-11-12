package ms

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// Version is set at build time with -ldflags "-X ...ms.Version=v1.2.3".
var Version = "dev"

const usage = `usage: git multistage [--agent <id>] <command> [<args>]

Each agent stages into its own stage (a private index) and commits to the
current branch without waiting for other agents.

Agent ID: --agent <id>, or the GIT_MULTISTAGE_ID environment variable.

Commands:
  init [<id>]                  create a stage; without <id>, pick a unique ID
  status                       show this stage's staged, unstaged, and untracked files
  diff [--cached] [<args>]     git diff against this stage (--cached: staged changes)
  hunks [--cached] [<path>...] list hunks with IDs (default: unstaged changes)
  add <path>...                stage whole files
  add <path>... --hunk <id>... stage only the listed hunks
  apply [-R] <patch>|-         apply a patch to this stage's index
  restore --staged <path>... [--hunk <id>...]
                               unstage files or hunks
  commit -m <msg> [-F <file>] [--no-verify]
                               commit this stage's staged changes to the branch
  log [<git log args>]         show commits made by this stage
  list                         list the stages of this worktree
  doctor                       check the repository, default index, and stages
  sync                         bring the default index up to the branch tip
  drop [--force]               delete this stage

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

var globalFlags = []flagSpec{{[]string{"--agent"}, true}, {[]string{"--json"}, false}, {[]string{"--help", "-h"}, false}}

func parseArgs(argv []string, specs []flagSpec) (*parsed, error) {
	specs = append(append([]flagSpec{}, specs...), globalFlags...)
	find := func(name string) *flagSpec {
		for i := range specs {
			for _, n := range specs[i].names {
				if n == name {
					return &specs[i]
				}
			}
		}
		return nil
	}
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
		name, val, hasVal := strings.Cut(a, "=")
		if !strings.HasPrefix(a, "--") && len(a) > 2 && find(a[:2]) != nil && find(a[:2]).value {
			name, val, hasVal = a[:2], a[2:], true // -mMessage
		}
		s := find(name)
		if s == nil {
			return nil, usageErr("unknown option %s", name)
		}
		if s.value && !hasVal {
			if i+1 >= len(argv) {
				return nil, usageErr("option %s needs a value", name)
			}
			i++
			val = argv[i]
		} else if !s.value && hasVal {
			return nil, usageErr("option %s takes no value", name)
		}
		p.vals[s.names[0]] = append(p.vals[s.names[0]], val)
	}
	return p, nil
}

type app struct {
	out, errw io.Writer
	json      bool
	agent     string
	repo      *Repo
	warnings  []string
}

// Main runs the command line and returns the exit code.
func Main(argv []string, stdout, stderr io.Writer) int {
	a := &app{out: stdout, errw: stderr}
	err := a.run(argv)
	if err == nil {
		a.flushWarnings()
		return ExitOK
	}
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: ExitError, Kind: "error", Message: err.Error()}
	}
	if a.json {
		a.flushWarnings()
		a.emit(map[string]any{"error": e, "warnings": a.warnings})
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

func (a *app) flushWarnings() {
	if a.json {
		return
	}
	for _, w := range a.warnings {
		fmt.Fprintf(a.errw, "warning: %s\n", w)
	}
	a.warnings = nil
}

func (a *app) emit(v any) {
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (a *app) printf(format string, args ...any) { fmt.Fprintf(a.out, format, args...) }

// takeAgent removes --agent and --json from a pass-through argument list.
func takeGlobal(argv []string) (rest []string, agent string, jsonOut bool, err error) {
	for i := 0; i < len(argv); i++ {
		switch x := argv[i]; {
		case x == "--":
			return append(rest, argv[i:]...), agent, jsonOut, nil
		case x == "--agent":
			if i+1 >= len(argv) {
				return nil, "", false, usageErr("option --agent needs a value")
			}
			i++
			agent = argv[i]
		case strings.HasPrefix(x, "--agent="):
			agent = strings.TrimPrefix(x, "--agent=")
		case x == "--json":
			jsonOut = true
		default:
			rest = append(rest, x)
		}
	}
	return rest, agent, jsonOut, nil
}

func (a *app) run(argv []string) error {
	// Global options may come before the command.
	var agentFlag string
	for len(argv) > 0 {
		switch {
		case argv[0] == "--agent" && len(argv) > 1:
			agentFlag, argv = argv[1], argv[2:]
			continue
		case strings.HasPrefix(argv[0], "--agent="):
			agentFlag, argv = strings.TrimPrefix(argv[0], "--agent="), argv[1:]
			continue
		case argv[0] == "--json":
			a.json, argv = true, argv[1:]
			continue
		}
		break
	}
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
		fmt.Fprint(a.out, usage)
		return nil
	}
	if argv[0] == "version" || argv[0] == "--version" {
		a.printf("git-multistage %s\n", Version)
		return nil
	}
	cmd, args := argv[0], argv[1:]
	a.agent = agentFlag
	if a.agent == "" {
		a.agent = os.Getenv("GIT_MULTISTAGE_ID")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if a.repo, err = OpenRepo(cwd); err != nil {
		return &Error{Code: ExitError, Kind: "repository", Message: err.Error()}
	}

	// Commands whose arguments go to Git unchanged.
	switch cmd {
	case "diff", "log":
		rest, agent, jsonOut, err := takeGlobal(args)
		if err != nil {
			return err
		}
		if agent != "" {
			a.agent = agent
		}
		if jsonOut {
			return usageErr("%s has no --json output", cmd)
		}
		if cmd == "diff" {
			return a.diff(rest, cwd)
		}
		return a.log(rest)
	}

	specs := map[string][]flagSpec{
		"init":    nil,
		"status":  nil,
		"hunks":   {{[]string{"--cached", "--staged"}, false}},
		"add":     {{[]string{"--hunk"}, true}, {[]string{"--force", "-f"}, false}, {[]string{"--allow-shared"}, false}, {[]string{"--patch", "-p"}, false}},
		"apply":   {{[]string{"--reverse", "-R"}, false}},
		"restore": {{[]string{"--staged", "-S"}, false}, {[]string{"--hunk"}, true}},
		"commit":  {{[]string{"--message", "-m"}, true}, {[]string{"--file", "-F"}, true}, {[]string{"--no-verify", "-n"}, false}},
		"list":    nil,
		"doctor":  nil,
		"sync":    nil,
		"drop":    {{[]string{"--force", "-f"}, false}},
	}
	spec, ok := specs[cmd]
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
	if p.has("--json") {
		a.json = true
	}
	if p.has("--agent") {
		a.agent = p.get("--agent")
	}
	switch cmd {
	case "list":
		return a.list()
	case "doctor":
		return a.doctor()
	case "sync":
		return a.sync()
	case "init":
		return a.init(p)
	}
	stage, err := a.repo.OpenStage(a.agent, false)
	if err != nil {
		return err
	}
	defer stage.Close()
	defer func() { a.warnings = append(stage.Warnings, a.warnings...) }()
	if stage.Created {
		stage.warn("created stage %s on %s", stage.Agent, stage.M.Branch)
	}
	if cmd == "drop" {
		return a.drop(stage, p)
	}
	if cmd == "commit" {
		return a.commit(stage, p)
	}
	f, err := stage.refresh()
	if err != nil {
		return err
	}
	a.noteFreshness(f)
	switch cmd {
	case "status":
		return a.status(stage, f)
	case "hunks":
		return a.hunks(stage, p)
	case "add":
		return a.add(stage, p)
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

func (a *app) init(p *parsed) error {
	if len(p.pos) > 1 {
		return usageErr("usage: git multistage init [<id>]")
	}
	id := a.agent
	if len(p.pos) == 1 {
		id = p.pos[0]
	}
	if id == "" {
		id = NewAgentID()
	}
	stage, err := a.repo.OpenStage(id, true)
	if err != nil {
		return err
	}
	defer stage.Close()
	a.warnings = append(a.warnings, stage.Warnings...)
	if a.json {
		a.emit(map[string]any{"agent": id, "branch": stage.M.Branch, "base": stage.M.Base, "stage_dir": stage.Dir})
		return nil
	}
	a.printf("Created stage %s on %s at %s\n", id, strings.TrimPrefix(stage.M.Branch, "refs/heads/"), short(stage.M.Base))
	a.printf("Use it with --agent %s, or set GIT_MULTISTAGE_ID=%s\n", id, id)
	return nil
}

func (a *app) status(l *Stage, f Freshness) error {
	st, err := l.Status(f)
	if err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"status": st, "warnings": a.warnings})
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
	a.printf("stage %s on %s (base %s, %s)\n", st.Agent, strings.TrimPrefix(f.Branch, "refs/heads/"), short(f.Base), state)
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
	if len(st.Untracked) > 0 {
		a.printf("untracked:\n")
		for _, u := range st.Untracked {
			a.printf("  ?? %s\n", u)
		}
	}
	if len(st.Shared) > 0 {
		a.printf("also staged by other agents:\n")
		for _, s := range st.Shared {
			a.printf("  %s (%s)\n", s.Path, strings.Join(s.Agents, ", "))
		}
	}
	if len(st.Staged)+len(st.Unstaged)+len(st.Untracked) == 0 {
		a.printf("nothing to commit, working tree clean\n")
	}
	return nil
}

func (a *app) hunks(l *Stage, p *parsed) error {
	paths, err := a.repo.repoPaths(p.pos)
	if err != nil {
		return err
	}
	files, err := l.Hunks(paths, p.has("--cached"))
	if err != nil {
		return err
	}
	if a.json {
		if files == nil {
			files = []FileHunks{}
		}
		a.emit(map[string]any{"files": files, "warnings": a.warnings})
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
		_, err := a.repo.Run(gitx.Opt{Index: l.IndexPath(), Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
			Env: []string{"GIT_EDITOR=" + editor()}}, append([]string{"add", "-p", "--"}, paths...)...)
		return err
	}
	if ids := p.vals["--hunk"]; len(ids) > 0 {
		staged, err := l.AddHunks(paths, ids)
		if err != nil {
			return err
		}
		if a.json {
			a.emit(map[string]any{"staged_paths": staged, "hunks": ids, "warnings": a.warnings})
			a.warnings = nil
			return nil
		}
		a.printf("staged %d hunk(s) in %s\n", len(ids), strings.Join(staged, ", "))
		return nil
	}
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
		a.emit(map[string]any{"staged": cs, "warnings": a.warnings})
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
		defer f.Close()
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
		a.emit(map[string]any{"staged": staged, "warnings": a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("applied patch to stage %s\n", l.Agent)
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
		a.emit(map[string]any{"unstaged_paths": restored, "warnings": a.warnings})
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
	res, err := l.Commit(msg, p.has("--no-verify"))
	if err != nil {
		return err
	}
	a.warnings = append(a.warnings, res.Sync.Warnings...)
	if a.json {
		a.emit(map[string]any{"commit": res, "warnings": a.warnings})
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

func (a *app) diff(args []string, cwd string) error {
	stage, err := a.repo.OpenStage(a.agent, false)
	if err != nil {
		return err
	}
	defer stage.Close()
	f, err := stage.refresh()
	if err != nil {
		return err
	}
	a.noteFreshness(f)
	a.warnings = append(stage.Warnings, a.warnings...)
	a.flushWarnings()
	gitArgs := []string{"diff"}
	for _, x := range args {
		if x == "--" {
			break
		}
		if x == "--cached" || x == "--staged" {
			base, err := a.repo.TreeOf(stage.M.Base)
			if err != nil {
				return err
			}
			gitArgs = append(gitArgs, base)
			break
		}
	}
	_, err = a.repo.Run(gitx.Opt{Index: stage.IndexPath(), Dir: cwd, Stdout: os.Stdout, Stderr: os.Stderr},
		append(gitArgs, args...)...)
	if err != nil && gitx.ExitCode(err) == 1 {
		return &Error{Code: ExitError, Kind: "diff"} // --exit-code reported differences
	}
	return err
}

func (a *app) log(args []string) error {
	if err := ValidateAgent(a.agent); err != nil {
		return err
	}
	m, err := readMeta(a.repo.stagesDir() + string(os.PathSeparator) + a.agent)
	if err != nil {
		return errorf(ExitError, "no-stage", "stage %q does not exist", a.agent)
	}
	var commits []string
	for i := len(m.Commits) - 1; i >= 0; i-- {
		if _, err := a.repo.Run(gitx.Opt{}, "cat-file", "-e", m.Commits[i]+"^{commit}"); err == nil {
			commits = append(commits, m.Commits[i])
		}
	}
	if len(commits) == 0 {
		a.printf("stage %s has made no commits\n", a.agent)
		return nil
	}
	gitArgs := append([]string{"log", "--no-walk=unsorted"}, args...)
	_, err = a.repo.Run(gitx.Opt{Stdout: os.Stdout, Stderr: os.Stderr}, append(append(gitArgs, commits...), "--")...)
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
		a.printf("%-20s %-20s base %s  %d staged  %d commit(s)%s\n", li.Agent,
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
		failed = failed || c.Level == "error"
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

func (a *app) sync() error {
	res, err := a.repo.SyncIndex("", syncTimeoutCommit)
	if err != nil {
		return err
	}
	a.warnings = append(a.warnings, res.Warnings...)
	if a.json {
		a.emit(map[string]any{"index_sync": res, "warnings": a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("default index: %d path(s) updated, %d kept\n", len(res.Updated), len(res.Kept))
	return nil
}

func (a *app) drop(l *Stage, p *parsed) error {
	tree, err := l.Drop(p.has("--force"))
	if err != nil {
		return err
	}
	if a.json {
		a.emit(map[string]any{"dropped": l.Agent, "staged_tree": tree, "warnings": a.warnings})
		a.warnings = nil
		return nil
	}
	a.printf("dropped stage %s\n", l.Agent)
	if tree != "" {
		a.printf("its staged tree was %s (inspect it with `git diff %s %s`)\n", tree, short(l.M.Base), tree)
	}
	return nil
}
