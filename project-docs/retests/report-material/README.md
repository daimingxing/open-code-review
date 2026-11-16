# 报告材料回归

覆盖 `ocr review --report` 的单提交、分支比较和工作区模式，包括原生输出兼容、身份与范围、Git 统计、知识来源、部分结果、失败恢复和安全保存。测试使用临时仓库与受控服务，不调用真实模型。

从仓库根目录使用 PowerShell 7 和 PATH 中的全局 Go 1.25+：

```powershell
make test 'PACKAGES=./internal/report'
if ($LASTEXITCODE -ne 0) { throw '报告材料包测试失败' }
make test "PACKAGES=./cmd/opencodereview -run 'Test(BuildReportMaterial|DefaultReportPath|PrepareReportTarget|SaveReportMaterial|SanitizeReportLabel|CollectBranchGitStatistics|ParseWorkspaceStatus|ReviewE2E_(Report|WorkspaceReport|CommitReport))' -timeout=10m"
if ($LASTEXITCODE -ne 0) { throw '报告材料 CLI 回归失败' }
```

预期两条命令退出码均为 0。对应本次集成的历史结果见[验收档案索引](../archive/README.md)。

## 2026-11-16 路径与搜索边界

基于 `878bd8f` 的 `feature/review-output-safety` 工作区：受影响的 `internal/tool`、`internal/config/allowlist`、`internal/report`、`cmd/opencodereview` 四包完整回归通过。新增样例覆盖 SVG 正文排除与引用保留、64 KiB 搜索输出限制、模型调用前准备报告目录、扩展名归一化、目录和文件名错误回退、三种审查模式同名编号及运行中目标失效恢复。Windows 保留名称另行定向复测。

本机 `CGO_ENABLED=0`，通过 `make test 'PACKAGES=./internal/tool ./internal/config/allowlist ./internal/report ./cmd/opencodereview -race=false'` 显式关闭 race；测试使用隔离目录与受控模型。`go vet` 受影响四包通过，`make build` 通过。`make check` 的许可证检查通过，英文检查仍受历史 `archive/review-report-2026-10/manual-sample` 中 31 行中文阻断，本次未扩大豁免。未重跑真实模型审查。

## 问题提交人

`findings[].committer_names` 由固定目标提交中的问题定位行采集，不调用模型；允许多人并去重。工作区、无定位、目标文件或行号缺失时保留空数组和限制说明。HTML 模板按每条问题展示全部姓名或“未确定”，旧材料缺少字段仍兼容。

`TestCollectFindingCommittersUsesPinnedLinesAndMultipleCommitters` 使用临时 Git 仓库，构造作者与提交者不同、同一范围由两名提交者修改的历史，并在固定目标之后再修改 HEAD，核对采集没有串入后来的提交人。实际 CLI 样例验证最终 JSON 字段；材料测试验证多人、空数组、旧 JSON、空姓名与重复姓名。

```powershell
$env:PATH = 'C:\Program Files\Git\usr\bin;' + $env:PATH
make test "PACKAGES=./cmd/opencodereview -race=false -run 'Report|Material|CollectFindingCommitters'"
make test 'PACKAGES=./internal/report -race=false'
```

上述相关 CLI 回归与报告包测试均通过，`go vet ./cmd/opencodereview ./internal/report`、`make build`、差异检查通过。`make license-add` 未修改文件；`make check` 通过许可证检查，仍因历史人工样例的 31 行中文停在英文检查，本次未新增违规。未启用 race，未调用付费模型或验证真实生成的 HTML 排版，也未改写既有用户报告。
