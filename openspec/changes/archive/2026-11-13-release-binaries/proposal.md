# Proposal

## Why

The Claude Code and Codex plugins call the `git-multistage` binary, but the only way to get it is `go install` or a clone, so every plugin user needs a Go toolchain. A tagged release with prebuilt binaries lets users install the tool, and therefore the plugins, without Go.

## What Changes

- Add a release workflow. It is the only workflow that runs on its own: pushing a tag `v*` builds `git-multistage` for Linux, macOS, and Windows on amd64 and arm64, and publishes the archives and a checksums file in a GitHub release.
- Add a local release command, `mise run release <version>`. It builds the same six archives and `checksums.txt` on the maintainer's machine and publishes them with `gh release create`. GitHub Actions minutes are not needed, because creating a release and uploading assets are API calls. The first release, `v0.0.1`, is made this way.
- Stamp the tag into the binary, so `git-multistage version` reports the release.
- Document the install from a release in `README.md`: a direct download, and `mise use github:chrisfls/git-multistage`. `go install` and the build from a clone stay.
- Tell plugin users in the plugin install steps that the binary comes first.

## Capabilities

### New Capabilities

- `releases`: what a release publishes: the targets, the archive and checksum names, the version stamp, and the install paths.

### Modified Capabilities

- `agent-plugins`: the plugin install steps require the binary, and name the release as the way to get it without Go.

## Impact

- New file `.github/workflows/release.yml`.
- `README.md` install and plugin install sections.
- No change to the CLI's commands or flags. `version` already exists.
- Out of scope: a Homebrew tap, a Scoop bucket, code signing, and a hook that downloads the binary by itself. Downloading and running a binary from a hook is a risk that this change does not take.
- The workflow only builds and publishes, on one Linux runner, and runs no tests: tests run locally (change `run-ci-locally`). It also has a manual trigger, so a build can be started without a tag.
- The workflow needs Actions to run (see the GitHub billing error), so the local command is the release path until it does. For users other than the owner to download a release, the repository must be public.
