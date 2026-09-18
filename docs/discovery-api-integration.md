# SMA 自动发现接口对接文档

> 接口版本：v1
> 适用 Agent：v0.2.0+
> 面向对象：前端、资产平台后端、CMDB 和监控平台

## 1. 功能说明

自动发现由外部系统触发，Agent 在后台执行受控探测器，识别服务器上正在运行或已配置的常见软件：

- 中间件：Nginx、Apache HTTP Server、Tomcat、Kafka、ZooKeeper、RabbitMQ、HAProxy、Envoy。
- 数据库：MySQL、MariaDB、PostgreSQL、MongoDB、Redis、Elasticsearch、ClickHouse、InfluxDB。
- 文件传输：OpenSSH SFTP、vsftpd、ProFTPD。

Agent 不接受调用方上传的脚本、脚本路径、命令或参数。它只执行：

1. 内置的只读进程和端口探测器 `process`。
2. `/opt/sma/discovery/scripts` 中由 root 安装、不可被普通用户修改的白名单脚本。

发现操作不会安装、停止或修改被发现的软件。

## 2. 推荐对接流程

```text
前端/业务系统
     │
     ├── GET  /v1/discovery/capabilities
     │       获取可用探测器
     │
     ├── POST /v1/discovery/runs
     │       返回 202 和任务 ID
     │
     ├── GET  /v1/discovery/runs/{id}
     │       每 1 秒轮询，直到进入终态
     │
     └── 展示 result.assets / result.errors / report
```

任务终态包括：`succeeded`、`partial`、`failed`。

## 3. 通用协议

### 3.1 基础地址

```text
http://<agent-host>:9108
```

### 3.2 鉴权要求

配置 `--web.auth-token-file` 后，所有发现接口要求：

```http
Authorization: Bearer <agent-token>
```

如果 Agent 没有配置 Token：

- 本机回环地址可以触发发现。
- 远程地址调用 `POST /v1/discovery/runs` 返回 HTTP 403。
- Agent 不信任 `X-Forwarded-For`；反向代理访问 Agent 时应配置 Token。

发现接口没有启用 CORS。浏览器前端应通过本系统的同源后端/BFF 转发请求，不应把 Agent Token 放入公开的前端代码或浏览器持久存储。

### 3.3 Content-Type

POST 请求和 JSON 响应使用：

```http
Content-Type: application/json; charset=utf-8
```

### 3.4 时间格式

所有时间均为 UTC RFC 3339，例如：

```text
2026-09-18T03:20:30.123456Z
```

## 4. 接口清单

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/v1/discovery/capabilities` | 获取功能状态和可用探测器 |
| `POST` | `/v1/discovery/runs` | 创建发现任务 |
| `GET` | `/v1/discovery/runs` | 查询最近的任务列表 |
| `GET` | `/v1/discovery/runs/{id}` | 查询单个任务和发现结果 |

## 5. 获取发现能力

### 请求

```http
GET /v1/discovery/capabilities HTTP/1.1
Authorization: Bearer <agent-token>
```

### 响应

```json
{
  "enabled": true,
  "detectors": [
    "process",
    "script:common-services"
  ],
  "reportEnabled": true,
  "maxConcurrency": 1
}
```

字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `enabled` | boolean | 自动发现功能是否启用 |
| `detectors` | string[] | 当前 Agent 可使用的探测器名称 |
| `reportEnabled` | boolean | 是否配置了固定结果上报地址 |
| `maxConcurrency` | integer | 单台 Agent 同时允许的发现任务数，当前固定为 1 |

前端应该使用这里返回的名称生成探测器选项，不要硬编码脚本名称。

## 6. 创建发现任务

### 请求

```http
POST /v1/discovery/runs HTTP/1.1
Content-Type: application/json
Authorization: Bearer <agent-token>

