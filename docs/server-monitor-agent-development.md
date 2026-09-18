# 轻量级服务器监控 Agent 开发文档

> 文档状态：Draft v0.1
> 目标平台：Linux（第一阶段）
> 建议实现语言：Go 1.23+
> 暂定项目名：`sma`（Server Monitor Agent）

## 1. 项目目标

开发一个以单二进制方式部署的服务器监控 Agent，从 Linux 内核接口采集主机运行指标，并通过 HTTP 对外提供数据。

第一阶段重点：

- 提供 CPU、内存、文件系统、磁盘 I/O、系统负载和运行时间指标。
- 兼容 Prometheus 文本格式，能够被 Prometheus、VictoriaMetrics 等系统直接抓取。
- 提供 JSON 快照接口，方便自建控制台或其他服务直接调用。
- 无数据库、无外部进程依赖、默认不以 root 用户运行。
- 空闲时 CPU 使用率接近 0，常驻内存目标小于 20 MiB，压缩后二进制目标小于 15 MiB。
- 默认只读访问系统信息，不执行命令，不修改主机状态。

## 2. 非目标

以下能力不纳入 MVP，避免 Agent 过早膨胀：

- 日志采集、链路追踪和应用性能探针。
- 告警规则计算与消息通知；由监控平台负责。
- 指标长期存储、聚合和可视化。
- 容器、Kubernetes、GPU、硬件传感器和进程级指标。
- Agent 远程控制、脚本执行、自动升级。
- Windows 和 macOS 支持。

这些能力后续应通过独立 Collector 或外围组件扩展，而不是进入核心采集路径。

## 3. 总体设计

```text
Linux kernel interfaces
  /proc/stat       /proc/meminfo      /proc/loadavg
  /proc/diskstats  /proc/uptime       statfs(2)
         │
         ▼
  collectors（独立、可开关）
         │
         ▼
  snapshot service ── metadata / timeout / error isolation
         │
         ├── GET /metrics       Prometheus text
         ├── GET /v1/snapshot   JSON
         ├── GET /healthz       process health
         └── GET /readyz        collector readiness
```

设计原则：

1. **拉取优先**：收到 HTTP 请求时采集当前数据，不启动高频后台轮询。
2. **输出原始计数器**：CPU 时间、磁盘字节数等以累计 counter 暴露，由监控平台通过 `rate()` 计算速率。
3. **模块隔离**：单个 Collector 失败不能导致全部指标不可用。
4. **低基数标签**：禁止把路径、错误文本、进程 ID 等无界值作为 label。
5. **可观测自身**：暴露采集耗时、采集失败和构建版本等 Agent 自监控指标。

## 4. 技术选型

### 4.1 Go

采用 Go 的理由：

- 可交叉编译为静态单二进制，部署和回滚简单。
- 标准库已包含 HTTP、JSON、并发、信号处理等所需能力。
- 适合直接读取 `/proc`，无需启动 `top`、`df` 等子进程。
- 在控制依赖和并发规模后，资源占用容易保持稳定。

MVP 尽量仅使用标准库。Prometheus 文本输出由内部 encoder 实现，只覆盖项目用到的 `HELP`、`TYPE` 和 sample 语法，并用兼容性测试约束。若后续需要复杂注册表、直方图或生态 Collector，再评估引入 `prometheus/client_golang`。

### 4.2 数据源

| 指标域 | Linux 数据源 | 说明 |
|---|---|---|
| CPU | `/proc/stat` | 各 CPU 和各 mode 的累计 jiffies |
| 内存 | `/proc/meminfo` | 总量、可用量、缓存、Swap |
| 系统负载 | `/proc/loadavg` | 1/5/15 分钟平均负载 |
| 运行时间 | `/proc/uptime` | 系统启动后的秒数 |
| 文件系统 | `statfs(2)` | 空间和 inode 使用情况 |
| 挂载点发现 | `/proc/self/mountinfo` | 按 Agent 所在 mount namespace 观察 |
| 磁盘 I/O | `/proc/diskstats` | 读写次数、扇区数、耗时 |

不通过 shell 调用外部命令，减少性能开销、解析差异和命令注入面。

## 5. 指标规范

### 5.1 命名和单位

