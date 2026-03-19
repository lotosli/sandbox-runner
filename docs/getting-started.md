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
./sandbox-runner run --json-summary --config configs/run.sample.yaml
./sandbox-runner run --config configs/run.output.sample.yaml
./sandbox-runner doctor
./sandbox-runner --version
```

## Common Commands

```bash
./sandbox-runner run --config configs/run.sample.yaml --policy configs/policy.sample.yaml -- go test ./...
./sandbox-runner run --json-summary --config configs/run.sample.yaml
./sandbox-runner run --config configs/run.output.sample.yaml
./sandbox-runner validate --config configs/run.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner k8s render-job --config configs/run.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner doctor
./sandbox-runner --version
```

## Sample Configs

Relative paths inside sample configs are resolved from the config file directory. That means both of these work and target the same repository workspace:

```bash
./sandbox-runner run --config configs/run.sample.yaml
(cd dist && ./sandbox-runner-darwin-amd64 run --config ../configs/run.sample.yaml)
```

For agent or automation use, prefer `--json-summary`. It prints a stable JSON summary with the artifact directory, core file paths, and recent stdout/stderr tail lines. Every run also writes `index.json` into the artifact directory so an LLM can discover `results.json`, `phases.json`, `commands.jsonl`, `stdout.jsonl`, `stderr.jsonl`, and `replay.json` without hard-coding file names.

- [`configs/run.sample.yaml`](../configs/run.sample.yaml): baseline direct/local sample
- [`configs/run.output.sample.yaml`](../configs/run.output.sample.yaml): direct/local sample that emits visible stdout and stderr lines
- [`configs/run.apple-container.sample.yaml`](../configs/run.apple-container.sample.yaml): Apple `container` backend
- [`configs/run.devcontainer.sample.yaml`](../configs/run.devcontainer.sample.yaml): Dev Container CLI backend
- [`configs/run.orbstack.docker.sample.yaml`](../configs/run.orbstack.docker.sample.yaml): Docker backend on OrbStack
- [`configs/run.orbstack.machine.sample.yaml`](../configs/run.orbstack.machine.sample.yaml): OrbStack machine backend
- [`configs/run.orbstack.k8s.sample.yaml`](../configs/run.orbstack.k8s.sample.yaml): K8s backend on OrbStack
- [`configs/run.k8s.minikube.sample.yaml`](../configs/run.k8s.minikube.sample.yaml): K8s backend on Minikube
- [`configs/run.k8s.k3s.sample.yaml`](../configs/run.k8s.k3s.sample.yaml): K8s backend on K3s
- [`configs/run.k8s.microk8s.sample.yaml`](../configs/run.k8s.microk8s.sample.yaml): K8s backend on MicroK8s
- [`configs/run.k8s.minikube.microvm.sample.yaml`](../configs/run.k8s.minikube.microvm.sample.yaml): Minikube microvm or firecracker smoke sample
- [`configs/run.k8s.k3s.microvm.sample.yaml`](../configs/run.k8s.k3s.microvm.sample.yaml): K3s microvm or firecracker smoke sample
- [`configs/run.k8s.microk8s.microvm.sample.yaml`](../configs/run.k8s.microk8s.microvm.sample.yaml): MicroK8s microvm or firecracker smoke sample
- [`configs/run.opensandbox.sample.yaml`](../configs/run.opensandbox.sample.yaml): OpenSandbox default runtime
- [`configs/run.opensandbox.kata.sample.yaml`](../configs/run.opensandbox.kata.sample.yaml): OpenSandbox runtime-profile negotiation

For `k8s` samples, `execution.provider` names the cluster flavor while `k8s.provider` keeps the legacy bridge value that existing render and submit commands still consume. `runtime.class_name` is the generic runtime-class hook for conditional profiles such as `kata` and `microvm`. These samples target the current K8s flow: `validate`, `k8s render-job`, and `k8s submit-job`.

Example `microvm` smoke run on a local K8s provider:

```bash
./sandbox-runner validate --config configs/run.k8s.k3s.microvm.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner k8s submit-job --config configs/run.k8s.k3s.microvm.sample.yaml --policy configs/policy.sample.yaml
kubectl --context k3d-k3s -n ai-sandbox-runner-runs wait --for=condition=complete job -l app=sandbox-runner --timeout=120s
kubectl --context k3d-k3s -n ai-sandbox-runner-runs logs job/$(kubectl --context k3d-k3s -n ai-sandbox-runner-runs get jobs -l app=sandbox-runner -o jsonpath='{.items[-1:].metadata.name}')
```

## Language Samples

The repository also ships runnable per-language examples under [`examples/`](../examples/README.md). Each one includes its own `run.local.sample.yaml`, stable stdout/stderr markers, and an `artifacts/proof.json` output so an agent can validate the full lifecycle without reverse-engineering the workspace.

- [`examples/go-basic/run.local.sample.yaml`](../examples/go-basic/run.local.sample.yaml)
- [`examples/python-basic/run.local.sample.yaml`](../examples/python-basic/run.local.sample.yaml)
- [`examples/node-basic/run.local.sample.yaml`](../examples/node-basic/run.local.sample.yaml)
- [`examples/java-basic/run.local.sample.yaml`](../examples/java-basic/run.local.sample.yaml)
- [`examples/shell-basic/run.local.sample.yaml`](../examples/shell-basic/run.local.sample.yaml)

Example command:

```bash
./sandbox-runner run --json-summary --config examples/python-basic/run.local.sample.yaml --policy configs/policy.sample.yaml
```

## Where To Go Next

- Architecture and lifecycle: [Architecture](./architecture.md)
- Execution triple and compatibility rules: [Execution Model](./execution-model.md)
- Backend caveats and platform notes: [Backends and Platforms](./backends-and-platforms.md)
- Build outputs and CI binaries: [Build and Release](./build-and-release.md)
- Test commands and integration notes: [Testing](./testing.md)
