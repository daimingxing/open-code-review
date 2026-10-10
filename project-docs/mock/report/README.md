# 报告功能人工验收

这里提供像使用产品一样验收本地二次开发 CLI 的步骤。命令直接运行构建出的可执行文件，不调用官方 NPM `ocr`，也不运行仓库里的测试脚本。

## 前提

- Windows、PowerShell 7，以及已加入系统 PATH 的全局 Go；命令在仓库根目录执行。新开终端后 `go version` 能打印出版本再开始，不要指定便携工具链或 `go.exe` 的绝对路径。
- 本机已经配置可用的模型服务。若尚未配置，先运行构建出的 CLI 的 `config provider` 和 `config model`；也可以按 `ocr config --help` 中的非交互示例配置。
- 准备一个要审查的 Git 仓库，并确认当前账号有权读取它和使用模型服务。

## 1. 构建可执行文件

下面的命令把本地源码打包成 `dist/ocr-local.exe`。以后验收只调用这个文件，源码不会被隐式解释执行。

```powershell
$ErrorActionPreference = 'Stop'
$repoRoot = (Get-Location).Path
$dist = Join-Path $repoRoot 'dist'
$binary = Join-Path $dist 'ocr-local.exe'

New-Item -ItemType Directory -Force -Path $dist | Out-Null
go build -trimpath -o $binary ./cmd/opencodereview
if ($LASTEXITCODE -ne 0) { throw 'CLI 构建失败' }

& $binary --version
& $binary report --help
$cli = (Resolve-Path -LiteralPath $binary).Path
```

`--version` 和 `report --help` 都能正常返回，才进入下一步。构建使用 PATH 中的全局 `go`，不要改成某个 `go.exe` 的绝对路径。

## 2. 配置模型服务（首次需要）

已经能运行现有 `ocr` 配置的，可以跳过。否则使用刚构建的文件配置：

```powershell
& $cli config provider
& $cli config model
```

这两个命令会把配置保存到用户目录的 `.opencodereview/config.json`。也可以使用非交互命令，例如：

```powershell
& $cli config set provider anthropic
& $cli config set model claude-opus-4-6
& $cli config set providers.anthropic.api_key $env:ANTHROPIC_API_KEY
```

不要把真实密钥写入仓库或提交到 Git。

## 3. 真实生成报告材料

把 `$targetRepo` 改成要审查的 Git 仓库。下面以单提交为例；分支比较时改用注释中的 `--from` 和 `--to`，两种模式不要同时传入。

```powershell
$targetRepo = 'D:\path\to\your\git-repo'
$outDir = Join-Path $targetRepo 'review-output'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

$native = Join-Path $outDir 'review.json'
$material = Join-Path $outDir 'review.report.json'

& $cli review `
  --repo $targetRepo `
  --commit HEAD `
  --format json `
  --output $native `
  --report $material
if ($LASTEXITCODE -ne 0) { throw '代码审查失败，请先查看命令行诊断信息' }

# 分支比较示例：
# & $cli review --repo $targetRepo --from main --to feature --format json --output $native --report $material

Get-Item -LiteralPath $native, $material | Select-Object FullName, Length
```

这一步是真实的代码审查，模型读取目标仓库的 Git 范围并生成两份独立结果：原生 JSON 和供报告阶段使用的 `review.report.json`。

## 4. 直接生成并查看 HTML

```powershell
$html = Join-Path $outDir 'review.html'
& $cli report --input $material --output $html
if ($LASTEXITCODE -ne 0) { throw 'HTML 报告生成失败，请查看命令行诊断信息并重试' }

Start-Process -FilePath $html
```

`report` 只读取报告材料，不会重新审查代码。生成阶段最多等待 10 分钟，失败时可以用同一份 JSON 再次执行上面的 `report` 命令。浏览器应能离线打开 HTML，并看到预设章节、发现的问题和基础工作统计；首版允许模型在排版和措辞上存在差异。

需要验证多份输入时，先为不同审查生成多份 `*.report.json`，再重复传入 `--input`：

```powershell
& $cli report `
  --input 'D:\path\to\first.report.json' `
  --input 'D:\path\to\second.report.json' `
  --output (Join-Path $outDir 'combined.html')
```

验收时记录使用的 CLI 构建提交、模型服务、审查命令、输出文件和实际观察结果；开发回归与历史证据见 [`project-docs/retests/`](../../retests/README.md)。
