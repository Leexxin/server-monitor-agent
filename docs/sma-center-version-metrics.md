# sma-center 对接：SMA 版本与指标解析

本文供 sma-center 的后端采集器和前端资源卡片直接使用。适用于 SMA JSON 接口 `schemaVersion = "v1"`；`agentVersion` 自 Linux v0.2.8 起出现。完整字段、状态码和发现协议分别见[指标接口](api-integration.md)及[自动发现接口](discovery-api-integration.md)。SMA 提供的是当前快照，由 sma-center 轮询、计算速率并保存历史，并非持续向 sma-center 推送主机指标；自动发现可另行配置固定地址主动上报。

## 1. 接入和版本判断

| 请求 | 用途 | 鉴权 |
| --- | --- | --- |
| `GET {agentBaseUrl}/healthz` | 进程存活 | 无 |
| `GET {agentBaseUrl}/readyz` | 核心采集器就绪；可能返回 503 | 无 |
| `GET {agentBaseUrl}/version` | 独立查询版本、修订和构建时间 | 无 |
| `GET {agentBaseUrl}/v1/snapshot` | 主机、CPU、内存、容量、I/O、负载和错误 | Agent 配置 Token 时需要 `Authorization: Bearer <原始Token>` |

`agentBaseUrl` 示例为 `http://192.168.33.124:9108`，不附加 `/v1/snapshot`。Agent 默认只监听 `127.0.0.1:9108`；跨主机接入前须在 SMA 上设置可达的监听地址，并按需限制防火墙来源。sma-center 的后端负责持有 Token 和代理请求；不要把 Token 发到浏览器、URL、前端日志或错误提示。SMA 不提供跨域 CORS。

建议每 15～30 秒采集一次，同一 Agent 串行或合并并发请求；轮询间隔不要低于 5 秒。存储采集时间 `timestamp` 和接收时间，两者用途不同。

版本来源按以下优先级：

1. 有效快照的顶层 `agentVersion`，例如 `v0.2.8`；字段从 v0.2.8 起提供。
2. 对旧版 Agent 或快照暂不可用时，使用 `GET /version` 的 `version` 字段。
3. 两者都不可用时显示“未知”，不要以 `schemaVersion`、主机 OS 版本或安装脚本版本代替。

`schemaVersion` 是 JSON 协议版本，目前为 `v1`，不是 Agent 程序版本。`/version` 还返回 `revision`、`buildTime`、`goVersion`。发现结果主动上报体从 v0.2.8 起也在顶层携带 `agentVersion`，与快照同义；旧报告可能缺失。

## 2. 最小快照示例与卡片映射

下例只展示重点字段，实际 CPU mode、文件系统和错误列表会因主机而异：

```json
{
  "schemaVersion": "v1",
  "agentVersion": "v0.2.8",
  "timestamp": "2026-10-17T06:00:00Z",
  "host": {"hostname": "node-01", "os": "linux", "architecture": "amd64", "systemMountpoint": "/"},
  "cpu": {"logicalCount": 2, "seconds": {"cpu0": {"user": 120, "system": 30, "idle": 900}, "cpu1": {"user": 110, "system": 35, "idle": 905}}},
  "memory": {"totalBytes": 8589934592, "availableBytes": 4294967296, "freeBytes": 1073741824, "cachedBytes": 2147483648, "buffersBytes": 134217728, "swapTotalBytes": 0, "swapFreeBytes": 0},
  "filesystems": [{"device": "/dev/vda1", "mountpoint": "/", "filesystemType": "ext4", "sizeBytes": 107374182400, "availableBytes": 42949672960, "freeBytes": 45097156608, "files": 1000000, "filesFree": 800000, "readOnly": false}],
  "disks": [{"device": "vda", "readsCompleted": 1200, "readBytes": 52428800, "readSeconds": 4.2, "writesCompleted": 800, "writtenBytes": 104857600, "writeSeconds": 6.1, "ioInProgress": 0, "ioSeconds": 5.1, "weightedIOSeconds": 7.3}],
  "load": {"load1": 0.3, "load5": 0.4, "load15": 0.5},
  "uptimeSeconds": 86400,
  "bootTimeSeconds": 1792111200,
  "errors": []
}
```

