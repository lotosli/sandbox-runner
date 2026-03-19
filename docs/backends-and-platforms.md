# Backends and Platforms

## Typical Use Cases

- `direct`: fastest local debugging on the host
- `docker`: Linux-like local validation with container boundaries
- `devcontainer`: reuse existing `.devcontainer/devcontainer.json`
- `apple-container`: Apple Silicon local VM containers
- `machine`: fuller Linux userspace on macOS through OrbStack
- `k8s`: STG, cluster, or Job-style execution
- `opensandbox`: keep OpenSandbox as provider while preserving runner semantics

Named K8s providers currently modeled in the repo:

- `native`: generic remote or pre-existing Kubernetes cluster
- `orbstack`: local OrbStack Kubernetes
- `minikube`: Minikube local cluster
- `k3s`: K3s cluster
- `microk8s`: MicroK8s cluster

## Apple Container and OrbStack

- Apple `container` is its own backend, not a provider alias
- OrbStack can act as a provider for Docker or K8s
- OrbStack machine is a separate `machine` backend
- provider choice refines backend behavior but does not replace backend class

Recommended defaults:

- prefer `apple-container` for lightweight Apple Silicon Linux containers
- prefer `machine/orbstack/default` when a fuller Linux userspace is needed
- prefer `docker/orbstack/default` for standard container workflows on macOS
- prefer `k8s/orbstack/default` for local single-node K8s validation
- prefer `k8s/minikube/default` when you want a standard local upstream-style cluster on Docker
- prefer `k8s/k3s/default` or `k8s/microk8s/default` when your target environment already standardizes on those distros

## Dev Container and Kata Notes

- Dev Container backend maps lifecycle to `read-configuration`, `up`, `exec`, and `down`
- `devcontainer exec` still uses the runner's normal stdout, stderr, timeout, and artifact chain
- `kata` is a runtime profile, not a backend
- the first release still rejects combinations such as `direct + kata`, `docker + kata`, and `devcontainer + kata`

## OpenSandbox Notes

- OpenSandbox keeps the same run, phase, artifact, and replay model
- `prepare` handles provider negotiation, create/start, and workspace sync
- `execute` and `verify` run through execd APIs
- provider-specific errors are normalized into `RunnerError`
- non-default runtime profiles use a live provider probe with a short-lived create/delete cycle before the main run

## Platform Rules

- macOS and Windows keep the runner contract but trim Linux-only capabilities
- unprivileged Linux may disable features that need kernel or privilege access
- provider switches such as OrbStack do not automatically relax host feature gates
- `runtime_profile=kata` is only valid on backends that explicitly support it

## Kubernetes Notes

- the current K8s control-plane flow is `validate`, `k8s render-job`, then `k8s submit-job`
- `execution.provider` is the first-class provider name; `k8s.provider` remains as the legacy bridge used by the current K8s submit path
- `minikube`, `k3s`, and `microk8s` are now explicit K8s providers instead of falling back to generic `native`
- the K8s job image must contain `/usr/local/bin/sandbox-runner`
- if the submitted command needs `go`, `python`, `node`, `java`, or other toolchains, that same image must also contain those runtimes
- repository integration tests currently prove per-language K8s provider configs through `validate` plus `k8s render-job`; real cluster submission remains environment-dependent

## Go Support Snapshot

| Capability | Mac Direct | Windows Direct | Linux Direct | K8s / STG Linux |
|---|---|---|---|---|
| Run lifecycle | Yes | Yes | Yes | Yes |
| stdout / stderr / exit code | Yes | Yes | Yes | Yes |
| Artifact / replay | Yes | Yes | Yes | Yes |
| Go helper package | Yes | Yes | Yes | Yes |
| Auto SDK bridge | No by default | No | Optional | Optional |
| OBI / eBPF | No | No | Optional | Optional |
