# Sandbox Runner Agent Handbook

This file applies to the whole repository.

Read this file first. Then read the relevant first-level directory `AGENTS.md` such as `internal/AGENTS.md` or `cmd/AGENTS.md`.

This repository intentionally keeps `AGENTS.md` only at the root and first-level subdirectories. There are no deeper nested `AGENTS.md` files.

## Repository Purpose

This repository implements `sandbox-runner`, a Go runner that turns a one-shot command into a structured run with stable phases, policy checks, telemetry, artifacts, replay data, and optional sandbox backends.

The core product idea is:

- upstream callers always talk in run semantics
- backends are replaceable execution providers
- observability and replay are first-class outputs
- policy is enforced before execution, not bolted on afterward

## Current Required Refactor

The current authoritative requirement is to normalize execution modeling to the standard triple:

- `execution.backend`
- `execution.provider`
- `execution.runtime_profile`

This triple is structural only. It is not a free-form Cartesian product.

Every execution request must pass the same preflight chain:

1. schema validation
2. compatibility matrix validation
3. capability probe
4. only then may it enter `RunEngine`

Future work in this repo must treat this as the source of truth, even where the current code still reflects older concepts such as `run_mode`, provider-specific config islands, or mixed sample naming.

## Fast Mental Model

The normal control flow is:

1. `cmd/sandbox-run/main.go` starts the CLI.
2. `internal/cli` parses subcommands and loads config plus policy.
3. `internal/phase.Engine` orchestrates `prepare -> setup -> execute -> verify -> collect`.
4. `internal/backend` exposes a uniform sandbox/provider interface.
5. `internal/executor` decides how commands actually run:
   - direct process
   - local Docker container
   - managed backend exec (`opensandbox`, `devcontainer`)
6. `internal/artifact` and `internal/telemetry` persist structured outputs.

If you only remember one rule, remember this: `internal/phase` owns run semantics. Everything else is a dependency of that orchestration layer.

## Stable Architecture Principles

### 1. One orchestration authority

`internal/phase/engine.go` is the only place that should coordinate end-to-end run state, phase transitions, result status, artifact emission order, and cleanup.

Do not re-implement phase logic inside:

- CLI handlers
- backends
- executors
- adapters

### 2. Backend and executor are different layers

Backends answer sandbox/provider questions:

- capabilities
- runtime info
- sandbox create/start/delete
- exec/log streaming
- file sync
- metadata/endpoints

Executors answer command-running questions:

- local direct process execution
- local Docker execution
- managed backend exec sessions

Do not move provider lifecycle into `internal/executor`. Do not move command streaming logic into `internal/phase`.

### 3. Policy is a gate, not an afterthought

`internal/policy` must be consulted before:

- command execution
- secret injection
- filesystem access
- network-profile use

Backends and executors may enforce their own operational limits, but policy decisions should remain centralized.

### 4. Artifacts and telemetry are append-only sinks

`internal/artifact` and `internal/telemetry` should not decide business logic. They serialize the outcome of orchestration.

If a new run concept becomes user-visible, usually the change spans:

- `internal/model`
- `internal/phase`
- `internal/artifact`
- sometimes `README.md` and sample configs

### 5. Config normalization is centralized

`internal/config` is the contract boundary for:

- defaults
- env overrides
- path normalization
- schema validation
- backend/provider/runtime-profile compatibility handoff

Do not spread config inference into unrelated packages.

### 6. Language adaptation is rewrite-only

`internal/adapter` and `internal/lang/go` only rewrite command and environment for instrumentation or runtime hints. They should not perform execution, file IO orchestration, or phase control.

### 7. Platform feature gates are explicit

`internal/platform` decides what features are available on the current host or mode. Avoid sprinkling ad hoc OS checks through unrelated packages when a feature gate should express the intent.

## Main Lifecycle Details

### Prepare

Owned by `internal/phase`.

Responsibilities:

- normalize `run_id`, attempt, workspace, artifact paths
- resolve the standard execution triple
- run compatibility validation and capability probe before backend creation
- detect execution target and resolve feature gates
- create artifact writer
- bootstrap collector and telemetry
- instantiate backend and inspect capabilities/runtime info
- create/start sandbox for managed backends
- sync workspace in when backend supports it
- write `context.json`, `provider.json`, `runtime.json`, optional `sandbox.json`, optional `devcontainer.json`

### Setup

Responsibilities:

- detect project type via `internal/envsync`
- generate setup plan and environment fingerprint
- create executor
- run setup steps under policy and telemetry

### Execute

Responsibilities:

- resolve secrets and merged env
- pick language adapter
- rewrite command/env
- classify command
- run the main command through executor
- record command metadata and execution target

### Verify

Responsibilities:

- optional smoke command
- pull expected artifacts from sandbox if needed
- validate expected artifact presence

### Collect

Responsibilities:

- sync workspace out when supported
- refresh sandbox or devcontainer metadata
- emit endpoint metadata
- run cleanup policy
- enumerate artifact refs
- upload artifacts if configured
- write replay, phases, and results

## What To Change Where

- CLI surface or subcommands: `internal/cli/`
- config schema/defaults/validation: `internal/model/` and `internal/config/`
- phase semantics or run status: `internal/phase/`
- provider lifecycle or sandbox APIs: `internal/backend/`
- local or Docker command execution: `internal/executor/` and `internal/proc/`
- OTel and run observability: `internal/telemetry/` and `internal/artifact/`
- K8s render/submit support: `internal/kubernetes/` and `pkg/sdk/`
- public helper APIs: `pkg/helper/`, `pkg/sdk/`
- execution triple compatibility and capability probing: `internal/compat/` and `internal/capability/`
- GitHub Actions and repository automation: `.github/workflows/`
- repository-facing docs and navigation: `README.md` and `docs/`

