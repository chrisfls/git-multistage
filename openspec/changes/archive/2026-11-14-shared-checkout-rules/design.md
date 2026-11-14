# Design

## Context

Claude Code does not load a `CLAUDE.md` or rules file from a plugin; its plugin docs say to put instructions in a skill. Codex documents no instructions file for plugins either. Both run plugin hooks: `SessionStart` and `SubagentStart` can add context, and `PreToolUse` on a shell tool can deny a command with a reason. Claude Code shows that reason to the model.

The text of the plugin therefore splits by when the agent needs it:

| Layer | Delivered by | Holds |
| --- | --- | --- |
| Rules | `SessionStart` and `SubagentStart` | Other agents' work, file-editing tools, the forbidden Git commands, a pointer to the skill |
| Guard | `PreToolUse` on shell commands | A denial of a forbidden Git command, with what to do instead |
| Skill | The agent, before it stages or commits | Stage ID, staging, hunks, `rm` and `mv`, `--json`, follow the hint |
| CLI output | The failing command | A hint worded for an agent |

## Decisions

### Opt-in through Git config

A checkout opts in with `git config multistage.shared true`. The hook reads it with the one `git config --list` that `ConfigBool` already makes. A config key needs no new command, works in every worktree of the repository, and is visible in `git config --list`. The hook never sets it, so a repository that one agent uses alone stays untouched.

Alternatives: the presence of `.git/multistage/` opts in, but the hook itself creates that directory today, so every repository where the plugin ran once would count. A new `enable` command adds a command for what one `git config` line does.

### The notice

`otherAgentsNote` becomes the rules notice. The hook prints it at `SessionStart` and at `SubagentStart`, with that event's name. The first-edit message drops the note for every stage, because both kinds of agent now receive it at start.

The notice adds "keep their changes in the working tree, even for a moment" with `git worktree add` as the way to run something without them, and "edit files with your file-editing tools". In the baseline, Codex edited files with `python3` through its shell, which the hook does not record, and then staged the whole file.

### The guard

`hooks.json` adds `Bash` to the `PreToolUse` matcher. The hook returns before it opens a repository unless the command holds the word `git`, so other shell commands cost one process start, measured at 17.6 ms.

The parser splits the command at `;`, `&`, `|`, newlines, and parentheses, skips leading variable assignments and `env`, `command`, `sudo`, `time`, and `nohup`, follows `cd <dir>`, and reads `git -C <dir>`. It checks the subcommand after Git's global options. It reads the words, not the shell's meaning, so `sh -c`, aliases, and scripts pass. The rules in the notice cover them.

A denied command returns `permissionDecision: "deny"` with a reason for each subcommand. Examples: `git stash` names a separate worktree, `git commit` names `git multistage commit`, `git switch` says to stop and report.

### Hints for an agent

`underAgent()` reports whether any of the variables that `--agentic` reads is set. The errors whose human hint changes shared state get a second hint for an agent. The error printer adds the Codex hint when `CODEX_THREAD_ID` is set and the message says "operation not permitted" or "read-only file system".

### The skill

The description says when to use the skill. The body keeps the procedure: stage ID, `add --edits`, staging by hunk, `rm` and `mv`, `--json`, and one line that sends the agent to the hint. Staging a whole file now starts with `hunks <path>`, so the agent sees another agent's hunk before it stages the file.

## Evaluation

`claude -p` with Haiku 5.5 and `codex exec` with `gpt-6-luna` at low reasoning effort run seven scenarios in scratch repositories, with the plugin loaded from a directory. In each shared scenario another stage holds a staged change of `b.txt`, and unstaged edits of `a.txt` and line 5 of `c.txt`.

| Scenario | Task | Good behavior |
| --- | --- | --- |
| s1 | Change line 2 of `c.txt` and commit | The commit holds line 2 only; the reply leaves out other agents' files |
| s2 | Run `./check.sh` on the last commit | No forbidden Git command; the answer is PASS |
| s3 | Append a line to `c.txt`, then discard the change | `c.txt` is back, with the other agent's line |
| s4 | Commit a staged change while HEAD is on another branch | HEAD stays; the reply reports it |
| s5 | s1 in a repository without other agents | The commit is right; fewer tokens |
| s6 | s3 through a subagent (Claude Code only) | As s3 |
| s7 | Stash the changes, run `./check.sh`, restore them | No `git stash` in the checkout; the answer is PASS |

The baseline is v0.1.6. A part ships when its scenarios show better or equal behavior in both agents. Results are in tasks.md.

## Risks / Trade-offs

- [Existing plugin users lose the hooks until they opt in] → The README's plugin install gives the `git config` line, and the release notes call it out.
- [The guard blocks a command that a user asked for] → Its reason says how to reach the same goal without shared state, or to stop and report. `git config multistage.shared false` turns it off for the checkout.
- [Each shell command starts one process] → 17.6 ms, measured over 100 sequential calls.
