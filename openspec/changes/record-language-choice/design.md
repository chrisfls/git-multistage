# Design

## Context

git-multistage is a Go program with no third-party modules: `go.mod` has no `require` entries. It runs Git as subprocesses (`internal/gitx/gitx.go`) and implements no Git format of its own. The candidates were Go, Python, and TypeScript on Node.

The criteria below are the ones the choice was weighed on. Each cites the spec or file that sets it, such as `openspec/config.yaml`, which requires Windows, macOS, and Linux. See proposal.md for why it is recorded.

## Goals / Non-Goals

**Goals:**

- Record the criteria, their order of weight, and how each candidate meets them.
- Record the measurements, with their unit and conditions, so a later comparison can repeat them.

**Non-Goals:**

- Candidates other than Go, Python, and TypeScript on Node, such as Rust.
- A plan to port the tool or to change how it is released.

## Decisions

### Go is the implementation language

Go meets the three criteria with the most weight from its standard library. Python meets the first and is slower on the second and third. Node fails the first without a native addon.

### Criteria, in order of weight

1. **A lock that the OS releases when its holder dies, on all three systems.** The `crash-recovery` spec requires that each command hold an OS lock on its stage, "so a dead process never leaves it held". A lock that is cleared by a timeout or an age check does not meet this requirement.
2. **Low start-up cost.** The plugins run `git multistage hook` before and after every agent edit (`agent-plugins` and `editor-hooks` specs). Each edit waits for two starts of the program.
3. **No language runtime on the user's machine.** Git runs `git multistage` by finding an executable named `git-multistage` on `PATH`. Windows has neither Python nor Node by default.
4. **OS interfaces from the standard library.** `--agentic` (`agent-stage-detection` spec) walks the process's ancestors (`internal/ms/procs.go`). On Windows this reads the Toolhelp32 process snapshot (`internal/ms/procs_windows.go`). Stage metadata is replaced through `fsync` and a rename.
5. **Few third-party dependencies.** The hook runs inside every agent's edit loop, so each dependency adds supply-chain exposure there.
6. **Running Git subprocesses.** All three candidates do this well. It decided nothing.
7. **Precise types for state.** The pending-commit journal and the stage metadata are state machines, and tagged unions would describe them best. This criterion has the least weight because the tests cover the states.

### How each candidate meets the criteria

| Criterion | Go | Python | TypeScript on Node |
| --- | --- | --- | --- |
| 1. OS lock | Meets: `syscall.Flock` and `LockFileEx` (`internal/filelock/`) | Meets: `fcntl.flock` on Unix, `msvcrt.locking` on Windows | Fails without a native addon: Node 26.12.1 has no `fs.flock`. `proper-lockfile` uses directories and age-based staleness |
| 2. Start-up | About 5 ms | About 30 to 42 ms | About 50 to 60 ms, before the program loads any module |
| 3. No runtime | One static executable for each OS and architecture | Needs Python 3 on `PATH`, or a PyInstaller build that unpacks itself on each start | Needs Node on `PATH`, or a single-executable build of tens of megabytes |
| 4. OS interfaces | `syscall` covers Toolhelp32, `fsync`, and rename | `ctypes` reaches Toolhelp32 with more code | Needs an addon, or a `tasklist` or PowerShell subprocess on Windows |
| 5. Dependencies | None | None needed | At least a lock library or addon, and likely an argument parser |
| 6. Git subprocesses | Good | Good | Good |
| 7. Types | Structs and interfaces, without tagged unions | Optional type hints | Discriminated unions |

### Measurements

Start-up was measured on macOS 26.7.2 (arm64) as 100 sequential starts, one in flight, in two runs:

| Program | Run 1 | Run 2 | Per start |
| --- | --- | --- | --- |
| `git-multistage --help` (Go) | 0.51 s | 0.55 s | about 5 ms |
| `python3 -c pass` (Python 3.9.6) | 3.04 s | 4.18 s | about 30 to 42 ms |
| `node -e ''` (Node 26.12.1) | 6.01 s | 5.06 s | about 50 to 60 ms |

Python and Node ran an empty program, so a real port would start slower than these figures. The end-to-end time of `git multistage hook` was not measured. Its Git subprocesses may take longer than the start-up itself.

## Risks / Trade-offs

- [Installation needs the Go toolchain: `README.md` offers only `go install` and a build from a clone] → Prebuilt executables for each OS would deliver criterion 3 to users. That is a separate change.
- [Contributors who know only TypeScript or Python must learn Go] → Accepted. The `git-multistage-development` skill gives the build, lint, and test commands.
- [Go has no tagged unions for the commit journal and stage metadata] → The crash and concurrency tests in `test/` cover each state.
