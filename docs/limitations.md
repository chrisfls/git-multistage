# Limitations, unsupported setups, and recovery

## The shared working tree

All agents read and write the same files on disk. git-multistage separates staging and commits; it does not coordinate edits to the files themselves.

- When two agents read the same version of a file, edit different lines, and write it back, the second write erases the first edit on disk. No index can recover an edit that is no longer in the file. The first agent's hunk ID then no longer matches, and `add --hunk` fails with exit code 4 instead of staging the wrong content. `TestLimitationConcurrentWritesToOneFile` in `test/crash_test.go` reproduces this.
- Agents that share a file must make their edits one at a time, for example with an editor tool that rewrites only the changed lines and re-reads the file before each edit.
- Untracked files are shared. Every agent's `status` lists every untracked file.
- `git multistage add <path>` stages the whole file as it is on disk, including other agents' unstaged edits in it. The tool refuses a whole-file add only when another agent has *staged* changes in that file; it cannot know about edits that nobody has staged. In a file that several agents edit, stage hunks.

## Hunk granularity

- Hunks are zero-context, so each run of changed lines is one hunk. Two agents' edits on adjacent lines, with no unchanged line between them, form one hunk, and a new file is one hunk. To stage part of such a hunk, write a patch and use `git multistage apply <patch>`.
- When two agents' changes sit on adjacent lines, Git's three-way merge reports a conflict when the second commit replays its change. This matches `git merge` and `git rebase`.

## Hooks

- Hooks run in the shared checkout. `pre-commit` sees a temporary index that holds the tree to be committed, with HEAD at the parent commit.
- Hooks that stash or rewrite unstaged changes, such as the pre-commit framework or lint-staged, change files that other agents are editing. They are unsafe with concurrent agents. The tool does not detect them.
- `pre-commit` runs once per commit attempt, so it runs again when the commit replays onto a newer tip.
- A `pre-commit` hook may rewrite staged files, as with `git commit`. A hook that adds paths outside the staged set is refused with exit code 3.

## Refused setups

| Setup | Behavior |
| --- | --- |
| Detached HEAD | Creating a stage fails with exit code 5. |
| HEAD on a different branch than the stage's target | `commit` fails with exit code 5. Switch back, or `drop` the stage. |
| Merge, rebase, cherry-pick, or revert in progress | `commit` fails with exit code 5. |
| `core.sparseCheckout`, `core.splitIndex`, or `index.sparse` enabled | Every stage command fails with exit code 5. |
| Bare repository | Fails: a stage needs a working tree. |
| Git older than 2.45 | Fails at startup. |

## Differences from `git commit`

- No editor: pass `-m` or `-F`. Without either, `commit` fails.
- The message is cleaned like `git commit -m` (`git stripspace`). `commit.cleanup` and `commit.template` are not applied.
- No `--amend`, `-a`, `--author`, `--date`, `--fixup`, or path arguments. Set `GIT_AUTHOR_*` and `GIT_COMMITTER_*` to change the identity.
- The reflog entry reads `commit (multistage <agent>): <subject>`.
- `GIT_INDEX_FILE` in the environment is ignored; the tool chooses each index itself.

## Default index

- After each agent commit, the default index is brought to the new tip, except at entries that someone staged natively. Those entries are kept, and `git status` shows them against the new HEAD. Check them before running `git commit`.
- With `multistage.syncIndex=false`, the default index stays behind until you run `git multistage sync`. Until then, `git status` shows agent commits as staged reversals, and a native `git commit` would undo them.
- A process killed while it holds `.git/index.lock` leaves the lock behind, as a killed `git commit` would. Git commands then fail with "Another git process seems to be running". See below.

## Recovery

| Symptom | Cause | Steps |
| --- | --- | --- |
| `commit` exits 3 with conflict paths | The branch changed the same lines. The stage keeps its changes. | Edit the files so they include both changes, `git multistage restore --staged <path>`, stage again, commit. Or `restore --staged` and give up the change. |
| `status` warns that the base is behind | Same as above, found before committing. | Same as above. |
| Exit 4, stale hunk | The file changed after `hunks` ran. | Run `hunks <path>` again and use the new IDs. |
| Exit 6, index locked | `.git/index.lock` exists. | Wait and retry. If no Git process runs (`ps`, Task Manager), delete `.git/index.lock`, then run `git multistage sync`. |
| Exit 6, stage busy | Another process of the same agent holds the stage. | Wait for it. The OS releases the lock when a process dies, so a crash never leaves it held. |
| A warning about an interrupted commit | A commit was killed. The next command finished or discarded it. | Nothing. `git multistage log` and `git log` show the result. |
| `git status` shows agent commits as staged reversals | The default index is behind the branch. | `git multistage doctor`, then `git multistage sync`. |
| A stage is no longer needed | | `git multistage drop`. With `--force`, the command prints the staged tree ID; `git diff <base> <tree>` shows its content until `git gc` prunes it. |

`git multistage doctor` changes nothing. It reports the Git version, the HEAD state, operations in progress, unsupported settings, a leftover `index.lock`, whether the default index matches the branch, and each stage's base, staged files, pending commit, and problems.
