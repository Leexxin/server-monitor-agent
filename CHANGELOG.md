# 变更记录

## Windows v0.3.0-rc.1

- Windows x64 原生指标、服务、MSI、完整验证 ZIP，附安装、诊断、Token、升级和卸载工具。
- 新增主机平台、系统盘、不支持字段，保持 v1 API；Linux 持续回归。
- Windows CI 验证，用户 Windows 10 LTSC 验收待完成，预览包未签名。

## v0.2.3

- 统一源码文档目录与维护约定，新增 sma-center 连通性排障和真实恢复案例。
- 所有包交付新增文档，Release 增加独立文档包及可追溯发布说明。
- 无 Agent API 和 Token 协议变更。

## v0.2.2

- 增加 sma-ctl 全生命周期管理和安装时自动配置 Bearer Token。
- 修复升级时 Token 文件属主被重置，以及手动恢复触发 systemd 启动频率限制的问题。
- 增加 Linux DEB 全生命周期验证。v0.2.1 发布验证未通过，未发布制品，由 v0.2.2 完成交付。

## v0.2.0

- 增加外部触发的服务发现及固定地址上报。
- 提供双架构 DEB、RPM、APK 和通用安装包。
