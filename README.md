# AI Sandbox Runner

A Go-based runner for AI agents, IDE plugins, and platform automation workflows.

`sandbox-runner` does not try to reinvent the shell. It elevates the full `prepare -> setup -> execute -> verify -> collect` lifecycle into a stable run semantic layer with structured observability, artifacts, and replay.

It can run standalone or sit on top of Docker, Apple `container`, OrbStack, Dev Container CLI, Kubernetes, or OpenSandbox. To upper layers, the model remains the same: a unified run / phase / telemetry / artifact contract instead of a provider-specific API surface.

## Why This Project

AI-driven execution loops need more than process spawning. They need:

- predictable run semantics
- phase-aware state transitions
- structured stdout / stderr capture
- artifacts and replay for debugging and audit
- backend portability without changing the upper-layer contract

`AI Sandbox Runner` is designed for exactly that.

## Design Goals

- **Built for AI agents.** Suitable for patch, setup, test, verify, retry, and self-healing execution loops.
- **Observability-first.** Focus on run timeline, phase status, stdout / stderr, OTel, artifacts, and replay rather than turning the sandbox itself into a business control plane.
- **Unified run semantics.** Switching between `direct`, `docker`, `devcontainer`, `k8s`, and `opensandbox` should not materially change the CLI, output schema, or result semantics.
- **Clear runtime/profile layering.** `kata`, `apple-container`, and `orbstack-*` are runtime profile or provider extensions, not new run engines.
- **Cross-platform operation.** The runner itself supports macOS, Linux, and Windows binaries, while capability gates are trimmed automatically based on OS / Arch / container context / K8s context / Linux capabilities.

## Typical Use Cases

- **Local direct debugging** for developers or AI agents running `go test`, `pytest`, `npm test`, or `mvn test` directly on the host with full run-level observability.
- **Local Docker debugging** when workload execution needs Linux container dependencies, networking behavior, or filesystem semantics.
- **Local Apple `container` debugging** on Apple Silicon macOS using lightweight VM containers while preserving the same run artifacts and telemetry model.
- **Local OrbStack debugging** using OrbStack as a Docker provider, Linux machine backend, or local K8s target for Linux-like validation.
- **Local Dev Container debugging** for projects that already ship with `.devcontainer/devcontainer.json` and want a unified patch / test / verify workflow.
- **STG / Linux sandbox validation** in K8s Jobs, remote Linux environments, or controlled sandboxes for real patch / build / verify execution.
- **OpenSandbox integration** for platforms that already use OpenSandbox but want to keep the Runner semantic layer.
- **Kata-enhanced isolation** for `k8s` or `opensandbox` workloads without changing the upper-layer run / phase / command model.

## Non-Goals

- The Runner is **not** the security boundary. Strong isolation is only guaranteed in Docker / K8s / Linux sandbox scenarios.
- Business code should **not** couple directly to provider DTOs or provider clients.
- The project does **not** claim full Linux kernel-level observability on local macOS or Windows with zero injection.

## Architecture

```text
AI Agent / IDE / CLI
        ↓
RunEngine
        ↓
PhaseEngine
        ↓
BackendProvider
   ├─ direct
   ├─ docker
   ├─ apple-container
   ├─ orbstack-machine
   ├─ devcontainer
   ├─ k8s
   └─ opensandbox
        ↓
RuntimeProfile
   ├─ native
   ├─ kata
   ├─ apple-container
   ├─ orbstack-docker
   ├─ orbstack-k8s
   └─ orbstack-machine
```

### Layer Responsibilities

- **RunEngine**: owns run lifecycle, attempt control, phase orchestration, status aggregation, and artifacts / replay output.
- **PhaseEngine**: drives the ordered lifecycle of `prepare -> setup -> execute -> verify -> collect`.
- **BackendProvider**: handles create / start / delete, exec, log streaming, upload / download, and pause / resume / renew operations.
- **RuntimeProfile**: describes isolation strength and runtime metadata without changing the command model.
- **Telemetry / Artifact**: emits OTLP traces / metrics / logs as well as local JSONL / JSON artifacts.

## Core Capabilities

