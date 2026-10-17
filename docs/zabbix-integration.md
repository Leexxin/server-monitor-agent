# SMA 接入 Zabbix：新手知识与操作指南

本文面向第一次使用 Zabbix 的维护者，从概念、连通性验证、界面配置到告警验收，完成 SMA 指标接入。示例以 **Zabbix 7.0 的界面与表达式**为基准，其他版本请对照对应版本官方手册，菜单中文翻译可能略有不同。核对日期：2026-10-18。

主流程面向 Linux SMA，使用 `GET /v1/snapshot`。Windows 的内存和运行时间可以复用；CPU 汇总结构、系统盘和不支持字段见[Windows 接口说明](windows/README.md#接口对接)。本文提供手工配置教程，不包含可直接导入的 Zabbix 模板文件。

验证边界：已对照仓库接口和 Zabbix 官方资料编写；未连接实际 Zabbix Server/Proxy，未进行目标环境界面配置、模板导入、通知送达或生产部署验证。文中阈值为入门示例，需要结合业务调整。

## 目录

1. [先理解接入方式和知识点](#1-先理解接入方式和知识点)
2. [准备环境与参数](#2-准备环境与参数)
3. [验证 SMA 和网络](#3-验证-sma-和网络)
4. [建立模板、主机和宏](#4-建立模板主机和宏)
5. [创建 HTTP 主监控项](#5-创建-http-主监控项)
6. [拆分内存、磁盘等指标](#6-拆分内存磁盘等指标)
7. [进阶：CPU 与磁盘速率](#7-进阶cpu-与磁盘速率)
8. [触发器、恢复和通知](#8-触发器恢复和通知)
9. [查看数据与验收](#9-查看数据与验收)
10. [多主机复用与自动发现](#10-多主机复用与自动发现)
11. [常见问题排查](#11-常见问题排查)
12. [最终接入步骤速查](#12-最终接入步骤速查)
13. [官方知识点索引](#13-官方知识点索引)

## 1. 先理解接入方式和知识点

### 1.1 数据怎样进入 Zabbix

```text
被监控服务器                               监控侧
SMA（内网 IP:9108） <-- HTTP GET + Token -- Zabbix Server 或 Proxy
       返回 /v1/snapshot JSON               |
                                           +-- 主监控项：完整快照
                                           +-- 依赖项：内存、容量、采集错误数
                                           +-- 触发器：判断异常
                                           +-- 动作：发送通知
```

这里是 Zabbix 主动请求 SMA。HTTP agent 是 Zabbix 的一种监控项类型，不是让你再安装一个同名 Agent。它由 Server 执行；主机交给 Proxy 监控时，由对应 Proxy 发请求。此路线不需要在 SMA 主机安装 Zabbix agent，也不要求 SMA 向 Zabbix 推送。详见[官方 HTTP agent 说明](https://www.zabbix.com/documentation/7.0/en/manual/config/items/itemtypes/http)。

### 1.2 操作前需要的术语

| 知识点 | 通俗解释 | 本文对应内容 |
| --- | --- | --- |
| Server / Frontend / 数据库 | 监控处理服务 / 配置和看图的网页 / 保存配置及历史数据 | 能打开网页不代表采集服务已经工作 |
| Proxy | 部署在被监控网络附近的采集代理 | 选择它后，要从它所在网络验证 SMA |
| Host（主机） | Zabbix 中代表一个监控对象的记录 | `sma-node-01` |
| Host group（主机群组） | 主机的分类，也用于分配访问权限 | `SMA servers` |
| Template（模板） | 可重复使用的监控项和告警配置集合 | `SMA by HTTP` |
| Item（监控项） | 一条要定期保存的数据 | 内存可用字节数 |
| Key（键值） | 同一主机上唯一的监控项标识 | `sma.memory.available`，不是接口 URL |
| Master / Dependent item | 一次获取原始数据，再从中拆分指标 | 一次快照拆成多个指标，减少请求 |
| Macro（宏） | 配置中的变量 | `{$SMA.URL}`、`{$SMA.TOKEN}` |
| Preprocessing（预处理） | 入库前提取、校验或计算数据 | JSONPath 和 JavaScript |
| Trigger / Problem | 异常判断规则 / 规则触发产生的问题 | 内存持续偏高 |
| Action / Media | 发生事件后的操作 / 通知渠道与收件地址 | 邮件发送到指定用户 |
| History / Trends | 原始采样记录 / 数值数据的小时汇总 | 示例保存 7 天明细、365 天趋势 |
| LLD（低级别自动发现） | 根据数组动态建立多个监控项 | 自动为每个挂载点生成容量指标 |

建议学习顺序：Host → Template → Item → Macro → Preprocessing → Trigger → Action → LLD。先做通一台，再复制到其他主机。

## 2. 准备环境与参数

### 2.1 如果还没有 Zabbix

先在独立实验环境准备 Zabbix Server、数据库和 Web Frontend。到[官方安装选择器](https://www.zabbix.com/download)选择 7.0、操作系统、数据库和 Web 服务，按生成的命令安装并完成网页初始化；不要混用不同发行版的安装命令。本文从已有可登录的 Zabbix 开始。

继续前确认：能登录网页、Server 正在运行、当前用户有模板和主机配置权限。邮件等通知渠道可稍后配置。使用 Proxy 的环境还需确认 Proxy 在线并已同步配置。安装在容器中时，采集请求来自 Server/Proxy 容器，而不是浏览器或 Frontend 容器。

### 2.2 准备一份参数表

以下均为示例，执行前替换成实际值：

| 参数 | 示例 | 用途 |
| --- | --- | --- |
| SMA 主机内网 IP | `192.0.2.10` | 此地址是文档示例地址，不能原样使用 |
| 实际采集方 | Server 或指定 Proxy | 防火墙允许它访问 SMA TCP 9108 |
| SMA 基地址 | `http://192.0.2.10:9108` | 不带末尾 `/`，也不包含 `/v1/snapshot` |
| Zabbix 主机名 | `sma-node-01` | 后续表达式使用的技术名称 |
| 模板名 | `SMA by HTTP` | 本文表达式按此名称填写 |
| Token | 当前 SMA 已有 Token | 只填主机 Secret text 宏，不写进模板文件 |
| 首个挂载点 | `/` | 先固定监控一个，之后再学 LLD |
| 采集周期 / 超时 | `30s` / `5s` | 常规起点，按实际采集耗时调整 |

HTTP 明文传输不会加密 Token。示例适用于受控内网；跨不可信网络时先配置 VPN 或 HTTPS 反向代理。SMA 当前 HTTP 服务本身不提供本文所需的 TLS 配置入口；HTTPS 场景需另设终止 TLS 的代理，并在 Zabbix 中启用证书和主机名校验。

## 3. 验证 SMA 和网络

### 3.1 在 SMA 主机上只读检查

Linux 标准安装可运行：

```sh
sudo sma-ctl status
sudo sma-ctl doctor
sudo sma-ctl config show
```

未安装 SMA 时，先按[安装与卸载](installation.md)或[全生命周期操作](lifecycle.md)完成安装。已有实例无需为接入而重新安装或重置 Token。

默认监听 `127.0.0.1:9108`，其他主机无法访问。如确实需要变更监听，在安排好变更窗口后运行以下示例；**此命令会重启 SMA**：

```sh
# 必须替换为本机实际拥有的内网 IP。
sudo sma-ctl config listen 192.0.2.10:9108
```

该命令保留其他参数并确保鉴权配置，具体行为见生命周期文档。管理员还需按实际防火墙平台，只允许采集方访问 TCP 9108。修改监听后，请求 URL 要使用新的监听 IP，不能继续假定回环地址可用。

通过 `sudo sma-ctl token show` 获取当前 Token，安全转存给有权限的 Zabbix 管理员。不要贴到工单、聊天记录、截图或 Git，也不要执行 `token reset` 来解决普通连通性问题。Windows 使用 SMA 管理中心查看连接配置和 Token。

### 3.2 从真正的 Server/Proxy 运行环境测试

先做无需 Token 的只读探测：

```sh
curl --connect-timeout 3 --max-time 5 -fsS http://192.0.2.10:9108/healthz
curl --connect-timeout 3 --max-time 5 -fsS http://192.0.2.10:9108/version
```

预期分别得到 `{"status":"ok"}` 和版本 JSON。健康接口可访问并不证明 Token 正确，也不证明所有采集器都正常。

然后验证带 Token 的快照。以下在 **Bash** 中执行，输入不会回显，命令历史不包含 Token 原文：

```bash
read -r -s -p 'SMA Token: ' SMA_TOKEN
printf '\n'
curl --connect-timeout 3 --max-time 5 -fsS \
  -H "Authorization: Bearer ${SMA_TOKEN}" \
  http://192.0.2.10:9108/v1/snapshot
unset SMA_TOKEN
```

在受控终端执行，勿开启 shell 调试或 `curl -v`，本机有权限的进程仍可能查看命令参数。预期得到 `schemaVersion` 为 `v1` 的 JSON；检查 `memory`、`filesystems` 和 `errors`。HTTP 200 允许部分采集器失败，应查看缺失字段和错误数组。完整字段口径见[指标接口对接](api-integration.md)。

如果本步失败，先排查网络或鉴权，再继续配置 Zabbix。浏览器能访问或 SMA 本机 curl 成功，都不能代替从 Server/Proxy 的测试。

## 4. 建立模板、主机和宏

### 4.1 创建模板

在 **数据采集（Data collection）→ 模板（Templates）→ 创建模板（Create template）**：

1. 模板名称填 `SMA by HTTP`。
2. 模板群组选择已有适当群组，或创建 `Templates/SMA`。
3. 保存。后续监控项和触发器都在这个模板中创建。

模板使规则统一管理，修改后可传播到关联主机，见[官方模板配置](https://www.zabbix.com/documentation/7.0/en/manual/config/templates/template)。

### 4.2 创建并关联主机

在 **数据采集（Data collection）→ 主机（Hosts）→ 创建主机（Create host）**：

1. 主机名称填 `sma-node-01`；可见名称可填便于识别的中文名称。
2. 主机群组选择或创建 `SMA servers`。
3. 模板关联 `SMA by HTTP`。
4. Monitored by 选择实际采集方：Server 或指定 Proxy。
5. 启用该主机，保存。

本文 URL 使用宏指定，不依赖主机接口 IP，故无需为 HTTP 采集添加 Agent 接口。已有其他用途的接口可以保留。不要把 SMA 9108 当成 Zabbix agent 10050 端口。主机名、群组的基础概念可对照[官方新增主机教程](https://www.zabbix.com/documentation/7.0/en/manual/quickstart/basic_config/host)。

### 4.3 配置主机宏

打开该主机的 **宏（Macros）** 页，添加：

| 宏 | 值 | 类型 |
| --- | --- | --- |
| `{$SMA.URL}` | `http://192.0.2.10:9108` | Text |
| `{$SMA.TOKEN}` | 当前主机的 Token 原文 | **Secret text** |

Token 值不包含 `Bearer `，前缀由请求头统一添加。每台主机设置自己的 URL 和 Token，不在共享模板里放真实 Token。

Secret text 用于遮蔽界面值，不是网络加密，也不能代替对 Zabbix 配置库、备份和管理权限的保护。测试窗口可能需要重新提供秘密宏值；不要为方便测试改成普通文本。见[官方秘密宏说明](https://www.zabbix.com/documentation/7.0/en/manual/config/macros/secret_macros)。

## 5. 创建 HTTP 主监控项

英文界面操作路径为 **Data collection → Templates → 在 SMA by HTTP 所在行点击 Items → Create item**。具体位置如下：

1. 展开左侧 **Data collection**，点击 **Templates**。
2. 在模板列表上方的 **Name** 筛选框输入 `SMA by HTTP`，点击 **Apply**。
3. 找到 `SMA by HTTP` 所在行，点击这一行的 **Items** 链接（通常带数量，如 `Items 0`）。点击模板名称会打开模板属性窗口；监控项入口在列表行的 Items 链接。
4. 进入监控项列表后，点击右上角 **Create item**。

`SMA by HTTP` 是第 4.1 节由你创建的自定义模板，不是 Zabbix 自带模板；如果列表中没有它，先点击 **Create template** 完成创建。如果左侧没有 Data collection，请先确认版本：Zabbix 6.0 的入口是 **Configuration → Templates**；7.0 使用 **Data collection → Templates**。如果整个配置菜单都不可见，再请管理员检查当前账号的角色和配置权限。

上述列表入口可对照[官方 Templates 页面说明](https://www.zabbix.com/documentation/7.0/en/manual/web_interface/frontend_sections/data_collection/templates)。进入 **Create item** 后填写：

| 字段 | 填写内容 |
| --- | --- |
| 名称（Name） | `SMA: Get snapshot` |
| 类型（Type） | `HTTP agent` |
| 键值（Key） | `sma.snapshot` |
| 信息类型（Type of information） | `Text` |
| URL | `{$SMA.URL}/v1/snapshot` |
| 请求方式（Request type） | `GET` |
| 请求头（Headers） | `Authorization` = `Bearer {$SMA.TOKEN}`；`Accept` = `application/json` |
| HTTP authentication | `None`；Bearer 已由请求头提供 |
| Retrieve mode | `Body` |
| Convert to JSON | 不勾选；响应已经是 JSON |
| Required status codes | `200` |
| Follow redirects | 不勾选；按已确认的 SMA 地址访问 |
| 更新间隔（Update interval） | `30s` |
| Timeout | `5s`；若界面使用全局超时，选择覆盖后填写 |
| History | `1d`，便于初次排障；以后按容量调整 |
| Enabled | 勾选 |

这里保存整份 JSON，而不是某个数值。主项先不加预处理，也不要添加“丢弃未变化值”，这样每个周期都能向依赖项传递快照。HTTP 状态码不符合要求时，主项会变为不支持；它不会自动存储一个可供触发器使用的 401/503 数值。相关字段见[HTTP agent 配置表](https://www.zabbix.com/documentation/7.0/en/manual/config/items/itemtypes/http)。

保存后到主机的监控项列表，检查已继承此项；可对主项选择“立即执行（Execute now）”，或等待配置同步和 1～2 个采集周期。在 **监测 → 最新数据（Monitoring → Latest data）**按主机过滤，检查主项有新 JSON。若模板测试窗口无法解析主机宏，应在关联主机上下文测试或等待真实轮询；不要把测试按钮成功等同于已持续入库。

## 6. 拆分内存、磁盘等指标

### 6.1 先建立一个最简单的依赖项

回到模板监控项列表，创建：

| 字段 | 值 |
| --- | --- |
| 名称 | `SMA: Memory available` |
| 类型 | `Dependent item` |
| 键值 | `sma.memory.available` |
| 主监控项（Master item） | `SMA: Get snapshot` / `sma.snapshot` |
| 信息类型 | `Numeric (unsigned)` |
| 单位 | `B` |
| History / Trends | `7d` / `365d` |
| 预处理 | `JSONPath`，参数 `$.memory.availableBytes` |

依赖项随主项的新数据更新，不另设轮询周期。主项失败时，依赖项也不会获得新样本。见[官方依赖项说明](https://www.zabbix.com/documentation/7.0/en/manual/config/items/itemtypes/dependent_items)。

**知识点：JSONPath** 是从 JSON 中取字段的路径。`$` 表示根对象，`$.memory.availableBytes` 表示进入 memory 后读取 availableBytes；不需要把整份 JSON 转成字符串再用正则拆。

### 6.2 按同样方法添加常用指标

以下项目均在模板创建，类型选 `Dependent item`，主项选 `sma.snapshot`；数值项沿用 `7d` / `365d`。

| 名称 | 键值 | 信息类型 | 单位 | JSONPath |
| --- | --- | --- | --- | --- |
| SMA: Hostname | `sma.hostname` | Character | 留空 | `$.host.hostname` |
| SMA: Memory total | `sma.memory.total` | Numeric (unsigned) | `B` | `$.memory.totalBytes` |
| SMA: CPU logical count | `sma.cpu.count` | Numeric (unsigned) | 留空 | `$.cpu.logicalCount` |
| SMA: Load 1m | `sma.load1` | Numeric (float) | 留空 | `$.load.load1` |
| SMA: Uptime | `sma.uptime` | Numeric (float) | `s` | `$.uptimeSeconds` |
| SMA: Collector error count | `sma.collector.errors` | Numeric (unsigned) | 留空 | `$.errors.length()` |
| SMA: Root size | `sma.fs.root.size` | Numeric (unsigned) | `B` | `$.filesystems[?(@.mountpoint == "/")].sizeBytes.first()` |
| SMA: Root available | `sma.fs.root.available` | Numeric (unsigned) | `B` | `$.filesystems[?(@.mountpoint == "/")].availableBytes.first()` |

字符项只设 History，不设 Trends。`B` 让容量按字节单位显示；`s` 表示秒。Load 是负载值，不能写成 `%`。JSONPath 过滤会产生匹配集合，`.first()` 取第一个匹配值；不能把 `[64424509440]` 这种数组作为数值入库。详见[官方 JSONPath 语法](https://www.zabbix.com/documentation/7.0/en/manual/config/items/preprocessing/jsonpath_functionality)。

`/` 是 Linux 示例。不要使用 `filesystems[0]` 代替根分区匹配，数组顺序没有这一含义。Windows 应读取 `host.systemMountpoint` 并匹配实际卷，另注意反斜杠转义；入门阶段先做内存和运行时间。Windows 不创建 Load 监控项。

### 6.3 内存使用率：同一快照中计算

创建依赖项 `SMA: Memory used percent`，键值 `sma.memory.used.pct`，信息类型 `Numeric (float)`，单位 `%`，主项 `sma.snapshot`。预处理选择 **JavaScript**，输入：

```javascript
var s = JSON.parse(value);
var m = s.memory;
if (!m || typeof m.totalBytes !== 'number' || m.totalBytes <= 0 ||
    typeof m.availableBytes !== 'number' || m.availableBytes < 0 ||
    m.availableBytes > m.totalBytes) {
    throw 'Invalid or missing memory data';
}
return 100 * (1 - m.availableBytes / m.totalBytes);
```

公式为 `100 × (1 - availableBytes / totalBytes)`。这是 SMA 推荐口径；freeBytes 只是完全空闲量，不能替代可用内存。例：总量 16 GiB、可用 6 GiB，使用率为 62.5%。

### 6.4 根分区使用率

创建依赖项 `SMA: Root used percent`，键值 `sma.fs.root.used.pct`，信息类型 `Numeric (float)`，单位 `%`，主项 `sma.snapshot`；JavaScript 预处理：

```javascript
var s = JSON.parse(value);
var list = s.filesystems || [];
for (var i = 0; i < list.length; i++) {
    var f = list[i];
    if (f.mountpoint === '/') {
        if (typeof f.sizeBytes !== 'number' || f.sizeBytes <= 0 ||
            typeof f.availableBytes !== 'number' || f.availableBytes < 0 ||
            f.availableBytes > f.sizeBytes) {
            throw 'Invalid root filesystem data';
        }
        return 100 * (1 - f.availableBytes / f.sizeBytes);
    }
}
throw 'Root filesystem not found';
```

这里按普通用户可用空间计算，可能与使用 freeBytes 的工具不同。缺失或异常字段让预处理失败，保留错误供排障，**不要把失败值替换成 0**，否则会把数据缺失显示为正常。

### 6.5 如何测试预处理

编辑预处理步骤时选择测试，把脱敏的快照 JSON 粘贴到输入值；检查最终输出是数字而非数组或 JSON 对象。JavaScript 的 `value` 就是主项传入的完整 JSON 文本。

测试样例可用：

```json
{"schemaVersion":"v1","memory":{"totalBytes":1000,"availableBytes":375},"filesystems":[{"mountpoint":"/","sizeBytes":1000,"availableBytes":200}],"errors":[]}
```

预期内存使用率 `62.5`、根分区使用率 `80`、采集错误数 `0`。同时测试删除 memory、sizeBytes 为 0、缺少根分区等情况，确认报错而不是返回正常零值。预处理顺序及失败处理见[官方预处理说明](https://www.zabbix.com/documentation/7.0/en/manual/config/items/preprocessing)。

## 7. 进阶：CPU 与磁盘速率

可以先跳过本节，完成第 8～9 节后再添加。

### 7.1 CPU 累计秒数不能直接当百分比

SMA 的 `cpu.seconds` 是开机以来的累计时间。两次样本才能求使用率：

```text
total = 所有 CPU、所有 mode 累计秒数之和
idle  = 所有 CPU 的 idle 累计秒数之和
CPU 使用率 = 100 × (1 - Δidle / Δtotal)
```

此口径把 iowait 纳入非 idle 时间，和某些工具扣除 iowait 的“忙碌率”会有差别。Linux 遍历 `cpu0` 等键；Windows 使用 `total` 汇总键，不能硬编码只取 cpu0。

可在模板创建两个依赖项，主项都为 `sma.snapshot`，信息类型均为 `Numeric (float)`：

- `sma.cpu.total.delta`：以下 JavaScript 后，添加第二步 **Simple change（简单变化）**。
- `sma.cpu.idle.delta`：复制相同 JavaScript，将 `var idleOnly = false;` 改成 `true`，再添加 Simple change。

```javascript
var idleOnly = false;
var s = JSON.parse(value);
if (!s.cpu || !s.cpu.seconds) throw 'CPU data missing';
var cpus = s.cpu.seconds;
var sum = 0, count = 0;
for (var name in cpus) {
    if (!Object.prototype.hasOwnProperty.call(cpus, name)) continue;
    var modes = cpus[name];
    if (typeof modes.idle !== 'number') throw 'CPU idle missing';
    count++;
    for (var mode in modes) {
        if (!Object.prototype.hasOwnProperty.call(modes, mode)) continue;
        if (idleOnly && mode !== 'idle') continue;
        var n = modes[mode];
        if (typeof n !== 'number' || !isFinite(n) || n < 0) throw 'Invalid CPU time';
        sum += n;
    }
}
if (!count) throw 'CPU list empty';
return sum;
```

然后创建 **Calculated（可计算）**监控项 `sma.cpu.used.pct`，信息类型 `Numeric (float)`，单位 `%`，更新间隔 `30s`，Formula 填：

```text
100*(1-last(//sma.cpu.idle.delta)/last(//sma.cpu.total.delta))
```

`//` 表示当前主机。首次没有差值、累计值下降（例如系统重启）时不产生有效差值；Zabbix 会等待新基线，分母为 0 时计算也会失败，不能用 0% 代替。此计算已对全部 CPU 求和，不再乘或除逻辑 CPU 数。

可计算项按自己的周期读取已存历史，不保证与依赖项在同一时刻完成。此配置适合入门观察；采集恢复、重启、CPU 热插拔或更新交错时，要核对两个差值的时间是否一致，暂不据此配置严格 CPU 告警。若需要严格同一窗口的 CPU 告警，应另行实现并验证成对样本校验。参考[变化值预处理](https://www.zabbix.com/documentation/7.0/en/manual/config/items/preprocessing)和[可计算项](https://www.zabbix.com/documentation/7.0/en/manual/config/items/itemtypes/calculated)。

### 7.2 磁盘吞吐和 IOPS

先在快照中确认实际设备名。以 `vda` 为例，创建依赖项 `sma.disk.vda.read.bps`，主项 `sma.snapshot`，信息类型 `Numeric (float)`，单位 `Bps`；按顺序添加：

1. JSONPath：`$.disks[?(@.device == "vda")].readBytes.first()`。
2. Change per second（每秒变化）。

读 IOPS 可用同样方式提取 `readsCompleted`，单位填 `ops/s`；写吞吐提取 `writtenBytes`。第一次和计数器下降后的基线阶段可能无数据，属于正常情况。不要把累计 readBytes 当吞吐，也不要同时使用 Simple change 和 Change per second；每个监控项只选一种变化计算。

## 8. 触发器、恢复和通知

### 8.1 在模板创建触发器

进入 **Data collection → Templates**，在 `SMA by HTTP` 所在行点击 **Triggers（触发器）**，再点击 **Create trigger（创建触发器）**。以下表达式中的 `SMA by HTTP` 必须与模板技术名称一致，可用表达式构造器选择监控项避免手打错误。

| 名称 | 严重性示例 | 问题表达式 | 恢复方式 |
| --- | --- | --- | --- |
| SMA: No snapshot for 3m | Average | `nodata(/SMA by HTTP/sma.snapshot,3m)=1` | 问题表达式变假，恢复收到快照 |
| SMA: Memory high | Warning | `min(/SMA by HTTP/sma.memory.used.pct,5m)>90` | Recovery expression：`max(/SMA by HTTP/sma.memory.used.pct,5m)<85` |
| SMA: Root filesystem high | Warning | `min(/SMA by HTTP/sma.fs.root.used.pct,5m)>85` | Recovery expression：`max(/SMA by HTTP/sma.fs.root.used.pct,5m)<80` |
| SMA: Collector errors | Warning | `min(/SMA by HTTP/sma.collector.errors,3m)>0` | 问题表达式变假 |
| SMA: Memory data missing | Warning | `nodata(/SMA by HTTP/sma.memory.used.pct,3m)=1` | 问题表达式变假 |
| SMA: Root data missing | Warning | `nodata(/SMA by HTTP/sma.fs.root.used.pct,3m)=1` | 问题表达式变假 |

需要独立恢复阈值的两项，在界面将 **OK event generation** 设为 **Recovery expression**，再填恢复表达式，其余按 Expression 默认方式。高阈值触发、低阈值恢复构成“滞回”，减少边界反复告警。恢复表达式的判断还要求问题表达式已变假，见[官方触发器表达式](https://www.zabbix.com/documentation/7.0/en/manual/config/triggers/expression)。

`min(...,5m)>90` 表示窗口内已有样本全部大于 90，不等同于 `avg(...,5m)>90`。刚开始监控、窗口尚无足量样本时，它也不能严格证明完整持续了五分钟；正式使用需结合采样完整性和业务要求调整。`nodata` 处理断流；主项存在历史值并不代表正在更新。

可以把其他触发器依赖于 No snapshot，减少整机断流时重复新告警；这不会自动修复数据，也不会自动关闭已经产生的问题。Proxy 断联会影响 nodata 的判断，需结合实际 Proxy 配置验收。HTTP 200 时仍要监控 errors 和关键依赖项缺失。

### 8.2 出现问题不等于已经发通知

完成下面的链路，才能收到邮件等消息：

1. 在 **告警（Alerts）→ 媒介类型（Media types）**配置并测试邮件服务或所需 Webhook。
2. 在 **用户（Users）→ 用户 → 对应用户 → 媒介（Media）**添加收件地址、允许时段和严重性。
3. 确认该用户所属群组拥有目标主机群组的读取权限。
4. 在 **告警 → 动作（Actions）→ 触发器动作（Trigger actions）**创建并启用动作，条件限制到 `SMA servers` 等目标范围。
5. Operations 添加发送给指定用户/用户群组；Recovery operations 添加恢复消息。
6. 在实验主机触发一次可控事件，检查问题记录、动作日志及实际收件情况。

通知渠道需要凭据时通过受控配置保存，不写进本指南。通知的完整机制见[官方问题通知教程](https://www.zabbix.com/documentation/7.0/en/manual/quickstart/basic_config/notification)。

安全的告警测试方法：在独立测试主机临时把内存问题阈值改为低于当前使用率、恢复阈值设为合理范围，触发后还原原有阈值并等待恢复。不要为了测试而耗尽内存、写满磁盘或停止生产 SMA。切勿修改共享模板阈值进行单机试验而影响所有关联主机。

## 9. 查看数据与验收

在 **监测 → 最新数据**选择 `sma-node-01`，搜索 `SMA:`，查看最近检查时间、最新值和历史；数值项可打开图形。也可在仪表盘添加 Item value / Graph 小组件，选择内存使用率、根分区使用率和运行时间。

新手首次验收逐项记录：

- [ ] 已从真正采集方访问快照，状态为 200，鉴权有效。
- [ ] 主机已启用并关联模板，URL/Token 宏属于当前主机。
- [ ] 主项每 30 秒左右产生新快照，依赖项也持续更新，而非只完成一次 Test。
- [ ] 内存、根分区、运行时间、errors 的值与同一时段快照一致；百分比范围为 0～100。
- [ ] 未把缺失字段、Windows 占位 0 或累计计数器误当正常瞬时指标。
- [ ] CPU/磁盘速率已等待至少两次有效采样，并理解重启后的基线行为。
- [ ] 测试主机上的问题、恢复、通知三条记录均已核对。
- [ ] 阈值已还原，Token 没有进入导出模板或排障材料。

界面里的 ZBX 可用性图标对应 Zabbix agent 接口，不是 SMA HTTP 检查的验收标准。主项、依赖项的最新时间和错误信息更直接。归档时记录 SMA/Zabbix 版本、采集方和脱敏结果，不保存完整凭据。

## 10. 多主机复用与自动发现

### 10.1 接入第二台主机

新增主机 → 关联 `SMA by HTTP` → 填该机 `{$SMA.URL}` 和 Secret text `{$SMA.TOKEN}` → 选择正确 Server/Proxy → 从采集方验证 → 查看最新数据。无需为每台主机复制一套模板或修改每个 URL。

模板导出用于共享前，应确认没有真实 Token、内部敏感地址或测试阈值。本文不声称已经生成或验证导出模板。

### 10.2 从一个挂载点扩展到全部挂载点

知识点：LLD 用发现规则读取列表，再用监控项原型生成多条实际监控项。它与 SMA `/v1/discovery/*` 的中间件发现不是同一套协议；此处只使用快照的 filesystems 数组，不触发 SMA 发现任务。

基础接入验收后，可在模板尝试：

1. 创建 Discovery rule，名称 `SMA: Filesystem discovery`，键值 `sma.fs.discovery`，类型 `Dependent item`，主项 `sma.snapshot`。
2. 预处理 JSONPath：`$.filesystems`，输出挂载点对象数组。
3. 在 LLD macros 添加 `{#SMA.MOUNT}` → `$.mountpoint`、`{#SMA.FSTYPE}` → `$.filesystemType`。
4. 在该规则下创建 Item prototype，名称 `SMA: FS {#SMA.MOUNT} available`，键值 `sma.fs.available["{#SMA.MOUNT}"]`，类型 Dependent item，主项仍选 `sma.snapshot`，类型 Numeric (unsigned)，单位 `B`。
5. 原型预处理 JSONPath：`$.filesystems[?(@.mountpoint == "{#SMA.MOUNT}")].availableBytes.first()`。
6. 保存并等待发现结果，检查实际生成项；先在含普通 Linux 挂载点的实验主机验证，再配置过滤和丢失资源生命周期。可增加 sizeBytes 原型和使用率原型。

对临时挂载点、伪文件系统、重复或包含特殊字符的挂载点，要单独验证过滤和宏转义；不要未经验证直接批量部署。了解发现规则、原型及宏映射，见[官方自定义 LLD](https://www.zabbix.com/documentation/7.0/en/manual/discovery/low_level_discovery/custom_rules)。初学者可以继续使用第 6 节的固定挂载点方案。

## 11. 常见问题排查

| 现象 | 可能原因 | 建议检查 |
| --- | --- | --- |
| Connection refused | SMA 未运行、监听地址或端口错误 | SMA status/config show；确认内网 IP 和端口 |
| Timeout | 防火墙、路由、容器网络、Proxy 网络不通 | 在实际 Server/Proxy 环境 curl；确认源地址放行 |
| 401 Unauthorized | Token 宏错误、缺失或请求头格式错误 | 主机 Secret text 宏填原文；请求头恰好一个 `Bearer ` 前缀 |
| 404 Not Found | URL 少了或重复了路径 | 基地址不带路径；主项 URL 追加 `/v1/snapshot` |
| 503 | 并发超限或暂时无法获取快照 | 检查 SMA 日志与负载；避免重复主项和高频轮询 |
| HTTP 200 但 memory 缺失 | 采集器禁用或失败 | errors、SMA 配置和日志；不把缺失补为 0 |
| Cannot extract value / JSONPath 不匹配 | 字段缺失、挂载点/设备名错误、传入的不是正文 JSON | 查看主项实际内容和预处理测试输入 |
| Numeric 类型收到数组 | 过滤式返回集合 | 核对是否需要 `.first()`；不要盲目取 filesystems[0] |
| 所有项无数据 | 主机/主项禁用、模板未关联、宏或采集方配置错误 | 主机继承项、主项错误、Server/Proxy 状态、配置同步 |
| CPU/吞吐第一次没值 | 差分需要两次样本 | 等待下一轮；重启后重新建立基线 |
| 值停止更新但图表仍有旧数据 | 主项断流或依赖项不支持 | 最近检查时间、nodata、预处理失败；历史值不代表健康 |
| 有 Problem 无邮件 | 通知链路未完成 | 媒介类型、用户媒介、权限、动作条件、严重性、时段和动作日志 |
| ZBX 图标灰色 | 本路线没有 Zabbix agent 检查 | 用 HTTP 主项最新数据确认，不靠 ZBX 图标 |

更多 SMA 地址、监听和鉴权排查见[连通性与鉴权排障](troubleshooting.md)。收集错误时只保存脱敏状态码、时间和必要字段，不提交 Token、数据库、会话或服务器配置备份。

## 12. 最终接入步骤速查

完成指南后，可按下面顺序复现接入：

1. **准备**：Zabbix 已可登录且 Server 正常，SMA 已运行；记录实际采集方、内网 URL 和当前 Token。
2. **连通**：只在必要时安排监听变更与重启，防火墙允许 Server/Proxy 访问 SMA；从采集方验证健康和鉴权快照。
3. **模板和主机**：创建 `SMA by HTTP`，关联 `sma-node-01`，选择正确采集方，配置 URL 和秘密 Token 宏。
4. **主项**：HTTP agent 每 30 秒 GET `/v1/snapshot`，Bearer 请求头，保存正文 Text，要求 HTTP 200。
5. **指标**：Dependent item + JSONPath 提取内存/容量/运行时间/errors；JavaScript 计算同一快照中的使用率。
6. **速率**：有需求再添加 CPU 差值和磁盘 Change per second，处理首次、重启和数据缺失。
7. **告警**：配置高使用率、快照断流、采集错误和关键字段缺失触发器，设置合理恢复阈值。
8. **通知**：配置媒介、用户权限和动作，在测试主机确认异常及恢复消息送达。
9. **验收与扩展**：检查持续更新和数据口径，再关联更多主机；挂载点多时引入 LLD。

接入的完成标准是：**真实采集方持续拿到快照，指标正确入库，异常能产生问题，恢复能结束问题，配置的通知实际送达**。只创建主机或一次 curl 成功，都还需要继续验收后续环节。

## 13. 官方知识点索引

以下为本文使用的官方 7.0 文档入口；其他版本通过页面版本选择器查阅：

| 知识点 | 官方资料 |
| --- | --- |
| 安装准备 | [Zabbix 安装选择器](https://www.zabbix.com/download) |
| 主机与群组 | [New host](https://www.zabbix.com/documentation/7.0/en/manual/quickstart/basic_config/host) |
| 模板 | [Configuring a template](https://www.zabbix.com/documentation/7.0/en/manual/config/templates/template) |
| HTTP 拉取与请求头 | [HTTP agent](https://www.zabbix.com/documentation/7.0/en/manual/config/items/itemtypes/http) |
| 一次采集拆分多项 | [Dependent items](https://www.zabbix.com/documentation/7.0/en/manual/config/items/itemtypes/dependent_items) |
| Token 遮蔽 | [Secret user macros](https://www.zabbix.com/documentation/7.0/en/manual/config/macros/secret_macros) |
| JSON 取值 | [JSONPath functionality](https://www.zabbix.com/documentation/7.0/en/manual/config/items/preprocessing/jsonpath_functionality) |
| JavaScript、差值和失败处理 | [Item value preprocessing](https://www.zabbix.com/documentation/7.0/en/manual/config/items/preprocessing) |
| 由历史指标计算新指标 | [Calculated items](https://www.zabbix.com/documentation/7.0/en/manual/config/items/itemtypes/calculated) |
| 告警函数与恢复 | [Trigger expression](https://www.zabbix.com/documentation/7.0/en/manual/config/triggers/expression) |
| 告警通知 | [Receiving problem notification](https://www.zabbix.com/documentation/7.0/en/manual/quickstart/basic_config/notification) |
| 发现规则与原型 | [Custom LLD rules](https://www.zabbix.com/documentation/7.0/en/manual/discovery/low_level_discovery/custom_rules) |
