# 03：分支比较生成准确范围的报告材料

Status: ready-for-agent

Blocked by: [02：单提交审查生成独立报告材料](02-commit-report-material.md)

阶段：2。规格来源：[Spec](../spec.md)；执行约定及依赖总览：[工单索引](../README.md)。关联用户故事：2、10、18、21、31、34。

## 交付行为

用户审查两个分支时，得到按原生比较语义生成的材料及可追溯的范围与统计。

## 验收条件

- [x] 复用原生 merge-base 到目标提交的审查范围；保存双方原始引用、完整 SHA 和实际比较起点，代码证据来自正确版本。
- [x] 默认名称使用双方分支标签与启动时间，覆盖包含斜线、空格、非法字符和长标签的名称；沿用已经实现的安全保存规则。
- [x] 报告材料区分最终差异统计与逐提交累计统计，保存实际选中、完成、失败、跳过范围及原因，不能把选中任务完成当作全部变更已审查。
- [x] 人员基础事实区分 Git 作者与提交者，不推断推送者或缺陷责任；各统计有明确口径。
- [x] 使用具有分叉历史的临时仓库完成 CLI 验证，断言范围、文件与提交证据；验证不启用报告时原生行为不变。

## 执行记录（2026-10-04）

- 执行者：主线程 Codex（实施智能体未在规定时间内形成可交付修改后由主线程接续）。工作树：`D:\WorkPlace\open-code-review-worktrees\review-report-03-branch`；分支：`codex/review-report-03-branch`；基线：`74d4b985c2a0aa84e2639634bf32b7213213dd1a`（`feature-review-report` 工单 02 集成提交）。
- 交付范围：`buildReportMaterial` 复用原生 `report.Scope` 的原始引用、解析后的 merge-base/head 和 exact range；新增只读 Git 统计采集器，分别保存 `final_diff`、逐提交首父差异累计、提交 SHA/父提交、Git author/committer 及时间；分支默认报告名使用双方标签和启动时间。统计采集失败时保留可编码的部分数据并将区段标记 `failed`，不会伪造完整统计。
- 原生兼容：未传 `--report` 的分支比较仍走原生输出路径；报告生成只在显式启用报告时调用材料构建器和 Git 统计。
- 已运行：`go test ./cmd/opencodereview -run 'Test(DefaultReportPathIncludesSanitizedRangeLabelsAndStartTime|CollectBranchGitStatisticsUsesResolvedRangeAndDistinctGitActors|ReviewE2E_ReportRangeUsesMergeBaseAndPreservesNativeCoverage|ReviewE2E_RangeWithoutReportKeepsNativeBehavior)' -count=1`，4 个测试通过。临时仓库包含 `release/main` 与 `feature/report` 分叉、两个 feature 提交及独立 release 提交；断言 merge-base、完整 SHA、最终两文件六行净增、逐提交累计、作者/提交者差异、覆盖集合和无报告原生行为。
- 已验证：`go test ./... -count=1` 通过（5128 个测试/25 包），`go vet ./...` 通过，`make check` 通过（license、english-check、`go mod tidy`、gofmt 和 vet 均通过）。`make test` 的 race 前提仍需 CGO 与 GCC，未在 Windows 本机验证。复测命令见[分支报告材料复测记录](../../../project-docs/mock/review-report-branch-material-03.md)。
- 独立 Spec 审查：实现逻辑和验收条件通过，无伪造事实的阻塞问题；复核指出的 coverage 断言过弱已改为完整集合 `reflect.DeepEqual`，复测记录与执行记录状态已统一。独立 Standards 审查：无阻塞级代码或安全问题，建议通过。

