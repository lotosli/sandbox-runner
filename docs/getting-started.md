# Getting Started

## Prerequisites

- Go toolchain matching [`go.mod`](../go.mod)
- A policy file such as [`configs/policy.sample.yaml`](../configs/policy.sample.yaml)
- A run config from [`configs/`](../configs/)

Backend-specific prerequisites vary. See [Backends and Platforms](./backends-and-platforms.md).

## Quick Start

```bash
make build
./sandbox-runner run --config configs/run.sample.yaml --policy configs/policy.sample.yaml -- go test ./...
./sandbox-runner doctor
./sandbox-runner --version
```

## Common Commands

```bash
./sandbox-runner run --config configs/run.sample.yaml --policy configs/policy.sample.yaml -- go test ./...
./sandbox-runner validate --config configs/run.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner k8s render-job --config configs/run.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner doctor
./sandbox-runner --version
```

## Sample Configs

- [`configs/run.sample.yaml`](../configs/run.sample.yaml): baseline direct/local sample
- [`configs/run.apple-container.sample.yaml`](../configs/run.apple-container.sample.yaml): Apple `container` backend
- [`configs/run.devcontainer.sample.yaml`](../configs/run.devcontainer.sample.yaml): Dev Container CLI backend
- [`configs/run.orbstack.docker.sample.yaml`](../configs/run.orbstack.docker.sample.yaml): Docker backend on OrbStack
- [`configs/run.orbstack.machine.sample.yaml`](../configs/run.orbstack.machine.sample.yaml): OrbStack machine backend
- [`configs/run.orbstack.k8s.sample.yaml`](../configs/run.orbstack.k8s.sample.yaml): K8s backend on OrbStack
- [`configs/run.opensandbox.sample.yaml`](../configs/run.opensandbox.sample.yaml): OpenSandbox default runtime
- [`configs/run.opensandbox.kata.sample.yaml`](../configs/run.opensandbox.kata.sample.yaml): OpenSandbox runtime-profile negotiation

## Where To Go Next

- Architecture and lifecycle: [Architecture](./architecture.md)
- Execution triple and compatibility rules: [Execution Model](./execution-model.md)
- Backend caveats and platform notes: [Backends and Platforms](./backends-and-platforms.md)
- Build outputs and CI binaries: [Build and Release](./build-and-release.md)
- Test commands and integration notes: [Testing](./testing.md)
