# Proposal

## Why

Parts of `README.md` repeat other sections, describe internals instead of what a user gets, or read awkwardly. `AGENTS.md` limits the README to the overview, the install, the quick start, the plugin install, the contributing commands, and links to the specs.

The "What else it does" section repeats other sections and is outside that list:

- **Stage part of a file** repeats the comparison table and step 2 of the quick start. "An ID listed before the file changed is refused" is a staging rule that belongs only in the staging spec.
- **Commit at the same time** repeats the table and step 4 of the quick start. "Lands on the newest tip of the branch, and none is lost" describes how commits are integrated.
- **Record agent edits** repeats the "Coding agents" section.
- **Keep `git status` correct** and **Survive crashes** describe the default-index sync and the commit journal. The "Specs" section already links both specs with a one-line summary.

Other sections:

- **Install:** "The binary must be on your `PATH`" sits between the clone build and its Windows variant, and repeats the release instructions. It belongs once, in the section's opening line.
- **Quick start, step 3:** `src/shared.ts` shows under both "staged" and "not staged", but the text explains only `src/login.ts`. The second list holds the unrelated edit of `src/shared.ts`.
- **Quick start, "Set the stage once":** "`git multistage init` creates a stage with a random ID" does not say when to use it.
- **Coding agents:** "the hooks print an error until `git multistage` runs" reads as if the command must be run once. The hooks fail while the binary is missing from `PATH`. The Codex rule paragraph joins two instructions with a semicolon.
- **Specs:** the list omits the `releases` spec in `openspec/specs/releases/`.

## What Changes

- Remove the "What else it does" section.
- Install: state the `PATH` requirement once, in the opening line, and remove the separate sentence.
- Quick start step 3: say that the second `src/shared.ts` entry is the unrelated edit.
- Quick start: say that `git multistage init` without an ID creates a stage with a random ID and prints it, for when no ID is at hand.
- Coding agents: say that the hooks print an error while `git-multistage` is not on `PATH`. Split the Codex rule paragraph into one sentence per instruction.
- Specs: add a `Releases` entry.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change edits the README only, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `README.md`.
- Out of scope: the CI badge, the `act` and stress-test lines, and the CI coverage sentence in "Contributing". The `run-ci-locally` change already rewrites them, and its tasks are not done. The badge points at branch `main`, but the default branch is `master`, so it shows no status until `run-ci-locally` removes it.
