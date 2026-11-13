# Design

## Approach

A plain GitHub Actions workflow, `.github/workflows/release.yml`, on `push` of tags `v*` and on `workflow_dispatch`. A manual run builds the six archives and uploads them as workflow artifacts without publishing a release. No release tool is added: the build is `go build` with `CGO_ENABLED=0` for six targets, which keeps the toolchain at the Go version that `mise.toml` pins and adds no dependency.

- **Build job:** a matrix over `linux`, `darwin`, `windows` and `amd64`, `arm64`, run on `ubuntu-latest`. It builds with `-trimpath -ldflags "-s -w -X github.com/chrisfls/git-multistage/internal/ms.Version=${tag#v}"`, then archives the binary with `LICENSE` and `README.md`. Cross-compiling needs no other runner, because the module has no cgo and no dependency.
- **Release job:** after the builds, it downloads the archives, writes `checksums.txt` with `sha256sum`, and creates the release with `gh release create` and `--generate-notes`.
- **Permissions:** the workflow asks for `contents: write` only.

## Local release command

`mise run release <version>` is a mise task that runs a script in the repository. The script refuses to run unless the checkout is on the default branch, equal to `origin`, and the release does not exist yet. The checkout may hold uncommitted changes, because other agents work in it, so the script builds from `git archive` of `HEAD`, not from the working tree. It builds the six archives into `dist/` with the same flags as the workflow, writes `checksums.txt`, and runs `gh release create v<version> dist/* --generate-notes`. That call creates the tag on the remote at the pushed commit. The checkout needs no push of its own, and `dist/` is already gitignored. Both paths build from one script: the workflow calls it with `--build-only`, which builds the working tree of the tag checkout and publishes nothing, and publishes the archives in a later step.

## Asset names

`git-multistage_<version>_<os>_<arch>.tar.gz` (`.zip` on windows), with `<version>` without the `v`. mise's `github:` backend picks the asset for the platform from names that carry the OS and architecture, so no mise configuration is needed in the repository. This is tested with a real release before the change is archived.

## Tests

The Version stamp is covered by a unit test of the flag: `go build -ldflags` into a temporary binary, then `version`. The workflow itself is checked by `act` against a tag event where a Docker engine exists, and by the first real tag.

## Risks

- The repository is private and Actions fails on billing today, so the first real release waits on that. Until then the workflow is validated with `act -n` and a dry build of all six targets on this machine.
- A release asset runs as the user's code. Checksums guard against a corrupted download, not against a stolen account; signing is a later change.
