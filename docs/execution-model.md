# Execution Model

## Standard Execution Triple

Execution is normalized to three fields:

- `execution.backend`
- `execution.provider`
- `execution.runtime_profile`

This is the canonical model for config, logs, telemetry, and artifacts.

## Validation Order

Every execution request must pass these checks in order:

1. schema validation
2. compatibility matrix validation
3. capability probe
4. RunEngine execution

The runner does not allow arbitrary `backend/provider/runtime_profile` combinations.

## Main Backend Values

- `direct`
- `docker`
- `k8s`
- `opensandbox`
- `devcontainer`
- `apple-container`
- `machine`

## Main Provider Values

- `native`
- `orbstack`
- `kind`
- `minikube`
- `docker-desktop`
- `colima`
- `gke`
- `eks`
- `aks`
- `opensandbox`

## Runtime Profiles

- `default`
- `kata`
- `gvisor`
- `firecracker`

Compatibility aliases:

- `microvm` input normalizes to the canonical `firecracker` runtime profile

## Typical Combinations

| Run mode | backend | provider | runtime_profile | Notes |
|---|---|---|---|---|
| `local_direct` | `direct` | `native` | `default` | host process execution |
| `local_docker` | `docker` | `native` | `default` | standard Docker |
| `local_docker` | `docker` | `orbstack` | `default` | Docker backend, OrbStack provider |
| `local_devcontainer` | `devcontainer` | `native` | `default` | Dev Container CLI |
| `local_apple_container` | `apple-container` | `native` | `default` | Apple `container` CLI |
| `local_orbstack_machine` | `machine` | `orbstack` | `default` | OrbStack Linux machine |
| `stg_linux` | `k8s` | `native` | `default` / conditional runtimes | K8s-backed execution |
| `local_opensandbox_docker` | `opensandbox` | `opensandbox` | `default` | local OpenSandbox flow |
| `stg_opensandbox_k8s` | `opensandbox` | `opensandbox` | `default` / conditional runtimes | OpenSandbox on K8s runtime |

## Compatibility and Capability

- unsupported triples fail fast during config validation
- conditional triples require environment proof before execution starts
- capability probe output is written into `context.json`, run metadata, and telemetry

For OpenSandbox, non-default runtime profiles are validated by a live provider probe instead of a static assumption.

For K8s-style execution, conditional runtime profiles also require `runtime.class_name` so the rendered Job can set `runtimeClassName`.

## Legacy Config Compatibility

Older sample names and legacy config fields can still exist as compatibility inputs, but internal execution semantics are normalized to the standard triple above.
