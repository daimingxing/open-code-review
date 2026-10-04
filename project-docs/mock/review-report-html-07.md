# 单份中文 HTML 报告复测记录（工单 07）

日期：2026-10-04。本文记录工单 07 的受控模型 CLI、材料事实/安全校验和真实浏览器离线检查。工单 07 当前分支的真实 DeepSeek 验收尚未执行；真实凭据及真实报告材料由主线程保管，本记录不保存凭据。

## 测试目的与前提

- 在既有报告材料上生成单份中文 HTML，不重新审查；拒绝不兼容输入、改写统计或材料外事实，失败时不得留下最终文件。
- 使用仓库内测试的受控 OpenAI 兼容 HTTP 服务，不访问真实模型或外部网络。长报告夹具包含 60 条 finding、四级风险、部分审查状态、知识未采集和需要 HTML 转义的证据文本。
- 在 PowerShell 7、Go `go1.25.14` 下，从本工单工作树运行；本次分支提交待后续记录。
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
      stats: document.querySelectorAll('[data-stat]').length,
      externalElements: document.querySelectorAll('script,link,img,iframe,object,embed,form,svg,video,audio').length,
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth
    }));
    await page.screenshot({ path: path.join(path.dirname(process.env.OCR_REPORT_HTML_EVIDENCE_FILE), 'desktop.png'), fullPage: true });
    await page.setViewportSize({ width: 375, height: 812 });
    const mobile = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth,
      mainWidth: Math.round(document.querySelector('main').getBoundingClientRect().width),
      findings: document.querySelectorAll('article[data-finding-id]').length
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
- HTML 必须由 Edge 以本地 `file:` URL 打开；没有页面错误和外部请求/资源，桌面与 375px 视口均无横向溢出，finding 数量不变。
- 浏览器输出的 `requests` 应只包含该 HTML 的 `file:` URL。受控测试通过不代表真实模型措辞或真实项目材料已验收。

## 验证记录

- 2026-10-04，分支 `codex/review-report-07-single`（本次改动尚未提交）：
  - 最近一次 `go test ./cmd/opencodereview -run '^TestReportCommand' -count=1 -timeout=75s`：退出码 0，4.272 秒。
  - 最近一次 `go test ./internal/report -count=1 -timeout=75s`：退出码 0，1.970 秒。
  - 最近一次 `TestReportCommandRendersExistingMaterialAndRejectsAlteredModelFacts`：退出码 0，2.672 秒；生成的本地临时 HTML 为 `C:\Users\60429\AppData\Local\Temp\ocr-report-07-controlled.html`。负例包括篡改统计和补造人员叙述，确认无效输出不会发布。
  - Edge `154.0.4258.53` 桌面检查：标题“审查报告”、`lang=zh-CN`、9 章、60 条 finding、60 条可读 finding、10 项统计、0 个外部资源元素、无页面错误；唯一请求是本地 HTML 的 `file:` URL。1440px 视口下 `scrollWidth=1440`。
  - Edge 375px 窄屏检查：文档宽度 375px，主体宽 343px；60 条 finding 均可读、无卡片超出视口，唯一请求仍是本地 HTML，页面无错误。桌面和窄屏截图为 `C:\Users\60429\AppData\Local\Temp\ocr-report-07-desktop.png` 与 `C:\Users\60429\AppData\Local\Temp\ocr-report-07-mobile.png`。
  - 首轮浏览器检查曾发现概览中的长 SHA 未换行，桌面多 2px、窄屏宽至 1298px。模板已要求长标识在窄屏换行，受控响应随之加入适用于全部 `data-fact` 的断行 CSS；重新生成报告后上述两种视口均通过。
- `go test ./cmd/opencodereview -count=1 -timeout=150s` 曾在既有 MCP 测试 `TestReviewE2E_ReportTokenBudgetFailureIsNotSuccessAcrossModes` 超时；堆栈等待于现有 `retryTestRepo` / MCP `streamableServerConn.Read`。该超时不属于上述 `TestReportCommand` 聚焦集合，需在上游 MCP E2E 故障修复后复测，不作为工单 07 的通过证据。
- 本地 Edge/CLI 测试没有使用真实 DeepSeek 凭据，也没有访问主线程中的真实报告材料；真实模型和真实材料验收待集成后由主线程执行。
- 临时 HTML 和截图均在系统临时目录，不进入 Git。复测后确认目录仅含本次测试产物再按需删除；不要对共享临时目录执行递归清理。
