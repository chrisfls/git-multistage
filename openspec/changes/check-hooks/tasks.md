# Tasks

## 1. Validate the plugin in CI

- [ ] 1.1 Pin the Claude Code CLI in `mise.toml`, and add a `plugin` job to `.github/workflows/ci.yml` that installs it with `jdx/mise-action` and runs `claude plugin validate --strict .`; verify that the command passes locally with an empty `HOME` and no API key, and that the workflow file parses as YAML

## 2. Replay real hook inputs

- [ ] 2.1 Add Claude Code and Codex hook inputs under `test/testdata/hooks/`, and a test that replays them through `git multistage hook` around a file edit and checks the recording stage, the SessionStart message, and the committed content; verify with `go test -count=1 -run TestHookReplay ./test/` and `GOOS=windows go test -c ./test/`

## Workflow follow-up

- Archive the change with `openspec-archive-change` after every task is done.