| sma-center 展示项 | 来源与计算 | 无法计算时 |
| --- | --- | --- |
| SMA 版本 | `agentVersion`；旧版回退到 `/version.version` | “未知” |
| 主机名、平台、架构 | `host.hostname`、`host.os`、`host.architecture` | 缺少的平台字段显示“未知” |
| CPU 核数 | `cpu.logicalCount` | “—” |
| CPU 使用率 | 相邻两次 `cpu.seconds` 的 mode 累计秒数差，见第 3 节 | 首次采样或计数器失效显示“—”，**不是 0%** |
| 内存容量和使用率 | `memory.totalBytes`；`1 - availableBytes / totalBytes` | 无 `memory` 或总量为 0 显示“—” |
| 系统盘容量和使用率 | 在 `filesystems` 按 `host.systemMountpoint` 找到对应挂载点；`1 - availableBytes / sizeBytes` | 不存在或总量为 0 显示“—” |
| 其他挂载点 | `filesystems[]` 按 `mountpoint` 列出，不能与 `disks[]` 混同 | 空数组显示“无数据” |
| 1/5/15 分钟负载 | `load.load1/load5/load15`，为负载值而非百分比 | 无 `load` 时显示“—/不支持” |
| 运行时间 | `uptimeSeconds`（秒） | “—” |
| 磁盘吞吐与 IOPS | 同一 `disks[].device` 相邻两次累计值差 / 时间差，见第 4 节 | 首次采样或计数器失效显示“—” |

容量为 bytes，可按 `1024^3` 换算 GiB；百分比最终乘以 100 并夹在 0～100 范围。`availableBytes` 是非特权用户可用空间；**不要**用 `freeBytes` 替代计算系统盘占用率。不要默认取 `filesystems[0]`，也不要把所有挂载点容量直接相加（可能重复计入同一设备）。

## 3. CPU 百分比：必须有两次有效采样

`cpu.seconds` 是按 mode 分组的开机累计秒数，不是瞬时百分比。Linux 的键通常是 `cpu0`、`cpu1`；Windows 是 `total`，不可硬编码 `cpu0`。对两次快照中相同 CPU 键和 mode 键计算差值：

```text
deltaTotal = Σ(本次各 CPU、各 mode 累计秒数 - 上次值)
deltaIdle  = Σ(本次各 CPU 的 idle 累计秒数 - 上次值)
cpuUsage   = 1 - deltaIdle / deltaTotal
```

按本口径，Linux `iowait` 计入非 idle 时间。若无上次快照、时间不递增、CPU 数或键集合变化、计数器下降、`deltaTotal <= 0`，放弃这次百分比并以本次数据建立新基线。Agent 升级、主机重启或来源主机变化时也重置基线。不要把已经汇总的结果再乘 CPU 核数。

## 4. 磁盘 I/O、缺失值与平台差异

`disks[]` 的 `readBytes`、`writtenBytes`、`readsCompleted`、`writesCompleted` 是累计计数器，不是每秒速率。对同一 `device` 的两次快照：

```text
seconds       = (本次 timestamp - 上次 timestamp) / 1000
readBytesPerS = (本次 readBytes - 上次 readBytes) / seconds
writeBytesPerS = (本次 writtenBytes - 上次 writtenBytes) / seconds
readIOPS      = (本次 readsCompleted - 上次 readsCompleted) / seconds
writeIOPS     = (本次 writesCompleted - 上次 writesCompleted) / seconds
```

`timestamp` 是 UTC 时间字符串，计算前需转成时间戳。时间不递增、设备消失或计数器下降时重置该设备基线；Windows 的操作次数计数器也可能回绕。Linux 设备与分区可能同时出现，Windows 的 `device` 则可能是 `C:` 等卷，不能盲目汇总所有 `disks[]` 来声称“物理盘总量”。`ioInProgress` 是当前值，不需要做差。

`cpu`、`memory`、`load` 等对象可缺失；`filesystems`、`disks`、`errors` 总是数组，但空数组不等于零使用率。部分采集器失败时 HTTP 仍可能为 200，有效字段照常使用，同时按 `errors[].collector` 标记对应指标异常。Windows 不支持的字段在 `host.unsupportedMetrics` 中列出；其中某些数字字段可能是占位 0，必须显示“—/不支持”，不能当作测量零值。Windows 的 `load` 不提供。

## 5. 后端解析参考（TypeScript）

