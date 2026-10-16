# HTML 报告回归

自动化测试检查智能体循环交付、材料路径引导、失败不覆盖和多份输入顺序。浏览器脚本再检查短/长单份与多份报告在桌面、窄屏、键盘和打印下的实际显示。

首轮不因 HTML 内容、脚本或样式检查拒绝交付；提示词建议优先读取报告 JSON 与模板/样式，但不会增加源码路径权限拦截。历史 01–09 验收证据仍在[验收档案索引](../archive/README.md)，本目录只保留可复跑入口。

在 PowerShell 7、已加入 PATH 的全局 Go 1.25+、Node.js、可加载 `playwright` 的 `NODE_PATH` 和 Microsoft Edge 环境中，从仓库根目录运行：

```powershell
$ErrorActionPreference = 'Stop'
$tempRoot = Join-Path ([IO.Path]::GetTempPath()) ('ocr-html-report-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempRoot | Out-Null
$env:OCR_REPORT_HTML_EVIDENCE_FILE = Join-Path $tempRoot 'single-long.html'
$env:OCR_REPORT_HTML_SHORT_EVIDENCE_FILE = Join-Path $tempRoot 'single-short.html'
$env:OCR_REPORT_HTML_SHORT_MULTI_EVIDENCE_FILE = Join-Path $tempRoot 'multi-short.html'
$env:OCR_REPORT_HTML_LONG_MULTI_EVIDENCE_FILE = Join-Path $tempRoot 'multi-long.html'

go test ./internal/report ./internal/tool -count=1 -timeout=90s
if ($LASTEXITCODE -ne 0) { throw 'report/tool package tests failed' }
go test ./cmd/opencodereview -run '^(TestReportCommandAgentLoopReadsMaterialsWritesHTMLAndFinishes|TestReportCommandMultiInputPreservesOrderAndRejectsDuplicateBeforeModel|TestReportCommandMultiInputCanRetryAfterModelFailure|TestReportCommandWritesLongMultiBrowserEvidence|TestReportCommandFailsWhenHTMLNeverWritten|TestReportCommandRejectsExistingExplicitOutputBeforeLoop)$' -count=1 -timeout=180s
if ($LASTEXITCODE -ne 0) { throw 'HTML agent-loop CLI tests failed' }

$env:NODE_PATH = 'C:\Users\60429\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\node_modules'
$node = 'C:\Users\60429\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
$env:OCR_REPORT_BROWSER_ROOT = $tempRoot
$env:OCR_EDGE_PATH = 'C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe'
& $node ./project-docs/retests/html-report/verify-browser-report.cjs
if ($LASTEXITCODE -ne 0) { throw 'Edge report browser check failed' }
Get-Content -LiteralPath (Join-Path $tempRoot 'browser-summary.json')
```

预期报告包测试、CLI 智能体循环测试和 Edge 检查均成功。临时输出含合成数据，检查后可删除 `$tempRoot`。`go` 使用 PATH 中的全局安装；运行前需按本机环境调整 Node/Playwright 和 Edge 路径。本测试使用可控模型替身，不调用真实模型。
