# SMA Windows v0.3.0-rc.1

Windows x64 首个验证版，交付 MSI、完整 ZIP、SHA-256 清单。

- 原生 CPU 汇总、内存、固定盘符卷容量、卷 I/O、运行时间，复用 JSON/Prometheus。
- 原生服务、管理员安装、自启、恢复、Token 管理、诊断、配置失败回退、升级与卸载。
- 平台、系统盘和不支持指标标识；不启用 Windows load 或自动发现。

操作与接口见 [Windows 指南](../windows/README.md)。CPU 最多 64 个逻辑 CPU，卷 I/O 依赖驱动。

制品门禁包括 Linux 回归、双架构编译、Windows Server 2022 runner 原生与 MSI 生命周期测试，以对应 Actions 成功记录为准。自启配置自动验证，真实重启未自动测试。

Windows 10 LTSC 19044（192.168.33.154）待用户验收，Windows 11、其他 Server 版本待分别验证。预览版未签名。Linux 稳定版索引不更新。
