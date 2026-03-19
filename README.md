# AI Sandbox Runner

A Go-based runner for AI agents, IDE plugins, and platform automation workflows.

`sandbox-runner` keeps the full `prepare -> setup -> execute -> verify -> collect` lifecycle stable while letting the execution substrate vary across direct host execution, Docker, Dev Container CLI, Kubernetes, Apple `container`, OrbStack, and OpenSandbox.

`microvm` is accepted as a compatibility alias for the canonical `firecracker` runtime profile.

## What It Gives You

- stable run semantics for agent-driven patch, build, test, and verify loops
- structured telemetry, artifacts, and replay data
- backend portability without changing upper-layer contracts
- a normalized execution model based on `backend / provider / runtime_profile`

## Quick Links

- [Getting Started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [Execution Model](docs/execution-model.md)
- [Backends and Platforms](docs/backends-and-platforms.md)
- [Build and Release](docs/build-and-release.md)
- [Testing](docs/testing.md)

## Quick Start

```bash
make build
./sandbox-runner run --config configs/run.local.sample.yaml --policy configs/policy.sample.yaml -- go test ./...
./sandbox-runner doctor
./sandbox-runner --version
```

More commands and sample configs live in [Getting Started](docs/getting-started.md).

## Binary Builds

Local release-style binaries are produced by `make dist`.

GitHub Actions now uses the same path:

- workflow: [`.github/workflows/build.yml`](.github/workflows/build.yml)
- steps: `go test ./...` then `make dist`
- artifact: `sandbox-runner-dist`

This keeps CI-generated binaries aligned with the current local `dist/` outputs instead of introducing a second build pipeline.

## Repository Layout

```text
cmd/
configs/
deployments/
docs/
examples/
internal/
pkg/
tests/
```

## Public Packages

- `github.com/lotosli/sandbox-runner/pkg/sdk`
- `github.com/lotosli/sandbox-runner/pkg/helper`
