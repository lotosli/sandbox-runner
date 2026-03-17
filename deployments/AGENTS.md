# deployments

Scope: deployment-oriented manifests and templates.

Current layout:

- `k8s/` contains service account, config map, and job-template manifests for running the runner on Kubernetes

Rules:

- keep deployment manifests aligned with runtime config and `internal/kubernetes`
- avoid putting source-of-truth business logic here
