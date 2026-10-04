# 09：报告阅读体验与长报告可靠性

Status: ready-for-agent

Blocked by: [08：多份报告材料汇总为一份 HTML](08-multi-input-html-report.md)

阶段：3。规格来源：[Spec](../spec.md)；执行约定及依赖总览：[工单索引](../README.md)。关联用户故事：29、30、32、36、38、40、41。

## 交付行为

为单份与多份报告交付筛选、折叠、窄屏和打印体验，并在大量问题或生成异常时完整交付或明确失败，支持凭已有材料有界重试。

## 验收条件

- [x] 覆盖大量问题、多仓库、长证据文本和长中文说明；不截断问题集合，超出能力或预算时明确失败。
- [x] 对 timeout、截断 HTML、漏问题、改等级、错误统计及无效输出执行有次数/预算上限的修复；最终失败保留诊断且不发布成功文件。
- [x] 分别记录材料与 HTML 阶段的模型用量及耗时；修复请求复用原 JSON，不重新审查或收集代码/知识。
- [x] 实现按仓库或审查单元、等级和类别筛选，以及可键盘访问的详情折叠；窄屏和打印布局已由长短单份/多份报告浏览器验收，未发现事实丢失、横溢或外联。
- [x] 可控 CLI 覆盖失败恢复、三次耗尽、同输入与不发布；真实 DeepSeek Flash 运行经一次事实诊断修复后成功生成 HTML，并用 Edge 检查真实离线文件。初版不固定标题、标签、布局或 finding 的 HTML 元素，只要求章节、finding、基本说明/统计、证据和安全事实正确。

## 执行记录（2026-10-05）

- 工作树：`C:\Users\60429\.codex\worktrees\review-report-09-experience\open-code-review`；分支 `codex/review-report-09-experience`；基线 `e66aeaef2c4f554b0f36ed788d47bbc9fab56237`。实现提交 `75066c6083964eb22619ea3db82a5f45f47bdfce` 经独立审查发现可见输出额度与 provider 总额度混用、原始 provider 错误可能泄露至重试提示两个问题；主线程已在该分支修复并补充定向测试，修复提交待复核后集成。
- 报告 writer 将事实准备/校验与原子安全发布分离，保留 07/08 的 `WriteHTML`、`WriteMultiHTML` 校验和不覆盖语义；添加报告体验包装、单/多份筛选、可键盘操作的详情折叠、focus-visible、窄屏和打印规则。
- `ocr report` 只重试已有材料，最多 3 次、总计 10 分钟；每次请求 timeout 取 endpoint 限制与剩余总时限较小值。预算硬限制：输入 128,000 tokens、每次可见输出 36,864 tokens、每次 provider completion 最多 98,304 tokens、所有请求 completion usage 总计 196,608 tokens；输入统计包含系统提示、原始材料、上一轮无效 HTML 草稿和修复诊断，单次请求 completion 上限还受剩余累计额度约束。任一超限明确失败。初始提示明确诊断与材料皆为不可信数据；修复提示把上一轮 HTML 作为不可信 assistant 草稿，并用独立 user 消息发送安全分类；不拼接原始 provider 错误正文、finish_reason 或事实值；provider 错误保留 HTTP 状态类别，调用者取消会停止重试。
- 受控验证：`go test ./internal/report -count=1 -timeout=90s` 通过；报告 CLI 聚焦测试组通过（含长报告 34,301 个可见 token / 70,000 completion token 预算断言、累计预算、敏感 provider 错误正文和取消后停止重试）。审查修复复核与最终集成版本复测结果待补。
- Edge `154.0.4258.53` 控制材料验收覆盖短/长单份及多份报告，实际切换审查单元/仓库、等级与类别，键盘操作 radio/details，并检查打印、事实数、安全 DOM、页面错误、外联及 375px 溢出。长单份为 60 个问题/702 个事实，长多份为 60/743；四份报告均通过，打印显示全部详情。临时浏览器产物位于 `C:\Users\60429\AppData\Local\Temp\ocr-report-09-browser-290c46abf9774666bdb2ac732501738f`。
- 真实模型最新运行使用已有前端 JSON，模型响应仍未通过章节/标题和其他 HTML 校验；3 次后 exit 1，耗时 3:12.603，累计输入/completion/可见输出分别为 17,818/57,028/24,814 tokens，没有发布 HTML。该运行发生在最近提示增强之前；禁止据此宣称真实模型视觉验收通过。提示增强后的真实调用由主线程审查后决定是否执行。
- 初版验收澄清后的一次复测因安全诊断过于笼统，3 次后 exit 1、耗时 2:09.936，累计 input/completion/visible 为 37,830/43,049/28,681 tokens，未发布文件。主线程补上错误事实值、未知事实键和 finding 放置位置的安全诊断，并增加分类测试；这能让修复请求指向具体问题，不改变核心内容/安全检查。
- 同一前端 JSON 与隔离配置的真实复测成功：DeepSeek Flash 共 2 次请求，HTML 阶段耗时 1:34.235，累计 input/completion/visible 为 21,927/30,418/18,971 tokens，completion usage 均由服务报告，exit 0 且发布 HTML。首轮报 `sections.achievements.data.model_summary_status` 值不符，第二轮修复成功。成品为 38,163 bytes，SHA-256 `CA641A8B2954348872D5FD79EBFBF7FC82BC779A14E2F0B215BB08FFA97CCA5C`，仅在临时目录，不入库。
- Edge `154.0.4258.53` 打开真实 `file://` 成品检查：1 条 finding、161 个事实标记；1440px 和 375px 无横溢，筛选/键盘 radio/details/打印可用，页面错误、危险 DOM、外联请求均为 0。实际首屏见复测记录所列临时截图；页面信息密集但基本可读，作为初版参考，不构成视觉模板。
- CLI 全包 `go test ./internal/report ./cmd/opencodereview -count=1 -timeout=180s` 中 `internal/report` 通过，CLI 包达到 180 秒包级测试上限。`go test -json ./cmd/opencodereview -count=1 -timeout=45s` 最后停留于既有 `TestReviewE2E_JSONHumanStreamsProgressToStderr`，堆栈落于 `retryTestGit` 的 `exec.Cmd.CombinedOutput`；此测试在 e66 基线已存在，单独运行通过（6.365s）。未确认全包超时根因，不修改无关 E2E。详情见[工单 09 复测记录](../../../project-docs/mock/review-report-experience-09.md)。
- 运行命令、预算限制、实际结果、浏览器脚本与隔离真实模型复测前提见[工单 09 复测记录](../../../project-docs/mock/review-report-experience-09.md)，已加入[复测索引](../../../project-docs/mock/README.md)。

