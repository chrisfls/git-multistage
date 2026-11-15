#!/usr/bin/env bash
# live-check.sh <claude|codex|opencode>...: run one real session of each
# named coding agent with this checkout's plugin and binary, in a scratch
# shared checkout, and check that the agent commits only its own change.
#
# The scratch checkout holds a staged change of b.txt in stage `other` and
# another agent's unstaged edit of a.txt. The agent changes c.txt with its
# file tool and commits with git multistage. The check passes when the new
# commit holds only c.txt, from a stage named after the agent, and b.txt and
# a.txt stay as they were.
#
# Each agent uses its own login: Claude Code and Codex their logged-in
# accounts, OpenCode OPENROUTER_API_KEY. Codex and OpenCode run with
# configuration in scratch folders, so the user's own stays untouched.
# Model calls cost money; each run is one short session.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/git-multistage-live.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"
go build -o "$work/bin/git-multistage" "$root/cmd/git-multistage"
export PATH="$work/bin:$PATH"

prompt='Change the second line of c.txt from "two" to "TWO" with your file editing tool. Then commit only that change with git multistage, as the git-multistage skill says. Reply with the commit hash only.'

# setup creates the scratch shared checkout in $1.
setup() {
	mkdir -p "$1" && cd "$1"
	git init -q -b main
	git config user.name test && git config user.email test@example.com
	printf 'a1\na2\n' >a.txt && printf 'b1\nb2\n' >b.txt && printf 'one\ntwo\nthree\n' >c.txt
	git add . && git commit -qm init
	git config multistage.shared true
	printf 'b1\nB2\n' >b.txt
	git multistage --stage other add b.txt >/dev/null 2>&1
	printf 'a1\nA2\n' >a.txt
}

# check verifies the result in $1 for the stage prefix $2.
check() {
	local dir=$1 prefix=$2 failures=()
	cd "$dir"
	[ "$(git rev-list --count HEAD)" = 2 ] || failures+=("the branch has $(git rev-list --count HEAD) commits, want 2")
	[ "$(git show --name-only --format= HEAD)" = c.txt ] || failures+=("the commit changes $(git show --name-only --format= HEAD | tr '\n' ' ')")
	[ "$(git show HEAD:c.txt 2>/dev/null)" = $'one\nTWO\nthree' ] || failures+=("the commit does not hold the new c.txt")
	[ "$(cat a.txt)" = $'a1\nA2' ] || failures+=("a.txt changed")
	git multistage --stage other status 2>/dev/null | grep -q b.txt || failures+=("stage other lost b.txt")
	ls .git/multistage/stages | grep -q "^$prefix-" || failures+=("no stage $prefix-*: $(ls .git/multistage/stages | tr '\n' ' ')")
	if [ ${#failures[@]} -eq 0 ]; then
		echo "PASS $prefix"
	else
		printf 'FAIL %s: %s\n' "$prefix" "${failures[@]}"
		return 1
	fi
}

run_claude() {
	setup "$work/claude"
	claude -p "$prompt" --plugin-dir "$root" --model haiku --setting-sources project --strict-mcp-config \
		--dangerously-skip-permissions >"$work/claude.log" 2>&1
	check "$work/claude" claude
}

run_codex() {
	local home="$work/codex-home"
	mkdir -p "$home" && cp "${CODEX_HOME:-$HOME/.codex}/auth.json" "$home/"
	CODEX_HOME=$home codex plugin marketplace add "$root" >/dev/null
	CODEX_HOME=$home codex plugin add git-multistage@git-multistage >/dev/null
	setup "$work/codex"
	CODEX_HOME=$home codex exec --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox \
		--dangerously-bypass-hook-trust "$prompt" >"$work/codex.log" 2>&1
	check "$work/codex" codex
}

run_opencode() {
	local xdg="$work/opencode-xdg"
	export XDG_CONFIG_HOME=$xdg/config XDG_DATA_HOME=$xdg/data XDG_CACHE_HOME=$xdg/cache XDG_STATE_HOME=$xdg/state
	setup "$work/opencode"
	printf '{"plugin": ["file://%s/plugins/opencode/git-multistage.js"], "permission": {"edit": "allow", "bash": "allow"}}\n' \
		"$root" >opencode.json
	echo opencode.json >>.git/info/exclude
	opencode run -m "${OPENCODE_MODEL:-openrouter/anthropic/claude-haiku-4.5}" "$prompt" >"$work/opencode.log" 2>&1
	check "$work/opencode" opencode
}

[ $# -gt 0 ] || set -- claude codex opencode
status=0
for agent in "$@"; do
	case $agent in
	claude | codex | opencode) "run_$agent" || status=1 ;;
	*) echo "unknown agent $agent; use claude, codex, or opencode" >&2 && exit 2 ;;
	esac
done
exit $status
