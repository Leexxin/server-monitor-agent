# sma-center 无法获取 SMA 数据的排查

## 先检查真正使用的 Agent 地址

服务器卡片显示的主机 IP、资源链接和 **SMA Agent 地址** 是不同字段。卡片显示正确 IP，不代表后端请求的 Agent 地址正确。

在 sma-center 编辑服务器资源时核对：

1. Agent 地址包含协议、主机和端口，例如 `http://192.168.33.124:9108`。
2. 逐段检查 IP，特别是 `102` 与 `192` 等笔误。
3. 地址是基础地址，不带 `/v1/snapshot`，应用会自动追加接口路径。
4. Token 输入凭据原文，不附加 `Bearer ` 前缀，也不要粘贴安装日志或文件路径。
5. 保存后刷新，等待指标请求完成。CPU 使用率通常需要相邻两次采样才能计算。

## 按响应判断故障

| 现象 | 含义与下一步 |
| --- | --- |
| 连接超时 | 检查实际 Agent 地址、路由、防火墙；从 sma-center 容器或后端进程所在环境测试 |
| Connection refused | 目标可到达，但该端口未监听或被主动拒绝；检查服务和监听地址 |
| `/healthz` 200 | HTTP 服务可达，不代表凭据正确或全部采集器正常 |
| `/readyz` 503 | 尚未完成就绪条件；等待后查看 Agent 日志 |
| `/v1/snapshot` 401 | 凭据缺失或无效；核对已保存 Token，换 Token 后需更新调用方 |
| 远程发现触发 403 | 检查是否启用了 Bearer 鉴权；远程发现触发要求鉴权 |
| 指标接口 200，但有 `errors` | 已成功连接；按 `collector`、`code`、`message` 单独排查采集器 |
| `credential_key_unavailable` | sma-center 无法使用本地加密密钥；检查其持久数据和恢复流程，勿直接重置密钥 |

管理页面的“运行中”可能是资源状态标记，不能代替 Agent 连通性、采集时间和鉴权结果。

## Ubuntu 更新后代理被同名软件替换

Ubuntu/Debian 的旧版监控代理 DEB 包名为 `sma`，与发行版中的无关软件重名。先检查包身份，不要直接卸载或 hold 所有名为 `sma` 的包：

```sh
dpkg-query -W -f='${binary:Package} ${Version} ${Status}\n' sma server-monitor-agent 2>/dev/null || true
dpkg -S /opt/sma/bin/sma 2>/dev/null || true
```

v0.2.9 起使用 `server-monitor-agent` 包名。按[一键安装](automatic-installation.md)重新安装后，运行 `sudo sma-ctl doctor` 并检查监听地址。脚本仅移除拥有 `/opt/sma/bin/sma` 的旧监控代理包，不卸载无关的 `sma` 软件。原配置和 Token 存在时会保留；如果已丢失，请从备份恢复并更新 sma-center 中的连接信息。

## 在 Agent 主机检查

```sh
sudo sma-ctl status
sudo sma-ctl doctor
sudo sma-ctl config show
sudo sma-ctl logs 100
sudo ss -lntp | grep 9108
```

若只监听 `127.0.0.1:9108`，外部后端无法访问。需要远程访问时，由管理员改为服务器实际内网 IP，并允许监控平台访问：

```sh
sudo sma-ctl config listen 192.168.33.124:9108
```

## 从调用方环境检查

```sh
curl --connect-timeout 3 --max-time 8 -i http://192.168.33.124:9108/healthz
curl --connect-timeout 3 --max-time 8 -i http://192.168.33.124:9108/readyz
curl --connect-timeout 3 --max-time 8 -i http://192.168.33.124:9108/version
```

若 sma-center 运行于 Docker，需从该容器内部测试，宿主机或个人电脑成功访问不能替代容器网络验证。容器内没有 curl 时可使用自带 Node.js：

```sh
docker exec sma-center node -e 'fetch("http://192.168.33.124:9108/healthz", {signal: AbortSignal.timeout(5000)}).then(r => console.log(r.status)).catch(e => { console.error(e.name, e.cause?.code); process.exitCode=1 })'
```

鉴权测试使用已有 Token。不要为了连通性排障先更换 Token，否则其他客户端也需要更新。以下命令只在 Agent 本机适用；若修改过监听地址，URL 需使用实际地址：

```sh
SMA_TOKEN=$(sudo sma-ctl token show)
curl --fail --max-time 8 -H "Authorization: Bearer ${SMA_TOKEN}" \
  http://192.168.33.124:9108/v1/snapshot
unset SMA_TOKEN
```

不要把真实 Token 放进 Git、截图、工单或 Release；不要通过公网/可公开上传下载的分享目录传递凭据。

## 2026-10-15 排障记录（北京时间）

- 症状：sma-center 卡片显示 `192.168.33.124` 和“运行中”，指标为空，提示“无法连接 SMA Agent”。
- Agent：`192.168.33.124:9108`，版本 `v0.2.2`；服务 active，健康及就绪接口均为 200。
- 根因：sma-center 实际保存的 Agent 地址为 `http://102.168.33.124:9108`，第一段 IP 填错；容器请求该错误地址超时。
- 凭据验证：在容器内仅于内存中使用已保存凭据，请求正确地址返回 200、schemaVersion v1，CPU、内存、负载和文件系统字段均存在；没有导出凭据。
- 部分采集状态：该次快照包含 1 条采集器错误。尚未进一步确认其内容；它与错误 IP 导致的连接超时属于不同问题，不据此断言全部采集器正常。
- 恢复：用户在界面修正配置后确认数据恢复。此次排查没有替换 Token，也没有修改生产数据库或重启生产服务。

## 建议的后续改进（尚未实现）

1. sma-center 保存配置前提供“测试连接”，在后端验证地址、版本和鉴权，并显示明确的超时/401/采集器错误。
2. 将资源状态与 Agent 状态分开显示，增加最近成功采集时间，避免“运行中”被理解为监控正常。
3. 失败提示显示请求的协议、主机和端口（不带凭据），帮助发现 IP 笔误；连续失败采用退避重试。
4. 为目标发行版和 ARM64 增加安装升级测试；目前实际自动化安装卸载验证主要覆盖 Ubuntu AMD64 DEB。
5. 制品下载改用 HTTPS 和只读入口；升级前备份配置，并记录版本、提交号、架构及校验结果。
## iStoreOS 一键安装报 `install: not found`

这是 v0.2.5 通用包安装脚本依赖 iStoreOS 可能未提供的 `install` 命令，并非下载或 SHA-256 校验失败。使用 v0.2.6 或更新版本的分享根目录 `install-sma.sh` 重新运行原一键命令；无需删除 `/opt/sma`。若仍失败，提交 `/var/log/sma-install.log` 的末尾日志（先删除 Token 等敏感值），并确认输出中的目标版本为 0.2.6 或更新。

## iStoreOS 一键安装报 `od: not found`

这是 v0.2.6 在创建 Bearer Token 时依赖目标系统未提供的 `od` 命令。使用 v0.2.7 或更新版本的分享根目录固定安装入口重试；该版本改用安装器已有的 `sha256sum`，不需要手动清理或重置 Token。若再次失败，提供脱敏安装日志，并检查日志中 `target=0.2.7` 或更新。
