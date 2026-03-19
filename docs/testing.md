# Testing

## Main Commands

```bash
go test ./...
go test ./tests/contract/...
go test ./tests/integration/...
go test ./tests/integration/... -run TestLocalLanguageSamples
go test ./tests/integration/... -run TestDockerLanguageSamples
go test ./tests/integration/... -run TestDevContainerLanguageSamples
go test ./tests/integration/... -run TestOpenSandboxLanguageSamples
go test ./tests/integration/... -run TestAppleContainerLanguageSamples
go test ./tests/integration/... -run TestK8sProviderLanguageSamplesRenderJobs
go test ./tests/integration/... -run TestK8sProviderLanguageSamplesRenderIsolatedJobs
```

For the normal local developer path:

```bash
make test
```

## Test Notes

- Dev Container backend unit tests use a fake CLI and do not require a real local `devcontainer` install
- local Docker integration requires a reachable Docker daemon; the executor follows the active Docker CLI context when `docker.context=default`
- end-to-end Dev Container validation requires a working Docker daemon and local `devcontainer` CLI
- Apple `container` integration requires macOS arm64 with the `container` CLI installed
- Apple `container` integration also requires a configured default kernel:
  `container system kernel set --recommended --arch arm64 --force`
  `container system start --disable-kernel-install`
- OrbStack Docker, Machine, and K8s integration requires OrbStack on macOS
- OpenSandbox integration tests require a local `opensandbox-server` and a working Docker daemon
- language sample integration tests cover `local_direct`, Docker, DevContainer, OpenSandbox, and Apple container sample configs
- K8s provider language tests cover `minikube`, `k3s`, and `microk8s` through `validate` plus `k8s render-job`
- K8s isolated-mode tests cover the same providers through the `microvm` compatibility alias, normalized `firecracker` runtime profile, and rendered `runtimeClassName`
- real `k8s submit-job` verification depends on a reachable local cluster context for that provider and is expected to stay environment-dependent
- a real local microvm smoke run also needs a `RuntimeClass` such as `sandbox-runner-microvm` and a job image that already contains `/usr/local/bin/sandbox-runner`
- language sample integration tests copy each example workspace into a temp directory before running `validate`, `run --json-summary`, and `replay`
- the local language sample matrix currently targets POSIX shells and checks `go`, `python3`, `node` + `npm`, `java` + `javac`, and `sh`
- integration tests are expected to skip automatically when local prerequisites are unavailable

## CI Relationship

The GitHub Actions build workflow runs `go test ./...` before `make dist`, so a binary artifact is only uploaded after the repository test suite passes.
