# 二次开发项目约定

本项目基于 Alibaba OpenCodeReview 二次开发，开始工作前须读取并遵循 [AGENTS.md](AGENTS.md) 中的上游规则；项目级规则冲突时，以本文件明确规定的差异为准。

## 项目文档

| 主题文档 | 用途与读取时机 | 维护原则 |
|---|---|---|
| [项目设计](project-docs/project-design.md) | 记录项目目标、需求、候选方案与未决事项；讨论需求、方案或开展实现前读取 | 需求、候选方案或决策变化时更新，区分讨论建议与已定方案 |
| [分支维护](project-docs/branch-maintenance.md) | 规定分支职责、上游同步、冲突处理和发布流程；开展相关 Git 操作前读取 | 用户调整维护策略时更新，具体规则集中在该文档中 |
| [OpenCodeReview 知识库](project-docs/open-code-review/README.md) | 保存 OCR 使用与集成的工具知识及证据；涉及相关工作时，先读索引，再按需阅读子文档 | 获得新知识、核实版本变化或发现记录有误时更新，注明来源、版本和验证程度 |

子文档索引及具体维护规则写在对应文档或索引中；文档移动或删除时，同步更新相关入口与链接。新增文档主题需由用户明确提出。

<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->