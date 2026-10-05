# 06：交付不完整审查材料与失败恢复行为

Status: ready-for-agent

Blocked by: [03：分支比较生成准确范围的报告材料](03-branch-report-material.md)；[04：工作区审查生成真实快照的报告材料](04-workspace-report-material.md)；[05：补齐成果、人员与知识来源材料](05-enrich-report-material.md)

阶段：2。规格来源：[Spec](../spec.md)；执行约定及依赖总览：[工单索引](../README.md)。关联用户故事：12、13、14、21、31、38、39、40、41。

## 交付行为

审查部分完成、知识缺失或材料整理失败时，用户能保留有效成果、理解限制，并得到明确的后续操作说明。

## 验收条件

- [x] 完整、部分完成、跳过和完全失败有明确可观察状态；有有效事实的部分结果可保存为材料，完全失败或空问题数组不能冒充审查通过。
- [x] 服务不可用、模型超时、预算耗尽、部分任务失败及材料整理失败时，保留原生结果并按契约返回状态和诊断；不得留下可被误认为完整成品的报告材料。CLI/material E2E 覆盖三模式模型超时部分结果与 token budget 全选项失败；临时文件写入及硬链接发布失败均不留下最终报告，既有目标不变。不支持硬链接的文件系统明确返回失败且不生成材料。
- [x] 保留实际覆盖、失败原因与未执行验证，知识缺失不抹去充分代码证据支持的问题，也不声称已完成依赖知识的校验。
- [x] 对原生支持恢复的提交和分支模式验证运行身份、范围及材料一致性，沿用原生恢复限制；不新增仅重做材料命令，不为工作区虚构恢复能力。
- [x] 整合 03、04 的范围材料与 05 的成果整理，使三种模式使用一致的材料契约和失败语义；在临时仓库和可控模型下验证完整及不完整材料、原生输出兼容、安全保存与恢复限制，完成后才进入 HTML 阶段。

## 执行记录（2026-10-04）

- 工作树：`D:\WorkPlace\open-code-review-worktrees\review-report-06-partial`；分支：`codex/review-report-06-partial`；基线：`191f84033b8d13f12d2999467f6634d0af9c4fd7`。
- 新增 CLI E2E 验收覆盖三种模式下部分与完全失败，比较原生 JSON manifest 和材料中的状态、运行 ID、范围、覆盖及 finding；覆盖工作区 `skipped`、真实 MCP `IsError` 时保留有依据的问题、三种模式材料保存失败时保留原生 JSON 并避免覆盖；commit/range 恢复比较 parent run、SHA 范围、source artifact 和材料，workspace resume 仍明确拒绝。
- 已完成 `TestReviewE2E_ReportResumePreservesIdentityAndScope`（commit/range，exit 0）及 `TestReviewE2E_ReportKnowledgeFailurePreservesCodeFinding`（本地 stdio MCP，exit 0）。复测命令与本次状态见[历史记录](../../../project-docs/retests/archive/review-report-2026-10/report-material-recovery.md)及[当前复测入口](../../../project-docs/retests/report-material/README.md)。
- 全聚焦 CLI/material 命令于 2026-10-04 退出码 0：`internal/report` 相关测试用时 1.846 秒，`cmd/opencodereview` 三模式 E2E 矩阵用时 74.506 秒。timeout 每种模式保留 3 项完成、1 项 `b.go` timeout、有效 finding 和 `partial` 材料；`--max-tokens-budget 1` 每种模式都为所有选中项记录 `budget` 失败，native/material 状态为 `failed`、不含 findings 且命令返回错误。材料保存失败注入验证写入/Sync/Close 失败不残留新目标，路径被替换时保留替换文件，显式已存在目标不变。
- 实施分支全量 `go test ./... -count=1` 于 2026-10-04 退出码 0（26 包）；`make check` 退出码 0。第三轮独立复审检查 `48cf83a`，确认超时/预算验收完整，最终路径采用临时文件完成写入后 `os.Link` 独占发布，无阻塞问题。
- 已按 Merge 约定集成至 `feature-review-report`，Merge 提交 `5de6fb8`。目标分支重跑 `go test ./internal/report -run 'TestWriteMaterial' -count=1`（10 项，1.744 秒）及完整 06 CLI E2E 矩阵（26 项，90.047 秒），均退出码 0；`git diff --check` 通过。工单 07 的前置门已满足。

## Comments

