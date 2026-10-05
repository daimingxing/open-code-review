# 07：单份报告材料生成中文 HTML

Status: ready-for-agent

Blocked by: [06：交付不完整审查材料与失败恢复行为](06-partial-review-material.md)

阶段：3。规格来源：[Spec](../spec.md)；执行约定及依赖总览：[工单索引](../README.md)。关联用户故事：27、28、29、30、31、33、34、35、36、37、40、41、42。

## 交付行为

用户使用报告命令和命名模板，将一份兼容材料生成内容完整、事实可核验且可离线阅读的中文 HTML；阅读交互与视觉完善由 09 交付。

## 验收条件

- [x] 实现报告命令的输入、模板与输出行为；至少提供一份输入，省略模板时选择 review-summary，未知名称报错，不增加模板管理入口或报告格式参数。
- [x] 拒绝原生 OCR JSON、版本不兼容及必需材料缺失；生成仅使用材料与模板，模型不获得代码、Git、知识库、会话或外部资料读取能力。可在原仓库不可访问时成功生成。
- [x] 允许模型组织 HTML 结构；程序检查所有 finding 恰好出现一次、定位和严重级别与材料一致、每条问题有可见中文说明且源代码/证据或未采集原因完整可见，基础统计与材料一致，必要章节有内容。`data-fact` 为可选校验提示：出现时值必须对应材料，不要求固定标签、标题、章节放置或穷举全部动态材料事实。
- [x] 九类内容按材料条件展示；中文叙述保留技术标识，缺结构检查时明确未提供，无治理建议不生成空卡片，建议在问题详情中呈现。
- [x] HTML 是基本可读的离线单文件，具有清晰标题和内容层次，信息不只靠颜色；证据文本安全呈现，无外部资源、凭据和无必要本机绝对路径。筛选、折叠、窄屏适配与打印完善放到 09，输入校验和安全约束从本工单起生效。
- [x] 显式输出路径和缺省日期名称遵循既定防覆盖规则；失败有诊断与用量，调用有预算边界。有效 JSON 可再次生成 HTML，不触发重审。
- [x] 通过 CLI 的受控模型测试及真实浏览器检查验证零问题、四级风险、部分完成和知识缺失样例；核对内容完整性、事实与基本可读性，不固定模型措辞或 DOM 快照。

## 初版 HTML 验收澄清（2026-10-05）

- HTML 原型与最终视觉规范尚未确定；标题、标签、布局和非关键措辞差异不阻止本工单通过。保持章节 ID 与基本内容、finding 集合/主要字段、原始证据和基础统计，实际生成的文件可离线安全阅读即可。
- finding 的机器标记不固定为 `<article>`；允许 `article`、`section`、`div`、`blockquote`、`li` 等可承载完整问题详情的分组元素，问题内部结构不固定。内联元素或 `<p>` 不能承载应用追加的详情控件与完整证据，因此不作为 finding 根元素。
- 此澄清落实于 `.scratch/review-report/spec.md` 的“模型生成 HTML”和验收矩阵；旧执行记录中固定中文标签数量属于当时样例证据，不是当前验收门槛。

## 执行记录（2026-10-04）

- 已实现 `ocr report` 单份输入命令、默认/显式输出路径、兼容材料校验、有限模型调用诊断和安全发布；程序校验完整 finding 集合、等级、统计、章节及材料事实，使用同目录临时文件和硬链接排他发布，失败不留下最终文件。
- `go test ./cmd/opencodereview -run '^TestReportCommand' -count=1 -timeout=75s` 与 `go test ./internal/report -count=1 -timeout=75s` 当前均通过；负例覆盖 CSS/style 隐藏、canvas/details/dialog/noscript 等未知元素、`ping`、Unix/Windows 绝对路径、body 外正文、重复 `data-fact`、people/achievement/status/source 篡改及无效输出不落盘。
- 受控 CLI 产物在最终代码提交 `dbfa92dc4f8a890c1ef3b17191d4b56b03840d18` 上重新生成，并用 Edge 154.0.4258.53 离线打开。桌面 1440px、窄屏 375px 均无横向溢出，完整呈现 9 章、60 条 finding、660 个带固定标签的事实行和十项统计；无页面错误、外部资源或非本地请求。真实 DeepSeek 在 `d34bb009` 上运行 `ocr report --input frontend-report.json` 后因输出未以 doctype 开头失败，无 HTML 文件生成；此项不计为通过，模型格式修复留给 09。复测结果和截图路径见[工单 07 复测记录](../../../project-docs/mock/review-report-html-07.md)。
- 真实 DeepSeek 首次调用未能生成可用 HTML：该格式失败已记录在复测文档，后续由 09 有界修复能力处理；不得把这次失败算作工单 07 的模型通过证据。
- 最后提交 `0bf782f96877a6b542b9e8e975d9bf858bf22074` 强制带 `data-finding-id` 的节点必须为 `<article>`；增加非 article 拒绝和 `WriteHTML` 不落盘测试。该改动不影响 HTML 渲染，故复用上一代码提交产物的 Edge 检查结果。最终两包聚焦测试通过；独立 reviewer `review_report_07` 对该提交结论为无 blocker、无残留 finding。reviewer 环境未找到 Go，复测由实施环境完成。
- 全 `cmd/opencodereview` 测试在既有 `TestReviewE2E_ReportTokenBudgetFailureIsNotSuccessAcrossModes` 的 MCP 读取路径超时；工单 07 聚焦测试已单独通过，故障与证据记于复测记录。
- 主线程于 2026-10-04 将工单分支以 `--no-ff` Merge 集成到 `feature-review-report`，集成提交 `27d7b41de049fd46854252dd35208884c2be8d8e`。目标分支复跑 `go test ./internal/report -count=1 -timeout=75s`（2.174s）、`go test ./cmd/opencodereview -run '^TestReportCommand' -count=1 -timeout=75s`（4.298s）及 `git diff --check` 均通过。真实 DeepSeek doctype 格式失败仍未通过，待工单 09 有界修复后复验。

