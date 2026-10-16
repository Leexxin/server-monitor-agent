# SMA 安装、使用、维护与卸载

本页为 Linux 操作；Windows MSI 和管理命令见 [Windows 全生命周期](windows/README.md#生命周期操作)。

Windows 新版也可直接从开始菜单“SMA 管理中心”执行上述管理，无需命令行；升级和卸载有独立结果窗口。

从本次更新起，自动安装和新构建的标准包提供 `sma-ctl`。命令需要 root 权限；直接执行 `sma-ctl` 查看帮助。若 PATH 不包含 `/usr/local/bin`，使用完整路径 `/opt/sma/bin/sma-ctl`。

## 1. 安装及旧安装补齐

```sh
curl -fL 'http://192.168.33.123:8080/web/client/pubshares/MFAWcGkHLBH2ZLquo63Mcm/browse?path=%2Finstall-sma.sh' -o install-sma.sh
sudo sh install-sma.sh --show-token
```

脚本自动选发行版、架构和版本，校验下载文件，安装到 `/opt/sma`，配置鉴权并启动服务、启用自启。同版本重新运行也会补齐管理命令和 Token，不需要卸载。已有 Token 保留，避免客户端凭据失效。

全新安装生成 32 字节随机 Token（64 位十六进制文本），存于 `/opt/sma/config/token`，属主 `sma:sma`、权限 `0600`，并把 `--web.auth-token-file` 写入 `sma.env`。自定义 Token 路径会被保留；缺失的自定义文件需要管理员恢复。

`--show-token` 在安装成功后把 Token 打印到标准输出，不写入安装器自己的日志，也不上传至共享目录。调用方如果录制终端或捕获 stdout，应当保护该记录。不加该选项时，安装结果显示查看 Token 的命令。

旧版本自动安装没有生成 Token，因此不存在通用的默认密码。更新脚本后重新运行即可补齐。

iStoreOS/OpenWrt 已登录 root 时不需要 `sudo`：运行 `sh install-sma.sh --show-token`，之后使用 `/opt/sma/bin/sma-ctl status`、`token show`、`logs 100`、`uninstall`。procd 服务为 `/etc/init.d/sma`，日志通过 `logread` 查看；`sma-ctl logs` 会显示最近的系统日志（不只 SMA）。请先确认 `/opt` 可写、有足够空间、CPU 为 x86_64 或 aarch64；不支持 32 位、MIPS 等架构。通用包依赖 `sma` 系统用户，缺少账户管理命令时安装器会尝试通过 `opkg` 安装对应 shadow 工具，离线环境需预先准备。

## 2. 验收及外部接入

```sh
sudo sma-ctl status
sudo sma-ctl doctor
sudo sma-ctl token show
curl -f http://127.0.0.1:9108/healthz
```

默认监听 `127.0.0.1:9108`。如需从监控平台访问，改为服务器内网 IP（示例）：

```sh
sudo sma-ctl config listen 192.168.33.100:9108
```

该操作自动确保鉴权已配置，保留其他参数，重启服务；新配置无法启动时恢复原配置。它不修改主机防火墙，需由管理员允许监控平台访问相应端口。

```sh
SMA_TOKEN=$(sudo sma-ctl token show)
curl -f -H "Authorization: Bearer ${SMA_TOKEN}" http://127.0.0.1:9108/v1/snapshot
unset SMA_TOKEN
```

如果更改了监听 IP，上述 URL 也要相应更改。`/healthz`、`/readyz` 和 `/version` 不要求 Token；指标和发现接口要求 Token。平台传入的值是 Token 原文，请求头为 `Authorization: Bearer <token>`。

## 3. 常用维护命令

| 操作 | 命令 |
|---|---|
| 状态和开机自启状态 | `sudo sma-ctl status` |
| 基本诊断（程序、配置、Token 权限、服务状态） | `sudo sma-ctl doctor` |
| 查看配置摘要，不显示 Token 原文 | `sudo sma-ctl config show` |
| 启动 / 停止 / 重启 | `sudo sma-ctl start` / `stop` / `restart` |
| 启用 / 禁用开机自启 | `sudo sma-ctl enable` / `disable` |
| 最近 200 行服务日志 | `sudo sma-ctl logs 200` |
| 查看安装日志 | `sudo tail -n 100 /var/log/sma-install.log` |
| 显示当前 Token | `sudo sma-ctl token show` |
| 补齐鉴权并重启 | `sudo sma-ctl token ensure` |
| 确认后更换 Token | `sudo sma-ctl token reset` |
| 非交互更换 Token | `sudo sma-ctl token reset --yes` |

停止服务不关闭开机自启；关闭开机自启也不停止当前服务。需要同时执行时，运行 `stop` 和 `disable`。

Token 更换后客户端必须更新凭据。重启失败时工具恢复旧 Token 并尝试恢复服务。用户手工修改 Token 文件后同样需要重启。升级不应改变已有 Token；新安装脚本还会修正旧包把 Token 属主变成 root 的问题。

OpenRC 新包把服务输出写入 `/var/log/sma.log`，systemd 使用 journal。旧版 OpenRC 包可能没有服务日志文件，升级后生效。

procd 将标准输出和错误送入 OpenWrt 系统日志，使用 `logread` 排查。OpenWrt 设备需要从 sma-center 远程读取时，仍需将默认 `127.0.0.1:9108` 改为设备内网地址，并放行对应来源；Token 不会自动写回 sma-center，需由管理员通过 `sma-ctl token show` 获取并配置到平台。

## 4. 升级及恢复

```sh
sudo sma-ctl upgrade
# 指定版本（须仍在共享目录的 checksums.txt 中）：
sudo sma-ctl upgrade --version 0.2.2
```

升级沿用配置和 Token。安装包管理器错误、下载失败、哈希不匹配或启动失败均返回非零退出码；失败日志保存在 `/var/log/sma-install.log`。下载失败不会开始安装该包。

修改监听地址和更换 Token 支持操作级恢复；**包升级没有自动回退整包的功能**。若升级失败，查看日志并修复配置或重装对应版本，必要时使用原生包管理器安装已校验的旧版本包。正式升级前可手工备份配置：

```sh
sudo install -d -m 0700 /var/backups/sma
sudo sh -c 'umask 077; tar -C /opt/sma -czf /var/backups/sma/config-before-upgrade.tar.gz config'
```

恢复配置后应确保 Token 属主为 `sma:sma`、权限为 `0600`，运行 `sudo sma-ctl token ensure` 和 `sudo sma-ctl doctor`。

## 5. 卸载与重新安装

```sh
sudo sma-ctl uninstall                 # 交互确认，保留配置
sudo sma-ctl uninstall --yes           # 非交互，保留配置
sudo sma-ctl uninstall --purge         # 交互确认，删除 /opt/sma
sudo sma-ctl uninstall --purge --yes    # 非交互，删除 /opt/sma
```

工具停止服务、关闭自启，按实际安装方式调用 DEB/RPM/APK 包管理器或清理通用安装文件，移除管理命令链接。默认在 `/var/backups/sma/uninstall.*/config` 备份配置，并保留 `/opt/sma/config`；重新安装可继续使用原 Token。

`--purge` 删除 `/opt/sma` 中的配置、Token 和程序，无法从该目录恢复；它不删除系统 `sma` 用户、日志、先前创建的备份或存放在 `/opt/sma` 外的自定义 Token。若需要凭据彻底销毁，请另行处理这些明确位置。

## 6. 管理员发布更新

共享目录除安装包、`latest.txt` 和 `checksums.txt` 外，还要提供 `install-sma.sh`、`install-sma.sh.sha256`、`sma-ctl.sh`、`sma-ctl.sh.sha256`。构建脚本自动生成这些文件。

先上传包及脚本，再上传对应校验文件，最后切换 `latest.txt`；发布窗口发生版本索引不一致时安装会停止，重试即可。HTTP 同源校验用于发现传输和发布错误，生产建议使用 HTTPS 和独立只读下载入口。
