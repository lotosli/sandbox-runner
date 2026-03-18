# Testing

## Main Commands

```bash
go test ./...
go test ./tests/contract/...
go test ./tests/integration/...
```

For the normal local developer path:

```bash
make test
```

## Test Notes

- Dev Container backend unit tests use a fake CLI and do not require a real local `devcontainer` install
- end-to-end Dev Container validation requires a working Docker daemon and local `devcontainer` CLI
- Apple `container` integration requires macOS arm64 with the `container` CLI installed
- OrbStack Docker, Machine, and K8s integration requires OrbStack on macOS
- OpenSandbox integration tests require a local `opensandbox-server` and a working Docker daemon
- integration tests are expected to skip automatically when local prerequisites are unavailable

## CI Relationship

The GitHub Actions build workflow runs `go test ./...` before `make dist`, so a binary artifact is only uploaded after the repository test suite passes.
