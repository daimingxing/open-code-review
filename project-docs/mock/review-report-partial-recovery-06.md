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
go test ./cmd/opencodereview -run 'TestReviewE2E_Report(SaveFailurePreservesNativeOutput|PartialAndFailedOutcomesAcrossModes|SkippedIsNotAnEmptySuccessfulReview|ResumePreservesIdentityAndScope|DoesNotAddWorkspaceResume|KnowledgeFailurePreservesCodeFinding|MarksRestrictedMCPReadPartial)$' -count=1
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
- `--resume` 只覆盖原生支持的 commit/range，父子运行 ID、完整 SHA、精确范围、来源工件摘要及材料字段一致；仓库身份缺失时材料如实标 `not_collected`，不补造值。workspace resume 报错且不生成材料。
- 聚焦测试、`go test ./... -count=1` 和 `make check` 的退出码为 0；任何未运行项单独说明，不能由聚焦测试推断完整通过。

## 验证记录

2026-10-04，Windows、PowerShell 7、Go 1.25.14。本分支当前已执行：

- 首轮聚焦验证已发现一个测试断言问题：测试要求 manifest 未提供的仓库 SHA 出现在材料中；材料的 `not_collected` 状态符合现有契约。断言已修正为不得凭空生成仓库身份。
- `go test ./cmd/opencodereview -run '^TestReviewE2E_ReportResumePreservesIdentityAndScope$' -count=1` 退出码 0，commit 和 range 恢复均通过。
- `go test ./cmd/opencodereview -run '^TestReviewE2E_ReportKnowledgeFailurePreservesCodeFinding$' -count=1` 退出码 0。真实本地 stdio MCP 对不存在文件返回 `IsError`；native finding 未被清除，材料报告知识读取失败且未证明知识应用。
- 最终聚焦命令 `go test ./cmd/opencodereview -run 'TestReviewE2E_Report(SaveFailurePreservesNativeOutput|PartialAndFailedOutcomesAcrossModes|SkippedIsNotAnEmptySuccessfulReview|ResumePreservesIdentityAndScope|DoesNotAddWorkspaceResume|KnowledgeFailurePreservesCodeFinding|MarksRestrictedMCPReadPartial)$' -count=1` 于 2026-10-04 退出码 0，包用时 52.739 秒。
- `go test ./... -count=1`、`make check` 和独立审查尚未运行；本记录不将其标为通过，由主线程在最终集成版本复测。

本次矩阵通过模型服务永久拒绝请求来验证任务级失败及完全失败，未单独驱动 CLI 的模型超时或 token budget 截止路径；已有共享 native budget 输出单元测试不等价于三模式报告材料端到端验收，后续整体验收应明确保留此差异。

人工重放 commit/range/workspace 的真实服务路径仍需主机上已授权的 OCR 模型与知识服务配置；按 [工单 01 复测记录](external-knowledge-01.md) 准备并隔离服务。模型与知识凭据不应写入命令文件或报告材料。
