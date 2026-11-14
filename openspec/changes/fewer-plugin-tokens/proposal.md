# Proposal

## Why

The agent plugin puts the same guidance into an agent's context several times. The skill states the delete and move rule three times, and repeats the note about other agents that the session start notice already gave. Its failure table restates most of the `hint:` lines that the CLI prints with each error. The first-edit message repeats the note about other agents in a main session that already received it at session start. Each agent that loads the skill pays for these tokens, and every session pays for the notice and the skill description.

Measured with the `o200k_base` tokenizer as a proxy, the skill body costs 1,684 tokens each time an agent loads it, the skill description 68 tokens in every session, the session start notice 94 tokens at each session start and compaction, and the first-edit message 122 tokens once per stage.

Two outside guides shaped the rewrite. The Caveman compression rules keep code, commands, paths, and every negation verbatim, merge bullets that say the same thing, and keep one example where several show the same pattern. Caveman also cites research that compressed prompts make answers worse, so the shipped text keeps full sentences, and nothing in it asks the agent to change how it writes to the user. ASD-STE100 keeps one term for each concept, active voice, and every condition and hedge.

## What Changes

- Rewrite `skills/git-multistage/SKILL.md` so it states each instruction once. The skill keeps every instruction that the CLI's hints do not give, and sends the agent to the error's hint for the other failures.
- Order the skill's sources of a stage ID: the stage that a git-multistage message names, then the agent ID from the task, then `init`. The current skill lists the first two without an order, and an agent that picked its task's ID would find no recorded edits.
- Shorten the skill description and the session start notice, with the same triggers and the same instructions.
- Leave the note about other agents out of the first-edit message of a main session, which received it at session start. A subagent receives no session start notice, so its first-edit message keeps the note.
- Raise the version in the Claude Code and Codex plugin manifests to the release version. Claude Code caches an installed plugin by its version, so a plugin whose version stays `0.1.0` never delivers the new text to existing installs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-plugins`: the skill states each instruction once, names the order of stage ID sources, and defers to the CLI's hints except where a hint does not fit a shared checkout. The plugin manifests carry the release version.
- `editor-hooks`: the first-edit message of a main session's stage leaves out the note about other agents.

## Impact

- `skills/git-multistage/SKILL.md`.
- `internal/ms/hook.go`: the session start notice and the first-edit message.
- `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, `.codex-plugin/plugin.json`: the version.
- `test/hook_replay_test.go`, `test/plugin_test.go`.
- Out of scope: CLI output and messages other than the hook's, the number of Git calls, and command speed.
