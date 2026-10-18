# SMA 安装与卸载

本页为 Linux 安装包；Windows MSI 和安装脚本见 [Windows 指南](windows/README.md)。

Windows v0.3.1 起提供中文安装向导、完成页和管理界面，旧版仅进度条的体验已改进；安装后通过开始菜单进入，不必执行 PowerShell。

SMA 的程序和配置统一位于 `/opt/sma`。安装和卸载必须使用 root 权限。

新安装包提供 `sma-ctl` 管理工具，并在安装时生成独立 Bearer Token。查看凭据：`sudo sma-ctl token show`。启停、自启、Token 更换、升级和卸载见[全生命周期操作说明](lifecycle.md)。旧包需重新运行更新后的自动安装脚本来补齐这些功能。

需要从内网分享目录自动选择版本与安装包时，请参见[自动安装方案](automatic-installation.md)。

## 包选择

| Linux 系统 | x86-64 | ARM64 |
|---|---|---|
| Debian、Ubuntu | `server-monitor-agent_*_linux_amd64.deb` | `server-monitor-agent_*_linux_arm64.deb` |
| RHEL、CentOS、Rocky、AlmaLinux、Fedora、SUSE | `sma-*-1.x86_64.rpm` | `sma-*-1.aarch64.rpm` |
| Alpine Linux | `sma_*_x86_64.apk` | `sma_*_aarch64.apk` |
| iStoreOS/OpenWrt（procd） | `sma_*_linux_x86_64.tar.gz` | `sma_*_linux_aarch64.tar.gz` |
| 其他 Linux | `sma_*_linux_x86_64.tar.gz` | `sma_*_linux_aarch64.tar.gz` |

## 校验

```bash
sha256sum -c checksums.txt
```

## DEB

```bash
sudo apt install ./server-monitor-agent_0.2.9_linux_amd64.deb
```

卸载并保留配置：

```bash
sudo apt remove server-monitor-agent
```

通过 SMA 工具卸载并清除 `/opt/sma`（含自动生成的 Token）：

```bash
sudo sma-ctl uninstall --purge
```

从 v0.2.9 起，DEB 包名从 `sma` 改为 `server-monitor-agent`，避免与 Ubuntu 仓库中的同名但无关的软件发生升级冲突。程序路径、`sma.service`、接口与已有 Token 不变。旧版监控代理请使用[一键安装脚本](automatic-installation.md)升级；脚本只在确认旧 `sma` 包拥有 `/opt/sma/bin/sma` 时才移除它。**不要对 Ubuntu 仓库的其他 `sma` 软件执行 `apt remove sma`。**直接执行 `apt install ./server-monitor-agent_...deb` 不负责旧包迁移，可能因文件归属冲突而失败。

## RPM

```bash
sudo dnf install ./sma-0.2.3-1.x86_64.rpm
# 老系统也可使用：sudo yum install ./sma-0.2.3-1.x86_64.rpm
```

卸载：

```bash
sudo dnf remove sma
```

## APK

本地未签名 APK 需要显式允许：

```bash
sudo apk add --allow-untrusted ./sma_0.2.3_x86_64.apk
```

卸载：

```bash
sudo apk del sma
```

## 通用包

```bash
tar -xzf sma_0.2.3_linux_x86_64.tar.gz
sudo ./install.sh
```

卸载但保留 `/opt/sma/config`：

```bash
sudo ./uninstall.sh
```

同时清除配置：

```bash
sudo ./uninstall.sh --purge
```

## 安装目录

```text
/opt/sma/
├── bin/sma
├── bin/sma-ctl
├── config/sma.env
├── config/token
├── discovery/scripts/common-services.sh
├── README.md
└── docs/
```

服务文件根据安装包类型选择，不会混装：

