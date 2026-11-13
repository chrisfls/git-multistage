# Proposal

## Why

A second outside review of `README.md`, after `restructure-readme`, found the following:

- **"What it keeps apart"** brings in file leases, `add --edits`, and `edit-lost` before the reader has seen one command. Its first sentence, "every agent edits the same files", reads as if all agents change identical files.
- **"Stage part of a file"** starts the story over and repeats the whole-file staging that the two-command example already shows. Step 3 (`status`) only adds output between the commands that matter.
- **Install** assumes that `$HOME/.local/bin/` exists. It also does not say where `go install` puts the binary, and that directory is often missing from `PATH`.
- **Wording:** several sentences read stiffly, for example "It commits without waiting for the others" and "Stage it by ID:".
- **Codex:** the setup paragraph moves from the plugin install to sandbox permissions in one block.

## What Changes

- **What it keeps apart:** reduce the section to three short statements:
  - Stages share a working tree, so agents can edit the same files, and git-multistage keeps their staged changes and commits apart.
  - With the plugin, an agent's edits through its file tools wait for each other per file. The plugin also records which lines each agent changed.
  - Edits made through shell commands are staged separately.
- **Coding agents:** say what `edit-lost` means next to `add --edits`, the command that can fail with it.
- **Quick start, "Stage part of a file":**
  - Continue from the two-command example, with `fix-login` already committed.
  - Remove the "Stage a whole file" step and the `status` step.
  - Keep the `hunks` and `add --hunk` steps, the commit, the `git log`, and the final `git status`.
  - Replace the commit hashes and outputs with those of a new real run.
- **Install:**
  - Add `mkdir -p "$HOME/.local/bin"` before the `mv` of the release binary.
  - Say that `go install` puts the binary in `$(go env GOPATH)/bin`, or in `GOBIN` when it is set, and that this directory must be on `PATH`.
- **Wording:** apply the review's rewrites:
  - "Stages share a working tree, so agents can edit the same files."
  - "Each agent can commit its changes independently."
  - "The commit contains only changes staged under `fix-login`."
  - "Two agents are working in the same checkout, each with its own stage."
  - "Stage only the first hunk by its ID:"
  - "the hooks report an error while the binary is not on your `PATH`."
- **Codex:** keep the sandbox reason, the rule, and the `drop --force` warning. Cut the connecting prose.

The review's fifth point, a shorter Contributing section, is out of scope, because the review itself recommends leaving that section as it is.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change edits the README only, so `.openspec.yaml` sets `skip_specs: true`. The README keeps what `releases` ("Install from a release") and `agent-plugins` ("Plugin manifests") require of it.

## Impact

- `README.md` only.
