#!/usr/bin/env bash
# Sets up a checkout of git-multistage for development: installs the tool
# versions, picks the RuleSync targets of this machine, and generates the agent
# files. Run it once after a clone, and again after installing an agent.
#
#   scripts/setup.sh
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

for tool in mise rulesync; do
  if ! command -v "$tool" >/dev/null; then
    echo "missing: $tool; install it first, such as with brew install $tool" >&2
    exit 1
  fi
done

mise install

# Each agent's home config directory names its RuleSync target. Codex always
# takes the rules, which the repository generates into AGENTS.md.
targets=(codexcli)
for entry in \
  "claudecode:.claude" \
  "antigravity-cli:.gemini" \
  "opencode:.config/opencode" \
  "cursor:.cursor" \
  "devin:.config/devin" \
  "grokcli:.grok"; do
  target="${entry%%:*}"
  dir="$HOME/${entry#*:}"
  if [[ -d $dir ]]; then
    targets+=("$target")
  fi
done

{
  echo '{'
  echo '  "$schema": "https://github.com/dyoshikawa/rulesync/releases/latest/download/config-schema.json",'
  echo '  "targets": {'
  for i in "${!targets[@]}"; do
    target="${targets[$i]}"
    if [[ $target == codexcli ]]; then
      value='["rules", "skills"]'
    else
      value='["skills"]'
    fi
    separator=,
    [[ $i -eq $((${#targets[@]} - 1)) ]] && separator=
    echo "    \"$target\": $value$separator"
  done
  echo '  }'
  echo '}'
} > rulesync.local.jsonc

rulesync install
rulesync generate
