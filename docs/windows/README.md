# Windows SMA：安装、对接与验收

首版 `v0.3.0-rc.1`，x64 预览版，交付 MSI 和包含 MSI、程序、脚本、文档的 ZIP。Mac 开发，Windows CI 构建及验证，用户机器实际验收。

## 范围和验证边界

目标验收机：Windows 10 Enterprise LTSC 21H2，19044，x64，约 4 GB 内存、6 个逻辑 CPU，`192.168.33.154`。此记录不代表已经验收。Windows 10、11、Server 2019/2022/2025 需分别验证；安装最低检查 build 17763，不构成全版本兼容承诺。

提供 CPU 汇总时间、逻辑 CPU 数、物理内存、固定盘符卷容量、卷 I/O、运行时间。直接调用 Windows API，无采集子进程及额外常驻运行时。首版 CPU 最多 64 个逻辑 CPU；无盘符卷、文件夹挂载卷、网络盘和可移动盘暂不枚举。自动发现禁用，显式启用报错。

## 安装和接入

下载 ZIP 和发布的 `checksums-windows.txt`，通过 `Get-FileHash .\sma_0.3.0-rc.1_windows_amd64.zip -Algorithm SHA256` 核对发布值。解压后，以管理员身份打开 64 位 PowerShell，进入解压目录：

```powershell
Set-ExecutionPolicy -Scope Process Bypass -Force
.\install-sma.ps1 -ListenAddress '0.0.0.0:9108' -AllowedSource '192.168.33.124' -ShowToken
```

示例防火墙只允许 sma-center 的 `192.168.33.124`，实际来源不同时替换。脚本识别已安装版本，执行 MSI，检查服务、自启及带鉴权快照，打印配置、日志、Token 获取方式。重复安装和升级保留配置、Token。Token 仅通过 `-ShowToken` 或显式查看输出，不写入安装日志。

直接双击 MSI 也能安装，但首次只监听 `127.0.0.1:9108`，不自动开放防火墙。远程使用需修改监听并限定防火墙来源；既有更宽泛的规则不由 SMA 接管。

sma-center 中配置 Agent URL `http://192.168.33.154:9108`，Token 填原始值，不带 `Bearer `。保存后等待两轮采样。

## 目录与服务

