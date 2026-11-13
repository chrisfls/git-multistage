# releases Specification

## Purpose
A release publishes prebuilt git-multistage binaries for every supported platform, so users and the agent plugins can install the tool without a Go toolchain.

## ADDED Requirements

### Requirement: Release assets
Pushing a tag that matches `v*` SHALL publish a GitHub release for that tag. The release SHALL hold one archive for each of linux, darwin, and windows on amd64 and arm64, named `git-multistage_<version>_<os>_<arch>`, as `.tar.gz` for linux and darwin and `.zip` for windows. Each archive SHALL hold the `git-multistage` binary (`git-multistage.exe` on windows), the license, and the README. The release SHALL also hold a `checksums.txt` file with the SHA-256 of every archive.

#### Scenario: Tag push
- **WHEN** a maintainer pushes the tag `v0.2.0`
- **THEN** the release `v0.2.0` holds six archives and `checksums.txt`

#### Scenario: Checksums
- **WHEN** a user downloads an archive and `checksums.txt`
- **THEN** the SHA-256 of the archive equals its line in `checksums.txt`

### Requirement: Local release
`mise run release <version>` SHALL build the same archives and `checksums.txt` from the pushed commit of the default branch, and publish them as the release `v<version>`. It SHALL build from an export of that commit, so uncommitted changes in the checkout never enter a release. `scripts/release.sh <version> --build-only` SHALL build the working tree into `dist/` and publish nothing.

#### Scenario: Uncommitted changes
- **WHEN** a maintainer runs `mise run release 0.0.1` on a pushed default branch while the checkout holds uncommitted changes
- **THEN** the release is built from the pushed commit, and the uncommitted changes are not in it

#### Scenario: Clean release
- **WHEN** a maintainer runs `mise run release 0.0.1` on a pushed default branch
- **THEN** the release `v0.0.1` holds six archives and `checksums.txt`

#### Scenario: Build only
- **WHEN** a maintainer runs `scripts/release.sh 0.0.1 --build-only`
- **THEN** `dist/` holds the six archives and `checksums.txt`, and nothing is published

### Requirement: Release refusals
`mise run release <version>` SHALL refuse to run, with a non-zero exit code and nothing published, when the checkout is not on the default branch, when `HEAD` differs from the remote default branch, or when the release or its tag already exists.

#### Scenario: Unpushed commit
- **WHEN** a maintainer runs `mise run release 0.0.1` and `HEAD` is ahead of the remote default branch
- **THEN** the command exits with a non-zero code and publishes nothing

#### Scenario: Existing release
- **WHEN** a maintainer runs `mise run release 0.0.1` and the release `v0.0.1` exists
- **THEN** the command exits with a non-zero code and publishes nothing

### Requirement: Version stamp
A binary from a release SHALL report the release tag, without the leading `v`, when run as `git-multistage version`. A binary that is not from a release SHALL report `dev`.

#### Scenario: Release binary
- **WHEN** the user runs `git-multistage version` with the binary from release `v0.2.0`
- **THEN** it prints `git-multistage 0.2.0`

### Requirement: Install from a release
The README SHALL describe how to install from a release without Go: downloading the archive for the platform, and `mise use github:chrisfls/git-multistage`. Both SHALL put a `git-multistage` binary on the user's `PATH`, so that `git multistage` runs.

#### Scenario: Install with mise
- **WHEN** the user runs `mise use -g github:chrisfls/git-multistage` and then `git multistage doctor`
- **THEN** the command runs, and `git multistage version` reports the release version
