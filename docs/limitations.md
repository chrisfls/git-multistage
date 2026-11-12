# Limitations, unsupported setups, and recovery

## The shared working tree

Every stage works on the same files on disk. git-multistage separates staging and commits; edits to the files themselves are coordinated only for editors that run the editor hook (see [Editor hooks](#editor-hooks)).

- When two writers read the same version of a file, edit different lines, and write it back, the second write erases the first edit on disk. No index can recover an edit that is no longer in the file. Without the editor hook, the first writer's hunk ID then no longer matches, and `add --hunk` fails with exit code 4 instead of staging the wrong content (`TestLimitationConcurrentWritesToOneFile` in `test/crash_test.go`). With the hook, hooked editors take turns on a file, and `add --edits` refuses an edit that another writer erased.
- Untracked files are shared. Every stage's `status` lists every untracked file.
- `git multistage add <path>` stages the whole file as it is on disk, including edits in it that belong to other stages. The tool refuses a whole-file add only when another stage has *staged* changes in that file; it cannot know about edits that nobody has staged. In a file that several writers edit, stage recorded edits or hunks.

## Hunk granularity

- Hunks are zero-context, so each run of changed lines is one hunk. Two writers' edits on adjacent lines, with no unchanged line between them, form one hunk, and a new file is one hunk. Recorded edits (`add --edits`) have no such limit: each edit is replayed at its own lines. Without the hook, write a patch and use `git multistage apply <patch>`.
- When two stages' changes sit on adjacent lines, Git's three-way merge reports a conflict. git-multistage then replays the staged lines at their positions, which succeeds when the tip left those lines unchanged. Changes to the same lines, or insertions at the same point, still conflict.

## Editor hooks

`git multistage hook claude` is a Claude Code `PreToolUse` and `PostToolUse` hook for the `Edit`, `MultiEdit`, and `Write` tools.

- Before an edit, it takes a lease on the file for up to 15 seconds and snapshots the file. A second hooked editor of the same file waits up to 20 seconds, then its edit is blocked with a message that asks the agent to retry.
- After the edit, it records the before and after content in the agent's stage: `claude-<agent_id>` inside a subagent, `claude-<session_id>` in the main session, or `GIT_MULTISTAGE_ID` when that is set. The first recorded edit tells the agent its stage ID.
- Edits made through Bash (`sed`, code generators, formatters) are not recorded; stage those with `add` or `add --hunk`.
- A lease that expires during a long permission prompt marks the edit as uncertain; `add --edits` then refuses it.
- Codex: its `PreToolUse` and `PostToolUse` hooks fire for `apply_patch`, but a hook definition runs only after a one-time trust review in Codex. git-multistage has no Codex hook yet.

## Hooks

- Hooks run in the shared checkout, with the caller's environment. `pre-commit` sees a temporary index that holds the tree to be committed, with HEAD at the parent commit.
- A `pre-commit` hook that changes the staged content, such as a formatter that re-adds files, makes the commit fail with kind `hook-modified`, and the stage keeps its changes. The hook reads re-added files from the shared working tree, where unstaged edits that belong to other stages may be. Stage your changes again after the hook has formatted the files on disk; the next commit then passes.
- Hooks that stash or rewrite unstaged changes, such as the pre-commit framework or lint-staged, change files that other writers are editing. They are unsafe while several stages are in use. The tool does not detect them.
- When a commit replays onto a newer tip, `pre-commit` runs again only if the replay changes what the commit introduces.

## Supported and refused setups

| Setup | Behavior |
| --- | --- |
| Detached HEAD | A stage created on a detached HEAD targets HEAD and commits with `update-ref --no-deref`. |
| HEAD on another branch or state than the stage's target | `commit` fails with exit code 5. Switch back, or `drop` the stage. |
| Merge, cherry-pick, or revert in progress | `commit` lands when the operation has not changed the same paths in the default index; the operation's own commit later includes it. Otherwise it fails with exit code 5 and lists the paths. |
| Rebase or `git am` in progress | `commit` fails with exit code 5: a rebase finishes by moving the branch from the commit it started on. |
| Sparse checkout, sparse index | Supported. Stage indexes copy the skip-worktree bits of the default index. |
| Split index | Supported. Stage indexes are written whole; the default index keeps its format. |
| Bare repository | Fails: a stage needs a working tree. |
| Git older than 2.45 | Fails at startup. |

## Differences from `git commit`

- No editor: pass `-m` or `-F`. Without either, `commit` fails.
- The message is cleaned like `git commit -m` (`git stripspace`). `commit.cleanup` and `commit.template` are not applied.
- No `--amend`, `-a`, `--author`, `--date`, `--fixup`, or path arguments. Set `GIT_AUTHOR_*` and `GIT_COMMITTER_*` to change the identity.
- The reflog entry reads `commit (multistage <stage-id>): <subject>`.
- `GIT_INDEX_FILE` in the environment is ignored; the tool chooses each index itself.

## Default index

- After each stage commit, the default index is brought from the commit's parent to the commit, except at entries that someone staged natively. Those entries are kept, and `git status` shows them against the new HEAD. Check them before running `git commit`.
- A tool that moves the branch without updating the index, such as a plain `git update-ref`, leaves the default index behind. The next stage commit catches that lag up when the index exactly matches an older commit and the branch's reflog shows only index-free updates since. `git multistage sync` finds the matching commit itself; when the index also holds natively staged changes, `doctor` names the closest commit for `sync --from <commit>`. Until then, `git status` shows those commits as staged reversals, and a native `git commit` would undo them.
- With `multistage.syncIndex=false`, stage commits leave the default index behind; their reflog entries say `index not synced`, so the next synced commit or `sync` catches up.
- A process killed while it holds `.git/index.lock` leaves the lock behind, as a killed `git commit` would. Git commands then fail with "Another git process seems to be running". See below.

## Garbage collection

While a stage has staged changes or recorded edits, `refs/worktree/multistage/<stage-id>` points to a commit that holds the staged tree and the recorded blobs, so `git gc` keeps them. The ref appears in `git log --all` and `git for-each-ref`, and disappears when nothing is staged. A pending commit that garbage collection pruned is treated as not landed.

## Recovery

| Symptom | Cause | Steps |
| --- | --- | --- |
| `commit` exits 3 with conflict paths | The branch changed the same lines. The stage keeps its changes. | Edit the files so they include both changes, `git multistage restore --staged <path>`, stage again, commit. Or `restore --staged` and give up the change. |
| `status` warns that the base is behind | Same as above, found before committing. | Same as above. |
| Exit 4, stale hunk | The file changed after `hunks` ran. | Run `hunks <path>` again and use the new IDs. |
| Exit 6, index locked | `.git/index.lock` exists. | Wait and retry. If no Git process runs (`ps`, Task Manager), delete `.git/index.lock`, then run `git multistage sync` (with `--from <parent of the last stage commit>` if it asks for one). |
| `commit` fails with `hook-modified` | The pre-commit hook changed staged files. | Look at the files on disk, stage your changes again, and commit. |
| `add --edits` fails with `edit-lost` or `edit-conflict` | Another writer erased or overlapped the recorded edit. | Redo the edit, or stage the file with `hunks` and `add --hunk`; `drop --edits <path>` forgets the records. |
| A command reports a damaged stage | The stage's index file is missing. | `git multistage drop --force`, then stage again. The staged tree may still be readable from `refs/worktree/multistage/<stage-id>` before the drop. |
| Exit 6, stage busy | Another process holds the same stage. | Wait for it. The OS releases the lock when a process dies, so a crash never leaves it held. |
| A warning about an interrupted commit | A commit was killed. The next command finished or discarded it. | Nothing. `git multistage log` and `git log` show the result. |
| `git status` shows stage commits as staged reversals | The default index is behind the branch. | `git multistage doctor`, then `git multistage sync`, or `sync --from <commit>` for the commit that the index matches. |
| A stage is no longer needed | | `git multistage drop`. With `--force`, the command prints the staged tree ID; `git diff <base> <tree>` shows its content until `git gc` prunes it. |

`git multistage doctor` changes nothing. It reports the Git version, the HEAD state, operations in progress, a leftover `index.lock`, whether the default index matches the branch, and each stage's base, staged files, pending commit, and problems.
