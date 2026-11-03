# 工单 04：工作区报告材料复测记录

日期：2026-10-04。验证对象为 `codex/review-report-04-workspace`，基线为 `feature-review-report` 的 `1e667f5`。

## 目的与前提

验证 `ocr review --report` 在工作区模式下记录审查启动时的 HEAD、暂存/未暂存、未跟踪和删除状态，并将快照摘要绑定原生 `SourceArtifactSHA256`；验证干净工作区的零文件事实、默认 `workspace` 命名，以及未启用报告时原生行为不变。

在 PowerShell 7、PATH 中的全局 Go 与 Make、Git 和仓库依赖可用的环境中，从仓库根目录运行。当时验收为 Go 1.25.14。自动化测试使用临时 Git 仓库和仓库内受控模型服务，不代表真实模型效果。

## 复测命令

```powershell
$ErrorActionPreference = 'Stop'
Set-Location 'D:\WorkPlace\open-code-review-worktrees\review-report-04-workspace'
go test ./cmd/opencodereview -run 'Test(ParseWorkspaceStatus|ReviewE2E_WorkspaceReport|DefaultReportPathUsesWorkspaceLabel)' -count=1
go test ./... -count=1
go vet ./...
```

运行仓库静态检查：

```powershell
$env:PATH = 'C:\Program Files\Git\bin;C:\Program Files\Git\usr\bin;' + $env:PATH
make check
```

## 通过条件

- 定向测试、全量普通测试、`go vet` 和 `make check` 退出码均为 0。
- 混合场景材料的 `workspace_snapshot.status` 为 `provided`，状态条目分别包含 staged、unstaged、untracked 和 deleted；`snapshot_sha256`、`status_sha256`、`captured_at`、HEAD 与 `source_artifact_sha256` 均存在。
- 干净工作区仍生成 `provided` 快照，`selected_count` 和状态条目为 0；这表示确认为零，而不是未采集。
- 快照采集仅在显式传入 `--report` 时执行；未启用报告的原生 JSON 与退出行为保持兼容。

## 验证记录

2026-10-04：修复 unborn HEAD 快照误报后，定向测试 5 个通过；`go test ./... -count=1` 通过（5133 个测试/25 个包）；`go vet ./...` 和 `make check` 通过。测试未运行 race 检查，因为本机缺少 `CGO_ENABLED=1` 与 GCC；工作区恢复仍沿用原生不支持的限制。修复提交为 `a266694`。
