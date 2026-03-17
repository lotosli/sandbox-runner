# AI Sandbox Runner

`sandbox-runner` 是一个面向 AI agent、IDE 插件、平台自动化任务的 Go runner。它的目标不是再造一个 shell，而是把一次 `prepare -> setup -> execute -> verify -> collect` 过程提升为稳定的 run 语义层，并输出结构化可观测数据、artifacts 和 replay。

Runner 可以独立运行，也可以挂在 Docker、Apple `container`、OrbStack、Dev Container CLI、Kubernetes 或 OpenSandbox 之上。上层看到的始终是同一套 run / phase / telemetry / artifact 模型，而不是某个 provider 的专有 API。

## 设计目标

- 为 AI agent 服务：适合 patch、setup、test、verify、retry、自修复这类自动化闭环。
- 主要聚焦可观测性：重点是 run timeline、phase 状态、stdout/stderr、OTel、artifacts、replay，而不是把 sandbox 本身做成业务控制面。
- 统一 run 语义：切换 `direct`、`docker`、`devcontainer`、`k8s`、`opensandbox` 时，尽量不改变 CLI、输出 schema 和结果状态含义。
- runtime/profile 分层：`kata`、`apple-container`、`orbstack-*` 是 runtime/profile 或 provider 扩展，不是新的 run engine。
- 跨平台运行：Runner 自身支持 macOS、Linux、Windows 二进制；能力根据 OS / Arch / 是否容器内 / 是否 K8s / Linux capabilities 自动裁剪。

## 适用场景

- 本地 direct 调试：开发者或 AI agent 在本机直接跑 `go test`、`pytest`、`npm test`、`mvn test`，保留完整 run 级观测。
- 本地 Docker 调试：需要更接近 Linux 容器依赖、网络和文件系统语义时，把 workload 放进容器执行。
- 本地 Apple `container` 调试：在 Apple Silicon macOS 上用轻量 VM 容器执行 workload，同时保留统一 run artifacts 和 telemetry。
- 本地 OrbStack 调试：把 OrbStack 当作 Docker provider、Linux machine backend 或本地 K8s target，用于更接近 Linux/STG 的本地验证。
- 本地 Dev Container 调试：项目已经有 `.devcontainer/devcontainer.json`，希望在统一开发容器内执行 patch/test/verify。
- STG / Linux sandbox 验证：在 K8s Job、远端 Linux 或受控 sandbox 中执行真实 patch/test/build/verify。
- OpenSandbox 集成：平台已经接入 OpenSandbox，希望保留 Runner 语义，同时复用 provider lifecycle / exec / files / endpoints。
- Kata 隔离增强：需要把 `k8s` 或 `opensandbox` 的运行时提升到 `kata`，但不改变上层 run / phase / command model。

## 非目标

- Runner 不是安全边界；强隔离只在 Docker/K8s/Linux sandbox 场景承诺。
- 不在业务代码里直接耦合 provider DTO 或 provider client。
- 不承诺 “本地 Mac/Windows + 零注入 + 完整 Linux 内核级观测”。

## 核心架构

```text
AI Agent / IDE / CLI
        ↓
RunEngine
        ↓
PhaseEngine
        ↓
BackendProvider
   ├─ direct
   ├─ docker
   ├─ apple-container
   ├─ orbstack-machine
   ├─ devcontainer
   ├─ k8s
   └─ opensandbox
        ↓
RuntimeProfile
   ├─ native
   ├─ kata
   ├─ apple-container
   ├─ orbstack-docker
   ├─ orbstack-k8s
   └─ orbstack-machine
```

分层职责：

- RunEngine：run 生命周期、attempt、phase orchestration、状态汇总、artifacts / replay 输出。
- PhaseEngine：`prepare -> setup -> execute -> verify -> collect` 的动作排序和状态推进。
- BackendProvider：create/start/delete、exec、stream logs、upload/download、pause/resume/renew。
- RuntimeProfile：只表达执行环境的隔离强度和 runtime 元数据，不改变 command model。
- Telemetry / Artifact：输出 OTLP traces / metrics / logs，以及本地 JSONL / JSON artifacts。

## 支持能力

