# 二次开发项目约定

本项目基于 Alibaba OpenCodeReview 二次开发，开始工作前须读取并遵循 [AGENTS.md](AGENTS.md) 中的上游规则；项目级规则冲突时，以本文件明确规定的差异为准。

## 项目文档

| 主题文档 | 用途与读取时机 | 维护原则 |
|---|---|---|
| [项目设计](project-docs/project-design.md) | 记录项目定位、二次开发共同原则与扩展边界；提出功能、设计方案或开展实现前读取 | 仅在项目级原则变化时更新；具体功能需求、候选方案及验收维护在对应 Spec |
| [分支维护](project-docs/branch-maintenance.md) | 规定分支职责、上游同步、冲突处理和发布流程；开展相关 Git 操作前读取 | 用户调整维护策略时更新，具体规则集中在该文档中 |
| [OpenCodeReview 知识库](project-docs/open-code-review/README.md) | 保存 OCR 使用与集成的工具知识及证据；涉及相关工作时，先读索引，再按需阅读子文档 | 获得新知识、核实版本变化或发现记录有误时更新，注明来源、版本和验证程度 |
| [审查测试与参考资料](project-docs/review-resources.md) | 集中维护测试仓库、参考 Skill 和知识库地址；准备真实效果测试或使用参考资料前读取 | 路径在此维护，Spec 和工单引用；资源变化时复核相关复测命令 |
| [开发测试与复测命令](project-docs/mock/README.md) | 保存可重跑的测试命令、执行前提和结果判定；开发测试、排障或复测前读取对应主题 | 开发与验收时记录可复用命令，标明日期、版本与验证状态；命令或前提变化时更新 |

子文档索引及具体维护规则写在对应文档或索引中；文档移动或删除时，同步更新相关入口与链接。新增文档主题需由用户明确提出。

## Agent skills

本项目的工程技能配置入口为 `AGENTS.override.md`，配置文件位于 `project-docs/agents/`。技能中默认的 `docs/agents/` 和 `docs/adr/` 路径分别映射到 `project-docs/agents/` 和 `project-docs/adr/`；配置更新写入本入口。

### 工作事项

需求、任务与讨论保存在本地 `.scratch/<feature>/`；创建、读取或更新工作事项前，读取 [工作事项约定](project-docs/agents/issue-tracker.md)。

### 分诊状态

任务分诊使用五个默认状态名称；评估任务或更新分诊状态前，读取 [分诊状态约定](project-docs/agents/triage-labels.md)。

### 领域文档

采用单一领域布局：根目录 `GLOSSARY.md` 与 `project-docs/adr/`；探索代码、使用领域术语或记录架构决策前，读取 [领域文档约定](project-docs/agents/domain.md)。

<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->
