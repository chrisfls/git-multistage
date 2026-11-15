# agent-plugins Specification

## MODIFIED Requirements

### Requirement: Harnesses that read the Claude Code plugin
The Claude Code plugin files SHALL install in each harness that reads them: Antigravity, Factory Droid, GitHub Copilot CLI, Devin CLI, Grok Build, ZCode, MiniMax Code, Qoder CLI, CodeBuddy, Goose, and Qwen Code take the skill and the hooks; OpenHands takes the skill; Oh My Pi takes the skill, and the Pi extension of `package.json`; Mistral Vibe takes the skill; DeepSeek Harness takes the hooks file. The repository SHALL hold no root `plugin.json`, `gemini-extension.json`, or `hooks.json`, which keep Antigravity from converting the hooks at install.

#### Scenario: Antigravity install
- **WHEN** the user runs `agy plugin install https://github.com/chrisfls/git-multistage`
- **THEN** Antigravity installs the `git-multistage` skill and hooks that run `git multistage hook` at `SessionStart` and around `write_to_file`, `replace_file_content`, `multi_replace_file_content`, and `edit_file`

#### Scenario: Droid install
- **WHEN** the user runs `droid plugin marketplace add https://github.com/chrisfls/git-multistage` and `droid plugin install git-multistage@git-multistage`
- **THEN** Droid installs the skill and the hooks

#### Scenario: Qwen Code install
- **WHEN** the user installs the plugin with `qwen extensions install` and runs a Qwen Code session that edits a file
- **THEN** the edit is recorded once, in stage `qwen-<session ID>`

### Requirement: JavaScript plugins
The repository SHALL hold a JavaScript plugin for each harness family that loads JavaScript plugins: OpenCode (also loaded by Kilo Code, MiMo Code, and CodeArts Agent), Pi (also loaded by Oh My Pi), and Amp. Before and after a call of a file tool, each plugin SHALL run `git multistage hook <editor>` with a hook input in Claude Code's shape, and SHALL add the text of the hook's message to the result of the tool call. It SHALL keep the session start notice of a session until the result of one of the session's calls carries it. A failure to run the hook SHALL let the tool call proceed.

#### Scenario: OpenCode edit
- **WHEN** the OpenCode plugin receives `tool.execute.before` and `tool.execute.after` for an `edit` call that changes a file in an opted-in checkout
- **THEN** the edit is recorded in stage `opencode-<session ID>`, and the tool output ends with the first-edit message

#### Scenario: Binary missing
- **WHEN** a JavaScript plugin cannot run `git multistage`
- **THEN** the tool call proceeds with its own result

#### Scenario: First call fails
- **WHEN** the first file-tool call of a session fails before it runs, so the host reports no result, and a later call succeeds
- **THEN** the later call's result carries the session start notice

#### Scenario: Host names
- **WHEN** the OpenCode plugin runs in Kilo Code, whose binary may be named `.kilo`, or the Pi extension runs in Oh My Pi
- **THEN** the stages are named `kilo-<session ID>`, or `omp-<session ID>`, and each session records into one stage

### Requirement: Hermes Agent plugin
The Hermes Agent plugin SHALL register the `git-multistage` skill, run `git multistage hook hermes` with the agent's working directory before and after a call of `write_file` or `patch`, and give the hook's messages to the model before its next call.

#### Scenario: Hermes edit
- **WHEN** the Hermes plugin receives `pre_tool_call` and `post_tool_call` for a `write_file` call that creates a file in an opted-in checkout
- **THEN** the edit is recorded in stage `hermes-<session ID>`, and the next `pre_llm_call` returns the first-edit message

#### Scenario: Hermes runs in its own folder
- **WHEN** the Hermes process runs in its install folder, and the agent works in a checkout elsewhere
- **THEN** the hook records the agent's edit in that checkout

### Requirement: Hook files to copy
For each harness that reads hooks only from its own settings, the repository SHALL hold a hook file that the user copies: `hooks/openhands.json`, the shared hooks file without `SubagentStart`, for OpenHands, `hooks/kiro.json`, the `hooks` object of a Kiro CLI agent configuration, and the executable files `hooks/cline/PreToolUse`, `hooks/cline/PostToolUse`, and `hooks/cline/TaskStart` for Cline. Each SHALL run `git multistage hook` with the harness's editor name.

#### Scenario: Kiro hook file
- **WHEN** the test suite reads `hooks/kiro.json`
- **THEN** it runs `git multistage hook kiro` at `agentSpawn`, and at `preToolUse` and `postToolUse` of `fs_write`
