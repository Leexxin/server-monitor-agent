# SMA — Server Monitor Agent

SMA 是一个面向 Linux 的轻量级服务器监控 Agent。它直接读取 `/proc` 和 `statfs(2)`，通过 Prometheus 文本接口和 JSON 快照接口提供 CPU、内存、负载、运行时间、文件系统及磁盘 I/O 指标。

## 快速开始

```bash
go test ./...
mkdir -p dist
go build -trimpath -o dist/sma ./cmd/sma
./dist/sma
```

默认监听 `127.0.0.1:9108`：

```bash
curl http://127.0.0.1:9108/healthz
curl http://127.0.0.1:9108/v1/snapshot
curl http://127.0.0.1:9108/metrics
```

生产环境远程访问示例：

```bash
./sma \
  --listen-address=0.0.0.0:9108 \
  --web.auth-token-file=/etc/sma/token
```

`/metrics` 和 `/v1/snapshot` 会要求 Bearer Token；`/healthz`、`/readyz` 和 `/version` 保持无鉴权，便于基础设施探测。

## 文档

- [开发设计](docs/server-monitor-agent-development.md)
- [接口对接](docs/api-integration.md)
- [安装与卸载](docs/installation.md)

## 发布与制品

源码由 Git 管理，`dist/` 不进入源码提交。DEB、RPM、APK、通用安装包和 SHA-256 校验文件通过 GitHub Releases 发布。推送 `v*` 标签会触发 CI 自动测试、跨架构构建并上传全部制品。

## 支持范围

- 运行平台：Linux
- 架构目标：amd64、arm64
- Go：1.23 或更高版本
- 部署方式：优先使用 systemd，也可运行于容器
