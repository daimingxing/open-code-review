# 工单 05：成果、人员与知识来源复测记录

日期：2026-10-04。工作树为 `D:\WorkPlace\open-code-review-worktrees\review-report-05-enrich`，分支为 `codex/review-report-05-enrich`。验证前已合入 `feature-review-report` 的 `3c09908`；测试时 `HEAD` 为合并提交 `f1da4548`，被测实现包含当时工作树中的改动。

## 目的与前提

使用临时 Git 仓库、受控模型和本地 stdio / HTTP MCP 服务，验证单提交报告根据真实 MCP 返回记录知识来源摘要，区分完整、部分、失败、未观察和同次版本变化；验证模块成果归纳仅依赖 Git 提交事实，失败时保留这些事实。受控服务测试不替代工单 01 的真实模型知识应用验收。

知识正文不会写入报告材料。报告只保留来源授权根下的相对标识、请求范围、观察状态、响应字节数和对原始 MCP 响应计算的 SHA-256。远端或无法映射到本地授权根的来源标记为 `source_unknown`，仍记录响应摘要但不保存路径或正文。目录与搜索工具不作为正文版本证据；只有配置中已知知识工具的启动失败或 allowlist 工具未注册会归因到知识来源，普通 GitHub/issue 工具失败不产生知识失败。

## 复测命令

PowerShell 7，在仓库根目录执行：

```powershell
$ErrorActionPreference = 'Stop'
Set-Location 'D:\WorkPlace\open-code-review-worktrees\review-report-05-enrich'
go test ./cmd/opencodereview ./internal/mcp -run 'TestReviewE2E_Report(RecordsMCPServerStartFailure|DoesNotRecordIssueMCPFailureAsKnowledgeFailure|RecordsMissingConfiguredKnowledgeTool|RecordsActualMCPContentVersionChanges|RecordsRemoteMCPReadWithUnknownSource)|TestProvider.*|TestRegisterAll.*' -count=1
go test ./...
$env:PATH = 'C:\Program Files\Git\bin;C:\Program Files\Git\usr\bin;' + $env:PATH
make check
```

## 通过条件

- 真实 MCP 响应被观察，而不是用工具调用计数代替读取；正文、MCP 返回原文、未授权绝对路径和错误诊断不进入报告材料。
- 文件正文读取成功、受限读取、MCP `IsError`、调用失败、MCP 启动失败和未观察分别保留状态；显式配置的知识工具未暴露或未注册时记录失败，非知识 MCP 服务不可用时知识来源仍为 `not_collected`。
- 同一来源的不同响应摘要形成版本变动记录；`read_multiple_files` 保留请求路径顺序，反序请求视为不同来源身份，不推断为同一来源发生变化。
- 空提交不调用成果摘要模型；普通提交的模型输入限于提交、文件、模块和有界 first-parent diff，不包含问题、需求或知识正文。模型失败或输出越界时仍保留 Git 事实。
- 不带 `--report` 时不启用材料观察器；非提交模式不推断成果和人员。

## 验证记录

2026-10-04，验证版本为已合入 `3c09908` 的实施工作树：

- 聚焦命令退出码为 0；`cmd/opencodereview` 用时 19.932 秒，`internal/mcp` 用时 2.365 秒。覆盖文件知识服务启动失败、issue 服务失败不归因知识、配置知识工具未注册、真实正文摘要与版本变化、remote 来源未知，以及 MCP provider / allowlist 注册。
- `go test ./...` 退出码为 0，25/25 个包通过；`cmd/opencodereview` 用时 175.770 秒，`internal/mcp` 用时 13.092 秒，其余包命中缓存。
- `make check` 退出码为 0，包括 license header、665 个源文件的 english-check、`go mod tidy`、`gofmt -s` 和 `go vet`。

报告中的 `application_status` 保持 `not_observed`，表示材料没有证明模型正确应用了已读取知识；这不是把未观察当成交付。真实知识应用仍按工单 01 的真实模型条件单独验收。本机 Windows 缺少 CGO/GCC 前提，未运行 race 测试；本次结果只覆盖上述受控 MCP/模型场景，不证明真实模型摘要质量。