- 所有指标使用 `sma_` 前缀。
- 累计值使用 `_total` 后缀并声明为 counter。
- 单位进入名称，例如 `_bytes`、`_seconds`。
- 比例使用 `0..1`；MVP 优先暴露基础量，由查询端计算比例。
- Prometheus 标签值必须经过转义。

### 5.2 CPU

| 指标 | 类型 | 标签 | 来源/含义 |
|---|---|---|---|
| `sma_cpu_seconds_total` | counter | `cpu`, `mode` | 每个逻辑 CPU 在各 mode 的累计时间 |
| `sma_cpu_logical_count` | gauge | 无 | 在线逻辑 CPU 数量 |

`mode` 固定为 `user`、`nice`、`system`、`idle`、`iowait`、`irq`、`softirq`、`steal`。jiffies 使用 `CLK_TCK` 转换成秒；实现阶段优先通过 `getconf CLK_TCK` 的构建/平台假设消除外部运行时调用，Linux 常见值不能硬编码为唯一事实，应提供架构测试。

CPU 使用率由查询侧计算，例如：

```promql
1 - avg(rate(sma_cpu_seconds_total{mode="idle"}[5m]))
```

### 5.3 内存

| 指标 | 类型 | 来源 |
|---|---|---|
| `sma_memory_total_bytes` | gauge | `MemTotal` |
| `sma_memory_available_bytes` | gauge | `MemAvailable` |
| `sma_memory_free_bytes` | gauge | `MemFree` |
| `sma_memory_cached_bytes` | gauge | `Cached + SReclaimable - Shmem` |
| `sma_memory_buffers_bytes` | gauge | `Buffers` |
| `sma_memory_swap_total_bytes` | gauge | `SwapTotal` |
| `sma_memory_swap_free_bytes` | gauge | `SwapFree` |

当老内核没有 `MemAvailable` 时，可用量回退为 `MemFree + Buffers + Cached + SReclaimable - Shmem`，并通过测试覆盖下溢保护。

推荐查询：

```promql
1 - sma_memory_available_bytes / sma_memory_total_bytes
```

### 5.4 文件系统

| 指标 | 类型 | 标签 |
|---|---|---|
| `sma_filesystem_size_bytes` | gauge | `device`, `mountpoint`, `fstype` |
| `sma_filesystem_available_bytes` | gauge | 同上 |
| `sma_filesystem_free_bytes` | gauge | 同上 |
| `sma_filesystem_files` | gauge | 同上 |
| `sma_filesystem_files_free` | gauge | 同上 |
| `sma_filesystem_readonly` | gauge | 同上；只读为 1 |

默认排除伪文件系统：`proc`、`sysfs`、`devtmpfs`、`devpts`、`cgroup`、`cgroup2`、`securityfs`、`debugfs`、`tracefs`、`pstore`、`configfs`、`fusectl`、`mqueue`、`hugetlbfs`。是否排除 `tmpfs` 和容器 overlay 由配置控制。

文件系统使用率建议用可用空间计算：

```promql
1 - sma_filesystem_available_bytes / sma_filesystem_size_bytes
```

### 5.5 磁盘 I/O

| 指标 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `sma_disk_reads_completed_total` | counter | `device` | 完成的读操作数 |
| `sma_disk_read_bytes_total` | counter | `device` | 累计读取字节数 |
| `sma_disk_read_seconds_total` | counter | `device` | 累计读耗时 |
| `sma_disk_writes_completed_total` | counter | `device` | 完成的写操作数 |
| `sma_disk_written_bytes_total` | counter | `device` | 累计写入字节数 |
| `sma_disk_write_seconds_total` | counter | `device` | 累计写耗时 |
| `sma_disk_io_now` | gauge | `device` | 当前进行中的 I/O 数 |
| `sma_disk_io_seconds_total` | counter | `device` | 设备至少有一个 I/O 的累计时间 |

扇区到字节的转换按 Linux `/proc/diskstats` ABI 使用 512 字节扇区。默认排除 loop、ram、fd、sr、dm 分区等不需要的设备，规则可配置。设备热插拔后 counter 可能重置，查询端应使用 `rate()` 处理。

### 5.6 系统状态

