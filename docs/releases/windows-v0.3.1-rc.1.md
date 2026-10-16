# SMA Windows v0.3.1-rc.1

Windows x64 GUI 预览版，MSI 三段版本 0.3.1，可从 0.3.0 原地升级。完整操作见 [Windows 指南](../windows/README.md)。

- 中文安装向导、明确完成页面、安装后打开管理中心、开始菜单入口、自动安装日志。
- 原生 Windows Forms 管理中心：状态、版本、自启、指标、Agent URL；连接与限定来源防火墙；Token 默认隐藏、定时隐藏和按归属清理剪贴板；服务、日志、诊断报告、升级和卸载。
- 维护时使用临时独立进程，避免 GUI 自身锁住安装文件。用户选择新版 MSI，验证 SMA ProductName/UpgradeCode 后执行；卸载和重置需确认，默认保留凭据。
- Go 服务与 HTTP 指标协议保持兼容，关闭 GUI 不停止采集；GUI 依赖 .NET Framework 4.7.2+，不添加 GUI 常驻进程。
- ZIP 路径统一为 `/`、校验清单使用 LF，方便跨平台核验。

发布门禁：Linux 回归、Windows 原生指标及 MSI 生命周期，从上一版真实 MSI 升级保留 Token，开始菜单入口、连接/防火墙变更，GUI 加载真实快照与四页渲染截图、凭据默认隐藏及原生 MSI 包识别。具体通过结果以对应 Actions 为准。

Windows 10 LTSC 19044 的上一版采集和远程接入已由用户验证；本次 GUI 实机、交互式 MSI 完成页、不同 DPI、真实重启及其他 Windows 版本仍需验收。CI 不代替人工体验测试。预览制品未做 Authenticode 签名。
