# 报告材料回归

覆盖 `ocr review --report` 的单提交、分支比较和工作区模式，包括原生输出兼容、身份与范围、Git 统计、知识来源、部分结果、失败恢复和安全保存。测试使用临时仓库与受控服务，不调用真实模型。

从仓库根目录使用 PowerShell 7 和 PATH 中的全局 Go 1.25+：

```powershell
go test ./internal/report -count=1
if ($LASTEXITCODE -ne 0) { throw '报告材料包测试失败' }
go test ./cmd/opencodereview -run '^Test(BuildReportMaterial|DefaultReportPath|SanitizeReportLabel|CollectBranchGitStatistics|ParseWorkspaceStatus|ReviewE2E_(Report|WorkspaceReport|CommitReport))' -count=1 -timeout=10m
if ($LASTEXITCODE -ne 0) { throw '报告材料 CLI 回归失败' }
```

预期两条命令退出码均为 0。对应本次集成的历史结果见[验收档案索引](../archive/README.md)。
