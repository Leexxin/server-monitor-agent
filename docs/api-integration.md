# SMA 接口对接文档

> 接口版本：v1
> 对应 Agent：v0.1.0+
> 更新日期：2026-09-15

## 1. 接入概览

SMA 提供两种指标接入方式：

| 场景 | 推荐接口 | 格式 |
|---|---|---|
| Prometheus、VictoriaMetrics、Grafana Agent 等监控系统 | `GET /metrics` | Prometheus text format 0.0.4 |
| 业务系统、资产平台、监控控制台直接读取当前状态 | `GET /v1/snapshot` | JSON |

默认服务地址为 `http://127.0.0.1:9108`。需要跨主机访问时，Agent 必须显式监听内网地址，例如：

```bash
sma --listen-address=0.0.0.0:9108
```

生产环境不要将未启用 TLS 或鉴权的端口直接暴露到公网。

## 2. 通用协议

### 2.1 路由

| 方法 | 路径 | 鉴权 | 用途 |
|---|---|---|---|
| `GET`, `HEAD` | `/metrics` | 可配置 | Prometheus 指标 |
| `GET`, `HEAD` | `/v1/snapshot` | 可配置 | 当前主机指标快照 |
| `GET`, `HEAD` | `/healthz` | 无 | 进程存活探测 |
| `GET`, `HEAD` | `/readyz` | 无 | 核心采集器就绪探测 |
| `GET`, `HEAD` | `/version` | 无 | Agent 版本信息 |
| `GET` | `/v1/discovery/capabilities` | 可配置 | 自动发现能力 |
| `POST` | `/v1/discovery/runs` | 可配置；远程触发必须鉴权 | 创建自动发现任务 |
| `GET` | `/v1/discovery/runs` | 可配置 | 最近发现任务 |
| `GET` | `/v1/discovery/runs/{id}` | 可配置 | 发现任务结果 |

除 `GET`、`HEAD` 外的方法返回 `405 Method Not Allowed`。未知路径返回 `404 Not Found`。

自动发现接口的完整约定参见[自动发现接口对接文档](discovery-api-integration.md)。

### 2.2 时间、数值和单位

- 时间使用 UTC RFC 3339，例如 `2026-09-15T02:10:30.123456Z`。
- 容量统一使用 bytes，不使用 KB/MB/GB。
- 时长统一使用 seconds，允许小数。
- 百分比不直接返回；由基础量计算，结果范围建议表达为 `0..1`。
- JSON 累计计数器使用无符号 64 位整数。JavaScript/TypeScript 调用方处理超过 `2^53-1` 的值时可能丢失精度；需要精确累计值时应优先使用 `/metrics`，或在解析层使用支持大整数的 JSON 方案。

### 2.3 公共响应头

```http
Cache-Control: no-store
X-Content-Type-Options: nosniff
```

接口没有开启 CORS。浏览器前端应通过自己的后端或反向代理访问。

### 2.4 Bearer Token 鉴权

当 Agent 配置了 `--web.auth-token-file` 时，`/metrics` 和 `/v1/snapshot` 要求：

```http
Authorization: Bearer <token>
```

示例：

```bash
curl \
  -H "Authorization: Bearer ${SMA_TOKEN}" \
  http://10.0.0.10:9108/v1/snapshot
```

Token 缺失或错误时：

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer
Content-Type: application/json; charset=utf-8

{"error":{"code":"unauthorized","message":"valid bearer token required"}}
```

健康、就绪和版本接口始终不需要 Token，方便负载均衡器和服务管理器探测。

### 2.5 通用错误格式

```json
{
  "error": {
    "code": "snapshot_unavailable",
    "message": "metric collection unavailable"
  }
}
```

| HTTP 状态 | `code` | 含义 | 调用方建议 |
|---|---|---|---|
| 401 | `unauthorized` | Token 缺失或无效 | 刷新配置，不要高频重试 |
| 404 | `not_found` | 路径不存在 | 检查接口版本和地址 |
| 405 | `method_not_allowed` | HTTP 方法不支持 | 改用 GET 或 HEAD |
| 503 | `snapshot_unavailable` | 请求取消或暂时无法获得快照 | 退避后重试 |
| 503 | `too_many_requests` | 超过 Agent 并发上限 | 降低并发并指数退避 |

推荐重试策略：只重试 503 和网络错误，初始等待 1 秒，指数退避到最大 30 秒，并增加随机抖动。不要重试 4xx 配置错误。

## 3. JSON 快照接口

### 3.1 请求

```http
GET /v1/snapshot HTTP/1.1
Host: 10.0.0.10:9108
Accept: application/json
Authorization: Bearer <token>
```

### 3.2 成功响应

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
```

