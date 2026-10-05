# 工单 06：不完整审查材料与失败恢复复测记录

日期：2026-10-04。工作树为 `D:\WorkPlace\open-code-review-worktrees\review-report-06-partial`，分支为 `codex/review-report-06-partial`，基线为 `feature-review-report` 集成提交 `191f84033b8d13f12d2999467f6634d0af9c4fd7`。测试使用临时 Git 仓库、本地可控模型和本地 stdio MCP，不需要外部凭据。

## 目的与前提

通过 CLI 验证提交、分支比较和工作区审查的完整、部分、跳过及完全失败语义；比较原生 JSON manifest 与独立报告材料的身份、范围、覆盖及问题；验证材料保存失败保留原生输出且不覆盖已有文件；验证提交/分支恢复产生可追溯的子运行并保持输入身份，工作区恢复仍被拒绝。测试不替代真实模型效果验收。

## 复测命令

PowerShell 7，在仓库根目录执行：

```powershell
$ErrorActionPreference = 'Stop'
$env:PATH = 'D:\WorkPlace\toolchains\go1.25.14\go\bin;' + $env:PATH
go version
go test ./internal/report -run 'TestWriteMaterial' -count=1
go test ./cmd/opencodereview -run 'TestReviewE2E_Report(SaveFailurePreservesNativeOutput|PartialAndFailedOutcomesAcrossModes|TimeoutPreservesPartialOutcomeAcrossModes|TokenBudgetFailureIsNotSuccessAcrossModes|SkippedIsNotAnEmptySuccessfulReview|ResumePreservesIdentityAndScope|DoesNotAddWorkspaceResume|KnowledgeFailurePreservesCodeFinding|MarksRestrictedMCPReadPartial)$' -count=1
```

普通完整测试与仓库检查：

```powershell
$ErrorActionPreference = 'Stop'
$env:PATH = 'D:\WorkPlace\toolchains\go1.25.14\go\bin;C:\Program Files\Git\bin;C:\Program Files\Git\usr\bin;' + $env:PATH
go test ./... -count=1
$make = 'D:\WorkPlace\toolchains\make-4.4.1\bin\make.exe'
& $make check
```

## 通过条件

- 六种“模式 × 部分/完全失败”组合均由 CLI 检查原生 manifest 和报告材料；部分结果保留成功覆盖，完全失败返回非零并带失败状态，不会把空问题数组显示成审查通过。
- 工作区无选中项时 native/report 都是 `skipped`。知识 MCP `IsError` 不导致代码审查失败；已有代码问题保留，知识来源记录 `failed`，`application_status` 保持 `not_observed`。
- 原生与材料的 run id、状态、覆盖集合一致；材料安全保存失败时原生 JSON 仍完整且显式目标原内容不变。
- 三种模式的单文件模型超时保留其余完成项、finding 和 `partial` 状态；`--max-tokens-budget 1` 使每个选中项都记录 `budget` 失败、原生及材料状态为 `failed`、没有 finding 且 CLI 非零退出。
- 临时材料写入失败或最终硬链接发布失败时不留下最终报告；硬链接不受支持时返回明确失败，自动命名不会留下编号材料，已存在目标原内容不变。常见 NTFS 和其他支持硬链接的文件系统沿用完整临时文件后原子发布的行为。
- `--resume` 只覆盖原生支持的 commit/range，父子运行 ID、完整 SHA、精确范围、来源工件摘要及材料字段一致；仓库身份缺失时材料如实标 `not_collected`，不补造值。workspace resume 报错且不生成材料。
- 聚焦测试、`go test ./... -count=1` 和 `make check` 的退出码为 0；任何未运行项单独说明，不能由聚焦测试推断完整通过。

## 验证记录

2026-10-04，Windows、PowerShell 7、Go 1.25.14。本分支当前已执行：

