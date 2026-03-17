# AI Sandbox Runner

`sandbox-run` 是一个面向本地开发与 STG/K8s 调试的 Go 实现 runner，负责把一次命令执行变成有 phase、策略、telemetry、artifacts 和 replay 语义的 run。

## Delivered

- 五阶段状态机：`prepare -> setup -> execute -> verify -> collect`
- 运行模式：`local_direct`、`local_docker`、`stg_linux`、`local_opensandbox_docker`、`stg_opensandbox_k8s`
- 语言适配：Go / Python / Node / Java / Shell
- 结构化产物：`context.json`、`environment.json`、`setup.plan.json`、`results.json`、`replay.json`
- OTel traces / metrics，collector 不可用时自动降级 JSONL
- Docker 模式、OpenSandbox provider、K8s Job 渲染与提交 SDK、Go helper 包

## Quick Start

```bash
go build ./cmd/sandbox-run
./sandbox-run run --config configs/run.sample.yaml --policy configs/policy.sample.yaml -- go test ./...
./sandbox-run run --config configs/run.opensandbox.sample.yaml --policy configs/policy.sample.yaml -- go test ./internal/proc
./sandbox-run doctor
./sandbox-run k8s render-job --config configs/run.sample.yaml --policy configs/policy.sample.yaml
```

## OpenSandbox Compatibility

- 新增 `backend.kind=opensandbox`，Runner 仍保持原有 run / phase / artifact / replay 语义。
- prepare 阶段会完成 capability negotiation、sandbox create/start、workspace tar sync-in。
- execute / verify 阶段通过 sandbox 内的 execd API 执行命令并流式采集 stdout/stderr。
- collect 阶段会输出额外的 `provider.json`、`sandbox.json`、`endpoints.json`，并按 cleanup policy 做 delete / pause / keep。
- OpenSandbox provider 错误会统一映射到 RunnerError，并保留 provider code。
- 本地联调前请先确认 `opensandbox-server` 可启动且 Docker daemon 可用；运行时可通过 `OPENSANDBOX_BASE_URL` 和 `OPENSANDBOX_API_KEY` 覆盖配置。
- 示例配置见 [configs/run.opensandbox.sample.yaml](/Users/lotosli/Documents/Sandbox%20Runer/configs/run.opensandbox.sample.yaml)。

## Go Support Matrix

| Capability | Mac Direct | Mac Docker Host-Runner | Mac Docker In-Container | Linux Direct | STG/K8s Linux |
|---|---|---|---|---|---|
| Run lifecycle | Yes | Yes | Yes | Yes | Yes |
| stdout/stderr/exit code | Yes | Yes | Yes | Yes | Yes |
| Artifact / Replay | Yes | Yes | Yes | Yes | Yes |
| Go helper package | Yes | Yes | Yes | Yes | Yes |
| Go Auto SDK bridge | No by default | No by default | Container-dependent | Optional | Optional |
| OBI / eBPF | No | No | Permission-dependent | Optional | Optional |

## Public Packages

- `github.com/lotosli/sandbox-runner/pkg/sdk`
- `github.com/lotosli/sandbox-runner/pkg/helper`

## Build

```bash
make test
make build
make dist
```

## Test

```bash
go test ./internal/... ./pkg/...
go test ./tests/contract/...
go test ./tests/integration/...
```

## Notes

- Runner 不是安全边界；文件系统/网络/内存硬隔离只在 Docker/K8s/Linux sandbox 场景承诺。
- `collector.mode=require` 适合 STG；`collector.mode=auto` 适合本地。
- S3 上传后端使用标准 AWS SDK v2 和 S3-compatible 配置。