- 五阶段状态机：`prepare -> setup -> execute -> verify -> collect`
- 运行模式：`local_direct`、`local_docker`、`local_devcontainer`、`local_apple_container`、`local_orbstack_machine`、`stg_linux`、`local_opensandbox_docker`、`stg_opensandbox_k8s`
- runtime profile：`native`、`kata`、`apple-container`、`orbstack-docker`、`orbstack-k8s`、`orbstack-machine`
- 语言适配：Go / Python / Node / Java / Shell
- 结构化输出：`context.json`、`environment.json`、`setup.plan.json`、`phases.json`、`results.json`、`replay.json`
- backend/runtime 输出：`provider.json`、`backend-profile.json`、`sandbox.json`、`runtime.json`
- backend 专用输出：`devcontainer.json`、`machine.json`、`container.json`
- devcontainer 输出：`devcontainer.json`
- 命令日志：`commands.jsonl`、`stdout.jsonl`、`stderr.jsonl`
- OTel：traces / metrics / logs；collector 不可用时可降级到本地 JSONL
- Provider 扩展：OpenSandbox provider、K8s Job 渲染 / 提交 SDK、Dev Container CLI backend、Apple `container` backend、OrbStack Docker/K8s provider profile、OrbStack machine backend
- Go 支持：Layer A run 级观测、Layer C helper 包、Linux/STG 条件下的 Layer B 能力开关

## 模式矩阵

| 模式 | backend.kind | runtime.profile | 适用场景 | 说明 |
|---|---|---|---|---|
| `local_direct` | `direct` | `native` | 本机快速调试 | 不依赖容器，保留完整 run 级观测 |
| `local_docker` | `docker` | `native` | 本地贴近 Linux | 支持 `host-runner` / `in-container-runner` |
| `local_docker` | `docker` | `orbstack-docker` | macOS + OrbStack Docker | 复用 Docker backend，provider 标记为 `orbstack` |
| `local_devcontainer` | `devcontainer` | `native` | 本地开发容器 | 驱动 `read-configuration` / `up` / `exec` / `down` |
| `local_apple_container` | `apple-container` | `apple-container` | Apple Silicon 本地 VM 容器 | Apple `container` 单独作为 backend |
| `local_orbstack_machine` | `orbstack-machine` | `orbstack-machine` | macOS 本地 Linux machine | 适合接近 STG 的完整 Linux 用户态 |
| `stg_linux` | `k8s` | `native` / `kata` | STG / K8s / Linux sandbox | `kata` 时写入 `runtimeClassName` |
| `stg_linux` | `k8s` | `orbstack-k8s` | OrbStack 本地单节点 K8s | 复用 K8s backend，provider 标记为 `orbstack` |
| `local_opensandbox_docker` | `opensandbox` | `native` | 本地 OpenSandbox | 使用 provider lifecycle 和 execd API |
| `stg_opensandbox_k8s` | `opensandbox` | `native` / `kata` | STG OpenSandbox | `kata` 作为 runtime request 透传给 provider |

## 可观测性与产物

Runner 的主要价值是把一次 AI agent 执行过程变成可回放、可归因、可上传的结构化 run。

核心输出包括：

- run timeline 与 phase status
- command execution records
- stdout / stderr line-level structured logs
- OTel spans / events / metrics
- 本地 artifacts 与 replay manifest
- backend/runtime snapshots：`provider.json`、`backend-profile.json`、`sandbox.json`、`runtime.json`
- backend 细分信息：Apple `container` 写 `container.json`，OrbStack machine 写 `machine.json`
- Dev Container resolved summary：`devcontainer.json`

## 平台与 Feature Gates

Runner 启动后会根据运行环境自动决定 feature gates，不把 Linux 专属能力硬套到 macOS 或 Windows。

规则摘要：

- `macOS` 默认关闭 `obi_ebpf`
- `windows` 默认关闭 `obi_ebpf`
- `windows` 默认关闭 `go_autosdk_bridge`
- `linux` 非特权进程默认关闭 `obi_ebpf`
- `local_direct` 默认不启用依赖特权或内核探针的能力
- `local_devcontainer` 默认不启用依赖特权或内核探针的能力
- `local_apple_container` 默认不启用依赖特权或内核探针的能力
- `local_orbstack_machine` 默认不启用依赖特权或内核探针的能力
- `docker(provider=orbstack)` / `k8s(provider=orbstack-local)` 只改变 provider，不放宽 Linux-only feature gate
- `stg_linux` 可按策略启用 `obi_ebpf`、`go_autosdk_bridge`
- `runtime.profile=kata` 只允许 `k8s` / `opensandbox`

## Go 支持矩阵

| Capability | Mac Direct | Windows Direct | Mac Docker Host-Runner | Mac Docker In-Container | Linux Direct | STG/K8s Linux |
|---|---|---|---|---|---|---|
| Run lifecycle | Yes | Yes | Yes | Yes | Yes | Yes |
| stdout/stderr/exit code | Yes | Yes | Yes | Yes | Yes | Yes |
| Artifact / Replay | Yes | Yes | Yes | Yes | Yes | Yes |
| Go helper package | Yes | Yes | Yes | Yes | Yes | Yes |
| Go Auto SDK bridge | No by default | No | No by default | Container-dependent | Optional | Optional |
| OBI / eBPF | No | No | No | Permission-dependent | Optional | Optional |

