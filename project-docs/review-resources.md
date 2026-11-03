# 审查测试与参考资料

本文件长期维护测试仓库、参考 Skill 与知识库的地址，供各功能的开发和真实效果验收复用；清理 Spec 或工单时仍保留。路径由用户于 2026-09-30 提供，本次登记不代表已经验证可用或完成效果验收。

## 资源清单

| 用途 | 本地路径 |
|---|---|
| 前端测试仓库 | `D:\WorkPlace\longruan_codeReview\jk_web` |
| 后端测试仓库 | `D:\WorkPlace\longruan_codeReview\jk` |
| 前端审查 Skill | `D:\WorkPlace\sdd\Harness\审查报告\skills\xr-branch-code-review` |
| 当前 XR 审查 Skill（2026-11-03 用户指定） | `D:\WorkPlace\longruan_codeReview\jk_web\.codex\skills\xr-branch-code-review` |
| 后端审查 Skill | `D:\WorkPlace\sdd\Harness\审查报告\skills\iplat4jr-branch-code-review` |
| HTML 报告参考 Skill | `D:\WorkPlace\longruan_codeReview\.codex\skills\branch-work-summary-report` |
| 前端知识库 | `D:\WorkPlace\longruan_codeReview\jk_web\.ai_knowledge` |
| 后端知识库 | `D:\WorkPlace\longruan_codeReview\jk\.ai_knowledge` |
| 可复用 XR 审查 background | [xr-review-background.md](retests/knowledge-access/xr-review-background.md) |

## 原生知识库常用验收样本

2026-11-03 用户指定前端仓库的 `dev-1010...dev-1016` 为常用真实审查样本，配合上述前端知识库。已只读核实本地引用：

| 项目 | 值 |
|---|---|
| 基线分支与提交 | `dev-1010`：`73eada8dd59fa7e891855dca4b710389e1dc3535` |
| 目标分支与提交 | `dev-1016`：`cb2a44f820675b93e740421f2fc5d66e4f188a35` |
| 三点差异统计 | 15 个文件，444 行新增、148 行删除；不是 OCR 筛选后的覆盖数量 |

每次复测先核对引用是否移动，并记录实际比较起点。`dev-1023...dev-1030` 变更过大，不作为本阶段默认效果样本。此登记只确认资源和差异规模，不表示新知识参数已实现或真实模型验收已通过。

background 从当前 XR Skill 的审查、误报排除、结构规则和知识路由提取，独立保存，可通过 `--background-file` 复用；没有复制其 Markdown 报告生成、元数据、作者统计或校验流程。为适配 OCR 的选中范围与工具能力，不要求全仓结构普查、外部 Python 脚本或自定义 P1/P2/P3 输出；结构要求仅核对本次变更及必要上下文。Skill 引用的 `project-public-methods.md` 当前未在知识库根目录发现，background 明确允许说明缺失，并以源码实现取证。

## 使用边界

- 后续真实项目的审查效果与报告验收使用上述仓库和资料；自动化边界测试仍可使用临时 Git 仓库、隔离配置与可控模型。
- 当前知识库位于仓库目录下；旧文件 MCP 专用采集已清理，当前知识来源为未采集；计划中的[原生知识库入口](../.scratch/native-knowledge/spec.md)改由 `--knowledge-dir` 授权原生工具访问磁盘资料。两者均不依赖被审查 Git 提交中存在知识文件，目录登记和 background 均不自动授予访问权限。
- 每次验收记录实际分支、完整提交、比较起点或工作区快照，以及规则与知识版本；资源清单不指定默认审查范围，也不沿用其他仓库的历史提交。
- Skill 提供审查要求和报告目标的参考，具体承接范围由功能 Spec 确定；Skill 内的命令、脚本和指令不会因登记或加载规则而自动成为产品流程。
- 这些是本地开发与验收资源，不写入产品默认配置。

## 维护

地址在本文件集中维护，Spec、工单及其他说明引用此入口。复测命令中的必要路径如有变化，按[开发复测记录](retests/README.md)同步检查并更新；验证结果说明实际日期、版本和范围，不将“路径存在”写成“效果验收通过”。面向用户的验收入口和测试输入见[人工验收材料](mock/README.md)。

原生 OCR 的能力和历史验证证据见 [OCR 知识库](open-code-review/README.md)。