```json
{
  "schemaVersion": "v1",
  "timestamp": "2026-09-15T02:10:30.123456Z",
  "host": {
    "hostname": "node-01"
  },
  "cpu": {
    "logicalCount": 2,
    "seconds": {
      "cpu0": {
        "user": 120.35,
        "nice": 0,
        "system": 31.22,
        "idle": 9120.71,
        "iowait": 2.11,
        "irq": 0,
        "softirq": 0.81,
        "steal": 0
      },
      "cpu1": {
        "user": 118.91,
        "nice": 0,
        "system": 30.75,
        "idle": 9124.53,
        "iowait": 1.82,
        "irq": 0,
        "softirq": 0.76,
        "steal": 0
      }
    }
  },
  "memory": {
    "totalBytes": 16777216000,
    "availableBytes": 8589934592,
    "freeBytes": 1073741824,
    "cachedBytes": 6442450944,
    "buffersBytes": 134217728,
    "swapTotalBytes": 2147483648,
    "swapFreeBytes": 2147483648
  },
  "filesystems": [
    {
      "device": "/dev/vda1",
      "mountpoint": "/",
      "filesystemType": "ext4",
      "sizeBytes": 107374182400,
      "availableBytes": 64424509440,
      "freeBytes": 69793218560,
      "files": 6553600,
      "filesFree": 6200000,
      "readOnly": false
    }
  ],
  "disks": [
    {
      "device": "vda",
      "readsCompleted": 30210,
      "readBytes": 1772093440,
      "readSeconds": 16.21,
      "writesCompleted": 50213,
      "writtenBytes": 9227468800,
      "writeSeconds": 71.39,
      "ioInProgress": 0,
      "ioSeconds": 54.12,
      "weightedIOSeconds": 88.7
    }
  ],
  "load": {
    "load1": 0.18,
    "load5": 0.21,
    "load15": 0.2
  },
  "uptimeSeconds": 86400.31,
  "bootTimeSeconds": 1789351830.01,
  "errors": []
}
```

### 3.3 字段定义

顶层字段：

| 字段 | 类型 | 必有 | 说明 |
|---|---|---|---|
| `schemaVersion` | string | 是 | 当前固定为 `v1` |
| `timestamp` | string(date-time) | 是 | 本轮采集开始时间 |
| `host.hostname` | string | 是 | Agent 所在主机的 hostname |
| `cpu` | object | 否 | CPU Collector 启用且成功时存在 |
| `memory` | object | 否 | Memory Collector 启用且成功时存在 |
| `filesystems` | array | 是 | 文件系统列表，禁用或无数据时为 `[]` |
| `disks` | array | 是 | 块设备列表，禁用或无数据时为 `[]` |
| `load` | object | 否 | Load Collector 启用且成功时存在 |
| `uptimeSeconds` | number | 否 | 系统运行时间 |
| `bootTimeSeconds` | number | 否 | Unix 启动时间戳 |
| `errors` | array | 是 | 本轮部分采集错误，无错误时为 `[]` |

CPU 字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `logicalCount` | integer | 在线逻辑 CPU 数 |
| `seconds` | object | key 为 `cpu0`、`cpu1` 等；value 为各模式累计秒数 |

CPU mode 集合为 `user`、`nice`、`system`、`idle`、`iowait`、`irq`、`softirq`、`steal`。这些值是开机以来的累计值，不是当前百分比。计算一段时间内的 CPU 使用率需要连续两次采样：

```text
delta_total = 所有 mode 的本次值之和 - 上次值之和
delta_idle  = 本次 idle - 上次 idle
usage       = 1 - delta_idle / delta_total
```

调用方必须处理 Agent 或主机重启造成的 counter 变小；此时丢弃本次差值，重新建立基线。

Memory 字段均为整数 bytes：

| 字段 | 说明 |
|---|---|
| `totalBytes` | 物理内存总量 |
| `availableBytes` | 内核估算的不发生交换即可供新应用使用的内存 |
| `freeBytes` | 完全空闲内存 |
| `cachedBytes` | 可回收缓存估算值 |
| `buffersBytes` | 块设备缓冲区 |
| `swapTotalBytes` | Swap 总量 |
| `swapFreeBytes` | Swap 空闲量 |

推荐内存使用率：

```text
memory_usage = 1 - availableBytes / totalBytes
```

Filesystem 字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `device` | string | 挂载来源设备 |
| `mountpoint` | string | 挂载点绝对路径 |
| `filesystemType` | string | 文件系统类型，如 `ext4`、`xfs` |
| `sizeBytes` | integer | 总空间 |
| `availableBytes` | integer | 非特权用户可用空间 |
| `freeBytes` | integer | 包含 root 保留块的空闲空间 |
| `files` | integer | inode 总数 |
| `filesFree` | integer | 空闲 inode 数 |
| `readOnly` | boolean | 是否只读挂载 |

