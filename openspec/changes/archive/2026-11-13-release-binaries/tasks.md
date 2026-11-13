# Tasks

## 1. Release script and workflow

- [x] 1.0 Add the release script and the `release` mise task: pushed-commit checks, six builds from an export of `HEAD`, archives, `checksums.txt`, and `gh release create`

- [x] 1.1 Add `.github/workflows/release.yml` as designed: build matrix, `-X ...ms.Version`, archives, `checksums.txt`, and `gh release create`
- [x] 1.2 Build all six targets locally with the same flags, and check that `version` prints the stamped value on the native one

## 2. Version stamp test

- [x] 2.1 Add an integration test that builds the CLI with the `-X` flag and checks `git-multistage version`

## 3. Docs

- [x] 3.1 Add the release install (download and `mise use github:chrisfls/git-multistage`) to the README install section
- [x] 3.2 Add the binary prerequisite and the link to the plugin install steps in the README

## 4. Verify

- [x] 4.1 No Docker engine is installed here, so `act` was not run against the workflow. `act -l` parses it, and `scripts/release.sh --build-only` ran the same build steps locally
- [x] 4.2 Publish `v0.0.1` with `mise run release 0.0.1`, install it with mise on a clean path, and run `git multistage doctor`