{
  "detectors": ["process", "script:common-services"],
  "report": true
}
```

请求字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `detectors` | string[] | 否 | 不传或传空数组时执行所有可用探测器 |
| `report` | boolean | 否 | 是否向管理员预配置的固定地址上报；不传时，有上报地址则默认为 true |

请求不能包含未知字段，也不能传入脚本路径、命令、参数或回调 URL。

### 接受响应

HTTP 202：

```json
{
  "id": "6f60f134db4c707925c096b7",
  "status": "queued",
  "createdAt": "2026-09-18T03:20:30.123456Z",
  "links": {
    "self": "/v1/discovery/runs/6f60f134db4c707925c096b7"
  }
}
```

响应头：

```http
Location: /v1/discovery/runs/6f60f134db4c707925c096b7
Retry-After: 1
```

前端收到 202 后应保存 `id`，并按照 `links.self` 轮询，不要等待 POST 请求同步返回发现结果。

### 并发冲突

同一台 Agent 已有任务运行时返回 HTTP 409：

```json
{
  "error": {
    "code": "discovery_busy",
    "message": "a discovery run is already active"
  }
}
```

前端可以读取任务列表并展示现有任务，不应立即高频重试。

## 7. 查询任务列表

### 请求

```http
GET /v1/discovery/runs HTTP/1.1
Authorization: Bearer <agent-token>
```

### 响应

```json
{
  "runs": [
    {
      "id": "6f60f134db4c707925c096b7",
      "status": "succeeded",
      "createdAt": "2026-09-18T03:20:30.123456Z",
      "completedAt": "2026-09-18T03:20:30.481902Z",
      "assetCount": 4
    }
  ]
}
```

列表按创建时间倒序排列。任务只保存在内存中，默认保留最近 20 条；Agent 重启后清空。

## 8. 查询任务结果

### 请求

```http
GET /v1/discovery/runs/6f60f134db4c707925c096b7 HTTP/1.1
Authorization: Bearer <agent-token>
```

### 运行中响应

```json
{
  "id": "6f60f134db4c707925c096b7",
  "status": "running",
  "detectors": ["process", "script:common-services"],
  "createdAt": "2026-09-18T03:20:30.123456Z",
  "startedAt": "2026-09-18T03:20:30.124102Z",
  "report": {
    "requested": true,
    "status": "pending",
    "attempts": 0
  }
}
```

### 完成响应

```json
{
  "id": "6f60f134db4c707925c096b7",
  "status": "succeeded",
  "detectors": ["process", "script:common-services"],
  "createdAt": "2026-09-18T03:20:30.123456Z",
  "startedAt": "2026-09-18T03:20:30.124102Z",
  "completedAt": "2026-09-18T03:20:30.481902Z",
  "result": {
    "schemaVersion": "v1",
    "host": {
      "hostname": "node-01"
    },
    "assets": [
      {
        "id": "4e33c73ef9e49addde835667",
        "category": "database",
        "product": "postgresql",
        "displayName": "PostgreSQL",
        "status": "running",
        "confidence": "high",
        "ports": [
          {"protocol": "tcp", "port": 5432}
        ],
        "evidence": [
          {"source": "process", "value": "postgres", "pid": 1421},
          {"source": "script:common-services", "value": "service:postgresql.service"}
        ],
        "detectedAt": "2026-09-18T03:20:30.481902Z"
      },
      {
        "id": "6d76f19b7cb8bb0d1a74913e",
        "category": "file_transfer",
        "product": "openssh-sftp",
        "displayName": "OpenSSH SFTP",
        "status": "running",
        "confidence": "high",
        "ports": [
          {"protocol": "tcp", "port": 22}
        ],
        "evidence": [
          {"source": "configuration", "value": "sshd Subsystem sftp"},
          {"source": "process", "value": "sshd", "pid": 821}
        ],
        "detectedAt": "2026-09-18T03:20:30.481902Z"
      }
    ],
    "summary": {
      "total": 2,
      "middleware": 0,
      "database": 1,
      "file_transfer": 1
    },
    "errors": []
  },
  "report": {
    "requested": true,
    "status": "delivered",
    "attempts": 1,
    "lastAttempt": "2026-09-18T03:20:30.490102Z"
  }
}
```

## 9. 状态枚举

任务 `status`：

| 值 | 是否终态 | 说明 |
|---|---|---|
| `queued` | 否 | 已接受，等待执行 |
| `running` | 否 | 正在执行探测器 |
| `succeeded` | 是 | 所有选定探测器和请求的上报均成功 |
| `partial` | 是 | 已取得部分数据，但某个探测器或上报失败 |
| `failed` | 是 | 所有探测器失败且没有可用资产 |

上报 `report.status`：

| 值 | 说明 |
|---|---|
| `not_requested` | 本次不要求上报 |
| `not_configured` | 请求上报，但 Agent 没有配置上报地址 |
| `pending` | 等待发现完成后上报 |
| `delivered` | 接收方返回 2xx |
| `failed` | 网络失败、超时或接收方返回非 2xx |

## 10. Asset 字段定义

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 同一 hostname、category、product 下稳定的 24 位十六进制 ID |
| `category` | string | `middleware`、`database`、`file_transfer` |
| `product` | string | 稳定的机器可读产品标识 |
| `displayName` | string | 前端展示名称 |
| `status` | string | `running` 或 `configured` |
| `confidence` | string | `high`、`medium`、`low` |
| `ports` | array | Agent 观察到的标准监听端口；空数组不代表服务没有端口 |
| `evidence` | array | 发现依据 |
| `detectedAt` | string | 本轮结果聚合时间 |

`id` 用于同一主机上跨发现批次匹配同类资产，不是全局 CMDB ID。一种产品的多个进程会合并为一个 Asset，并在 `evidence` 中保留多个 PID。

### Evidence

| 字段 | 类型 | 说明 |
|---|---|---|
| `source` | string | `process`、`configuration` 或 `script:<name>` |
| `value` | string | 已脱敏的发现依据，不包含完整进程命令行 |
| `pid` | integer | 仅进程证据可能出现 |

Agent 使用完整命令行进行内部分型，但不会把命令行返回给调用方，避免泄露密码、Token 和连接串。

## 11. 部分成功和错误

探测器错误位于 `result.errors`：

```json
{
  "detector": "script:common-services",
  "code": "timeout",
  "message": "discovery timed out"
}
```

错误码：

| code | 说明 |
|---|---|
| `timeout` | 整体发现超过配置时间 |
| `discovery_failed` | 探测器执行或输出解析失败 |

前端在 `partial` 状态下仍应展示 `result.assets`，并把错误作为非阻断警告显示。

## 12. HTTP 错误

| HTTP 状态 | code | 前端处理建议 |
|---|---|---|
| 400 | `invalid_request` | 检查 JSON 格式和字段 |
| 400 | `unknown_detector` | 重新读取 capabilities |
| 401 | `unauthorized` | 由后端检查 Agent Token |
| 403 | `discovery_auth_required` | 远程触发必须配置 Token |
| 404 | `discovery_run_not_found` | 任务已过期、Agent 已重启或 ID 错误 |
| 409 | `discovery_busy` | 展示已有任务，稍后再试 |
| 503 | `discovery_unavailable` | 功能被禁用或暂不可用 |

标准错误体：

```json
{
  "error": {
    "code": "unknown_detector",
    "message": "unknown discovery detector: script:not-installed"
  }
}
```

## 13. 前端轮询示例

推荐由同源业务后端代理 `/agents/{agentId}/discovery/...` 到 SMA。以下 TypeScript 示例假定已经存在该代理：

```typescript
type RunStatus = "queued" | "running" | "succeeded" | "partial" | "failed";

