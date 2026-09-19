# 结果、会话与 HTML 导出

[返回知识库主文件](README.md)

## 三种数据用途

| 数据 | 用途 | 获取方式 |
|---|---|---|
| 结果 JSON | 程序消费审查发现和运行摘要 | `ocr review --format json --output ...` |
| 会话 JSONL | 保存模型与工具执行记录，供恢复和 Viewer 使用 | OCR 运行时保存到会话目录 |
| 离线 HTML | 将一次会话页面交给浏览器阅读 | `ocr session export` |

JSONL 是逐行记录 JSON 事件的日志，不等于结果 JSON。HTML 导出读取会话记录，不能直接把结果 JSON 文件作为其输入。

## 输出格式与字段

v1.12.7 的 `ocr review` 帮助列出 `text`、`json`、`sarif`，没有 `--format html`。`--output` 将结果写入 UTF-8 文件，默认输出到 stdout。

在线 CLI 文档的 JSON 顶层是对象，主要字段如下：

| 字段 | 含义 |
|---|---|
| `status` | `success`、`completed_with_warnings`、`completed_with_errors`、`skipped` |
| `comments` | 问题数组，可以为空 |
| `llm` | 模型与供应商标识 |
| `summary` | 文件、问题、Token、耗时等统计，可能缺省 |
| `warnings` | 部分失败等警告，按情况出现 |
| `session_id` | 已保存会话的标识，按情况出现 |
| `resume` | 恢复运行信息，按情况出现 |

安装的 Skill 描述评论含 `path`、`content`、`start_line`、`end_line`、`severity`、`category`，以及可选的 `existing_code`、`suggestion_code`、`thinking`。行号同时为 `0` 表示定位失败。严重程度列为 critical/high/medium/low。

这些是文档和 Skill 所述契约，尚未通过本机真实审查结果完整验证。不要把示例当作所有版本的完整 Schema，也不能假定存在独立的规则编号、知识引用、人员归属或工作量字段。

## 完成状态

退出码 `0` 可以包含非致命警告、部分失败或预算耗尽后的部分结果；`1` 表示致命错误等失败。集成方应结合状态、警告和覆盖信息判断完整性。

空问题数组可能表示没有发现问题，也可能是无符合条件的文件、跳过或执行失败，不能一律解释为审查通过。`--preview` 的范围信息也不等于最终实际完成的覆盖范围。

## Viewer

`ocr viewer` 启动本地服务，默认地址 `localhost:5483`。会话记录通常位于 `~/.opencodereview/sessions/`，按仓库组织。

Viewer 展示会话元数据、审查问题、代码片段、覆盖统计、模型响应和工具调用过程。在线文档还描述了会话比较和筛选。

Fixed / Ignored 标记存于浏览器 `localStorage`，按会话隔离，不写回原始审查记录，不代表跨浏览器共享的处理状态。

会话日志可能包含代码和模型交互内容。将会话导出给读者时，导出范围不只是最终问题列表。

## v1.12.7 HTML 导出

```powershell
rtk pwsh -Command 'ocr session export "实际会话ID" --repo "E:\代码仓库" --output "E:\审查结果\review.html"'
```

不指定会话 ID 时导出该仓库最新会话。自动化应优先传本次 JSON 返回的 `session_id`，避免并发任务或后续审查改变“最新会话”。

固定版本实现使用 `LoadSession` 加载会话，复用 `session.html`，内联样式和脚本，并先在内存中完整渲染再写出。结果是独立 HTML 文件，可通过 `file://` 离线打开，无需启动 Viewer，也无需重新调用模型。

页面结构沿用 Viewer，包括仓库及范围信息、覆盖、Token 使用、执行过程和问题。v1.12.7 的导出命令没有报告模板参数、精简模式参数或多仓库合并参数。

因此，更改审查提示词不能改变这个 HTML 的页面结构。应用可以另行消费 JSON 生成自己的页面，但这属于消费方实现，不属于 OCR 原生模板能力。

已核实命令帮助与固定版本源码，尚未用真实会话完成离线页面视觉验收。CLI 有导出入口不代表同版本 Viewer 页面必然有下载按钮。

## 数据留存与集成边界

- 需要重新导出原生 HTML，应保留相应会话记录；只留结果 JSON 不够。
- 需要重新排版自定义报告，消费方可保存 JSON 与必要的业务元数据。
- 会话只代表指定仓库的一次运行，多仓库汇总需要应用层组织。
- 修改报告外观不会提高审查覆盖，也不会补出原始结果缺失的事实。

## 来源

- [CLI 文档](https://open-codereview.ai/docs/cli-reference)
- [Viewer 文档](https://open-codereview.ai/docs/viewer)
- [HTML 导出需求 #1167](https://github.com/alibaba/open-code-review/issues/1167)
- [固定版本导出实现](https://github.com/alibaba/open-code-review/blob/85cecfe/internal/viewer/export.go)
- [固定版本会话模板](https://github.com/alibaba/open-code-review/blob/85cecfe/internal/viewer/templates/session.html)
- 本机 `ocr session export --help` 与安装的 Skill，证据范围见主文件。
