# SMA 文档目录

本目录随源码提交和发布标签维护。服务器上安装包附带的文档位于 `/opt/sma/docs/`，也可从 Release 下载独立文档包。

| 使用场景 | 文档 |
| --- | --- |
| Windows MSI、接口、完整生命周期及验收 | [Windows 指南](windows/README.md) |
| Windows 首版验证范围 | [Windows v0.3.0-rc.1](releases/windows-v0.3.0-rc.1.md) |
| 安装、Token、启停、自启、升级、卸载 | [全生命周期操作](lifecycle.md) |
| 从 GOFTP 自动选择版本和架构 | [自动安装](automatic-installation.md) |
| 原生 DEB/RPM/APK、通用包与监听配置 | [安装与卸载](installation.md) |
| sma-center 配置后无法获取数据 | [连通性与鉴权排障](troubleshooting.md) |
| CPU、内存、磁盘接口与鉴权 | [指标接口对接](api-integration.md) |
| 中间件、数据库、SFTP 自动发现与上报 | [自动发现接口对接](discovery-api-integration.md) |
| 架构和开发设计 | [开发设计](server-monitor-agent-development.md) |
| 修改、验证、Git 提交与发布规范 | [文档维护与发布](maintenance.md) |
| 当前版本交付范围及验证边界 | [v0.2.3 发布说明](releases/v0.2.3.md) |

首次使用建议从全生命周期操作开始；遇到面板无数据时，先按照排障文档从调用方环境验证地址和鉴权。
