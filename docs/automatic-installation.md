# SMA 一键安装与版本目录

Linux 使用本页；Windows MSI/ZIP 与 `checksums-windows.txt` 独立，见 [Windows 指南](windows/README.md)。

## 给安装者：一条命令

在 iStoreOS/OpenWrt 的 root 终端运行（无须 `sudo`）：

```sh
curl -fL 'http://192.168.33.123:8080/web/client/pubshares/MFAWcGkHLBH2ZLquo63Mcm/browse?path=%2Finstall-sma.sh' -o install-sma.sh && sh install-sma.sh --show-token
```

在 Debian/Ubuntu 等非 root 终端，把后半段换成 `sudo sh install-sma.sh --show-token`。脚本自动读取分享根目录的 `latest.txt`，按系统和 CPU 架构选择 DEB/RPM/APK 或通用包，再从 `v版本/` 目录下载、校验、安装到 `/opt/sma` 并配置开机自启。安装日志位于 `/var/log/sma-install.log`。

需要固定版本时运行 `sh install-sma.sh --version 0.2.6 --show-token`；此时无需根目录 `latest.txt`，直接读取 `v0.2.6/`。`--force` 可重装同版本。仅支持 x86_64/amd64 和 aarch64/arm64。iStoreOS/OpenWrt 使用 procd；若缺少创建 `sma` 系统用户的命令，安装器会尝试经 `opkg` 安装 shadow 工具。请确保 `/opt` 可写、有足够空间。v0.2.5 通用包曾在部分 iStoreOS 上因缺少 `install` 命令失败；v0.2.6 已修复，直接重新运行一键命令，无需清理失败现场。

安装时生成本机独立 Bearer Token，已有 Token 保留。`--show-token` 仅在终端显示，不写入安装日志；之后可用 `/opt/sma/bin/sma-ctl token show` 查看。默认只监听 `127.0.0.1:9108`，从 sma-center 远程访问时需修改监听地址并配置 Token，见[全生命周期操作](lifecycle.md)。

## 给维护者：固定入口、版本目录

```text
分享根目录/
├── install-sma.sh             # 唯一固定安装入口，升级时也从这里获取
├── install-sma.sh.sha256      # sma-ctl upgrade 校验入口
├── latest.txt                 # 单行稳定版本号，例如 0.2.6；最后更新
├── v0.2.4/                   # 已发布版本保持不变
│   ├── checksums.txt
│   ├── sma-ctl.sh
│   └── 该版本安装包及其他发布文件
├── v0.2.5/                   # 历史版本保持不变
└── v0.2.6/
    ├── checksums.txt
    ├── sma-ctl.sh
    ├── sma-ctl.sh.sha256
    ├── sma_0.2.6_linux_x86_64.tar.gz
    ├── sma_0.2.6_linux_aarch64.tar.gz
    └── 同版本 DEB/RPM/APK 等文件
```

版本目录不可覆盖不同内容；先完整上传并回读校验，再更新根目录入口和 `latest.txt`。不要把所有版本的包混放根目录。GitHub Release 工作流会生成 `sma_版本_gofpt_share.tar.gz`，其中已经按上述结构排好文件。从源码目录执行以下命令即可下载该 Release、校验并发布到已配置的 SFTPGo 分享，最后切换 `latest.txt`：

```sh
sh scripts/deploy-share.sh 0.2.6
```

如需改用其他分享地址，设置 `SMA_SHARE_URL` 为公共分享根 URL（不要加 `/upload`、`/browse` 或 `/v版本`）。发布器使用 SFTPGo 上传接口，版本目录已有不同内容时会停止，不会覆盖；根目录入口与索引可以更新。发布后请从目标机执行上面的一键命令验收。源码本地已构建包时也可先执行 `sh scripts/prepare-share.sh 0.2.6`，再执行 `sh scripts/publish-share.sh 0.2.6`。

## 排错与安全

若下载的 `install-sma.sh` 第一行不是 `#!/bin/sh`，说明分享返回了 HTML 网页；先检查根目录固定入口和分享权限，不要执行该文件。安装器也会拒绝下载到的 HTML。检查日志：`tail -n 100 /var/log/sma-install.log`；服务状态：`/opt/sma/bin/sma-ctl status`。

当前内网分享使用 HTTP，同源 SHA-256 主要用于发现发布或传输错误，不能防止链路同时被篡改。生产环境应使用 HTTPS，并从可信渠道核对入口脚本。完整升级与卸载命令见[全生命周期操作](lifecycle.md)。
