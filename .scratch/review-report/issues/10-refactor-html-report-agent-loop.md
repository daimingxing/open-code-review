# 10：复用智能体循环重构 HTML 报告生成

**What to build:** 在已交付的报告功能上重构 HTML 生成链路，将单轮生成和固定次数的内容修复改为复用 review 智能体循环。保留报告材料、多份汇总、阅读交互与文件输出行为；按当前 Spec 更新模型运行、工具调用、上下文压缩和失败处理方式。

**Blocked by:** None（01–09 已完成并集成，本工单从现有报告行为开始重构。）

**Status:** ready-for-agent

- [x] `ocr report` 使用 review 已有的智能体循环、模型运行配置、上下文容量、压缩阈值与压缩任务；允许模型多轮调用查找、读取、搜索及 HTML 写入工具，最终通过写入工具交付 HTML。报告材料通过读取工具按需进入对话，不把大份 JSON 固定塞入不可压缩的初始消息。
- [x] 不新增 report 专用上下文/token 上限、固定轮数重试或独立压缩实现；遵循共享循环已有的预算和停止行为。移除旧单轮 HTML completion 与固定次数内容修复的命令路径。
- [x] 提示词把本次报告 JSON 和模板/样式资源设为默认依据，建议避免重复读取源码或重新审查；这只是行为引导，不增加源码路径权限拦截。若模型判断其他文件有帮助，可使用循环已有的读取能力；发现报告材料缺少必要事实时，优先补充 review 阶段生成 JSON。
- [x] 首轮不解析、清洗、校验或限制 HTML 的结构、事实、标签、脚本、样式、外部资源或视觉效果；不因 HTML 内容触发修复或拒绝交付。保留 JSON 输入校验，以及输出路径校验、防覆盖、原子保存和写入失败诊断。
- [x] 保留已交付的 CLI 参数、单份及多份报告材料处理、输入顺序与审查单元归属、阅读交互、模型用量/耗时记录和输出命名行为；不要求固定 DOM、CSS、文案或视觉布局。更新与新生成流程矛盾的旧测试和说明，但不删除 01–09 的历史验收记录。
- [x] 用可控模型端到端验证多轮读材料、调用工具、写入 HTML 并正常结束；验证报告生成沿用现有上下文压缩路径。另覆盖模型/循环失败、未写入 HTML、文件写入失败及已有目标文件等恢复行为，确认不会因 HTML 内容检查拒绝产物或覆盖已有文件。
- [x] 若实施验证证明现有报告 JSON 缺少生成所需的必要事实，只对 review JSON 生产端作完成该链路所必需的补充，并同步验证其既有审查行为；不另建 report 专用材料补采、校验或修复流程。
- [x] 更新用户说明、工单执行记录和可复跑的开发复测入口，明确提示词引导不是源码访问保证，且首轮产物没有 HTML 内容或浏览器安全检查。

## 执行记录

- 执行者：implement 会话（Cursor Grok 4.5）
- 分支：`feature-review-report`
- 基线：实施前 `feature-review-report` tip（含 01–09 已集成交付）
- 实现要点：
  - `ocr report` 改为 `llmloop.Runner.RunMainTask`，复用 review 默认模板的 `MaxTokens` / `MaxToolRequestTimes` / `MemoryCompressionTask`
  - 工具：`file_read` / `file_find` / `code_search` / `write_html` / `task_done`；材料与模板/样式写入工作区暂存目录后按路径提供，不塞入初始用户消息
  - 移除单轮 completion、固定 3 次内容修复与 report 专用 token/attempt 上限
  - `DeliverHTML` 仅做路径/防覆盖/原子保存，交付 `write_html` 原文；阅读交互由提示词/模板引导模型生成，不在交付前解析改写 HTML
  - 本次未发现必须补齐的 review JSON 字段缺口
  - 独立审查后去掉交付前 SoftEnhance，以符合 Spec「交付本次写入的 HTML、首轮不解析清洗」
- 测试：
  - `go test ./internal/tool -run WriteHTML`
  - `go test ./internal/report -run 'DeliverHTML|SoftEnhance'`
  - `go test ./cmd/opencodereview -run TestReportCommand`
  - 复测入口：[html-report](../../../project-docs/retests/html-report/README.md)
- 说明：未改写 01–09 历史验收档案；用户说明见 mock/report README
