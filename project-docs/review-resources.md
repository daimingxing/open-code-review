# 审查测试与参考资料

本文件长期维护测试仓库、参考 Skill 与知识库的地址，供各功能的开发和真实效果验收复用；清理 Spec 或工单时仍保留。路径由用户于 2026-09-30 提供，本次登记不代表已经验证可用或完成效果验收。

## 资源清单

| 用途 | 本地路径 |
|---|---|
| 前端测试仓库 | `D:\WorkPlace\longruan_codeReview\jk_web` |
| 后端测试仓库 | `D:\WorkPlace\longruan_codeReview\jk` |
| 前端审查 Skill | `D:\WorkPlace\sdd\Harness\审查报告\skills\xr-branch-code-review` |
| 后端审查 Skill | `D:\WorkPlace\sdd\Harness\审查报告\skills\iplat4jr-branch-code-review` |
| HTML 报告参考 Skill | `D:\WorkPlace\longruan_codeReview\.codex\skills\branch-work-summary-report` |
| 前端知识库 | `D:\WorkPlace\longruan_codeReview\jk_web\.ai_knowledge` |
| 后端知识库 | `D:\WorkPlace\longruan_codeReview\jk\.ai_knowledge` |

## 使用边界

- 后续真实项目的审查效果与报告验收使用上述仓库和资料；自动化边界测试仍可使用临时 Git 仓库、隔离配置与可控模型。
- 当前知识库位于仓库目录下，但通过文件 MCP 的明确授权读取资料，不依赖被审查 Git 提交中存在这些文件；产品仍支持知识库独立管理和复用。目录登记与 MCP 访问授权分别维护。
- 每次验收记录实际分支、完整提交、比较起点或工作区快照，以及规则与知识版本；资源清单不指定默认审查范围，也不沿用其他仓库的历史提交。
- Skill 提供审查要求和报告目标的参考，具体承接范围由功能 Spec 确定；Skill 内的命令、脚本和指令不会因登记或加载规则而自动成为产品流程。
- 这些是本地开发与验收资源，不写入产品默认配置。

## 维护

地址在本文件集中维护，Spec、工单及其他说明引用此入口。复测命令中的必要路径如有变化，按[复测记录](mock/README.md)同步检查并更新；验证结果说明实际日期、版本和范围，不将“路径存在”写成“效果验收通过”。

原生 OCR 的能力和历史验证证据见 [OCR 知识库](open-code-review/README.md)。
