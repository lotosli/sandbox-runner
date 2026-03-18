#!/bin/sh
set -eu

proof_path=".sandbox-runner/artifacts/proof.json"
mkdir -p "$(dirname "$proof_path")"
printf '{\n  "language": "shell",\n  "phase": "execute",\n  "otel_service_name": "%s"\n}\n' "${OTEL_SERVICE_NAME:-}" > "$proof_path"
printf '__SHELL_EXECUTE__\n'
printf 'OTEL_SERVICE_NAME=%s\n' "${OTEL_SERVICE_NAME:-}"
printf '__SHELL_STDERR__\n' >&2
