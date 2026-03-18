# Build and Release

## Local Build Targets

```bash
make test
make build
make dist
```

- `make build` builds the current host binary in the repository root
- `make dist` builds the cross-platform release-style binaries under `dist/`

## Current Dist Outputs

`make dist` produces:

- `dist/sandbox-runner-darwin-amd64`
- `dist/sandbox-runner-darwin-arm64`
- `dist/sandbox-runner-linux-amd64`
- `dist/sandbox-runner-linux-arm64`
- `dist/sandbox-runner-linux-arm-v7`
- `dist/sandbox-runner-windows-amd64.exe`
- `dist/sandbox-runner-windows-arm64.exe`
- `dist/SHA256SUMS`

These names are the release contract. CI should reuse them exactly.

## Version Metadata

`--version` includes:

- `version`
- `git_sha`
- `build_time`
- `target_os`
- `target_arch`
- `execution_target`
- `feature_gates`

## GitHub Actions

Workflow file:

- [`.github/workflows/build.yml`](../.github/workflows/build.yml)
- [`.github/workflows/release.yml`](../.github/workflows/release.yml)

Behavior:

- runs on `push`, `pull_request`, and `workflow_dispatch`
- uses Node 24-ready official GitHub actions (`actions/checkout@v6`, `actions/setup-go@v6`, `actions/upload-artifact@v6`)
- executes `go test ./...`
- executes `make dist`
- uploads the generated `dist/` outputs as the `sandbox-runner-dist` workflow artifact

This keeps CI binaries aligned with the same Makefile-driven process developers use locally.

## GitHub Releases

Release workflow:

- [`.github/workflows/release.yml`](../.github/workflows/release.yml)

Behavior:

- runs when a `v*` tag is pushed
- can also be started manually with an existing tag input such as `v0.1.0`
- reruns `go test ./...`
- reruns `make dist`
- uploads the resulting binaries both as workflow artifacts and as GitHub Release assets
- creates the release if missing, or updates assets if the release already exists

This intentionally rebuilds from the tag commit instead of trying to reuse a previous workflow artifact from the web UI.

Important note:

- tags created before this workflow existed do not trigger a retroactive release run
- for those older tags, use `Actions -> Release -> Run workflow` and provide the existing tag name

## Release Branches and Tags

Recommended model:

- `main` for ongoing development
- `release/0.1` for `0.1.x` maintenance
- `v0.1.0`, `v0.1.1`, `v0.1.2` style tags for actual published releases

Important rule:

- a tag created from `release/0.1` does not merge anything into `main`

Tags only point at commits. If a bugfix is released from `release/0.1`, you still need to sync that fix back to `main`, usually by `cherry-pick` or a targeted PR.

## If You Change Build Outputs

Update these in the same pass:

- [`Makefile`](../Makefile)
- [`.github/workflows/build.yml`](../.github/workflows/build.yml)
- [`.github/workflows/release.yml`](../.github/workflows/release.yml)
- [`README.md`](../README.md)
- [`docs/build-and-release.md`](./build-and-release.md)
