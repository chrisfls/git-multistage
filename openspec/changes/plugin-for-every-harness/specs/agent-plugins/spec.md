# agent-plugins Specification

## MODIFIED Requirements

### Requirement: Plugin manifests
The repository SHALL provide an install path from `chrisfls/git-multistage` for each supported harness: the Claude Code plugin and marketplace (`.claude-plugin/`), the Codex plugin and marketplace (`.codex-plugin/`, `.agents/plugins/`), and the install files that the other requirements of this spec name. The README SHALL give the install steps of each supported harness. The plugin install steps SHALL say that the `git-multistage` binary must be on the `PATH` first, and SHALL link to the install from a release.

#### Scenario: Claude Code install
- **WHEN** the user runs `/plugin marketplace add chrisfls/git-multistage` and `/plugin install git-multistage@git-multistage`
- **THEN** Claude Code installs the hooks and the `git-multistage` skill

#### Scenario: Install steps name the binary
- **WHEN** a user reads the plugin install steps in the README
- **THEN** they say that the binary is needed first, and link to how to get it without Go

#### Scenario: Every supported harness
- **WHEN** a user reads the plugin install steps in the README
- **THEN** they find an entry for each of Claude Code, Codex, Antigravity, OpenCode, Devin CLI, Grok Build, Kimi Code, Muse Code, DeepSeek Harness, ZCode, Qwen Code, GitHub Copilot CLI, Cursor CLI, Kiro CLI, Mistral Vibe, MiniMax Code, MiMo Code, Qoder CLI, Pi, Hermes Agent, Factory Droid, CodeBuddy, Amp, Cline, Kilo Code, Goose, Oh My Pi, Aider, OpenHands, CodeArts Agent, and Gemini CLI

### Requirement: Plugin hooks
The plugin's hooks file `hooks/hooks.json` SHALL run `git multistage hook` at `SessionStart` and `SubagentStart`, before and after the file tools of every harness that reads the file, and before `Bash`. The file tools SHALL include `Edit`, `MultiEdit`, `Write`, and `apply_patch`, and the file tools of Antigravity, Factory Droid, GitHub Copilot CLI, Devin CLI, Grok Build, ZCode, MiniMax Code, Qoder CLI, CodeBuddy, OpenHands, Goose, and DeepSeek Harness.

#### Scenario: Edit in a plugin session
- **WHEN** an agent with the plugin installed edits a file with `Edit` in an opted-in checkout
- **THEN** the edit is recorded in the agent's stage

#### Scenario: Session start in a plugin session
- **WHEN** a session with the plugin installed starts in an opted-in checkout
- **THEN** the agent receives the shared-checkout notice

#### Scenario: Shell command in a plugin session
- **WHEN** an agent with the plugin installed runs `git stash` in an opted-in checkout
- **THEN** the hook denies the command

#### Scenario: File tool of another harness
- **WHEN** the test suite reads `hooks/hooks.json`
- **THEN** the matcher of its `PreToolUse` and `PostToolUse` entries selects `write_to_file`, `replace_file_content`, `Create`, `ApplyPatch`, `search_replace`, `file_editor`, `write`, and `edit`

### Requirement: Plugin version
Every plugin manifest SHALL carry one version: the Claude Code plugin, its marketplace entry, the Codex plugin, and the manifests that the other requirements of this spec name. A release that changes a file of the plugin SHALL raise that version to the release version, so that an installed plugin updates to the new files.

#### Scenario: One version
- **WHEN** the test suite reads every plugin manifest
- **THEN** each names the same version

#### Scenario: Release that changes the skill
- **WHEN** a maintainer releases `v0.2.0` with a changed `skills/git-multistage/SKILL.md`
- **THEN** the plugin manifests of that release name version `0.2.0`

## ADDED Requirements

### Requirement: Harnesses that read the Claude Code plugin
The Claude Code plugin files SHALL install in each harness that reads them: Antigravity, Factory Droid, GitHub Copilot CLI, Devin CLI, Grok Build, ZCode, MiniMax Code, Qoder CLI, CodeBuddy, OpenHands, and Goose take the skill and the hooks; Oh My Pi, Qwen Code, and Mistral Vibe take the skill; DeepSeek Harness takes the hooks file. The repository SHALL hold no root `plugin.json`, `gemini-extension.json`, or `hooks.json`, which keep Antigravity from converting the hooks at install.

#### Scenario: Antigravity install
- **WHEN** the user runs `agy plugin install https://github.com/chrisfls/git-multistage`
- **THEN** Antigravity installs the `git-multistage` skill and hooks that run `git multistage hook` at `SessionStart` and around `write_to_file`, `replace_file_content`, `multi_replace_file_content`, and `edit_file`