| 指标 | 类型 | 说明 |
|---|---|---|
| `sma_load1` | gauge | 1 分钟负载 |
| `sma_load5` | gauge | 5 分钟负载 |
| `sma_load15` | gauge | 15 分钟负载 |
| `sma_boot_time_seconds` | gauge | Unix 启动时间戳 |
| `sma_uptime_seconds` | gauge | 启动后的秒数 |

### 5.7 Agent 自监控

| 指标 | 类型 | 标签 | 说明 |
|---|---|---|---|
| `sma_build_info` | gauge | `version`, `revision`, `go_version` | 固定值 1 |
| `sma_scrapes_total` | counter | 无 | 请求 `/metrics` 的次数 |
| `sma_scrape_errors_total` | counter | 无 | 至少一个 Collector 失败的次数 |
| `sma_collector_duration_seconds` | gauge | `collector` | 最近一次采集耗时 |
| `sma_collector_success` | gauge | `collector` | 最近一次是否成功 |

`collector` 只能取配置中已知的有限集合。

## 6. HTTP API

### 6.1 通用约定

- 默认监听：`127.0.0.1:9108`，避免未配置鉴权时暴露到公网。
- 默认请求超时：5 秒。
- 仅支持 `GET` 和 `HEAD`；其他方法返回 `405`。
- 响应包含 `X-Content-Type-Options: nosniff`。
- 不启用 CORS；如需浏览器访问，由反向代理配置。
- HTTP server 设置 `ReadHeaderTimeout`、`IdleTimeout` 和响应头大小限制。

### 6.2 `GET /metrics`

返回 Prometheus text exposition format。

```http
HTTP/1.1 200 OK
Content-Type: text/plain; version=0.0.4; charset=utf-8

# HELP sma_memory_total_bytes Total physical memory in bytes.
# TYPE sma_memory_total_bytes gauge
sma_memory_total_bytes 16777216000
```

Collector 部分失败时仍返回 `200` 和成功采集的指标，并将 `sma_collector_success` 置为 0。只有无法生成合法响应时才返回 `500`。

### 6.3 `GET /v1/snapshot`

JSON 面向程序直接读取，字段稳定但不与 Prometheus sample 一一复制。

```json
{
  "schemaVersion": "v1",
  "timestamp": "2026-09-14T08:00:00Z",
  "host": {"hostname": "node-01"},
  "cpu": {
    "logicalCount": 4,
    "seconds": {
      "cpu0": {"user": 123.4, "system": 45.6, "idle": 987.0}
    }
  },
  "memory": {
    "totalBytes": 16777216000,
    "availableBytes": 8589934592
  },
  "filesystems": [],
  "disks": [],
  "load": {"load1": 0.18, "load5": 0.21, "load15": 0.20},
  "uptimeSeconds": 86400,
  "errors": []
}
```

部分采集失败时仍返回 `200`，并在 `errors` 中返回结构化错误：

```json
{"collector":"filesystem","code":"timeout","message":"collector timed out"}
```

错误消息不得包含敏感文件内容或完整内部堆栈。

### 6.4 健康检查

- `GET /healthz`：进程 HTTP 循环正常即返回 `200 {"status":"ok"}`。
- `GET /readyz`：至少 CPU、内存两个核心 Collector 最近一次成功时返回 200，否则返回 503。
- `GET /version`：返回版本、Git revision、构建时间和 Go 版本。

## 7. 配置设计

配置优先级：命令行参数 > 环境变量 > YAML 文件 > 默认值。为保持 MVP 零依赖，第一版可先实现参数和环境变量，YAML 放在第二阶段。

建议参数：

```text
--listen-address=127.0.0.1:9108
--web.telemetry-path=/metrics
--collector.enabled=cpu,memory,filesystem,disk,load,uptime
--collector.timeout=2s
--filesystem.exclude-fs-types=...
--filesystem.exclude-mountpoints=...
--disk.exclude-devices=...
--web.auth-token-file=/etc/sma/token
--web.tls-cert-file=/etc/sma/tls.crt
--web.tls-key-file=/etc/sma/tls.key
--log.level=info
--log.format=json
```

约束：

- Token 只允许从文件读取，避免出现在进程参数和 shell 历史中。
- Token 文件权限应为 `0600` 或更严格，且不能允许 group/others 访问；使用常量时间比较校验 Bearer token。
- 同时提供证书和私钥才启用 TLS；缺一项时启动失败。
- 正则配置在启动时预编译，非法表达式直接启动失败。
- `SMA_` 前缀用于环境变量，例如 `SMA_LISTEN_ADDRESS`。

