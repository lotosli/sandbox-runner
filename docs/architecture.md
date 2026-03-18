# Architecture

## Why This Project Exists

AI-driven execution loops need more than process spawning. They need stable run semantics, phase-aware status, structured logs, artifacts, replay data, and portable execution backends.

`sandbox-runner` keeps that semantic layer stable while letting the execution substrate vary.

## Design Goals

- built for AI agents, IDE plugins, and automation workflows
- keep `prepare -> setup -> execute -> verify -> collect` as the central lifecycle
- make observability and replay first-class outputs
- preserve a unified contract across `direct`, `docker`, `devcontainer`, `k8s`, `opensandbox`, `apple-container`, and `machine`
- separate orchestration, provider lifecycle, command execution, policy, and telemetry cleanly

## Non-Goals

- the runner is not the ultimate security boundary
- business code should not depend directly on provider-specific DTOs
- local macOS and Windows modes do not promise Linux-kernel-level observability features

## Layered View

```text
AI Agent / IDE / CLI
        ↓
Run semantics
        ↓
Phase engine
        ↓
Backend abstraction
   ├─ direct
   ├─ docker
   ├─ devcontainer
   ├─ apple-container
   ├─ machine
   ├─ k8s
   └─ opensandbox
        ↓
Telemetry + artifacts + replay
```

## Core Responsibilities

- Run semantics: overall run state, attempts, result status, replay, artifact chain
- Phase engine: ordered `prepare -> setup -> execute -> verify -> collect`
- Backends: sandbox lifecycle, exec, log streaming, sync, metadata, endpoints
- Executors: local direct process, Docker execution, backend exec sessions
- Policy: command, path, secret, network, timeout gates before execution
- Telemetry and artifacts: append-only structured outputs

## Lifecycle Summary

### Prepare

- normalize run metadata and paths
- resolve execution triple
- run schema, compatibility, and capability checks before execution
- create the backend and inspect capabilities
- create or start managed sandboxes when required

### Setup

- detect project type
- generate setup plan and environment fingerprint
- run setup commands under policy and telemetry

### Execute

- resolve env and secrets
- rewrite commands with language adapters when needed
- run the main command through the selected executor

### Verify

- run optional smoke checks
- validate expected artifacts

### Collect

- sync workspace out when supported
- refresh provider metadata
- write replay, phase results, and final outputs

## Key Structured Outputs

- `context.json`
- `environment.json`
- `setup.plan.json`
- `phases.json`
- `results.json`
- `replay.json`
- `provider.json`
- `backend-profile.json`
- `sandbox.json`
- `runtime.json`
- `commands.jsonl`
- `stdout.jsonl`
- `stderr.jsonl`
