# OpenCodeReview 知识库

本知识库保存原生 OCR 的使用方式、实现边界与验证证据。项目级原则见[项目设计](../project-design.md)，审查与报告的具体行为见[功能 Spec](../../.scratch/review-report/spec.md)；不能把计划新增的 `--report`、`ocr report` 当作上游已有能力。

## 文档索引

| 文档 | 内容 |
|---|---|
| [规则、背景与知识读取](rules-and-context.md) | rule 文件、背景、原生文件边界、外部 MCP 配置与验证、上下文成本 |
| [结果、会话与 HTML 导出](results-and-sessions.md) | 原生 JSON / JSONL、输出路径、状态与覆盖、Viewer 和会话 HTML |

[XR 规则示例](../examples/xr-review-rule.json) 仅演示内联审查规则，不是默认配置，也不包含完整知识库接入。

## 能力与入口

OCR 是代码审查 CLI：获取 Git 差异、筛选文件、加载规则、调用模型，通过上下文读取与评论工具产生行级结果。

| 能力 | 原生入口与边界 |
|---|---|
| 工作区审查 | `ocr review`，默认包含暂存、未暂存和未跟踪变更 |
| 分支比较 | `ocr review --from A --to B`，比较 `merge-base(A, B)` 至 B，而非两个尖端直接比较 |
| 单提交审查 | `ocr review --commit <SHA>`，与父提交比较 |
| 完整文件审查 | `ocr scan`，读取工作树文件 |
| 规则与背景 | `--rule`、`--background`、`--background-file` |
| 范围预览 | `--preview`，不调用模型，不代表最终完成覆盖 |
| 模型配置 | 供应商配置及 `--provider`、`--model`；配置通常位于 `~/.opencodereview/config.json` |
| 外部工具 | MCP 客户端接入；文件 MCP 服务需要另行安装和配置 |
| 结果与会话 | `--format`、`--output`、`ocr session`、`ocr viewer` |
| 委托宿主 | `ocr delegate preview` / `ocr delegate rule`，自身不调用模型，后续审查由宿主负责 |

自动化应记录完整提交编号与实际比较起点。原生范围/提交模式的代码工具读取目标版本，磁盘上临时添加的资料不会因此进入该版本。

## 验证基准

下表是已有证据的范围，不表示所有功能均已完成端到端验收。

| 日期 | 版本或对象 | 已确认内容 |
|---|---|---|
| 2026-09-19 | CLI v1.12.7、提交 `85cecfe` | 命令帮助；原生会话 HTML 导出的固定版本源码 |
| 2026-09-28 | 本地源码 `578e647` | 规则解析、Markdown 引用、`merge_system_rule`、提示词和相关集成边界 |
| 2026-09-28 | CLI v1.12.10 | 真实模型单提交审查、结果 JSON 文件输出与覆盖信息 |
| 2026-09-29 | 已安装 CLI 与本地源码 | 外部 JSON / Markdown 规则命中、内置文件读取边界 |
| 2026-09-30 | 文件 MCP `2026.8.31`、原生 OCR、模型替身 | 外部索引及章节读取、工具白名单和目录外读取拒绝 |

仍待验证：真实模型的知识选读与应用效果、业务规则效果、完整结果字段与异常/恢复路径、原生会话 HTML 视觉效果。新报告功能尚未实现。在线主分支文档和 Skill 描述仅作为参考，不能覆盖已验证版本的实际行为。

## 执行与集成注意事项

- `--audience agent` 抑制进度输出；human 模式配合 JSON 输出时，进度写 stderr，结果写 stdout。
- `--concurrency` 控制并行任务；`--max-tokens` 控制每组提示上限；`--max-tokens-budget` 控制整次审查预算，不能混为模型真实窗口大小。
- `--resume` 用于兼容的分支/提交审查；工作区不应假定支持恢复。引用、规则或筛选变化可能使恢复被拒绝。
- `--no-filter` 跳过评论后处理，不扩大文件范围。
- 退出码、状态、警告和覆盖需要结合判断；零问题不等于审查完整通过。
- 委托命令、rule 和工具定义文件不是通用 Skill 执行环境，不自动执行脚本或继承宿主权限。

## 来源与维护

- [上游仓库](https://github.com/alibaba/open-code-review)
- [CLI 文档](https://open-codereview.ai/docs/cli-reference)
- [模型配置文档源码](https://github.com/alibaba/open-code-review/blob/main/pages/src/content/docs/en/configuration.md)
- 各主题文档中的本地源码链接、版本与实测记录。

每项能力注明依据及验证程度；升级后优先复核规则解析、MCP、结果字段、状态和导出行为。资料变化更新对应主题，不叠加讨论流水账或过期结论。保留当前索引结构，新增主题须按项目约定确认。
