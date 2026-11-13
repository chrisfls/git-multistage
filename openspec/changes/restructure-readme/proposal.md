# Proposal

## Why

An outside review of `README.md` found that it leaves two questions open and makes the first minute of reading hard:

- It never says that stages share the working tree. A reader can assume that agents' edits are isolated, when only their staging and commits are kept apart.
- It shows the plugin recording edits and then shows the agent running `add --edits`, without saying which step does what. A reader cannot tell whether installing the plugin is enough.
- The quick start asks the reader to follow two stages, hunk IDs, stage status, and the log before they run one command.
- Install lists five ways with no default. The release way says "unpack the binary" without showing how.
- The "Specs" list of eleven entries and the long Codex sandbox paragraph push the main use cases down.

`AGENTS.md` limits the README to the overview, the install, the quick start, the plugin install, the contributing commands, and links to the specs, and `openspec/config.yaml` forbids new documentation files. The review's ideas are applied inside those limits.

## What Changes

- **Overview:** add a short "What it keeps apart" paragraph. Stages share one working tree. The plugin takes a short lease on each file while an agent edits it, and `add --edits` stages only that agent's lines. When another writer overwrites those lines, `add --edits` refuses with `edit-lost`. Edits made through shell commands are not recorded and are staged with `add` or `add --hunk`. Source: `openspec/specs/editor-hooks/spec.md`, requirements "File leases", "Stage recorded edits", and "Shell edits are not recorded".
- **The problem:** soften "each one needs its own build and dependencies" to say that each worktree can need its own dependency install and build output.
- **Comparison table:** replace "Write a patch by hand" with "Build a patch and run `git apply --cached`".
- **Install:** lead with one command, `mise use -g github:chrisfls/git-multistage`. Put the release archive, `go install`, and the clone build under "Other ways". Show the release archive steps: download, check with `checksums.txt`, unpack, and move the binary to a directory on `PATH`.
- **Quick start:** open with a two-command example that stages one file and commits it, with one sentence on what stays out of the commit. Move the hunk walkthrough, stage status, and log into a "Stage part of a file" subsection, and keep its outputs real. Reword "When no ID is at hand" to "No stage ID yet? `git multistage init` creates one and prints it."
- **Coding agents:** say in one place what the plugin does and what the agent does. The plugin records edits and teaches the commands; the agent stages with `add --edits` and commits. Reword the opening sentence to "Give each agent its own stage ID and point it to the agent guide." Keep the Codex rule, shortened, and state the `drop --force` risk in its own sentence.
- **Specs:** replace the eleven-entry list with one sentence and one link to `openspec/specs/`.
- **Contributing:** keep the commands. Shorten the RuleSync and CI prose.

The review proposed three things that this change leaves out:

- A `CONTRIBUTING.md` and an agent setup guide: `openspec/config.yaml` forbids new documentation files, and `AGENTS.md` puts the contributing commands in the README.
- Trimming the `warning: created stage` line from the outputs: the README shows real output. The two-command example drops output instead, so the warning appears only in the subsection.
- Escaped links and `&#x20;` entities: the file has none, so the review saw a paste artifact.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change edits the README only, so `.openspec.yaml` sets `skip_specs: true`. The README keeps what `releases` ("Install from a release") and `agent-plugins` ("Plugin manifests") require of it: the release archive and mise install, and the plugin's note that the binary must be on `PATH` first, with a link to the install.

## Impact

- `README.md` only.