推荐磁盘空间使用率：

```text
filesystem_usage = 1 - availableBytes / sizeBytes
```

Disk 字段：

| 字段 | 类型 | 单调累计 | 说明 |
|---|---|---|---|
| `device` | string | 否 | Linux 块设备名 |
| `readsCompleted` | integer | 是 | 已完成读操作数 |
| `readBytes` | integer | 是 | 已读字节数 |
| `readSeconds` | number | 是 | 读操作累计耗时 |
| `writesCompleted` | integer | 是 | 已完成写操作数 |
| `writtenBytes` | integer | 是 | 已写字节数 |
| `writeSeconds` | number | 是 | 写操作累计耗时 |
| `ioInProgress` | integer | 否 | 当前进行中的 I/O 数 |
| `ioSeconds` | number | 是 | 至少存在一个 I/O 的累计时间 |
| `weightedIOSeconds` | number | 是 | 按并发 I/O 数加权的累计时间 |

磁盘吞吐、IOPS 必须通过相邻快照的累计值差除以时间差计算。

### 3.4 部分成功语义

某个 Collector 失败时，接口仍返回 HTTP 200，同时：

- 对应可选对象字段不存在，或对应数组为空。
- 其他 Collector 数据照常返回。
- `errors` 包含结构化错误。

```json
{
  "collector": "filesystem",
  "code": "timeout",
  "message": "collector timed out"
}
```

`collector` 当前可能为 `cpu`、`memory`、`filesystem`、`disk`、`load`、`uptime`；调用方应允许未来出现新值。`code` 当前为 `timeout` 或 `collection_failed`。

建议调用方以“字段是否存在”为准，不要因为 `errors` 非空而丢弃整个快照。

### 3.5 采集和缓存语义

- 请求会触发采集，但默认在 500 ms 内复用同一不可变快照。
- 并发请求共享同一轮采集。
- `timestamp` 表示采集开始时间，不保证等于 HTTP 响应时间。
- 接口不保存历史数据；历史趋势由调用方持久化。
- 推荐轮询间隔不低于 5 秒，常规场景建议 15～30 秒。

## 4. Prometheus 指标接口

### 4.1 请求与响应

```bash
curl -H "Authorization: Bearer ${SMA_TOKEN}" \
  http://10.0.0.10:9108/metrics
```

```http
HTTP/1.1 200 OK
Content-Type: text/plain; version=0.0.4; charset=utf-8
```

即使部分 Collector 失败，接口仍返回 200 和其余合法指标。通过 `sma_collector_success` 判断单个采集器状态。

### 4.2 指标清单

| 指标 | 类型 | 标签 | 单位/含义 |
|---|---|---|---|
| `sma_cpu_seconds_total` | counter | `cpu`, `mode` | CPU mode 累计秒数 |
| `sma_cpu_logical_count` | gauge | - | 逻辑 CPU 数 |
| `sma_memory_total_bytes` | gauge | - | 物理内存总量 |
| `sma_memory_available_bytes` | gauge | - | 可用内存 |
| `sma_memory_free_bytes` | gauge | - | 空闲内存 |
| `sma_memory_cached_bytes` | gauge | - | 缓存内存 |
| `sma_memory_buffers_bytes` | gauge | - | 缓冲内存 |
| `sma_memory_swap_total_bytes` | gauge | - | Swap 总量 |
| `sma_memory_swap_free_bytes` | gauge | - | Swap 空闲量 |
| `sma_filesystem_size_bytes` | gauge | `device`, `mountpoint`, `fstype` | 文件系统总空间 |
| `sma_filesystem_available_bytes` | gauge | 同上 | 非特权用户可用空间 |
| `sma_filesystem_free_bytes` | gauge | 同上 | 空闲空间 |
| `sma_filesystem_files` | gauge | 同上 | inode 总数 |
| `sma_filesystem_files_free` | gauge | 同上 | 空闲 inode |
| `sma_filesystem_readonly` | gauge | 同上 | 只读为 1 |
| `sma_disk_reads_completed_total` | counter | `device` | 完成读操作数 |
| `sma_disk_read_bytes_total` | counter | `device` | 累计读取 bytes |
| `sma_disk_read_seconds_total` | counter | `device` | 累计读耗时 |
| `sma_disk_writes_completed_total` | counter | `device` | 完成写操作数 |
| `sma_disk_written_bytes_total` | counter | `device` | 累计写入 bytes |
| `sma_disk_write_seconds_total` | counter | `device` | 累计写耗时 |
| `sma_disk_io_now` | gauge | `device` | 当前 I/O 数 |
| `sma_disk_io_seconds_total` | counter | `device` | I/O 活跃累计秒数 |
| `sma_disk_io_weighted_seconds_total` | counter | `device` | 加权 I/O 累计秒数 |
| `sma_load1`、`sma_load5`、`sma_load15` | gauge | - | 系统负载 |
| `sma_uptime_seconds` | gauge | - | 运行时间 |
| `sma_boot_time_seconds` | gauge | - | Unix 启动时间 |
| `sma_build_info` | gauge | `version`, `revision`, `go_version` | Agent 构建信息 |
| `sma_scrapes_total` | counter | - | `/metrics` 抓取次数 |
| `sma_scrape_errors_total` | counter | - | 有部分采集错误的抓取次数 |
| `sma_collector_duration_seconds` | gauge | `collector` | 最近一次采集耗时 |
| `sma_collector_success` | gauge | `collector` | 最近一次成功为 1 |