## 8. 代码结构

```text
server-monitor-agent/
├── cmd/sma/main.go
├── internal/
│   ├── buildinfo/
│   ├── collector/
│   │   ├── collector.go
│   │   ├── cpu_linux.go
│   │   ├── memory_linux.go
│   │   ├── filesystem_linux.go
│   │   ├── disk_linux.go
│   │   ├── load_linux.go
│   │   └── uptime_linux.go
│   ├── config/
│   ├── metric/
│   │   ├── model.go
│   │   └── prometheus.go
│   ├── snapshot/
│   └── web/
├── packaging/
│   ├── systemd/sma.service
│   └── docker/Dockerfile
├── docs/
├── go.mod
├── Makefile
└── README.md
```

Collector 接口建议：

```go
type Collector interface {
    Name() string
    Collect(ctx context.Context) (Result, error)
}
```

`Result` 是内部强类型快照，不直接绑定 HTTP 或 Prometheus 格式。这样可由同一份采集结果生成文本和 JSON，并方便单元测试。

## 9. 采集与并发模型

每个请求触发一次 snapshot：

1. 创建总超时 context。
2. 并发执行已启用 Collector；MVP Collector 数量固定且很小。
3. 每个 Collector 使用独立子超时和 panic recovery。
4. 汇总成功结果和结构化错误。
5. 编码为 Prometheus 或 JSON 响应。

为了避免并发抓取造成 I/O 放大，snapshot service 使用 single-flight 思路：

- 同一时刻只执行一轮实际采集。
- 并发请求共享该轮结果。
- 可配置 500 ms 的短缓存窗口；默认启用。
- 缓存保存不可变结果，编码阶段不得修改它。

不为每个样本创建 goroutine，不保存历史时序数据，不在热路径输出逐指标日志。

## 10. 安全设计

- 默认绑定 loopback；需要远程访问时由用户显式改为 `0.0.0.0` 或指定内网地址。
- systemd 以专用 `sma` 用户运行，`NoNewPrivileges=true`，不授予 Linux capabilities。
- 仅访问 `/proc`、`/sys` 和挂载点元数据，不读取业务文件。
- 限制同时处理的请求数，建议默认 32；超出返回 `503`。
- 限制 URL、header 和 auth token 大小，避免内存放大。
- 日志不记录 Authorization header、token、完整请求 header。
- JSON 仅暴露 hostname；默认不暴露 IP、MAC、内核命令行和环境变量。
- TLS/鉴权可内置，但生产环境优先通过已有网关、service mesh 或反向代理统一处理。
- 依赖保持最少，构建输出 SBOM 和 SHA-256 校验文件。

容器运行时若读取宿主机指标，需要显式挂载宿主机 `/proc`、`/sys` 和根文件系统。由于容易采到容器自身而非宿主机视图，裸机 systemd 部署是默认推荐方式。

## 11. 性能目标与测量方法

目标基于 4 核、常规云主机、约 50 个挂载点和 32 个块设备：

| 项目 | MVP 目标 |
|---|---|
| 空闲 CPU | 接近 0%，无周期采集时钟 |
| 单次采集 CPU 时间 | p95 < 10 ms |
| `/metrics` 响应时间 | p95 < 50 ms |
| 常驻内存 RSS | < 20 MiB |
| 响应体 | 常规主机 < 256 KiB |
| 并发抓取 | 32 个连接，有背压 |

性能测试必须包含：

- 1、10、50 并发抓取基准。
- 10、100、1000 个挂载点的合成 mountinfo。
- 10、100、1000 个块设备的合成 diskstats。
- 运行 24 小时后的 RSS 和 goroutine 数变化。
- 慢客户端、请求取消和 Collector 超时场景。

## 12. 错误处理和日志

- 启动配置错误：记录清晰错误并非零退出。
- 单个 Collector 解析失败：标记失败，继续返回其他指标。
- 核心 Collector 首次采集全部失败：`/readyz` 返回 503。
- HTTP server 异常：非零退出，由 systemd 重启。
- 收到 SIGTERM/SIGINT：停止接收新请求，最多等待 10 秒完成在途请求。

