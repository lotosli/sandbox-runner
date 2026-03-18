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

Behavior:

- runs on `push`, `pull_request`, and `workflow_dispatch`
- executes `go test ./...`
- executes `make dist`
- uploads the generated `dist/` outputs as the `sandbox-runner-dist` workflow artifact

This keeps CI binaries aligned with the same Makefile-driven process developers use locally.

## If You Change Build Outputs

Update these in the same pass:

- [`Makefile`](../Makefile)
- [`.github/workflows/build.yml`](../.github/workflows/build.yml)
- [`README.md`](../README.md)
- [`docs/build-and-release.md`](./build-and-release.md)
