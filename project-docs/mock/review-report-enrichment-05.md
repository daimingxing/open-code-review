# 工单 05：成果、人员与知识来源复测记录

日期：2026-10-04。验证对象为 `codex/review-report-05-enrich`，基线为 `feature-review-report` 的 `8251970`。

## 目的与前提

验证单提交报告材料从 Git 提交事实取得成果、文件、模块、作者和提交者，且不把修改者当作缺陷责任人；验证知识来源区分未观察、成功调用但未证明正文应用、部分失败和完全失败。测试使用临时 Git 仓库和受控模型服务，不代替工单 01 的真实模型与文件 MCP 效果验收。

## 复测命令

```powershell
$ErrorActionPreference = 'Stop'
Set-Location 'D:\WorkPlace\open-code-review-worktrees\review-report-05-enrich'
$env:PATH = 'D:\WorkPlace\toolchains\go1.25.14\go\bin;' + $env:PATH
go test ./cmd/opencodereview -run 'Test(ReviewE2E_CommitReportContainsEvidenceBackedEnrichment|ReviewE2E_CommitWithoutReportKeepsNativeBehavior|KnowledgeSourcesSectionDistinguishesObservedPartialAndFailure|EnrichReportSectionsDoesNotInferNonCommitResults|CollectCommitMaterialUsesFirstParentForMergeCommit)' -count=1
go test ./... -count=1
go vet ./...
```

仓库标准检查：

```powershell
$env:PATH = 'D:\WorkPlace\toolchains\go1.25.14\go\bin;C:\Program Files\Git\bin;C:\Program Files\Git\usr\bin;' + $env:PATH
$make = 'D:\WorkPlace\toolchains\make-4.4.1\bin\make.exe'
& $make check
```

## 通过条件

- 单提交材料中 `achievements`、`people` 为 `provided`，提交 SHA、文件、模块、Git author/committer 和中文摘要可回溯；`Basis` 明确不表示推送者、工时或缺陷责任。
- 没有外部知识工具调用时 `knowledge_sources` 为 `not_collected` 并说明未观察；有成功调用但没有正文版本时只记录调用计数和“不能证明正确应用”；混合失败为 `failed` 且保留成功计数。
- 非提交模式的成果/人员不被推断，结构检查保持未提供；未知不转成零。
- 聚焦测试、全量普通测试、`go vet` 和 `make check` 退出码均为 0 才记录为已验证。race 测试需要 CGO/GCC，缺失时记录未验证。

## 验证记录

2026-10-04：Spec 复审指出工具总调用数含失败尝试、merge commit `diff-tree` 不提供 first-parent 文件以及材料阶段耗时/用量未明确；实现均已修复并新增回归测试。修复后聚焦测试 5 个通过，全量普通测试 `go test ./... -count=1` 通过（5137 个测试/25 个包），`go vet` 和 `make check` 通过（665 个源文件英文检查）。`ProjectSummary` 为空时材料明确记录模型成果归纳未采集；知识正文版本和正确应用没有可验证事实时明确标记 `not_observed`。race 测试仍需 CGO/GCC；真实知识正文读取与应用仍以工单 01 的真实模型证据为准。