说明：

- Mac / Windows 主要提供 Layer A run 级观测，不承诺 Linux 内核级增强能力。
- Linux / STG 在满足权限与能力条件时，可以叠加 OBI / eBPF / Auto SDK bridge。


示例配置：

- [run.sample.yaml](/Users/lotosli/Documents/Sandbox%20Runer/configs/run.sample.yaml)
- [run.apple-container.sample.yaml](/Users/lotosli/Documents/Sandbox%20Runer/configs/run.apple-container.sample.yaml)
- [run.devcontainer.sample.yaml](/Users/lotosli/Documents/Sandbox%20Runer/configs/run.devcontainer.sample.yaml)
- [run.orbstack.docker.sample.yaml](/Users/lotosli/Documents/Sandbox%20Runer/configs/run.orbstack.docker.sample.yaml)
- [run.orbstack.machine.sample.yaml](/Users/lotosli/Documents/Sandbox%20Runer/configs/run.orbstack.machine.sample.yaml)
- [run.orbstack.k8s.sample.yaml](/Users/lotosli/Documents/Sandbox%20Runer/configs/run.orbstack.k8s.sample.yaml)
- [run.opensandbox.sample.yaml](/Users/lotosli/Documents/Sandbox%20Runer/configs/run.opensandbox.sample.yaml)
- [run.opensandbox.kata.sample.yaml](/Users/lotosli/Documents/Sandbox%20Runer/configs/run.opensandbox.kata.sample.yaml)

## Quick Start

```bash
make build
./sandbox-runner run --config configs/run.sample.yaml --policy configs/policy.sample.yaml -- go test ./...
./sandbox-runner validate --config configs/run.apple-container.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner run --config configs/run.devcontainer.sample.yaml --policy configs/policy.sample.yaml -- go test ./...
./sandbox-runner validate --config configs/run.orbstack.docker.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner validate --config configs/run.orbstack.machine.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner validate --config configs/run.orbstack.k8s.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner run --config configs/run.opensandbox.sample.yaml --policy configs/policy.sample.yaml -- go test ./internal/proc
./sandbox-runner validate --config configs/run.opensandbox.kata.sample.yaml --policy configs/policy.sample.yaml
./sandbox-runner doctor
./sandbox-runner --version
```

K8s / SDK 示例：

```bash
./sandbox-runner k8s render-job --config configs/run.sample.yaml --policy configs/policy.sample.yaml
```

## Build

```bash
make test
make build
make dist
```

`make dist` 当前生成：

- `dist/sandbox-runner-darwin-amd64`
- `dist/sandbox-runner-darwin-arm64`
- `dist/sandbox-runner-linux-amd64`
- `dist/sandbox-runner-linux-arm64`
- `dist/sandbox-runner-linux-arm-v7`
- `dist/sandbox-runner-windows-amd64.exe`
- `dist/sandbox-runner-windows-arm64.exe`
- `dist/SHA256SUMS`

`--version` 输出包含：

- `version`
- `git_sha`
- `build_time`
- `target_os`
- `target_arch`
- `execution_target`
- `feature_gates`

## Test

```bash
go test ./...
go test ./tests/contract/...
go test ./tests/integration/...
```

说明：

- DevContainer backend 的单元测试使用 fake CLI，不依赖本机真实 `devcontainer`。
- DevContainer 端到端验证依赖本机 `devcontainer` CLI 和可用的 Docker daemon。
- Apple `container` 真实联调只在 macOS arm64 + 已安装 `container` CLI 下执行。
- OrbStack Docker / Machine / K8s 真实联调只在已安装 OrbStack 的 macOS 上执行。
- OpenSandbox integration test 依赖本机 `opensandbox-server` 和可用的 Docker daemon。
- 当前仓库的 integration test 在 Docker 不可用时会自动 skip；若本机 OpenSandbox 版本与当前 fixture schema 不一致，也会自动 skip。


## 注意事项

- Runner 不是安全边界；文件系统 / 网络 / 内存硬隔离只在 Docker/K8s/Linux sandbox 场景承诺。
- `collector.mode=require` 更适合 STG；`collector.mode=auto` 更适合本地开发。
- S3 上传后端使用标准 AWS SDK v2 与 S3-compatible 配置。
- 本地 OpenSandbox 联调前，请先确认 `opensandbox-server` 和 Docker daemon 可用。