- 首轮聚焦验证已发现一个测试断言问题：测试要求 manifest 未提供的仓库 SHA 出现在材料中；材料的 `not_collected` 状态符合现有契约。断言已修正为不得凭空生成仓库身份。
- `go test ./cmd/opencodereview -run '^TestReviewE2E_ReportResumePreservesIdentityAndScope$' -count=1` 退出码 0，commit 和 range 恢复均通过。
- `go test ./cmd/opencodereview -run '^TestReviewE2E_ReportKnowledgeFailurePreservesCodeFinding$' -count=1` 退出码 0。真实本地 stdio MCP 对不存在文件返回 `IsError`；native finding 未被清除，材料报告知识读取失败且未证明知识应用。
- 最终聚焦命令 `go test ./cmd/opencodereview -run 'TestReviewE2E_Report(SaveFailurePreservesNativeOutput|PartialAndFailedOutcomesAcrossModes|SkippedIsNotAnEmptySuccessfulReview|ResumePreservesIdentityAndScope|DoesNotAddWorkspaceResume|KnowledgeFailurePreservesCodeFinding|MarksRestrictedMCPReadPartial)$' -count=1` 于 2026-10-04 退出码 0，包用时 52.739 秒。
- 新增 timeout/budget 命令 `go test ./cmd/opencodereview -run 'TestReviewE2E_Report(TimeoutPreservesPartialOutcomeAcrossModes|TokenBudgetFailureIsNotSuccessAcrossModes)$' -count=1` 于 2026-10-04 退出码 0，包用时 23.385 秒；timeout 和预算耗尽在 commit/range/workspace 三模式都通过，原生结果与材料状态、覆盖一致。
- 上一版的最终路径复制回退与目标写入/`Sync`/`Close` 清理测试已删除。当前命令 `go test ./internal/report -run 'TestWriteMaterial' -count=1` 于 2026-10-04 退出码 0，包用时 1.921 秒：临时文件写入、`Sync` 或 `Close` 失败都不发布目标；注入硬链接不支持时显式输出无目标，自动输出保留预先存在内容且不留下编号目标；现有并发自动命名测试覆盖支持硬链接文件系统上的原子发布。
- 最终聚焦命令 `go test ./cmd/opencodereview -run 'TestReviewE2E_Report(SaveFailurePreservesNativeOutput|PartialAndFailedOutcomesAcrossModes|TimeoutPreservesPartialOutcomeAcrossModes|TokenBudgetFailureIsNotSuccessAcrossModes|SkippedIsNotAnEmptySuccessfulReview|ResumePreservesIdentityAndScope|DoesNotAddWorkspaceResume|KnowledgeFailurePreservesCodeFinding|MarksRestrictedMCPReadPartial)$' -count=1` 于 2026-10-04 退出码 0，包用时 74.506 秒。
- 当前 link-only 版本 `go test ./... -count=1` 于 2026-10-04 退出码 0，26 个包通过；`cmd/opencodereview` 用时 268.380 秒，`internal/report` 用时 2.758 秒。
- 当前 link-only 版本的 `make check` 于 2026-10-04 退出码 0；`git diff --check` 退出码 0，未发现空白错误；`go mod tidy` 未改变模块文件。
- 新增提交的独立复审及目标分支集成尚未完成；本记录只报告当前实施分支证据，主线程仍需在最终集成版本验收。

本记录的 CLI E2E 使用可控本地模型故障，不替代真实模型效果验收。模型超时由本地 HTTP 服务对 `b.go` 主审查请求延迟 1.5 秒并设置 `OCR_LLM_TIMEOUT=1` 驱动；预算耗尽通过 CLI `--max-tokens-budget 1` 驱动，未依赖共享 native 单元测试推断材料行为。报告保存依赖同目录硬链接来原子发布；不支持硬链接的文件系统会返回错误且不生成报告材料，常见 NTFS 与其他支持硬链接的文件系统行为不变。

人工重放 commit/range/workspace 的真实服务路径仍需主机上已授权的 OCR 模型与知识服务配置；按[外部知识历史复测记录](knowledge-access.md)准备并隔离服务。模型与知识凭据不应写入命令文件或报告材料。
