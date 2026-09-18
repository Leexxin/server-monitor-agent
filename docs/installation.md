# SMA 安装与卸载

SMA 的程序和配置统一位于 `/opt/sma`。安装和卸载必须使用 root 权限。

## 包选择

| Linux 系统 | x86-64 | ARM64 |
|---|---|---|
| Debian、Ubuntu | `sma_*_linux_amd64.deb` | `sma_*_linux_arm64.deb` |
| RHEL、CentOS、Rocky、AlmaLinux、Fedora、SUSE | `sma-*-1.x86_64.rpm` | `sma-*-1.aarch64.rpm` |
| Alpine Linux | `sma_*_x86_64.apk` | `sma_*_aarch64.apk` |
| 其他 Linux | `sma_*_linux_x86_64.tar.gz` | `sma_*_linux_aarch64.tar.gz` |

## 校验

```bash
sha256sum -c checksums.txt
```

## DEB

```bash
sudo apt install ./sma_0.1.0_linux_amd64.deb
```

卸载并保留配置：

```bash
sudo apt remove sma
```

彻底清除配置：

```bash
sudo apt purge sma
```

## RPM

```bash
sudo dnf install ./sma-0.1.0-1.x86_64.rpm
# 老系统也可使用：sudo yum install ./sma-0.1.0-1.x86_64.rpm
```

卸载：

```bash
sudo dnf remove sma
```

## APK

本地未签名 APK 需要显式允许：

```bash
sudo apk add --allow-untrusted ./sma_0.1.0_x86_64.apk
```

卸载：

```bash
sudo apk del sma
```

## 通用包

```bash
tar -xzf sma_0.1.0_linux_x86_64.tar.gz
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
├── config/sma.env
├── README.md
└── docs/
```

服务文件根据安装包类型选择，不会混装：

- DEB/RPM 只安装 systemd 单元：`/usr/lib/systemd/system/sma.service`
- APK 只安装 OpenRC 脚本：`/etc/init.d/sma`
- 通用安装器同时携带两种模板，但只会安装当前系统实际使用的服务文件；systemd 单元安装到 `/etc/systemd/system/sma.service`

从 `0.1.0-1` DEB 升级到 `0.1.0-2` 或更高版本时，安装脚本会清理由旧包错误安装的 `/etc/init.d/sma`，防止 systemd 与 SysV/OpenRC 入口并存。

安装器会创建无登录权限的 `sma` 系统用户。程序以该用户运行，安装内容归 root 所有。

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
