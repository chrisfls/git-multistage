## ADDED Requirements

### Requirement: Move trailers
A commit whose staged change holds moves made with `mv` SHALL end its message with one `Multistage-Moved: <old> -> <new>` trailer for each move. A path that holds `->`, a double quote, a backslash, or a control character, or that starts or ends with whitespace, SHALL be written in double quotes with C-style escapes, as `git diff` quotes paths. The moves of one commit's trailers SHALL take effect at the same time. A move whose new path the commit adds with `--recreate` SHALL get no trailer.

#### Scenario: Recreated move
- **WHEN** stage `a` staged `mv x.go y.go`, stage `b` committed `rm x.go`, and stage `a` runs `commit --recreate y.go`
- **THEN** the commit carries no `Multistage-Moved` trailer for `x.go`, and another stage's later commit of `x.go` fails with `path-deleted`

#### Scenario: Path with an arrow
- **WHEN** stage `a` runs `mv "p -> q.go" r.go` and commits
- **THEN** the trailer is `Multistage-Moved: "p -> q.go" -> r.go`, and another stage's later commit of `p -> q.go` lands in `r.go`

#### Scenario: Path with surrounding spaces
- **WHEN** stage `a` runs `mv " p.go" "r.go "` and commits
- **THEN** another stage's later commit of ` p.go` lands in `r.go `

#### Scenario: Commit a move
- **WHEN** stage `a` ran `mv x.go y.go` and commits
- **THEN** the commit message ends with `Multistage-Moved: x.go -> y.go`

### Requirement: Follow moves onto the tip
When the staged change is replayed onto a newer tip, each staged path that the base holds and that a `Multistage-Moved` trailer of a commit since the stage's base moved SHALL be replayed at the new path, following chains of moves across commits. Paths that no trailer names SHALL follow Git's rename detection. A path that the stage added SHALL stay where the stage put it.

#### Scenario: Several moves in one commit
- **WHEN** stage `b` committed the trailers `a.go -> c.go` and `c.go -> d.go`, and stage `a` then commits its staged change to `a.go`
- **THEN** stage `a`'s commit changes `c.go`

#### Scenario: Edit after another stage's move
- **WHEN** stage `b` committed `mv x.go y.go` with heavy edits of the file, and stage `a` then commits its staged change to `x.go`
- **THEN** stage `a`'s commit changes `y.go` and does not create `x.go`

### Requirement: Staged change of a deleted file
When a commit since the stage's base deleted a path that the staged change modifies or moves, `commit` SHALL exit with code 3 and error kind `path-deleted`, commit nothing, and keep the stage. The error SHALL name the path and the deleting commit, and hint at `--recreate <path>` and `restore --staged <path>`. `commit --recreate <path>...` SHALL commit the stage's version of each named path as a new file.

#### Scenario: Another stage deleted the file
- **WHEN** stage `b` committed `rm x.go` and stage `a` then commits its staged change to `x.go`
- **THEN** the command exits with code 3, names `x.go` and stage `b`'s commit, and stage `a` keeps its staged change

#### Scenario: Recreate the file
- **WHEN** stage `a` then runs `commit --recreate x.go -m "Keep x"`
- **THEN** the commit adds `x.go` with stage `a`'s version

### Requirement: Two stages move one file
When a commit since the stage's base moved a path to a new path, and the stage moved the same path to another new path, `commit` SHALL exit with code 3 and error kind `conflict`, commit nothing, and keep the stage.

#### Scenario: Two names for one file
- **WHEN** stage `b` committed `mv x.go z.go`, and stage `a` staged `mv x.go y.go` and commits
- **THEN** the command exits with code 3, names `x.go`, and stage `a` keeps its staged move

## MODIFIED Requirements

### Requirement: Unsupported commit options
`commit` SHALL accept only `-m`/`--message`, `-F`/`--file`, `-n`/`--no-verify`, `--recreate`, and the global options. Any other option, such as `--amend`, `-a`, `--author`, `--date`, or `--fixup`, SHALL exit with code 2. It SHALL never open an editor and SHALL ignore `GIT_INDEX_FILE` in its environment.

#### Scenario: Unsupported option
- **WHEN** the user runs `commit --amend -m "x"`
- **THEN** the command exits with code 2 and no commit is created

#### Scenario: GIT_INDEX_FILE set
- **WHEN** `GIT_INDEX_FILE` points to another index and the user runs `commit`
- **THEN** the commit uses the stage's own index