#### Scenario: Droid install
- **WHEN** the user runs `droid plugin marketplace add https://github.com/chrisfls/git-multistage` and `droid plugin install git-multistage@git-multistage`
- **THEN** Droid installs the skill and the hooks

### Requirement: Harness manifests
The repository SHALL hold a manifest for each harness that reads its own: `.cursor-plugin/` with `hooks/cursor.json` for Cursor CLI, `.kimi-plugin/plugin.json` for Kimi Code, `.muse-plugin/` for Muse Code, and `.hermes-plugin/` for Hermes Agent. Each SHALL ship the `git-multistage` skill and run `git multistage hook <editor>` with that harness's editor name.

#### Scenario: Kimi Code manifest
- **WHEN** the test suite reads `.kimi-plugin/plugin.json`
- **THEN** it names the skill, and hooks that run `git multistage hook kimi` at `SessionStart`, `PreToolUse`, `PostToolUse`, and `PostToolUseFailure`

#### Scenario: Cursor manifest
- **WHEN** the test suite reads `.cursor-plugin/plugin.json` and `hooks/cursor.json`
- **THEN** the hooks run `git multistage hook cursor` at `sessionStart` and around the `Write` tool

### Requirement: Plugin folders
The repository SHALL hold a self-contained plugin folder for each harness that installs a folder and runs only its own hook format: `plugins/vibe/` for Mistral Vibe and `plugins/gemini/` for Gemini CLI. Each folder SHALL hold its manifest, its hooks, which run `git multistage hook vibe` or `git multistage hook gemini`, and a copy of `skills/git-multistage/SKILL.md` with the same content.

#### Scenario: Skill copies
- **WHEN** the test suite reads the skill in `plugins/vibe/` and `plugins/gemini/`
- **THEN** each has the content of `skills/git-multistage/SKILL.md`

#### Scenario: Gemini CLI hooks
- **WHEN** the test suite reads `plugins/gemini/hooks/hooks.json`
- **THEN** it runs `git multistage hook gemini` at `SessionStart`, `BeforeTool`, and `AfterTool`, around `write_file` and `replace`

### Requirement: JavaScript plugins
The repository SHALL hold a JavaScript plugin for each harness family that loads JavaScript plugins: OpenCode (also loaded by Kilo Code, MiMo Code, and CodeArts Agent), Pi (also loaded by Oh My Pi), and Amp. Before and after a call of a file tool, each plugin SHALL run `git multistage hook <editor>` with a hook input in Claude Code's shape, and SHALL add the text of the hook's message to the result of the tool call. A failure to run the hook SHALL let the tool call proceed.

#### Scenario: OpenCode edit
- **WHEN** the OpenCode plugin receives `tool.execute.before` and `tool.execute.after` for an `edit` call that changes a file in an opted-in checkout
- **THEN** the edit is recorded in stage `opencode-<session ID>`, and the tool output ends with the first-edit message

#### Scenario: Binary missing
- **WHEN** a JavaScript plugin cannot run `git multistage`
- **THEN** the tool call proceeds with its own result

### Requirement: Hermes Agent plugin
The Hermes Agent plugin SHALL register the `git-multistage` skill, run `git multistage hook hermes` before and after a call of `write_file` or `patch`, and give the hook's messages to the model before its next call.

#### Scenario: Hermes edit
- **WHEN** the Hermes plugin receives `pre_tool_call` and `post_tool_call` for a `write_file` call that creates a file in an opted-in checkout
- **THEN** the edit is recorded in stage `hermes-<session ID>`, and the next `pre_llm_call` returns the first-edit message

### Requirement: Hook files to copy
For each harness that reads hooks only from its own settings, the repository SHALL hold a hook file that the user copies: `hooks/qwen.json` for Qwen Code, `hooks/kiro.json` for Kiro CLI, and the executable files `hooks/cline/PreToolUse`, `hooks/cline/PostToolUse`, and `hooks/cline/TaskStart` for Cline. Each SHALL run `git multistage hook` with the harness's editor name.

#### Scenario: Kiro hook file
- **WHEN** the test suite reads `hooks/kiro.json`
- **THEN** it runs `git multistage hook kiro` at `SessionStart`, and before and after `fs_write`

### Requirement: Aider setup
The README SHALL tell an Aider user to load the skill as a read-only file and to turn off Aider's own commits, because Aider has no plugins and no hooks.

#### Scenario: Aider
- **WHEN** a user reads the Aider entry of the plugin install steps
- **THEN** it gives `--read` with the skill file and `--no-auto-commits`, or the same settings in `.aider.conf.yml`
