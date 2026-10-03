# 工单 02：审查报告材料

日期：2026-10-04。源码提交：待实现提交生成后补录。验证对象为 `feature-review-report` 上包含该源码提交的版本。

## 目的与前提

验证 `ocr review` 的可选独立 JSON 报告材料、原生输出兼容、事实与缺失状态校验、路径安全和失败保留行为。CLI 端到端测试使用临时 Git 仓库及仓库内可控测试模型服务；这些测试不需要模型凭据，也不代替真实模型或外部知识服务的效果验收。

复测使用 PowerShell 7、Go 1.25.14 和 `feature-review-report` 集成分支。先切换到包含下方“源码提交”字段所列提交的集成版本；工作区应干净。测试依赖首次下载需要可访问 Go module proxy。`make check` 还需要仓库支持的 Make、Git Bash 和 Go 位于 `PATH`。

## 命令

在仓库根目录运行聚焦测试：

```powershell
$ErrorActionPreference = 'Stop'
Set-Location 'D:\WorkPlace\open-code-review'
git switch feature-review-report
$sourceCommit = '待实现提交生成后补录'
git merge-base --is-ancestor $sourceCommit HEAD
if ($LASTEXITCODE -ne 0) { throw '集成分支不包含工单 02 源码提交' }
go version
go test ./internal/report ./cmd/opencodereview -run 'Test(ValidateMaterial|WriteMaterial|CopyMaterialExclusively|PathsConflict|ParseReviewFlagsOptionalReportPath|DefaultReportPath|SanitizeReportLabel|ReviewE2E_Report|BuildReportMaterial|MaterialFindings)' -count=1

# OCR 复核意见对应的参数互斥与缺失 manifest 回归测试
go test ./cmd/opencodereview -run 'Test(ParseReviewFlagsOptionalReportPath|BuildReportMaterialRejectsMissingManifest)' -count=1
```

运行完整 Go 测试与静态检查：

```powershell
go test ./... -count=1
go vet ./...
```

仓库标准检查会整理模块、格式化和运行 vet：

```powershell
$env:PATH = 'D:\WorkPlace\toolchains\go1.25.14\go\bin;C:\Program Files\Git\bin;C:\Program Files\Git\usr\bin;' + $env:PATH
$make = 'D:\WorkPlace\toolchains\make-4.4.1\bin\make.exe'
& $make check
```

仓库标准 race 测试需要启用 CGO，并在 `PATH` 中提供可用的 GCC 兼容 C 编译器：

```powershell
$env:PATH = 'D:\WorkPlace\toolchains\go1.25.14\go\bin;C:\Program Files\Git\bin;C:\Program Files\Git\usr\bin;' + $env:PATH
$env:CGO_ENABLED = '1'
$make = 'D:\WorkPlace\toolchains\make-4.4.1\bin\make.exe'
& $make test
```

## 预期与验收

- 聚焦测试覆盖 CLI 参数顺序、裸 `--report`、空格路径、相邻参数、三种审查范围、零问题及有问题材料、限制原因、报告文件冲突、显式路径重名、并发排他保存和失败时保留原生 JSON。
- 完整测试与 vet 退出码均为 0。`make check` 输出 `check passed`。
- 真实服务测试如需额外执行，应使用主机已授权的 OCR 模型与知识服务配置，并按工单 01 的隔离要求准备知识服务；不能把本文件中的受控测试服务结果表述为真实效果证据。
- `make test` 只有在 CGO 与 GCC 前提满足时才以退出码 0 为通过；缺失编译器时的构建错误表示未验证 race 测试。
- 报告文件系统不支持硬链接时会回退到 `O_EXCL` 独占创建并复制，仍不覆盖已有目标，但复制过程不是原子发布；复制失败可能留下部分 JSON。为避免竞态删除被并发替换的文件，不会按路径清除该目标；显式重试会拒绝已有路径，需先检查并人工处理残留文件。

## 验证记录

2026-10-04，Windows 11、PowerShell 7、Go 1.25.14：聚焦 `internal/report` 与 `cmd/opencodereview` 测试退出码 0；OCR finding 回归测试退出码 0；完整 `go test ./... -count=1` 退出码 0。合并 `e229220` 后 `make check` 退出码 0，license 检查通过，659 个扫描源文件无未豁免文本，`go mod tidy`、全仓 gofmt 与 go vet 通过。源码 SHA 在工单提交后补录。

同日 `make test` 在默认 `CGO_ENABLED=0` 下因 race 检测要求 CGO 而失败；设为 `CGO_ENABLED=1` 后因当前 `PATH` 没有 `gcc` 而构建失败。因此本机未验证 race 测试。`go test ./... -count=1` 不启用 race，单独记录为普通测试结果。

OCR 最终自审部分完成：审查了 5 个文件并提出 2 条意见，另 3/5 个选择文件因总 token 预算而未完成；命令总耗时 5m15s。manifest 缺失分支原已由构建器返回明确错误并阻止保存，新增 `TestBuildReportMaterialRejectsMissingManifest` 回归测试；Cobra 的 `Args` 在 `RunE` 前解析可选报告参数，新增裸 `--report` 和显式路径与 `--preview` 的互斥测试。两项定向测试均退出码 0。另一次 OCR 尝试在分发阶段即因估算预算不足失败，不作为审查通过。主线程提交后的独立复审结果需追加于此。
