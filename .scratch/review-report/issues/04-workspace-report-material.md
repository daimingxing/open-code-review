# 04：工作区审查生成真实快照的报告材料

Status: ready-for-agent

Blocked by: [02：单提交审查生成独立报告材料](02-commit-report-material.md)

阶段：2。规格来源：[Spec](../spec.md)；执行约定及依赖总览：[工单索引](../README.md)。关联用户故事：2、18、21、31、34。

## 交付行为

用户审查未提交变更时，材料准确反映本次工作区范围，并能区别于同一 HEAD 上的其他运行。

## 验收条件

- [x] 沿用原生暂存、未暂存和未跟踪变更的范围及过滤行为，材料记录实际审查内容和覆盖。
- [x] 保存实际审查快照标识，不能只用 HEAD 代表未提交修改；工作区在运行过程中变化时仍可说明证据对应的范围。
- [x] 默认名称使用工作区标签和启动时间，沿用安全保存规则；不伪造提交数或将未提交修改归于 HEAD 作者。
- [x] 验证混合暂存与未暂存变更、未跟踪文件、文件删除和零可审查文件；未知、缺失与确认为零分别表示。
- [x] 通过 CLI 检查生成材料及退出状态，不假定工作区支持原生恢复；不启用报告时保持原生行为。

## 执行记录（2026-10-04）

- 执行者：Codex。工作树：`D:\WorkPlace\open-code-review-worktrees\review-report-04-workspace`；分支：`codex/review-report-04-workspace`；基线：`1e667f5`（`feature-review-report` 已集成工单 03）。
- 交付范围：审查启动前采集 `git status --porcelain=v1 -z --no-renames --untracked-files=all`、HEAD 和状态摘要，报告 `workspace_snapshot` 保存状态条目、暂存/工作区状态、未跟踪及删除路径、快照摘要、采集时间、原生选中数量和 `SourceArtifactSHA256`。快照摘要同时绑定状态摘要和实际审查工件摘要，不能以 HEAD 单独代表工作区变更；采集失败只将该分区标记为 `failed`，不改变原生审查。
- 默认报告名称沿用共享 `workspace` 标签和启动时间；未传 `--report` 不采集快照、不改变原生输出路径或退出行为。工作区 `SourceArtifact` 保留原生选择结果，干净工作区以 `provided`、零选中条目表达，不把零条目当作缺失。
- 已运行：定向 CLI 测试 `go test ./cmd/opencodereview -run 'Test(ParseWorkspaceStatus|ReviewE2E_WorkspaceReport|DefaultReportPathUsesWorkspaceLabel)' -count=1` 通过；全量 `go test ./... -count=1` 通过（25 个包）；`go vet ./...` 通过；`make check` 通过（license、english-check、`go mod tidy`、gofmt、vet）。复测命令见[工作区报告材料复测记录](../../../project-docs/mock/review-report-workspace-material-04.md)。
- 限制：本机未启用 `make test` 的 race 检查（需要 `CGO_ENABLED=1` 与 GCC）；测试使用仓库内受控模型，不代表真实模型效果。工作区不增加原生恢复能力；运行期间文件变动由启动时快照和工件摘要分别披露，后续报告阶段不重新读取工作区。
- 独立审查修复：Standards 审查发现 unborn/异常仓库的 `HEAD` 解析错误被忽略，已在 `a266694` 中改为快照采集失败并新增回归测试；复核结论为无阻塞。
- 主线程复测：修复后聚焦测试 5 个通过，`go test ./... -count=1` 通过（5133 个测试/25 包），`go vet ./...` 和 `make check` 通过。提交前 race 仍受 Windows CGO/GCC 前提限制。

