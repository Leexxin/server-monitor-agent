# SMA 自动安装方案

本页的 `latest.txt` 和脚本仅服务 Linux；Windows 预览使用独立 MSI/ZIP、`checksums-windows.txt`，不覆盖 Linux 稳定索引，操作见 [Windows 指南](windows/README.md)。

Windows 交互安装优先 MSI 与 GUI；无人值守继续使用带参数的 PowerShell 或 `/qn`，静默安装不自动弹出 GUI。

## 目录约定

在 SFTPGo 公共分享根目录放置以下文件：

```text
install-sma.sh
install-sma.sh.sha256
sma-ctl.sh
sma-ctl.sh.sha256
latest.txt                    # 单行版本号，例如 0.2.0
checksums.txt                 # 该版本所有包的 SHA-256，格式同 sha256sum
sma_0.2.0_linux_amd64.deb
sma_0.2.0_linux_arm64.deb
sma-0.2.0-1.x86_64.rpm
sma-0.2.0-1.aarch64.rpm
sma_0.2.0_x86_64.apk
sma_0.2.0_aarch64.apk
sma_0.2.0_linux_x86_64.tar.gz
sma_0.2.0_linux_aarch64.tar.gz
```

脚本读取 `latest.txt` 决定目标版本，再用 `checksums.txt` 找到适合当前系统与架构的文件名。新版本发布时，先上传所有包和新的 `checksums.txt`，最后更新 `latest.txt`。安装过程会验证 SHA-256，失败即停止。

## 在 Linux 目标机运行

```bash
curl -fL 'http://192.168.33.123:8080/web/client/pubshares/MFAWcGkHLBH2ZLquo63Mcm/browse?path=%2Finstall-sma.sh' -o install-sma.sh
sudo sh install-sma.sh
```

可选：`sudo sh install-sma.sh --version 0.2.0` 指定版本；`--force` 强制重装同版本。只有 root 可以执行实际安装。日志同步打印到终端并写入 `/var/log/sma-install.log`。临时下载文件安装结束后自动删除。

脚本自动识别 Debian/Ubuntu、RPM 系发行版、Alpine，以及其他 Linux（通用包），支持 x86_64/amd64 与 aarch64/arm64。安装完成后，它会启用并启动 systemd、OpenRC 或 procd 服务，检查运行和开机自启状态。未检测到这些服务管理器时会停止。

iStoreOS/OpenWrt 使用 procd，自动安装器会选用相应架构的 Linux 通用包，安装 `/etc/init.d/sma`，启用开机自启并检查服务状态。在这些系统上通常已直接登录 root，且没有 `sudo`，请运行 `sh install-sma.sh --show-token`。如系统缺少创建 `sma` 用户所需的命令，通用安装器会尝试通过 `opkg` 安装 `shadow-groupadd`、`shadow-useradd`；离线或无可用软件源时需先备好这些工具。确认 `/opt` 所在文件系统可写且空间足够；当前只提供 x86_64 和 aarch64，其他路由器架构不支持。

安装时自动生成并启用本机独立 Bearer Token，保留已有 Token。使用 `--show-token` 在成功后显示凭据，或执行 `sudo sma-ctl token show` 查看；Token 不写入安装日志。默认配置仍只监听 `127.0.0.1:9108`。完整运维命令见[全生命周期操作说明](lifecycle.md)。

## 检查与排错

```bash
sudo tail -n 100 /var/log/sma-install.log
systemctl status sma.service      # systemd
rc-service sma status             # OpenRC
/etc/init.d/sma status             # iStoreOS/OpenWrt procd
curl -f http://127.0.0.1:9108/healthz
```

分享地址目前是内网 HTTP，脚本和校验文件均经同一 HTTP 服务下载。该 SHA-256 只能发现损坏或上传错误，不能防止链路被同时篡改。生产部署应为 SFTPGo 启用 HTTPS，并通过可信渠道分发或校验安装脚本。
