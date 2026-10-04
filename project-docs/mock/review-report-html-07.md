# 单份中文 HTML 报告复测记录（工单 07）

日期：2026-10-04。本文记录工单 07 的受控模型 CLI、材料事实/安全校验和真实浏览器离线检查。真实 DeepSeek 报告生成已执行但失败；真实凭据及真实报告材料由主线程保管，本记录不保存凭据。

## 测试目的与前提

- 在既有报告材料上生成单份中文 HTML，不重新审查；拒绝不兼容输入、改写统计或材料外事实，失败时不得留下最终文件。
- 使用仓库内测试的受控 OpenAI 兼容 HTTP 服务，不访问真实模型或外部网络。长报告夹具包含 60 条 finding、四级风险、部分审查状态、知识未采集和需要 HTML 转义的证据文本；每条 finding 展示 11 个带固定中文标签的事实行。
- 在 PowerShell 7、Go `go1.25.14` 下，从本工单工作树运行；代码基线为 `dbfa92dc4f8a890c1ef3b17191d4b56b03840d18`。
- 浏览器使用安装版 Microsoft Edge `154.0.4258.53` 和运行时捆绑的 Playwright（通过 `NODE_PATH` 加载），以 `file:` URL 直接打开临时 HTML。

## 复测命令

在工单工作树的 PowerShell 7 中执行。第一条聚焦命令会把成功生成的受控报告另存到独立临时目录，供 Edge 打开；目录包含合成测试数据，不应作为真实审查报告分享。

```powershell
$go = 'D:\WorkPlace\toolchains\go1.25.14\go\bin\go.exe'
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('ocr-report-07-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempRoot | Out-Null
$env:OCR_REPORT_HTML_EVIDENCE_FILE = Join-Path $tempRoot 'report.html'

& $go test ./cmd/opencodereview -run '^TestReportCommandRendersExistingMaterialAndRejectsAlteredModelFacts$' -count=1 -timeout=75s
if ($LASTEXITCODE -ne 0) { throw '受控长报告 CLI 验收失败' }
& $go test ./cmd/opencodereview -run '^TestReportCommand' -count=1 -timeout=75s
if ($LASTEXITCODE -ne 0) { throw '报告命令聚焦测试失败' }
& $go test ./internal/report -count=1 -timeout=75s
if ($LASTEXITCODE -ne 0) { throw '报告材料与 HTML 校验包测试失败' }

$env:NODE_PATH = 'C:\Users\60429\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\node_modules'
$node = 'C:\Users\60429\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
$browserScript = @'
const { chromium } = require('playwright');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
(async () => {
  const browser = await chromium.launch({
    executablePath: 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
    headless: true
  });
  try {
    const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
    const pageErrors = [];
    const requests = [];
    page.on('pageerror', error => pageErrors.push(error.message));
    page.on('request', request => requests.push(request.url()));
    await page.goto(pathToFileURL(process.env.OCR_REPORT_HTML_EVIDENCE_FILE).href, { waitUntil: 'load' });
    const desktop = await page.evaluate(() => ({
      title: document.title,
      lang: document.documentElement.lang,
      sections: document.querySelectorAll('main section[data-section]').length,
      findings: document.querySelectorAll('article[data-finding-id]').length,
      labeledFacts: document.querySelectorAll('article[data-finding-id] .fact-row').length,
      stats: document.querySelectorAll('[data-stat]').length,
      externalElements: document.querySelectorAll('script,link,img,iframe,object,embed,form,svg,video,audio,canvas,details,dialog,noscript,noembed,noframes').length,
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth
    }));
    await page.screenshot({ path: path.join(path.dirname(process.env.OCR_REPORT_HTML_EVIDENCE_FILE), 'desktop.png'), fullPage: true });
    await page.setViewportSize({ width: 375, height: 812 });
    const mobile = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth,
      mainWidth: Math.round(document.querySelector('main').getBoundingClientRect().width),
      findings: document.querySelectorAll('article[data-finding-id]').length,
      labeledFacts: document.querySelectorAll('article[data-finding-id] .fact-row').length
    }));
    await page.screenshot({ path: path.join(path.dirname(process.env.OCR_REPORT_HTML_EVIDENCE_FILE), 'mobile.png'), fullPage: true });
    console.log(JSON.stringify({ desktop, mobile, pageErrors, requests }, null, 2));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error.stack || String(error)); process.exitCode = 1; });
'@
& $node -e $browserScript
if ($LASTEXITCODE -ne 0) { throw 'Edge 浏览器检查未能运行' }
```

## 结果判定

