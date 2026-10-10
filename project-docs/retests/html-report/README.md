# HTML 报告回归

自动化测试检查材料事实、安全输出、失败修复、重试预算和不覆盖行为。浏览器脚本再检查短/长单份与多份报告在桌面、窄屏、键盘和打印下的实际显示。

在 PowerShell 7、已加入 PATH 的全局 Go 1.25+、Node.js、可加载 `playwright` 的 `NODE_PATH` 和 Microsoft Edge 环境中，从仓库根目录运行：

```powershell
$ErrorActionPreference = 'Stop'
$tempRoot = Join-Path ([IO.Path]::GetTempPath()) ('ocr-html-report-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempRoot | Out-Null
$env:OCR_REPORT_HTML_EVIDENCE_FILE = Join-Path $tempRoot 'single-long.html'
$env:OCR_REPORT_HTML_SHORT_EVIDENCE_FILE = Join-Path $tempRoot 'single-short.html'
$env:OCR_REPORT_HTML_SHORT_MULTI_EVIDENCE_FILE = Join-Path $tempRoot 'multi-short.html'
$env:OCR_REPORT_HTML_LONG_MULTI_EVIDENCE_FILE = Join-Path $tempRoot 'multi-long.html'

go test ./internal/report -count=1 -timeout=90s
if ($LASTEXITCODE -ne 0) { throw 'HTML 事实与安全测试失败' }
go test ./cmd/opencodereview -run '^(TestReportCommandRendersExistingMaterialAndRejectsAlteredModelFacts|TestReportCommandMultiInputFailureDoesNotPublishAndCanRetry|TestReportCommandWritesLongMultiBrowserEvidence)$' -count=1 -timeout=120s
if ($LASTEXITCODE -ne 0) { throw 'HTML 浏览器材料生成失败' }

$env:NODE_PATH = 'C:\Users\60429\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\node_modules'
$node = 'C:\Users\60429\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
$env:OCR_REPORT_BROWSER_ROOT = $tempRoot
$env:OCR_EDGE_PATH = 'C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe'
& $node ./project-docs/retests/html-report/verify-browser-report.cjs
if ($LASTEXITCODE -ne 0) { throw 'Edge 报告浏览器验收失败' }
Get-Content -LiteralPath (Join-Path $tempRoot 'browser-summary.json')
```

预期报告包测试、CLI 测试和 Edge 检查均成功，摘要中的四种报告都有事实，桌面与窄屏无横溢、页面错误和外联请求。临时输出含合成数据，检查后可删除 `$tempRoot`。`go` 使用 PATH 中的全局安装；运行前需按本机环境调整 Node/Playwright 和 Edge 路径。本测试不调用真实模型；历史集成证据见[验收档案索引](../archive/README.md)。
