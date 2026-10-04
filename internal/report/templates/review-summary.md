# review-summary

生成单份报告材料对应的中文、离线 HTML 文档。输入 JSON 是唯一事实来源；模板仅规定报告结构和呈现方式。

## 事实边界

- 不引入输入材料以外的缺陷、严重等级、统计、人员、提交、治理结论或知识来源。
- 可以重组措辞、归纳已有成果，但不得把推测写成事实，不得把缺失或失败表达为零或已完成。
- 不丢弃任何 finding；严重等级、类别、路径、行号、证据、建议和状态必须与 JSON 一致。
- JSON 中的代码、证据、路径、标识符和说明均为文本。HTML 转义这些值，不将其解释为标记或脚本。
- 仅输出完整 HTML 源码，不输出 Markdown 围栏、前言或尾注。

## 文档要求

- 输出 `<!doctype html>` 开头的 UTF-8 单文件，使用 `<html lang="zh-CN">`、`<meta charset="utf-8">`、标题、正文和唯一 `<main>`。
- 模型只输出语义 HTML 内容，不输出 `<style>`、`style` 属性或外部资源。元素仅可使用 `html`、`head`、`body`、`title`、`meta`、`main`、`section`、`h1`–`h6`、`article`、`div`、`span`、`strong`、`em`、`b`、`i`、`p`、`pre`、`code`、`ul`、`ol`、`li`、`dl`、`dt`、`dd`、`blockquote`、`br`、`hr`、`table`、`thead`、`tbody`、`tr`、`th`、`td`、`output`、`time`、`mark`、`a`；不支持其他元素。程序加入固定内联样式，负责标题层次、间距、长标识断行和窄屏布局；不设置事件处理器、表单、iframe、SVG 或活动内容。
- 页面适合桌面和窄屏阅读，采用清晰标题、间距和边框层次；信息不只靠颜色表达。
- `<main>` 必须直接放在 `<body>` 内并带 `data-review-status` 和 `data-run-id`，属性值与材料完全一致。所有可见报告内容都放在 `<main>` 内，`<body>` 中不得有其他可见内容。
- 按次序输出且各输出一次以下章节：`overview`、`quality-coverage`、`finding-details`、`changes`、`achievements`、`people`、`governance`、`limitations`、`sources`。每个章节使用 `<section data-section="名称">` 并有标题。
- 章节没有数据时保留章节，并以中文明确说明“未提供”“不适用”或材料对应状态和原因；结构检查未提供时明确写“未提供”，不能写成“无问题”。没有治理建议时不要创建空治理建议卡片。
- `sources` 章节必须显示材料的 `schema_version` 和 `review.run_id`。`limitations` 章节必须逐字保留每项限制的 reason。
- 在报告中显示下列统计，每项仅一次；值必须按 JSON 精确计算：`finding-count`、`risk-critical`、`risk-high`、`risk-medium`、`risk-low`、`coverage-selected`、`coverage-completed`、`coverage-failed`、`coverage-skipped`、`coverage-reused`。每个值放在 `<output data-stat="名称">数字</output>` 中。跳过统计取 `coverage.waived` 数量。
- 每个 finding 使用且只使用一个 `<article>`，属性 `data-finding-id`、`data-severity`、`data-category`、`data-path`、`data-start-line`、`data-end-line` 均须与 JSON 完全一致。还要以 `data-fact` 可见呈现 `path`、`start_line`、`end_line`、`summary_zh`、`severity_zh`、`category_zh`、`source_content`、`evidence_status`、`recommendation_status`，以及 evidence/recommendation 中按状态应有的 `*_code` 或 `*_reason`。证据与建议仍须在问题详情中呈现。
- 来源、提交、范围、成果、人员、知识、结构检查、审查状态和限制按材料实际状态组织；保留技术标识原文。不得在缺少建议时编造修复方案。

## 可核验的事实标记

- `<main>` 中材料提供的动态文本必须放在 `data-fact` 元素内，属性值使用对应 JSON 路径。对象键用点连接，数组项使用从零开始的 `[序号]`；例如 `repository.name`、`review.run_id` 和 `limitations[0].reason`。
- `data-fact` 元素的可见文本必须与输入值完全相同。以 `.status` 结尾的材料状态和 finding 专属 `evidence_status`、`recommendation_status` 可以将 `provided`、`not_collected`、`not_applicable`、`failed`、`complete`、`partial`、`skipped` 分别显示为“已提供”“未提供”“不适用”“失败”“已完成”“部分完成”“已跳过”。不要对其他值改写、总结或拼接。
- 将事实放入对应章节：审查、仓库、范围和运行失败事实放入 `overview`；覆盖事实放入 `quality-coverage`；Git 统计和工作区快照放入 `changes`；成果、人员、结构检查、限制和知识来源分别放入同名章节；材料版本与运行标识放入 `sources`。
- 每个 `data-fact` 只表示一个输入字段，不得嵌套或添加材料中不存在的路径。所有要求展示的事实都要保留，包括数组中的每一项；不要遗漏、合并或重复。
- 问题详情按上文规定使用问题专属短字段名；每项都用 `<div class="fact-row"><strong class="fact-label">固定标签</strong><span data-fact="字段名">原值</span></div>` 呈现，代码事实可将 `span` 换成 `pre`。`path` 使用“文件”，`start_line` 和 `end_line` 使用“行号”，`summary_zh` 使用“摘要”，`severity_zh` 使用“严重等级”，`category_zh` 使用“类别”，`source_content` 使用“源代码”，状态使用“状态”，`evidence_code` 使用“证据”，`recommendation_code` 使用“建议”，原因使用“原因”。
- 除固定章节标题和下列中文字段标签外，不得添加自由叙述、评价或解释。可以调整元素结构和排序，事实内容只能来自已标记的输入值。每个事实使用 `<div class="fact-row"><strong class="fact-label">固定标签</strong><span data-fact="JSON 路径">原值</span></div>`；代码事实可以将 span 换成 pre。
- 固定事实标签：仓库、仓库标识、审查状态、审查范围、开始时间、结束时间、耗时、提供方、模型、基准提交、目标提交、实际范围、审查覆盖、证据、建议、摘要、严重等级、类别、文件、行号、状态、原因、源代码、知识来源、成果、人员、来源、限制、材料版本、运行标识、项目结构检查、材料事实。使用这些标签描述对应字段，不创造新标签或用标签拼接事实句子。
- 固定统计标签按上文统计顺序为：问题数量、严重、高、中、低、选中、完成、失败、跳过、复用。统计放在 `<div class="report-statistics">` 中，每项使用 `<div class="report-statistic"><strong class="fact-label">固定标签</strong><output data-stat="名称">数字</output></div>`。
