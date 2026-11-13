#!/usr/bin/env bash
# Builds the git-multistage release archives into dist/ and publishes them.
#
#   scripts/release.sh <version>               build the pushed default branch and publish v<version>
#   scripts/release.sh <version> --build-only  build the working tree into dist/; publish nothing
#
# The publishing form builds from `git archive` of the pushed commit, so
# uncommitted changes in a shared checkout never enter a release.
set -euo pipefail

version="${1:-}"
mode="${2:-}"
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]]; then
  echo "usage: scripts/release.sh <version, such as 0.0.1> [--build-only]" >&2
  exit 2
fi
if [[ -n $mode && $mode != --build-only ]]; then
  echo "unknown option: $mode" >&2
  exit 2
fi

root="$(git rev-parse --show-toplevel)"
dist="$root/dist"
src="$root"
commit="$(git -C "$root" rev-parse HEAD)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [[ -z $mode ]]; then
  default="$(git -C "$root" symbolic-ref --short refs/remotes/origin/HEAD | cut -d/ -f2-)"
  git -C "$root" fetch --quiet origin
  if [[ "$(git -C "$root" rev-parse --abbrev-ref HEAD)" != "$default" ]]; then
    echo "refusing: the checkout is not on $default" >&2
    exit 1
  fi
  if [[ "$commit" != "$(git -C "$root" rev-parse "origin/$default")" ]]; then
    echo "refusing: HEAD differs from origin/$default; push or pull first" >&2
    exit 1
  fi
  if git -C "$root" rev-parse -q --verify "refs/tags/v$version" >/dev/null ||
    gh release view "v$version" >/dev/null 2>&1; then
    echo "refusing: v$version already exists" >&2
    exit 1
  fi
  src="$tmp/src"
  mkdir "$src"
  git -C "$root" archive "$commit" | tar -x -C "$src"
fi

rm -rf "$dist"
mkdir -p "$dist"
stage="$tmp/stage"

ldflags="-s -w -X github.com/chrisfls/git-multistage/internal/ms.Version=$version"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}"
  arch="${target#*/}"
  exe=git-multistage
  [[ $os == windows ]] && exe=git-multistage.exe
  name="git-multistage_${version}_${os}_${arch}"
  dir="$stage/$name"
  mkdir -p "$dir"
  (cd "$src" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$ldflags" -o "$dir/$exe" ./cmd/git-multistage)
  cp "$src/LICENSE" "$src/README.md" "$dir/"
  if [[ $os == windows ]]; then
    (cd "$dir" && zip -q "$dist/$name.zip" "$exe" LICENSE README.md)
  else
    tar -czf "$dist/$name.tar.gz" -C "$dir" "$exe" LICENSE README.md
  fi
  echo "built $name"
done

(cd "$dist" && shasum -a 256 git-multistage_* >checksums.txt)

if [[ -z $mode ]]; then
  gh release create "v$version" "$dist"/* --target "$commit" --title "v$version" --generate-notes
else
  echo "built $dist; nothing published"
fi