### 4.3 Prometheus 配置

无鉴权：

```yaml
scrape_configs:
  - job_name: sma
    scrape_interval: 15s
    scrape_timeout: 5s
    static_configs:
      - targets:
          - 10.0.0.10:9108
```

Bearer Token：

```yaml
scrape_configs:
  - job_name: sma
    scrape_interval: 15s
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/secrets/sma-token
    static_configs:
      - targets:
          - 10.0.0.10:9108
```

### 4.4 常用 PromQL

整机 CPU 使用率：

```promql
1 - avg by (instance) (rate(sma_cpu_seconds_total{mode="idle"}[5m]))
```

内存使用率：

```promql
1 - sma_memory_available_bytes / sma_memory_total_bytes
```

文件系统空间使用率：

```promql
1 - sma_filesystem_available_bytes / sma_filesystem_size_bytes
```

磁盘读取吞吐（bytes/s）：

```promql
rate(sma_disk_read_bytes_total[5m])
```

磁盘写 IOPS：

```promql
rate(sma_disk_writes_completed_total[5m])
```

Collector 故障：

```promql
sma_collector_success == 0
```

## 5. 健康和版本接口

### 5.1 `GET /healthz`

只表示进程及 HTTP 服务可响应，不验证采集器：

```json
{"status":"ok"}
```

正常状态为 HTTP 200。

### 5.2 `GET /readyz`

如果启用了 CPU、Memory Collector，则二者最近一次都成功才算就绪。

就绪，HTTP 200：

```json
{"status":"ready"}
```

未就绪，HTTP 503：

```json
{"status":"not_ready"}
```

### 5.3 `GET /version`

```json
{
  "version": "v0.1.0",
  "revision": "e5f6a7b",
  "buildTime": "2026-09-15T01:00:00Z",
  "goVersion": "go1.25.0"
}
```

## 6. 调用示例

### 6.1 Go

```go
type Snapshot struct {
    SchemaVersion string    `json:"schemaVersion"`
    Timestamp     time.Time `json:"timestamp"`
    Memory        *struct {
        TotalBytes     uint64 `json:"totalBytes"`
        AvailableBytes uint64 `json:"availableBytes"`
    } `json:"memory,omitempty"`
}

req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/snapshot", nil)
req.Header.Set("Authorization", "Bearer "+token)

client := &http.Client{Timeout: 5 * time.Second}
res, err := client.Do(req)
if err != nil {
    return err
}
defer res.Body.Close()
if res.StatusCode != http.StatusOK {
    return fmt.Errorf("SMA returned %s", res.Status)
}

var snapshot Snapshot
if err := json.NewDecoder(res.Body).Decode(&snapshot); err != nil {
    return err
}
```

### 6.2 Python

```python
import requests

response = requests.get(
    "http://10.0.0.10:9108/v1/snapshot",
    headers={"Authorization": f"Bearer {token}"},
    timeout=5,
)
response.raise_for_status()
snapshot = response.json()

memory = snapshot.get("memory")
if memory and memory["totalBytes"]:
    usage = 1 - memory["availableBytes"] / memory["totalBytes"]
```

## 7. 兼容性约定

在 `schemaVersion = "v1"` 范围内：

- 现有字段不会改名或改变类型、单位和语义。
- 新增可选字段、新增数组元素字段、新增错误码和 Collector 名称属于兼容变更。
- 调用方必须忽略未知字段，不应对对象字段数量做严格判断。
- 调用方不应依赖 JSON 对象字段顺序或数组外部的响应头顺序。
- 删除字段或改变单位需要升级新的 schemaVersion 和 API 路径。

建议对接系统保存 Agent 的 `/version` 结果，出现数据解析问题时一并上报，便于定位版本差异。