- CLI 聚焦测试和 `internal/report` 测试退出码为 0；长报告需包含九个章节、60 条 finding、十项统计、四级风险、部分结果与知识缺失事实。
- HTML 必须由 Edge 以本地 `file:` URL 打开；没有页面错误和外部请求/资源，桌面与 375px 视口均无横向溢出，finding 数量及固定标签事实行不变。
- 浏览器输出的 `requests` 应只包含该 HTML 的 `file:` URL。受控测试通过不代表真实模型措辞或真实项目材料已验收。

## 验证记录

- 2026-10-04，分支 `codex/review-report-07-single`，最终代码提交 `dbfa92dc4f8a890c1ef3b17191d4b56b03840d18`：
  - `go test ./cmd/opencodereview -run '^TestReportCommand' -count=1 -timeout=75s`：退出码 0，4.409 秒（包含 article 结构校验修复后复跑）。
  - `go test ./internal/report -count=1 -timeout=75s`：退出码 0，1.814 秒（包含 article 结构校验修复后复跑）。
  - `TestReportCommandRendersExistingMaterialAndRejectsAlteredModelFacts`：退出码 0，2.729 秒；生成 Edge 输入 `C:\Users\60429\AppData\Local\Temp\ocr-report-07-final.html`。负例覆盖篡改统计、补造人员事实、CSS/style、body 外正文及无效输出不留最终文件。
  - Edge `154.0.4258.53` 桌面检查：标题“审查报告”、`lang=zh-CN`、9 章、60 条 finding、660 个 finding 事实行、10 项统计、0 个外部/默认隐藏容器，`scrollWidth=1440`。每条 finding 以 11 个固定中文标签字段逐行呈现，状态显示为中文。
  - Edge 375px 窄屏检查：`scrollWidth=375`、主体宽 343px、60 条 finding 仍有 660 个带标签事实行。页面无 JS 错误；唯一网络请求为本地 HTML 的 `file:` URL。
  - 最终截图位于系统临时目录：`C:\Users\60429\AppData\Local\Temp\ocr-report-07-final-desktop-viewport.png`、`C:\Users\60429\AppData\Local\Temp\ocr-report-07-final-finding-viewport.png`、`C:\Users\60429\AppData\Local\Temp\ocr-report-07-final-desktop.png`、`C:\Users\60429\AppData\Local\Temp\ocr-report-07-final-mobile.png`。
  - 首轮浏览器检查曾发现长 SHA 横溢；后续检查发现 finding 字段缺少固定标签。模板现要求固定事实标签和路径/行号绑定，并使用程序固定样式；标签版最终报告桌面、窄屏均通过。
  - 模型元素采用 fail-closed 语义 allowlist；`article` 保留在允许项，finding 的 `data-finding-id` 必须属于 `<article>`；`div[data-finding-id]` 有拒绝与不落盘负例。`canvas`、关闭的 `details`/`dialog`、`noscript`、`noembed`、`noframes` 等未知元素均有校验和不落盘负例。
  - 最后一项 article 校验仅收紧发布前验证，不改变 HTML 渲染，因此未重跑 Edge；Edge 的输入报告由前一代码提交 `dbfa92dc4f8a890c1ef3b17191d4b56b03840d18` 生成，最终修复提交为 `0bf782f96877a6b542b9e8e975d9bf858bf22074`。
  - 独立 reviewer `review_report_07` 复核最终提交，确认无 blocker、无残留 finding。reviewer 当前环境无法定位 Go，未自行重跑测试；上列聚焦测试由实施环境在最终提交前执行。
- `go test ./cmd/opencodereview -count=1 -timeout=150s` 曾在既有 MCP 测试 `TestReviewE2E_ReportTokenBudgetFailureIsNotSuccessAcrossModes` 超时；堆栈等待于现有 `retryTestRepo` / MCP `streamableServerConn.Read`。该超时不属于上述 `TestReportCommand` 聚焦集合，需在上游 MCP E2E 故障修复后复测，不作为工单 07 的通过证据。
- 真实模型失败复现（主线程执行，2026-10-04，基于 `d34bb009`）：运行 `ocr report --input frontend-report.json`，使用已有 DeepSeek 配置。请求耗时 58.216 秒，input/output 用量分别为 5,645/18,345 tokens（reported）；CLI 因响应未以 HTML doctype 开始而拒绝发布，错误为 `HTML document must begin with an HTML doctype`，没有生成 HTML 文件。该运行明确记为失败，不计入通过证据；不放宽 HTML 校验，模型响应修复由工单 09 的有界修复能力处理后再复测。
- 本地 Edge/受控 CLI 验收没有使用真实 DeepSeek 凭据；真实模型这一轮失败，主线程尚未对修复后的真实输出复测。
- 临时 HTML 和截图均在系统临时目录，不进入 Git。复测后确认目录仅含本次测试产物再按需删除；不要对共享临时目录执行递归清理。