- DEB/RPM 只安装 systemd 单元：`/usr/lib/systemd/system/sma.service`
- APK 只安装 OpenRC 脚本：`/etc/init.d/sma`
- 通用安装器携带三种模板，但只安装当前系统实际使用的服务文件；systemd 单元安装到 `/etc/systemd/system/sma.service`，iStoreOS/OpenWrt 安装 procd 脚本到 `/etc/init.d/sma`

从 `0.1.0-1` DEB 升级到 `0.1.0-2` 或更高版本时，安装脚本会清理由旧包错误安装的 `/etc/init.d/sma`，防止 systemd 与 SysV/OpenRC 入口并存。

安装器会创建无登录权限的 `sma` 系统用户。程序以该用户运行，程序文件归 root 所有；Token 归 sma 所有且权限为 0600。原生包管理器的 purge 不保证删除运行时生成的 Token，完整清理请使用 `sma-ctl uninstall --purge`；日志、系统用户和已有备份的保留范围见生命周期文档。

## 配置和启动

### 默认监听地址

SMA 安装后默认只监听：

```text
127.0.0.1:9108
```

这是安全默认配置，表示只有本机程序可以访问 SMA。此时从其他服务器访问 `http://<服务器IP>:9108` 会连接失败，但不代表 Agent 启动异常。

可以执行以下命令确认当前监听地址：

```bash
sudo ss -lntp | grep 9108
```

### 开放远程访问

编辑 `/opt/sma/config/sma.env`：

```bash
sudo editor /opt/sma/config/sma.env
```

推荐只监听服务器的内网 IP：

```text
SMA_OPTS="--listen-address=192.168.1.10:9108"
```

如果确实需要监听所有 IPv4 网卡，可以配置：

```text
SMA_OPTS="--listen-address=0.0.0.0:9108"
```

监听 `0.0.0.0` 会使所有能够连接该服务器 9108 端口的设备都能访问 Agent。生产环境必须使用防火墙限制来源，并建议同时启用 Bearer Token；不要把未鉴权端口直接暴露到公网。

修改配置后重启：

```bash
sudo systemctl restart sma
# OpenRC：sudo rc-service sma restart
# iStoreOS/OpenWrt（已是 root）：/etc/init.d/sma restart
```

查看状态：

```bash
sudo systemctl status sma
curl --fail http://127.0.0.1:9108/healthz
curl --fail http://127.0.0.1:9108/readyz
```

如果配置了远程监听，还应从监控服务器进行验证：

```bash
curl --fail http://192.168.1.10:9108/healthz
```

远程访问前建议创建仅 Agent 用户可读的 Token 文件，并配置：

```bash
sudo install -o sma -g sma -m 0600 /path/to/token /opt/sma/config/token
```

```text
SMA_OPTS="--listen-address=0.0.0.0:9108 --web.auth-token-file=/opt/sma/config/token"
```

## 自动发现配置

自动发现默认启用。安装包提供两个探测器：

- `process`：Agent 内置的进程、监听端口和 OpenSSH SFTP 配置探测。
- `script:common-services`：位于 `/opt/sma/discovery/scripts/common-services.sh` 的服务管理器探测脚本。

查看当前能力：

```bash
SMA_TOKEN=$(sudo sma-ctl token show)
curl --fail -H "Authorization: Bearer ${SMA_TOKEN}" http://127.0.0.1:9108/v1/discovery/capabilities
```

本机触发一次发现：

```bash
curl --fail \
  -X POST \
  -H "Authorization: Bearer ${SMA_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{}' \
  http://127.0.0.1:9108/v1/discovery/runs
```

远程触发必须配置 `--web.auth-token-file`。如需把结果自动上报给 CMDB 或资产平台，在 `/opt/sma/config/sma.env` 中添加：

```text
--discovery.report-url=https://cmdb.example.com/api/v1/sma/discovery
--discovery.report-token-file=/opt/sma/config/report-token
```

上报地址由服务器管理员固定配置，外部触发请求不能传入回调地址、脚本路径或命令。详细接口参见 `/opt/sma/docs/discovery-api-integration.md`。
