# Tasks

## 1. Hook input

- [x] 1.1 Map the Cline CLI's `agent_start`, `tool_call`, and `tool_result`, find the nested event object by the mapped name, and add `editor` and Grok's `hashline_edit` to the file tools and to the matcher of `hooks/hooks.json`. Verify with the Cline CLI case of `TestReadHookCall` and a live Cline CLI run
- [x] 1.2 Take `droid` from `DROID_PLUGIN_ROOT` and `qwen` from `QWEN_PROJECT_DIR`. Verify with `TestPickEditor` and a live Qwen Code run that records into one `qwen-` stage

## 2. Plugins and hook files

- [x] 2.1 Keep the session start notice by session in the OpenCode, Pi, and Amp plugins. Verify that `TestJSPlugins`, whose driver now makes a first call that never reports a result, fails on the old plugins and passes on the new ones
- [x] 2.2 Strip a leading `.` from the host binary's name in the OpenCode plugin, take the editor `omp` from `process.title` in the Pi extension, and remove `hooks/pre/git-multistage.js`. Verify with live Kilo Code and Oh My Pi runs that each records into one `kilo-` or `omp-` stage
- [x] 2.3 Remove `hooks/qwen.json`, write `hooks/kiro.json` as a Kiro agent's `hooks` object, add `hooks/openhands.json`, and give the Hermes hook the agent's working directory. Verify with `TestHarnessHookFiles`, `TestOpenHandsHooks`, and `TestHermesPlugin`

## 3. README and checks

- [x] 3.1 Correct the install steps of OpenCode, Kilo Code, MiMo Code, Oh My Pi, Copilot, CodeBuddy, Qwen Code, Gemini CLI, Kiro CLI, OpenHands, CodeArts Agent, Muse Code, and Aider
- [x] 3.2 Add `scripts/live-check.sh`, and verify that it prints `PASS claude`, `PASS codex`, and `PASS opencode`
- [x] 3.3 Raise every manifest to the release version of this change, the next patch version after the newest tag: 0.1.13, after v0.1.12
- [x] 3.4 Run `mise run ci` once, and verify that it passes. It passed on `0574bf2` in 66 s, run on an export of that commit, and `scripts/live-check.sh` then printed `PASS claude`, `PASS codex`, and `PASS opencode` on the same commit

## Results

Live runs: one real session that changed `c.txt` and committed it with `git multistage`, checked by the commit holding only `c.txt`, a stage named after the harness, stage `other` keeping `b.txt`, and `a.txt` keeping the other agent's edit.

| Harness | Version | Install checked | Live run |
| --- | --- | --- | --- |
| Claude Code | 2.1.310 | `--plugin-dir` | PASS, `scripts/live-check.sh` |
| Codex | 0.168.0 | local marketplace | PASS, `scripts/live-check.sh` |
| OpenCode | 1.19.5 | `plugin` entry with the `file://` path; `opencode plugin` refuses GitHub specs | PASS, `scripts/live-check.sh` |
| Qwen Code | 0.27.3 | `qwen extensions install` of a local copy | PASS, one `qwen-` stage |
| Kilo Code | 7.8.27 | `kilo.json` `plugin` with the `file://` path, and `git:file://` | PASS, `kilo-` stage |
| MiMo Code | 0.1.17 | `mimocode.json` `plugin` with the `file://` path | PASS |
| Pi | 1.5.2 | `pi install <clone>`, `pi -e <clone>` | PASS |
| Oh My Pi | 18.20.2 | `omp plugin marketplace add`, `omp plugin install` | PASS, one `omp-` stage (Haiku wrote `2:TWO` into the file in Oh My Pi's line format; Sonnet passed) |
| Grok Build | current | local-path install, 1 skill and 4 hooks | PASS |
| Kimi Code | 2.6.2 | skill and hooks from config; `/plugins install` needs the TUI | PASS |
| ZCode | 3.18 | README steps with a local clone; 3 of 4 hooks (no `SubagentStart`) | PASS |
| Cline CLI | 0.0.13 | `hooks/cline/` in the hooks folder, skill in `.clinerules/` | PASS after 1.1 |
| Goose | 1.59.1 | `goose plugin install file://<clone>` | PASS; the first-edit message did not reach the model |
| Hermes Agent | 0.23.1 | `hermes plugins install` of a `file://` clone | PASS after the working-directory fix |
| OpenHands CLI | 1.16.0 | skill copy and `hooks/openhands.json` | PASS |
| Mistral Vibe | 3.1.2 | `plugins/vibe` copy; headless runs need `--auto-approve` | PASS |
| Gemini CLI | 0.68.0 | extension install, skill and hooks loaded, `SessionStart` fired | not run: needs a Gemini key |
| Antigravity | 1.5.8 | `agy plugin install` converts the hooks | not run: account refused (HTTP 403) |
| Factory Droid | 1.12.1 | README steps; a marker hook ran at `SessionStart` | not run: needs a login |
| Qoder CLI | 1.1.89 | `plugins validate`: 1 skill, 4 hooks | not run: needs a login |
| GitHub Copilot CLI | 1.0.112 | `copilot plugin install <clone>` loads the skill | not run: needs a login; hooks unconfirmed |
| DeepSeek Harness | main | its Claude bridge parses `hooks/hooks.json` | not run |
| Aider | 0.86.2 | `--read` loads the skill; no hooks | edits; prints the commit commands without running them |
| Cursor CLI | 2026.11.13 | every plugin command needs a login | not run |
| Kiro CLI | 3.3.0 | agent validation needs a login | not run; offline replay records |
| Devin CLI | 3000.12.57 | every plugin command needs a login | not run; offline replay records |
| Amp | current | `amp plugins add` needs a login | not run |
| CodeBuddy | current | local install works, then needs `plugin enable` | not run: needs a login |
| MiniMax Code | 0.7.7 | a clone in `~/.minimax/plugins` is not listed | not run |
| Muse Code, CodeArts Agent | — | need vendor accounts | not run |

## Workflow follow-up

- Archive the change after every task is done.
- Release the patch version of task 3.3 with `mise run release <version>`, after the archive is pushed.
