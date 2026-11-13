# Proposal

## Why

A third outside review of `README.md`, after `polish-readme`, found these issues:

- The second sentence of the introduction repeats "each agent" and "changes".
- "Contributing" says that RuleSync generates "each agent's skills, so a fresh clone has neither". "Coding agents" links to `skills/git-multistage/SKILL.md`, so a reader can think the link points at a file that a clone lacks. The file is tracked: it is the plugin's skill. RuleSync generates only the skills in the agent folders, such as `.claude/skills/`, which `.gitignore` excludes.
- "The first command creates the stage `fix-login`" is true only on first use. The `stages` spec ("Implicit stage creation") creates a stage the first time its ID is used.
- "edits that agents make to one file through their file tools wait for each other" is hard to read.

## What Changes

- **Introduction:** "Each agent gets its own staging area and can commit independently, without including changes staged by other agents."
- **What it keeps apart:** "With the agent plugin, agents that edit the same file through their file tools take turns, so their edits do not overlap."
- **Quick start:** "The first command creates `fix-login` if it does not exist yet."
- **Contributing:** RuleSync generates `AGENTS.md` and the skills in each coding agent's folder, such as `.claude/skills/`, so a fresh clone has neither.

The review also asked for `mkdir -p` before the clone build. This change leaves it out: `go build -o` creates missing parent directories. I checked this by building into a directory that did not exist.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change edits the README only, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `README.md` only.
