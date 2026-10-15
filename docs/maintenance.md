# 文档维护与发布约定

`docs/README.md` 是统一入口。文档是源码交付的一部分，必须随 Git 提交、PR 和发布标签维护，不能只保存在聊天或临时分享目录。

## 变更与文档对应关系

| 改动 | 同一提交必须检查 |
| --- | --- |
| 指标、鉴权、JSON schema | api-integration.md |
| 自动发现、脚本与上报 | discovery-api-integration.md |
| 安装、升级、Token、服务、自启、卸载 | lifecycle.md、installation.md、automatic-installation.md |
| 连接问题、诊断或生产排障结论 | troubleshooting.md |
| 架构和资源开销 | server-monitor-agent-development.md |
| 新增或移动文档 | README.md、docs/README.md、打包清单及通用安装脚本 |
| 版本发布和兼容性变化 | 根目录 CHANGELOG.md、docs/releases/vX.Y.Z.md |

不把“必须同步文档”描述成已自动证明文档正确。维护者需核对链接、路径、命令、默认值和真实测试结果。

## 提交与发布流程

Windows 预览使用 `windows-v*` 标签、独立 Windows 工作流与校验清单，避免改变 Linux `latest.txt`。Windows 文档位于 `docs/windows/`；MSI/ZIP 包含完整 docs，Linux 原生及通用包也同步文档。Windows Server 2022 CI 不能代替 Windows 10/11/其他 Server 用户验证，真实重启需人工验收。

1. 检查工作区、同步远端，保护已有修改；按 CONTRIBUTING.md 创建分支和 PR。
2. 在同一提交维护功能说明和操作文档，示例使用占位凭据。历史事故明确日期、证据、恢复方式和未验证项。
3. 文档改动检查链接与 `git diff --check`；行为改动运行相应测试。禁止在生产执行破坏性的安装卸载测试。
4. 新增服务器交付文档必须检查 `packaging/nfpm.yaml`、通用包构建和通用安装器，使各格式交付一致。
5. 合并通过 CI 的 PR，再基于合并提交创建不可变 `vX.Y.Z` 标签。发布说明写入 `docs/releases/vX.Y.Z.md`，由工作流读取。
6. Release 工作流测试、构建双架构 DEB/RPM/APK/通用包，并在临时 Linux VM 执行 DEB 生命周期测试，成功后发布制品和文档包。
7. 下载发布制品核对 SHA-256，核对 `/opt/sma/docs` 所含文档。发布成功不代表已更新生产服务器。
8. 如需同步 GOFTP，先上传包与脚本，再上传校验文件，最后更新 `latest.txt`；回读确认脚本、版本索引和目标架构安装包。

## 验证与保密边界

- 单元测试和跨架构编译不能替代各发行版实际安装测试；只声明实际执行的验证。
- 原生包升级没有整包自动回退，升级前应保留旧包和配置备份。
- Git 和 Release 不包含真实 Token、密钥、数据库、会话和完整配置备份。
- 文档包为对应发布标签的快照；GitHub main 上的文档可能更近。引用故障处理步骤时注明版本。