默认日志级别为 `info`，只记录启动、关闭、配置摘要和状态变化。重复采集错误应限频，避免故障期间刷满磁盘。

## 13. 测试策略

### 13.1 单元测试

所有 `/proc` 解析器以 `io.Reader` 为输入，使用 `testdata` 覆盖：

- 正常数据、缺失字段、多余字段、超大数值、非法数字和截断行。
- 不同 Linux 内核版本格式。
- 内存 fallback、算术下溢和计数器溢出边界。
- mountinfo 转义（如 `\040`）和重复挂载。
- Prometheus label 转义、排序和浮点特殊值。

### 13.2 集成测试

- 启动真实 HTTP server，验证全部路由、方法、超时和优雅关闭。
- 使用 Prometheus 官方解析器或 `promtool check metrics` 验证 `/metrics`。
- 在支持的 Linux CI runner 上与 `/proc` 真实数据集成测试。
- 使用 race detector 检查共享快照。

### 13.3 验收标准

MVP 完成需同时满足：

1. 六类系统 Collector 均可独立启停。
2. `/metrics` 可被 Prometheus 成功抓取且无格式错误。
3. `/v1/snapshot` schema 有自动化兼容性测试。
4. 一个 Collector 故障不会影响其他指标。
5. 非 root systemd 服务可运行。
6. amd64 和 arm64 构建通过。
7. 单元测试、集成测试、race test、静态检查全部通过。
8. 性能基准达到第 11 节目标，或对偏差形成明确记录。

## 14. 构建、发布与部署

构建时注入版本信息：

```bash
go build -trimpath \
  -ldflags "-s -w -X sma/internal/buildinfo.Version=v0.1.0" \
  -o dist/sma ./cmd/sma
```

发布产物：

- `sma_<version>_linux_amd64.tar.gz`
- `sma_<version>_linux_arm64.tar.gz`
- `checksums.txt`
- SBOM（SPDX 或 CycloneDX）
- 可选的最小 OCI 镜像

systemd 单元关键项：

```ini
[Service]
User=sma
Group=sma
ExecStart=/usr/local/bin/sma --listen-address=127.0.0.1:9108
Restart=on-failure
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryDenyWriteExecute=true
```

部署后验证：

```bash
curl --fail http://127.0.0.1:9108/healthz
curl --fail http://127.0.0.1:9108/readyz
curl --fail http://127.0.0.1:9108/metrics
```

Prometheus 配置示例：

```yaml
scrape_configs:
  - job_name: sma
    scrape_interval: 15s
    static_configs:
      - targets: ["10.0.0.10:9108"]
```

## 15. 里程碑

### M0：工程骨架

- Go module、配置、日志、信号处理、HTTP server。
- 健康检查、版本接口、CI 和构建脚本。

### M1：核心 MVP

- CPU、内存、负载、运行时间 Collector。
- Prometheus encoder、JSON snapshot 和自监控指标。
- 单元测试与 Linux 集成测试。

### M2：存储指标

- mountinfo、statfs 和 diskstats Collector。
- 排除规则、设备热插拔和异常挂载处理。
- 性能基准和 24 小时稳定性测试。

### M3：生产化

- Token/TLS、并发限制、错误限频。
- systemd、OCI 镜像、交叉编译、SBOM 和发布流程。
- 运维手册及 Prometheus/Grafana 示例。

## 16. 后续扩展原则

后续可考虑网络、进程、容器、温度和自定义文本指标，但必须满足：

- Collector 默认可关闭，且失败可隔离。
- 对指标基数、权限、采集成本进行评审。
- 不引入任意命令执行能力。
- 不破坏已有指标语义；废弃指标至少跨一个次版本保留。
- JSON schema 使用新字段向前演进，不改变既有字段类型。

## 17. 开发前待确认项

以下选择不阻塞 MVP 骨架开发，但应在进入生产化前确认：

1. Agent 仅供 Prometheus 拉取，还是 JSON API 也属于长期稳定接口。
2. 目标环境是否包含容器内运行、只读根文件系统或高度裁剪的 Linux。
3. 是否必须内置 TLS/Token，还是统一交给现有网关。
4. 是否需要网络指标以及网卡、磁盘、挂载点的业务白名单。
5. 最低支持的 Linux 内核、CPU 架构和发行版范围。
