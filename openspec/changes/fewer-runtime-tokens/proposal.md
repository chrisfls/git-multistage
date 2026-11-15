# Proposal

## Why

The plugin's skill still asks for tokens that change no behavior. Its description, which every session of every repository with the plugin installed reads, names a trigger that its first trigger already covers. Its commit procedure runs `diff --cached`, which prints the agent's whole change into its context again, although `add --edits` already prints the files it staged. Measured with the `o200k_base` tokenizer, the description costs 48 tokens and the skill file 746.

Two outside guides were checked for what they add. Caveman's useful rule here is "read less": cut what the agent reads, such as a command's output, and leave the prompt in full sentences, because Caveman cites research that compressed prompts make answers worse. ASD-STE100 keeps one term for each concept, active voice, and every condition and hedge; the skill already follows it. Neither guide's style goes into the shipped text, and nothing in it asks the agent to change how it writes to the user.

## What Changes

- The skill description keeps one sentence and the two triggers that differ: before staging or committing in a shared checkout, and after a failed `git multistage` command.
- The skill's commit procedure is `add --edits`, then `commit`. It no longer runs `diff --cached`. The output of `add` names the files it staged.
- The skill drops the `--json` line and the sentences that repeat an instruction. Every instruction stays.
- The session start notice and the first-edit message stay as they are. A rewrite of the notice saves 7 of its 180 tokens, and its wording came from the ablations of `shared-checkout-rules`.
- The plugin manifests carry the release version, so installed plugins update to the new skill.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The skill keeps every instruction that the `agent-plugins` requirements name, and its description still starts with `Use ` and is one sentence. `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `skills/git-multistage/SKILL.md` and its copies in `plugins/gemini/skills/` and `plugins/vibe/skills/`.
- The plugin manifests' version.
- Out of scope: the CLI's output and hints, the number of Git calls, and the stage ID format, which other changes own.