| 内容 | 默认路径 |
| --- | --- |
| 程序、工具、脚本、文档 | `C:\Program Files\SMA\` |
| 配置 | `C:\ProgramData\SMA\config.json` |
| Token | `C:\ProgramData\SMA\token` |
| 运行日志 | `C:\ProgramData\SMA\logs\sma.log`，10 MiB 轮转，另保留一份 |
| 安装、卸载日志 | 当前管理员 `%TEMP%\sma-install-*.log*`、`sma-uninstall-*.log` |

路径按系统环境变量解析。服务名 `SMA`，自动启动、LocalSystem、失败恢复。LocalSystem 用于首版卷 I/O，暂未承诺低权限账户支持。配置、Token 和运行日志仅允许 SYSTEM 与提升权限的 Administrators 访问。

## 生命周期操作

```powershell
$ctl = "$env:ProgramFiles\SMA\sma-ctl.exe"
& $ctl help
& $ctl status
& $ctl doctor
& $ctl logs
& $ctl version
& $ctl config show
& $ctl config listen '0.0.0.0:9108'
& $ctl token show
& $ctl token reset --yes
& $ctl stop
& $ctl start
& $ctl restart
& $ctl disable
& $ctl enable
```

管理操作需提升权限。监听修改失败尝试恢复旧配置并重启；Token 轮换失败尝试恢复旧 Token。恢复失败会返回合并错误，需查日志处理。`config listen` 不修改防火墙；更换端口后再用安装脚本 `-AllowedSource` 更新规则。`doctor` 检查配置、Token 权限、就绪和鉴权，局部采集错误打印 WARNING。诊断 HTTP 禁用代理和重定向。

升级运行新 ZIP 的安装脚本；固定 MSI UpgradeCode、禁止降级、使用 Windows Installer 事务。升级前安全备份旧 MSI 和配置。预览包 MSI 三段版本为 `0.3.0`，下一次可升级制品必须提高三段版本，不能用相同 MSI 版本发布不同包来模拟升级。

在解压目录卸载，避免脚本执行中被移除：

```powershell
.\uninstall-sma.ps1
# 无人值守卸载，保留配置、Token、运行日志
.\uninstall-sma.ps1 -Yes
# 明确删除配置、Token、运行日志，不可恢复，先备份
.\uninstall-sma.ps1 -Purge -Yes
```

Windows“应用和功能”卸载同样默认保留数据。脚本额外删除自己的 `SMA-Agent-Inbound` 防火墙规则；从系统界面卸载需自行删除该规则。`-Purge` 仅删除默认 SMA 数据目录，不删除自定义 Token、导出文件或 `%TEMP%` 安装日志。保留卸载后重装复用原配置与 Token。

## 验收

安装后从解压目录运行 `.\test-windows.ps1`。默认模式只读取状态、鉴权采样两次，验证 CPU 累计时间、内存、容量、运行时间和 Prometheus，不打印 Token。反馈输出并在任务管理器对照；CPU 采样窗口不同，无需瞬时一致。人工验证重启后自启及 sma-center 恢复、调用方网络连通、内存约 4 GB。虚拟机指标为来宾资源。卷 I/O 依赖驱动，错误在 `errors` 返回。

**仅干净临时机/CI**：设置 `SMA_RUN_WINDOWS_INSTALL_TESTS=1` 后运行 `test-windows.ps1 -Lifecycle -MsiPath <路径>`，执行安装、重装、启停、自启配置、配置回退、Token 轮换、保留卸载、重装和清理，会删除测试配置。它拒绝已有安装，不用于生产检查。

## 接口对接

沿用 [指标接口](../api-integration.md) 的路径、状态码、Bearer Token、`schemaVersion=v1`。`/healthz`、`/readyz`、`/version` 无鉴权，`/v1/snapshot`、`/metrics` 使用 Token。首版服务配置开放监听与 Token 路径；TLS 可通过受控反向代理实现。

新增 `host.os=windows`、`host.architecture=amd64`、`host.systemMountpoint`（例如 `C:\`）、`host.unsupportedMetrics`。Linux 也返回平台和系统盘信息。

CPU `seconds` 为 `{"total":{"idle":123,"system":20,"user":30}}`，是全机累计秒数。对所有键求和，不硬编码 `cpu0`；两次计算 `100 × (1 - Δidle / Δtotal)`，首次、计数器下降或差值无效时显示“—”，不再乘逻辑 CPU 数。内存使用率 `(totalBytes - availableBytes) / totalBytes`。系统盘匹配 `host.systemMountpoint`，不能仅匹配 `/` 或假定数组第一项。

Windows 不提供 Linux load average，`load` 缺失，应显示“—/不支持”。未采集物理空闲、缓存、Buffers、Linux swap、inode、Linux 磁盘 busy/weighted 时间。为兼容旧 v1 数字结构，相关 JSON 字段可能为 0，**必须根据 `host.unsupportedMetrics` 视为不可用，不能当成测量零值**；Prometheus 不输出这些不支持序列。不可把 Windows commit limit 当成 swap。

磁盘 `device` 为 `C:` 等盘符卷，I/O 字节、操作数、读写时间为累计量，速率两次作差；操作数为驱动 32 位计数器，下降时重置基线。卷不等同物理盘，不能重复相加。`errors` 可与有效部分数据同时返回。

当前 sma-center CPU 求和及文件系统首项回退可复用基础对接；系统盘字段透传和“根分区”标签改为“系统盘”属后续 sma-center 适配，本次未改该仓库。

## 构建和发布

Mac：`make check`、`CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`。Windows：Go 1.23、x/sys v0.30.0、WiX 3.14，运行 `scripts/build-windows.ps1`。代码和文档同一 PR，制品不进 Git。

Windows 工作流执行原生测试、PowerShell 5.1 语法检查、MSI 构建、Windows Server 2022 runner 生命周期测试。`windows-v*` 标签发布预览包；Linux 发布和稳定版索引独立。CI 验证自启配置，不重启 runner。用户 LTSC、Windows 11 和其他 Server 版本待分别验证。

制品未做 Authenticode 签名，可能提示未知发布者。发布范围见 [预览版说明](../releases/windows-v0.3.0-rc.1.md)。