- Five-phase state machine: `prepare -> setup -> execute -> verify -> collect`
- Execution modes:
  - `local_direct`
  - `local_docker`
  - `local_devcontainer`
  - `local_apple_container`
  - `local_orbstack_machine`
  - `stg_linux`
  - `local_opensandbox_docker`
  - `stg_opensandbox_k8s`
- Runtime profiles:
  - `native`
  - `kata`
  - `apple-container`
  - `orbstack-docker`
  - `orbstack-k8s`
  - `orbstack-machine`
- Language adapters: Go / Python / Node / Java / Shell
- Structured outputs:
  - `context.json`
  - `environment.json`
  - `setup.plan.json`
  - `phases.json`
  - `results.json`
  - `replay.json`
- Backend/runtime outputs:
  - `provider.json`
  - `backend-profile.json`
  - `sandbox.json`
  - `runtime.json`
- Backend-specific outputs:
  - `devcontainer.json`
  - `machine.json`
  - `container.json`
- Command logs:
  - `commands.jsonl`
  - `stdout.jsonl`
  - `stderr.jsonl`
- OTel traces / metrics / logs with fallback to local JSONL when the collector is unavailable
- Extensible provider support for OpenSandbox, K8s Job rendering / submit SDK, Dev Container CLI backend, Apple `container`, and OrbStack profiles / backends
- Go support across run-level observability, helper packages, and Linux/STG-gated advanced capabilities

## Mode Matrix

| Mode | backend.kind | runtime.profile | Best For | Notes |
|---|---|---|---|---|
| `local_direct` | `direct` | `native` | Fast host-side debugging | No container required; full run-level observability |
| `local_docker` | `docker` | `native` | Local Linux-like execution | Supports both `host-runner` and `in-container-runner` |
| `local_docker` | `docker` | `orbstack-docker` | macOS + OrbStack Docker | Reuses Docker backend with `orbstack` provider markers |
| `local_devcontainer` | `devcontainer` | `native` | Standardized developer container workflows | Drives `read-configuration` / `up` / `exec` / `down` |
| `local_apple_container` | `apple-container` | `apple-container` | Apple Silicon local VM containers | Apple `container` as a dedicated backend |
| `local_orbstack_machine` | `orbstack-machine` | `orbstack-machine` | Full Linux userspace on macOS | Good for validation close to STG |
| `stg_linux` | `k8s` | `native` / `kata` | STG / K8s / Linux sandbox | Writes `runtimeClassName` when `kata` is enabled |
| `stg_linux` | `k8s` | `orbstack-k8s` | OrbStack local single-node K8s | Reuses K8s backend with `orbstack` provider markers |
| `local_opensandbox_docker` | `opensandbox` | `native` | Local OpenSandbox workflows | Uses provider lifecycle and execd APIs |
| `stg_opensandbox_k8s` | `opensandbox` | `native` / `kata` | STG OpenSandbox execution | Passes `kata` runtime requests through provider metadata |

## Observability and Artifacts

The primary value of the Runner is that it turns an AI-agent execution workflow into a structured, replayable, attributable run.

Key outputs include:

- run timeline and phase status
- command execution records
- line-level structured stdout / stderr logs
- OTel spans, events, and metrics
- local artifacts and replay manifests
- backend/runtime snapshots through `provider.json`, `backend-profile.json`, `sandbox.json`, and `runtime.json`
- backend-specific details such as `container.json`, `machine.json`, and `devcontainer.json`

## Apple Container and OrbStack

These integrations do not change the run semantic layer. They only replace or extend the execution substrate.

- **Apple `container`** acts as an independent backend for local Linux container workloads on Apple Silicon.
- **OrbStack Docker** is not a new backend. It is a provider profile on top of `docker`, with `backend.kind=docker` unchanged and `runtime.profile=orbstack-docker`.
- **OrbStack Machine** is an independent backend for fuller Linux machine workloads on macOS. It adds `machine.json` and `machine.name` telemetry metadata.
- **OrbStack K8s** is not a new backend. It is a provider profile on top of `k8s`, with `runtime.profile=orbstack-k8s`.

### Recommended Defaults

