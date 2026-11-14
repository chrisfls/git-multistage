# Tasks

The tasks that change `internal/ms/hook.go`, `hooks/hooks.json`, `skills/git-multistage/SKILL.md`, or `README.md` start after the change `shared-checkout-rules` has pushed its final edits of those files.

## 1. Hook input

- [x] 1.1 Add `internal/ms/hookinput.go`, which reads the event, tool, tool input, tool response, session, subagent, tool call ID, and directory under every field name of design.md, maps the event names, and reads a failed post-tool call as `PostToolUseFailure`. Verify with replay inputs of Antigravity, Vibe, Cline, Cursor, Copilot (camelCase), Grok, OpenHands, and Goose in `test/testdata/hooks/` that `go test ./test -run TestHookHarnesses` (and `TestReadHookCall` for the inputs it does not replay) records each edit in the expected stage, and that `add --edits` commits exactly it
- [x] 1.2 Find the files of a call from a patch in any string of the tool input, or from the path field of a file tool in design.md's list, and use the patch in place of the `apply_patch` tool name in the snapshot, the move, and the own change. Verify that the Codex replay tests still pass, and that an OpenCode `apply_patch` with `patchText` and a Hermes `patch` with a move record as Codex's do
- [x] 1.3 Key the pending record of a call without a tool call ID by a hash of the session, the tool, and the tool input. Verify with the OpenHands and Gemini CLI replays
- [x] 1.4 Take any editor name of lower-case letters, digits, and hyphens in `hook [<editor>]`, and pick the editor of a bare `hook` as design.md says. Keep the Claude Code rule for the change of a call to the editor `claude`. Verify that `hook Claude.Code` exits with code 2, that a Claude-shaped input with `MINIMAX_PLUGIN_ROOT` set records into `minimax-<session>`, and that a Codex input still records into `codex-<session>`
- [x] 1.5 Write the session start notice and the first-edit message in each harness's output format, and name the stage in the notice of Antigravity and Kiro. Verify the output of each replay test against the table in design.md

## 2. Shared plugin files

- [x] 2.1 Extend the matcher of `hooks/hooks.json` to the file tools of design.md. Make `TestSharedHooksFile` check the tools of the matcher and that no root `hooks.json` exists, and verify that `claude plugin validate --strict .` passes
- [x] 2.2 Install the repository with `agy plugin install` into a scratch home, and verify that the converted hooks run `git multistage hook` at `SessionStart` and around `write_to_file`, `replace_file_content`, `multi_replace_file_content`, and `edit_file`, and that `agy plugin validate` passes on the result

## 3. Harness files

- [x] 3.1 Add `.cursor-plugin/plugin.json` and `hooks/cursor.json`, `.kimi-plugin/plugin.json`, and `.muse-plugin/plugin.json` with `.muse-plugin/marketplace.json`. Verify with `TestPluginFiles` that each parses and runs `git multistage hook <editor>` with its editor name at the events of design.md
- [x] 3.2 Add `plugins/gemini/` and `plugins/vibe/` with their manifests, hooks, and skill copies. Verify with `TestPluginFiles` that the skill copies equal `skills/git-multistage/SKILL.md`, and with the Gemini and Vibe replays that their hook commands record an edit
- [x] 3.3 Add `hooks/qwen.json`, `hooks/kiro.json`, and the executable `hooks/cline/PreToolUse`, `hooks/cline/PostToolUse`, and `hooks/cline/TaskStart`. Verify with `TestPluginFiles` that each runs the hook with its editor name, and that the Cline files are executable in the Git index

## 4. Plugins with code

- [x] 4.1 Add `plugins/opencode/git-multistage.js` and the root `package.json`. Verify with a Node driver run by `go test ./test -run TestJSPlugins` that a fake OpenCode host records an `edit` and an `apply_patch`, appends the first-edit message and the session start notice to the tool output, and keeps working when `git multistage` is missing
- [x] 4.2 Add `plugins/pi/git-multistage.js` and `hooks/pre/git-multistage.js`. Verify with the same test that a fake Pi host records an `edit` and adds the message as a text part, and that the Oh My Pi wrapper records into `omp-<session>`
- [x] 4.3 Add `plugins/amp/git-multistage.js`. Verify with the same test that a fake Amp host records an `edit_file` and appends the message to the output
- [x] 4.4 Add `.hermes-plugin/plugin.yaml` and `.hermes-plugin/__init__.py`. Verify with a Python driver run by `go test ./test -run TestHermesPlugin` that a fake Hermes context registers the skill, records a `write_file`, and returns the first-edit message from `pre_llm_call`

## 5. Text

- [ ] 5.1 Change the sentence of `skills/git-multistage/SKILL.md` that names Claude Code's and Codex's file tools so that it names the file tools of the agent's harness, and copy the skill into `plugins/gemini/` and `plugins/vibe/`. Verify that `TestPluginFiles` passes
- [ ] 5.2 Give the install steps of each supported harness in the README's plugin install section, with the harnesses whose edit recording is untested marked. Verify that each harness of the `agent-plugins` scenario "Every supported harness" has an entry
- [ ] 5.3 Change the Purpose of `openspec/specs/agent-plugins/spec.md` to name every supported harness

## 6. Version and checks

- [ ] 6.1 Set every manifest of design.md to the release version of this change, the next patch version after the newest tag, and make `TestPluginVersion` read them all. Verify that it passes
- [ ] 6.2 Run `mise run ci` once, and verify that it passes

## Workflow follow-up

- Archive the change after every task is done.
- Release the patch version that task 6.1 names with `mise run release <version>`, after the archive is pushed.
