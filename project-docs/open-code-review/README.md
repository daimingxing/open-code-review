# OpenCodeReview 知识库

本知识库说明 OpenCodeReview 自身的用途、使用方式、实现边界与集成注意事项。项目需求、产品形态及自定义报告方案单独记录在 [本项目设计记录](../project-design.md)，不作为上游能力写入本知识库。

## 文档索引

| 文件 | 内容 |
|---|---|
| [规则、背景与知识读取](rules-and-context.md) | `--rule`、`--background`、Markdown 背景、工具读取与 Skill 边界 |
| [结果、会话与 HTML 导出](results-and-sessions.md) | JSON、状态、Viewer、离线 HTML、数据留存 |

现有 [XR 规则示例](../examples/xr-review-rule.json) 是可选参考，不是默认配置。

## 工具用途

OpenCodeReview（简称 OCR）是代码审查 CLI。它获取 Git 差异，筛选和组织待审查文件，解析规则，调用已配置模型，并通过代码读取、搜索及评论工具产生行级审查结果。

内置工具支持读取上下文，不要求智能体具备编辑代码能力才能产生报告。保存结果、浏览会话与导出 HTML 由程序实现。

| 能力 | 入口与边界 |
|---|---|
| 审查本地工作区 | `ocr review`，默认包含暂存、未暂存及未跟踪变更 |
| 审查分支范围 | `ocr review --from <基准> --to <目标>` |
| 审查单个提交 | `ocr review --commit <提交>`，与父提交比较 |
| 完整文件审查 | `ocr scan`，读取工作树文件，不依赖有意义的差异 |
| 自定义规则与背景 | `--rule`、`--background`、`--background-file` |
| 预览范围 | `--preview`，跳过模型，查看筛选结果 |
| 模型选择 | 复用供应商配置，运行时可用 `--provider`、`--model` 覆盖 |
| 会话与 HTML | `ocr session`、`ocr viewer`，详见结果文档 |
| 委托外部智能体 | `ocr delegate preview`、`ocr delegate rule` |

## 核实范围

基准日期：2026-09-19。已确认本机版本为 `@alibaba-group/open-code-review@1.12.7`，`ocr version` 显示提交 `85cecfe`。

证据分为三类：

- **本机确认**：CLI 帮助可见的参数；XR 示例通过 `ocr rules check` 命中 `Custom (--rule)`。
- **固定版本源码确认**：`85cecfe` 的会话 HTML 导出使用同一 Viewer 模板，并内联 CSS、JavaScript。
- **文档说明、运行待验证**：在线 `main` 分支文档与安装的 Skill 所述行为。文档可能领先于固定版本，不能等同于本机端到端验证。

尚未在调研中实际调用模型完成审查，也未生成真实会话 HTML 做视觉验收。规则效果、完整 JSON 契约与恢复行为需在集成版本上验证。

## 分支审查语义

`--repo` 指向本地 Git 仓库。`--from A --to B` 计算共同祖先 `merge-base(A, B)` 至 B 的差异，不是 A、B 两个尖端的直接文件比较。

自动化调用应记录实际提交编号及比较起点，避免分支移动后无法确定当时审查对象。范围模式中的上下文工具面向目标提交；不能假设工作目录里临时添加的知识文件会被读取。

以下均为 PowerShell 示例，仓库、分支与文件路径为占位值。

```powershell
ocr review --repo "E:\代码仓库" --from "main" --to "feature/example" --preview
ocr review --repo "E:\代码仓库" --from "main" --to "feature/example" --background-file "E:\审查资料\background.md" --rule "E:\审查资料\rule.json" --format json --output "E:\审查结果\result.json"
```

模型需提前配置。供应商与连接配置通常位于 `~/.opencodereview/config.json`，第三方服务支持范围以所用版本为准。以上命令不包含密钥，知识库也不保存密钥。

## 执行控制

- `--audience agent` 抑制进度输出，适合宿主智能体；默认 human 模式在 JSON 输出时将进度写到 stderr，stdout 保持机器可读。界面需要进度时不必强制选 agent。
- `--concurrency` 控制并行审查子任务数量。
- `--max-tokens` 控制每组提示词上限；它不等于总调用预算。
- `--max-tokens-budget` 控制整个审查的 Token 预算；预算不足可能保留部分结果。
- `--resume` 用于恢复兼容的范围或提交审查。文档说明工作区模式不支持恢复；引用解析结果、规则或筛选变化可能导致恢复被拒绝。
- `--no-filter` 跳过模型对评论的后处理，不代表扩大审查文件范围。

不要仅凭退出码 `0` 判定完整成功；详见 [结果状态](results-and-sessions.md)。

## 委托模式的含义

`ocr delegate preview` 提供可审查文件与模式、引用元数据；`ocr delegate rule` 提供解析后的规则。委托模式自身不调用模型。

外部宿主负责执行审查，不能据此推断 OCR 会执行任意 Skill，或宿主自动继承 OCR 托管审查的调度、后处理、会话记录及 Viewer 集成。

## 来源与维护

- [上游仓库](https://github.com/alibaba/open-code-review)，包许可证标注为 Apache-2.0。
- [CLI 文档](https://open-codereview.ai/docs/cli-reference)
- [模型配置文档源码](https://github.com/alibaba/open-code-review/blob/main/pages/src/content/docs/en/configuration.md)
- 本机 Skill：`C:\Users\user\.agents\skills\open-code-review\SKILL.md`。此路径仅是核实来源，阅读本知识库不依赖该文件存在。

新增知识放入对应主题，并记录来源及适用版本。新增子文件时更新本页索引。升级时优先复核规则解析、结果字段、状态码及导出命令；将待验证项标明，不能把项目设想写成上游已实现能力。