以下代码只处理 CPU 与卡片核心数值；实际请求须由 sma-center 后端发起并在服务端读取 Token。解析输入前仍应校验 JSON 类型和大小，不应直接信任外部对象。

```ts
type Cpu = { logicalCount: number; seconds: Record<string, Record<string, number>> };
type Snapshot = {
  schemaVersion: string;
  agentVersion?: string; // SMA < v0.2.8 时不存在
  timestamp: string;
  host: { hostname: string; os?: string; systemMountpoint?: string; unsupportedMetrics?: string[] };
  cpu?: Cpu;
  memory?: { totalBytes: number; availableBytes: number };
  filesystems: Array<{ mountpoint: string; sizeBytes: number; availableBytes: number }>;
  errors: Array<{ collector: string; code: string; message: string }>;
};

function percent(usedFraction: number): number | null {
  return Number.isFinite(usedFraction)
    ? Math.max(0, Math.min(100, usedFraction * 100)) : null;
}

function cpuPercent(previous: Snapshot | undefined, current: Snapshot): number | null {
  if (!previous?.cpu || !current.cpu || previous.host.hostname !== current.host.hostname ||
      previous.cpu.logicalCount !== current.cpu.logicalCount ||
      (previous.agentVersion && current.agentVersion && previous.agentVersion !== current.agentVersion)) return null;
  const elapsed = Date.parse(current.timestamp) - Date.parse(previous.timestamp);
  if (!Number.isFinite(elapsed) || elapsed <= 0) return null;
  const currentCpu = current.cpu.seconds, previousCpu = previous.cpu.seconds;
  if (Object.keys(currentCpu).length !== Object.keys(previousCpu).length) return null;
  let total = 0, idle = 0;
  for (const [cpu, modes] of Object.entries(currentCpu)) {
    const before = previousCpu[cpu];
    if (!before || Object.keys(modes).length !== Object.keys(before).length ||
        !("idle" in modes)) return null;
    for (const [mode, value] of Object.entries(modes)) {
      const old = before[mode], delta = value - old;
      if (!Number.isFinite(delta) || delta < 0) return null;
      total += delta;
      if (mode === "idle") idle += delta;
    }
  }
  return total > 0 ? percent(1 - idle / total) : null;
}

function coreCard(current: Snapshot, previous?: Snapshot) {
  const root = current.filesystems.find(
    fs => fs.mountpoint === (current.host.systemMountpoint ?? "/")
  );
  return {
    version: current.agentVersion ?? null, // null 时单独查询 GET /version
    cpuPercent: cpuPercent(previous, current),
    memoryPercent: current.memory?.totalBytes
      ? percent(1 - current.memory.availableBytes / current.memory.totalBytes) : null,
    systemDiskPercent: root?.sizeBytes
      ? percent(1 - root.availableBytes / root.sizeBytes) : null,
  };
}
```

业务层应缓存每台 Agent 的上一次有效快照，并在断连、切换 Agent 地址、重启、升级、采集器报错后清除对应计算基线。大于 JavaScript 安全整数 `2^53-1` 的累计 bytes/计数可能丢失精度，精确长期统计建议使用 `/metrics` 或支持大整数的 JSON 解析器。

## 6. 接入验收清单

1. 从 **sma-center 后端所在环境** 测试 Agent 地址，不以浏览器或资源卡片 IP 代替；`/healthz` 200 只表示进程存活，`/readyz` 200 表示核心采集器可用。
2. 配置项只保存 Token 原文，发送请求时后端添加 `Authorization: Bearer `；401 提示核对 Token，404 检查接口地址，503/网络错误按退避策略重试。错误提示不显示凭据。
3. 对 v0.2.8+ 读取 `agentVersion`；旧版回退 `/version.version`。同时展示或记录 `schemaVersion`，但不要把它当程序版本。
4. 首次 CPU 与 I/O 采样显示“—”，第二次有效采样才出现速率；重启、计数器回退或快照时间不递增时重新建立基线。
5. 在 Linux 和 Windows 各核对一次根盘匹配、缺失字段、部分采集错误；不要把 Windows 不支持的占位 0 展示为 0%。

发布/部署说明：本文件随源码维护。已发布的 v0.2.8 制品没有包含本文件；只有后续按项目流程发布的新安装包才会将它安装到 `/opt/sma/docs/`。本文件的增补不改变 Agent API 行为，也不要求仅为了阅读文档升级 SMA。
