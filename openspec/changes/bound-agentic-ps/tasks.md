# Tasks

## 1. Bound the read

- [ ] 1.1 Add an end-to-end test (Unix only) that puts a `ps` that sleeps 60 seconds first on `PATH`, sets `CLAUDE_CODE_SESSION_ID` and `PASEO_AGENT_ID`, and checks that `--agentic status --json` uses `claude-<id>` within 15 seconds; verify that it fails before the change
- [ ] 1.2 Run `ps` under a 3-second deadline with a `WaitDelay`; verify that the test of 1.1, `go test -count=1 ./...`, and `GOOS=windows go vet ./...` pass

## Workflow follow-up

- Archive the change with `openspec-archive-change` after every task is done.
