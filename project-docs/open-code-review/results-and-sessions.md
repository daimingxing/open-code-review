# 结果、会话与 HTML 导出

[返回知识库索引](README.md)。本文件描述原生 OCR；二次开发的报告 JSON 和模型生成 HTML 另见[功能 Spec](../../.scratch/review-report/spec.md)。

## 三类原生数据

| 数据 | 用途与入口 |
|---|---|
| 结果 JSON | `ocr review --format json --output <文件>`；程序消费审查问题和运行摘要 |
| 会话 JSONL | 运行时保存的逐行事件，供恢复、工具过程追溯和 Viewer 使用 |
| 会话 HTML | `ocr session export` 读取会话记录，导出原生 Viewer 页面 |

`--format json` 指最终结果格式，不是流式 JSONL。原生会话 HTML 不能直接把结果 JSON 当作输入；只保存结果 JSON 不足以重建完整会话页面。

## 输出路径

`ocr review` 帮助列出 `text`、`json`、`sarif`，没有 `--format html`。`--output` 指 UTF-8 结果文件；未传或传 `-` 时输出到 stdout，没有默认结果目录。

基准源码的 `resolveOutputWriter` / `lazyFileWriter.Close` 将临时文件提交到指定路径；显式目录或缺失父目录会报错。已存在的结果文件可被替换，不自动生成 `(1)` 副本。自定义报告的防覆盖和自动编号属于新增产品行为，不能套用到原生输出。

## 真实审查验证

2026-09-28，使用 CLI v1.12.10 和真实模型审查 `2dgis-f` 的提交 `4b588a3807d38a66a4edc8900e08ea2897311b7e`：

```powershell
ocr review --repo "D:\WorkPlace\dunde-Project\2DGIS-project\2dgis-f" --commit 4b588a3807d38a66a4edc8900e08ea2897311b7e --audience agent --background "<本次背景>" --format json --output "<绝对文件路径>"
```

- 退出码 0，结果文件可解析，`status=complete`，问题数 0，耗时约 49 秒。
- 提交变更 15 个文件，默认选中 4 个且全部完成；不能描述成 15 个文件全部审查。
- 结果包含 `manifest`、固定提交范围、运行版本和 `coverage.selected/completed/failed` 等信息。
- 相同路径先前的 `skipped` 结果被替换，没有自动编号。
- 会话标识：`09e8ff1a-69fd-472f-98d3-d7790a0f3dbb`。

该样例验证结果输出及覆盖，不验证全部评论字段、异常状态或业务规则质量。

## 字段与完成状态

字段和状态必须以实际集成版本为准。较早文档使用过 `success` 等状态名，本机样例是 `complete`；不能把历史示例列表作为当前完整状态枚举。

| 信息 | 集成时关注的内容 |
|---|---|
| 状态与覆盖 | `status`、警告、选中/完成/失败/跳过项及原因；字段按版本读取 |
| 问题 | `comments`；Skill 描述含 `path`、`content`、`start_line`、`end_line`、`severity`、`category`，以及可选的 `existing_code`、`suggestion_code`、`thinking` |
| 运行摘要 | 模型、供应商、Token、耗时和统计；允许字段缺省 |
| 会话与恢复 | `session_id`、`resume` 等可选信息 |

Skill 所述评论以行号同时为 `0` 表示定位失败，等级为 `critical/high/medium/low`；当前真实样例没有评论，尚未完整验证该契约。不能假定原生结果已有独立的规则编号、知识引用、人员归属和成果字段。

退出码 `0` 可能包含警告或部分结果，不能单独证明审查完整。空问题列表也可能源于跳过、无匹配文件或失败；应结合状态、警告及最终覆盖判断，`--preview` 不能替代实际覆盖。

## Viewer 与原生 HTML

`ocr viewer` 启动本地服务，默认 `localhost:5483`；会话通常位于 `~/.opencodereview/sessions/`。页面展示范围、覆盖、问题、片段、模型和工具调用等过程信息。Fixed / Ignored 标记保存在浏览器 `localStorage`，按会话隔离，不写回原始审查记录，也不代表跨浏览器共享状态。

v1.12.7 的会话 HTML 导出入口：

```powershell
ocr session export "实际会话ID" --repo "E:\代码仓库" --output "E:\审查结果\review.html"
```

未指定会话 ID 时导出该仓库最新会话；自动化应使用本次结果返回的 `session_id`，避免选到其他运行。

固定版本 `85cecfe` 的实现通过 `LoadSession` 读取会话，复用 `session.html`，内联 CSS / JavaScript，先在内存渲染再写出。成品可离线打开，无需启动 Viewer 或重新调用模型；命令没有自定义模板、精简模式或多仓库合并参数。更改审查规则不会改变该页面的结构。

已核实命令帮助和固定版本源码，尚未完成真实导出页面的视觉验收。会话可能包含代码与模型交互，导出范围不限于最终问题；CLI 提供导出也不代表同版本 Viewer 必然有下载按钮。

多仓库自定义报告、重新排版和来源补充由产品另行实现。报告外观不会增加原生覆盖，也不能补造未记录的事实。

## 来源

- [CLI 文档](https://open-codereview.ai/docs/cli-reference)、[Viewer 文档](https://open-codereview.ai/docs/viewer)
- [固定版本导出实现](https://github.com/alibaba/open-code-review/blob/85cecfe/internal/viewer/export.go)
- [固定版本会话模板](https://github.com/alibaba/open-code-review/blob/85cecfe/internal/viewer/templates/session.html)
- `ocr session export --help`、安装的 Skill 与上述本机审查结果；在线文档描述不作为跨版本完整契约。