- Prefer `apple-container` for lightweight VM containers on Apple Silicon.
- Prefer `orbstack-machine` when a fuller Linux userspace is required.
- Prefer `docker(provider=orbstack)` for standard Docker / Compose-oriented workflows.
- Prefer `k8s(provider=orbstack-local)` for local single-node K8s validation.


## Platform Rules and Feature Gates

The Runner trims capabilities automatically based on platform and execution context instead of forcing Linux-specific functionality onto macOS or Windows.

### Rules Summary

- `macOS` disables `obi_ebpf` by default
- `windows` disables `obi_ebpf` by default
- `windows` disables `go_autosdk_bridge` by default
- unprivileged `linux` disables `obi_ebpf` by default
- `local_direct` does not enable capabilities that depend on privileged access or kernel probes
- `local_devcontainer` does not enable capabilities that depend on privileged access or kernel probes
- `local_apple_container` does not enable capabilities that depend on privileged access or kernel probes
- `local_orbstack_machine` does not enable capabilities that depend on privileged access or kernel probes
- `docker(provider=orbstack)` and `k8s(provider=orbstack-local)` change the provider only; they do not relax Linux-only gates
- `stg_linux` may enable `obi_ebpf` and `go_autosdk_bridge` according to policy
- `runtime.profile=kata` is only valid with `k8s` or `opensandbox`

## Go Support Matrix

| Capability | Mac Direct | Windows Direct | Mac Docker Host-Runner | Mac Docker In-Container | Linux Direct | STG/K8s Linux |
|---|---|---|---|---|---|---|
| Run lifecycle | Yes | Yes | Yes | Yes | Yes | Yes |
| stdout / stderr / exit code | Yes | Yes | Yes | Yes | Yes | Yes |
| Artifact / Replay | Yes | Yes | Yes | Yes | Yes | Yes |
| Go helper package | Yes | Yes | Yes | Yes | Yes | Yes |
| Go Auto SDK bridge | No by default | No | No by default | Container-dependent | Optional | Optional |
| OBI / eBPF | No | No | No | Permission-dependent | Optional | Optional |

### Notes

- macOS and Windows primarily provide Layer A run-level observability and do not guarantee Linux kernel-level enhancements.
- Linux and STG can layer in OBI / eBPF / Auto SDK bridge when permissions and runtime conditions allow.


## Example Configs

- `configs/run.sample.yaml`
- `configs/run.apple-container.sample.yaml`
- `configs/run.devcontainer.sample.yaml`
- `configs/run.orbstack.docker.sample.yaml`
- `configs/run.orbstack.machine.sample.yaml`
- `configs/run.orbstack.k8s.sample.yaml`
- `configs/run.opensandbox.sample.yaml`
- `configs/run.opensandbox.kata.sample.yaml`

## Quick Start

```bash
make build
./sandbox-runner run --config configs/run.sample.yaml --policy configs/policy.sample.yaml -- go test ./...
./sandbox-runner validate --config configs/run.apple-container.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner run --config configs/run.devcontainer.sample.yaml --policy configs/policy.sample.yaml -- go test ./...
./sandbox-runner validate --config configs/run.orbstack.docker.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner validate --config configs/run.orbstack.machine.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner validate --config configs/run.orbstack.k8s.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner run --config configs/run.opensandbox.sample.yaml --policy configs/policy.sample.yaml -- go test ./internal/proc
./sandbox-runner validate --config configs/run.opensandbox.kata.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner doctor
./sandbox-runner --version
```

### K8s / SDK Example

```bash
./sandbox-runner k8s render-job --config configs/run.sample.yaml --policy configs/policy.sample.yaml
```

## Build

```bash
make test
make build
make dist
```


## Important Notes

- The Runner is not the security boundary; hard isolation for filesystem / network / memory is only guaranteed in Docker / K8s / Linux sandbox scenarios.
- `collector.mode=require` is better suited for STG, while `collector.mode=auto` is usually better for local development.
- The S3 upload backend uses standard AWS SDK v2 and S3-compatible configuration.
- Before local OpenSandbox validation, make sure `opensandbox-server` and Docker daemon are availab
