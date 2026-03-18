# .github

Scope: repository automation and GitHub-native workflow behavior.

Rules:

- keep build automation aligned with `Makefile` targets instead of re-implementing build logic in YAML
- prefer official GitHub actions unless a third-party action is clearly justified
- binary artifact names must stay consistent with `make dist`
- when workflow triggers, artifact names, or release behavior change, update [`docs/build-and-release.md`](../docs/build-and-release.md) in the same pass
- CI should prove the same core path developers use locally: `go test ./...` and `make dist`