const terminal = new Set<RunStatus>(["succeeded", "partial", "failed"]);

export async function discover(agentId: string) {
  const created = await fetch(`/api/agents/${agentId}/discovery/runs`, {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({report: true}),
  });

  if (created.status === 409) {
    throw new Error("该服务器已有发现任务正在执行");
  }
  if (!created.ok) {
    throw new Error(`创建发现任务失败：HTTP ${created.status}`);
  }

  const task = await created.json();
  for (;;) {
    await new Promise(resolve => setTimeout(resolve, 1000));
    const response = await fetch(`/api/agents/${agentId}/discovery/runs/${task.id}`);
    if (!response.ok) {
      throw new Error(`查询发现任务失败：HTTP ${response.status}`);
    }
    const run = await response.json();
    if (terminal.has(run.status)) {
      return run;
    }
  }
}
```

前端建议：

- 轮询间隔使用 1 秒，不要并发轮询。
- 页面刷新后可以通过任务列表恢复状态。
- `partial` 应显示结果和警告，不应按完全失败处理。
- 对未知 `product`、`source` 和未来新增字段保持兼容。
- 终态后停止轮询。

## 14. 固定地址上报协议

管理员通过 Agent 启动参数配置上报地址，调用方不能在触发请求中修改地址：

```text
--discovery.report-url=https://cmdb.example.com/api/v1/sma/discovery
--discovery.report-token-file=/opt/sma/config/report-token
```

Agent 对该地址发起：

```http
POST /api/v1/sma/discovery HTTP/1.1
Content-Type: application/json
Authorization: Bearer <report-token>
User-Agent: sma-discovery/1
```

请求体：

```json
{
  "schemaVersion": "v1",
  "runId": "6f60f134db4c707925c096b7",
  "status": "succeeded",
  "startedAt": "2026-09-18T03:20:30.124102Z",
  "completedAt": "2026-09-18T03:20:30.481902Z",
  "result": {
    "schemaVersion": "v1",
    "host": {"hostname": "node-01"},
    "assets": [],
    "summary": {
      "total": 0,
      "middleware": 0,
      "database": 0,
      "file_transfer": 0
    },
    "errors": []
  }
}
```

接收方返回任意 2xx 表示成功。Agent 不跟随 HTTP 重定向，单次上报超时为 10 秒，当前版本不自动重试；失败状态会保留在任务的 `report` 字段中。

## 15. 配置参数

| 参数 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `--discovery.enabled` | `SMA_DISCOVERY_ENABLED` | `true` | 启用发现接口 |
| `--discovery.timeout` | `SMA_DISCOVERY_TIMEOUT` | `30s` | 单次任务总超时 |
| `--discovery.retention` | `SMA_DISCOVERY_RETENTION` | `20` | 内存中保留的任务数 |
| `--discovery.script-dir` | `SMA_DISCOVERY_SCRIPT_DIR` | `/opt/sma/discovery/scripts` | root 管理的脚本目录 |
| `--discovery.report-url` | `SMA_DISCOVERY_REPORT_URL` | 空 | 固定上报地址 |
| `--discovery.report-token-file` | `SMA_DISCOVERY_REPORT_TOKEN_FILE` | 空 | 上报 Bearer Token 文件 |

远程触发还应配置：

```text
--web.auth-token-file=/opt/sma/config/token
```

配置文件 `/opt/sma/config/sma.env` 示例：

```text
SMA_OPTS="--listen-address=0.0.0.0:9108 --web.auth-token-file=/opt/sma/config/token --discovery.report-url=https://cmdb.example.com/api/v1/sma/discovery --discovery.report-token-file=/opt/sma/config/report-token"
```

## 16. 兼容性约定

在发现 schema `v1` 范围内：

- 已有字段不会改变类型、单位或语义。
- 新增可选字段、产品、探测器、证据来源和错误码属于兼容变更。
- 前端必须忽略未知字段和枚举值，不能依赖 JSON 字段顺序。
- 破坏性变更会使用新的 API 路径和 `schemaVersion`。
