# 工单 03：分支报告材料复测记录

日期：2026-10-04。验证对象为工单 02 集成后的 `codex/review-report-03-branch`，集成前应确认该提交已合入 `feature-review-report`。

## 目的与前提

验证 `ocr review --from <ref> --to <ref> --report` 使用原生 merge-base 和解析后的目标提交，报告材料同时保留原始引用、最终净差异、逐提交累计、Git 作者/提交者和原生覆盖集合；验证未传 `--report` 时原生输出不变。

在 PowerShell 7、Go 1.25.14、Git 和仓库依赖可用的环境中，从仓库根目录运行。测试使用临时 Git 仓库和仓库内受控模型服务，不代表真实模型或外部知识服务效果。

## 复测命令

```powershell
$ErrorActionPreference = 'Stop'
Set-Location 'D:\WorkPlace\open-code-review'
$env:PATH = 'D:\WorkPlace\toolchains\go1.25.14\go\bin;' + $env:PATH
git switch feature-review-report
go test ./cmd/opencodereview -run 'Test(DefaultReportPathIncludesSanitizedRangeLabelsAndStartTime|CollectBranchGitStatisticsUsesResolvedRangeAndDistinctGitActors|ReviewE2E_ReportRangeUsesMergeBaseAndPreservesNativeCoverage|ReviewE2E_RangeWithoutReportKeepsNativeBehavior)' -count=1
```

运行完整普通测试和静态检查：

```powershell
go test ./... -count=1
go vet ./...
$env:PATH = 'D:\WorkPlace\toolchains\go1.25.14\go\bin;C:\Program Files\Git\bin;C:\Program Files\Git\usr\bin;' + $env:PATH
$make = 'D:\WorkPlace\toolchains\make-4.4.1\bin\make.exe'
& $make check
```

## 通过条件

- 聚焦测试退出码为 0，并报告 `4 passed`；测试材料中的 `Scope.RequestedFrom/RequestedHead` 为原始分支标签，`ResolvedBase/ResolvedHead` 为完整 SHA，`ExactRange` 为实际 `base..head`。
- `final_diff` 只表示 base 到 head 的净差异；`per_commit_cumulative` 和每个提交的首父差异单独存在。作者和提交者字段分别来自 Git，不能互相覆盖。
- 原生覆盖的 selected/completed/failed/waived 集合在报告材料中保持一致；不传 `--report` 时不产生报告 JSON，原生 JSON 仍可解析。
- 完整普通测试、`go vet` 和 `make check` 均退出码为 0 才能记录为已验证。`make test` 需要 `CGO_ENABLED=1` 与 GCC；缺少 GCC 时只能记录未验证，不能作为通过。

## 验证记录

2026-10-04：聚焦测试已实际执行并通过（4 个测试）；coverage 断言覆盖 selected/completed/reused/failed/waived 全集合。完整普通测试 `go test ./... -count=1` 通过（5128 个测试/25 包），`go vet ./...` 通过，`make check` 通过。独立 Spec 与 Standards 审查均无阻塞；测试不调用真实模型，真实知识读取仍以工单 01 的证据为准。Windows race 测试因本机未提供 CGO/GCC 未验证。