## Directory Map

- `.github/`
  GitHub workflows and repository automation.
- `cmd/`
  Thin binary entrypoint only.
- `internal/cli/`
  CLI parsing and command dispatch.
- `internal/config/`
  Defaults, loading, normalization, validation.
- `internal/compat/`
  Planned static compatibility matrix for `backend/provider/runtime_profile`.
- `internal/capability/`
  Planned backend-specific capability probes before execution.
- `internal/model/`
  Shared enums, DTOs, artifact schemas, result types.
- `internal/phase/`
  End-to-end run orchestration and phase state machine.
- `internal/backend/`
  Provider abstraction and concrete managed backends.
- `internal/executor/`
  Execution adapters for direct, Docker, and backend exec.
- `internal/proc/`
  Local process runner, log streaming, command classification, redaction.
- `internal/policy/`
  Command/path/network/secret/time-budget enforcement.
- `internal/platform/`
  Host detection and feature gating.
- `internal/envsync/`
  Setup-plan generation and environment fingerprinting.
- `internal/artifact/`
  Structured artifact writing and uploading.
- `internal/telemetry/`
  OTel traces, logs, metrics emission.
- `internal/collector/`
  Collector health check and optional local bootstrap.
- `internal/kubernetes/`
  Job/ConfigMap rendering and Kubernetes client helpers.
- `internal/opensandbox/client/`
  Raw OpenSandbox HTTP client.
- `internal/adapter/`, `internal/lang/`
  Language-specific command/env rewriting.
- `pkg/sdk`, `pkg/helper`
  Public package surface.
- `configs/`
  Sample configs and local collector config.
- `deployments/`
  Kubernetes manifests.
- `docs/`
  Detailed documentation pages. Keep root `README.md` concise and link-driven.
- `examples/`
  Example instrumented service.
- `tests/`
  Contract and integration tests.

## Common Change Playbooks

### Add or change a backend

Touch at least:

- `internal/model` if capability or config schema changes
- `internal/config` for schema validation and normalization
- `internal/compat` for allowed triples
- `internal/capability` for probe behavior
- `internal/backend/interface.go` if the protocol changes
- `internal/backend/factory.go`
- concrete backend implementation
- `internal/phase` if lifecycle semantics change
- tests in `internal/backend`, `internal/phase`, and `tests/` when applicable

Rules:

- preserve the backend interface instead of special-casing phase logic
- expose capabilities explicitly
- `provider` refines backend behavior; it does not replace backend selection
- return provider-specific detail through metadata, matrix rules, and probe results instead of ad hoc control-flow branching everywhere

### Add a new artifact or result field

Touch at least:

- `internal/model`
- `internal/artifact/writer.go`
- `internal/phase/engine.go`
- README if user-visible

Keep file names stable and JSON schema predictable.

### Add language-specific behavior

Touch:

- `internal/adapter`
- maybe `internal/lang/<lang>`
- tests near the adapter/classifier

Rules:

- keep it rewrite-only
- do not embed phase logic
- prefer env injection and command rewriting over bespoke execution paths

### Change config schema

Touch:

- `internal/model`
- `internal/config/defaults.go`
- `internal/config/loader.go` if env overrides or normalization change
- `internal/config/validate.go`
- `internal/compat` and `internal/capability` if execution semantics change
- sample files in `configs/`
- `README.md`

### Change phase behavior

Start in `internal/phase/engine.go`, then propagate only the minimum downstream changes required. This file is the source of truth for lifecycle semantics.

## Repo-Level Rules

- Keep `prepare -> setup -> execute -> verify -> collect` semantics centralized in `internal/phase`.
- Do not bypass policy checks by adding direct command execution elsewhere.
- Treat `internal/model` as the shared contract surface; schema changes usually ripple into docs, config samples, artifacts, and tests.
- Keep provider-specific behavior behind backend interfaces instead of scattering conditionals across the repo.
- Do not allow arbitrary `backend/provider/runtime_profile` triples into execution; enforce schema, compatibility, and capability in order.
- Telemetry, structured logs, and `context.json` must converge on the standard execution triple plus compatibility level and capability probe result.
- Update nearby tests when behavior changes.

## Current Reality and Caveats

- The fully wired main path is `direct`, `docker`, `devcontainer`, `k8s`, and `opensandbox`.
- `internal/model` and config defaults already contain `apple-container` and `orbstack` concepts, but current validation and backend factory do not fully treat them as supported production paths. Assume they are planned or partial unless the implementation is completed end to end.
- `internal/backend/LocalBackend` is mostly a capability/runtime shim. Real local command execution happens in `internal/executor`.
- The current codebase still mixes older `run_mode`-centric modeling with provider-specific fields. New work should reduce that drift and move toward the explicit execution triple model.
- Hidden `.sandbox-run/` directories and `dist/` are generated outputs, not architecture sources.

## Generated Or External Directories

Do not treat these as source areas for architecture work:

- `.git/`
- `.sandbox-run/`
- `cmd/sandbox-run/.sandbox-run/`
- `dist/`

They are repository metadata or generated runtime output.

## How An Agent Should Work In This Repo

1. Read this file first.
2. Then read the first-level directory `AGENTS.md` for the area you are changing.
3. Prefer edits that stay inside one layer.
4. If a user-visible behavior changes, sync docs, sample configs, and tests in the same pass.
5. If a change crosses phase semantics, backend capability contracts, or artifact schema, verify the whole chain rather than patching one file in isolation.
