# 工单 09：报告阅读体验与有界修复复测

日期：2026-10-05。工单实现分支为 `codex/review-report-09-experience`，基线为 `e66aeaef2c4f554b0f36ed788d47bbc9fab56237`；本文也记录 `feature-review-report` 上的集成验收。真实凭据、原始材料、模型正文和截图不进入 Git。

## 当前行为与预算

HTML 阶段仅复用已经读取的 JSON，不读取仓库、Git、知识或重新审查。程序验证模型的完整事实与安全 HTML 后，加入审查单元或仓库、等级、类别三个原生 radio 筛选器和原生 `details` 折叠，再安全发布。无效 HTML、遗漏问题、改等级、错统计、timeout 或截断会带校验诊断修复；最终失败不产生输出，显式已有路径拒绝覆盖。

报告阶段默认硬限制为最多 3 次请求、总时间 10 分钟、每次可见 HTML 36,864 tokens、每次 provider completion 请求最多 98,304 tokens、全阶段累计 provider completion usage 196,608 tokens（包括服务报告的 reasoning/计费用量）。每次请求的 provider token 数还会受剩余累计预算约束。输入上限为 128,000 tokens，统计系统提示、原始材料、上一轮无效 HTML 草稿和修复诊断；每次修复也检查完整提示预算。可见内容按本地 tokenizer 计数，provider completion usage 独立累计；不会用较小的可见计数覆盖真实用量。任一输出预算超限拒绝当前成品，累计预算超限停止后续请求。每次 timeout 取 endpoint timeout 与剩余总时间的较小值；10 分钟总限在 endpoint timeout 缺省或无效时防止无限等待。预算不足时明确失败，不截断事实。

## HTML 初版验收范围

HTML 是模型生成的初版报告，不要求匹配固定样式、标题、标签、措辞或 DOM 快照；finding 可使用 `article`、`section`、`div`、`blockquote` 或 `li` 等可容纳完整详情的分组元素，不固定为 `<article>`。通过条件是离线安全打开；九个预设章节有可见内容；报告材料中的每条 finding 恰好出现一次，主要定位、等级、中文说明及源代码/证据（或缺失原因）可见；基础风险与覆盖统计正确；不编造证据或改变结论。布局、普通叙述和视觉细节允许差异，后续可依据真实样例迭代。程序校验事实和安全边界，不对视觉打分，也不因非关键标题/标签差异无限重试。

材料阶段指标与 HTML 阶段指标分别记录。HTML 的耗时、输入、provider completion 和 visible output token 包含所有修复请求；`usage=estimated` 表示服务未返回用量，由本地估算，不能视为服务账单。

## 前提与受控 CLI

使用 PowerShell 7、Go 1.25.14。以下命令在工单工作树运行，不调用真实模型。四份合成浏览器材料包含短/长、单/多份；长单份有 60 条问题及长证据、中文说明，长多份有 60 条问题和不同审查单元。

```powershell
Set-Location 'C:\Users\60429\.codex\worktrees\review-report-09-experience\open-code-review'
if ($PSVersionTable.PSVersion.Major -lt 7) { throw '需要 PowerShell 7' }
$go = 'D:\WorkPlace\toolchains\go1.25.14\go\bin\go.exe'
$env:GOPROXY = 'https://goproxy.cn,direct'
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('ocr-report-09-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempRoot | Out-Null
$env:OCR_REPORT_HTML_EVIDENCE_FILE = Join-Path $tempRoot 'single-long.html'
$env:OCR_REPORT_HTML_SHORT_EVIDENCE_FILE = Join-Path $tempRoot 'single-short.html'
$env:OCR_REPORT_HTML_SHORT_MULTI_EVIDENCE_FILE = Join-Path $tempRoot 'multi-short.html'
$env:OCR_REPORT_HTML_LONG_MULTI_EVIDENCE_FILE = Join-Path $tempRoot 'multi-long.html'

& $go test ./internal/report -count=1 -timeout=90s
if ($LASTEXITCODE -ne 0) { throw '报告事实、安全与 writer 测试失败' }
& $go test ./cmd/opencodereview -run '^(TestReport|TestRunHTMLReport)' -count=1 -timeout=120s
if ($LASTEXITCODE -ne 0) { throw '报告 CLI 测试失败' }
git diff --check
if ($LASTEXITCODE -ne 0) { throw '差异空白检查失败' }
```

预期两组测试退出 0。CLI 覆盖 heading 错误后恢复、安全元素错误后恢复、timeout/截断/漏问题/改等级/错统计/无效输出三次耗尽、不发布/不覆盖、相同 JSON 与无工具调用、累计用量和总时间预算；预算测试既检查可见 HTML 又检查累计 completion 上限。生成的四份 HTML 必须通过原 07/08 事实校验，合成事实不能当作真实项目结果分享。

2026-10-05 实际：`internal/report` 通过（2.367 秒）；报告 CLI 聚焦组通过（7.691 秒）。独立审查发现请求 `MaxTokens` 曾错误等于可见输出额度，修复后测试用 34,301 个可见 token 与 70,000 个 completion token 覆盖长文档，并断言 provider 请求额度包含 reasoning 空间；累计上限耗尽、HTTP 400 错误正文不泄露、用户取消后不重试也通过。默认 Go proxy 曾因 IPv6 连接失败；仅本地构建/测试设置上述 `GOPROXY` 后成功，没有写入用户配置。以上是受控模型结果，不替代真实模型验收。

## Edge 交互、窄屏与打印

使用安装版 Edge 和运行时捆绑的 Playwright。脚本只输出计数与验收结论，截图/PDF 在临时目录保存。通过前一段命令生成四份 HTML 后执行：

```powershell
$env:NODE_PATH = 'C:\Users\60429\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\node_modules'
$node = 'C:\Users\60429\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
$env:OCR_REPORT_BROWSER_ROOT = $tempRoot
$env:OCR_EDGE_PATH = 'C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe'
& $node ./project-docs/mock/review-report-experience-09.cjs
if ($LASTEXITCODE -ne 0) { throw 'Edge 交互/打印复测失败' }
Get-Item (Join-Path $tempRoot 'browser-summary.json') | Select-Object FullName,Length
```

脚本逐项实际切换三个筛选组，比较可见问题与原事实归属；使用 Tab/Space 操作 radio、Enter/Space 折叠详情并确认 `:focus-visible`；在 1440 与 375 视口检查横溢、事实数量、安全 DOM、页面错误与外联。最后关闭详情并模拟 print，确认筛选器隐藏、全部详情可见，输出 A4 PDF。预期所有检查成功，网络仅打开本地 `file:` HTML。

2026-10-05 已执行 Edge `154.0.4258.53` 四份合成报告验收：短单份 0 问题/33 facts，长单份 60/702，短多份 4/127，长多份 60/743。桌面与 375px 三类筛选、键盘、打印均通过；事实数不变，横溢、页面错误、危险 DOM 和外联均为 0。浏览器生成 `browser-summary.json`、每份 HTML 的桌面/窄屏截图和 A4 PDF。原证据目录：`C:\Users\60429\AppData\Local\Temp\ocr-report-09-browser-290c46abf9774666bdb2ac732501738f`。该目录仅包含合成资料，真实模型没有可发布成品，不能将本轮浏览器结果称作真实模型视觉验收。

## 真实模型：只复用已有材料

前提是 PowerShell 7、Go 1.25.14、资源索引指定的前端材料，以及隔离目录中已有的 DeepSeek 配置均可用；配置不复制、不回显。命令会真实调用模型，输出名使用 GUID，不覆盖旧产物。真实服务可能计费并受服务端速率限制。

```powershell
Set-Location 'D:\WorkPlace\open-code-review'
if ((git branch --show-current) -ne 'feature-review-report') { throw '请在 feature-review-report 集成分支执行' }
$go = 'D:\WorkPlace\toolchains\go1.25.14\go\bin\go.exe'
$liveRoot = 'C:\Users\60429\AppData\Local\Temp\ocr-review-report-live-11fe67599dc746d6add6daab79ec12b8'
$inputPath = Join-Path $liveRoot 'results\frontend-report.json'
if (!(Test-Path -LiteralPath $inputPath)) { throw '缺少既有报告材料' }
$exe = Join-Path $liveRoot 'ocr-report-09-current.exe'
& $go build -o $exe ./cmd/opencodereview
if ($LASTEXITCODE -ne 0) { throw '构建失败' }
$runId = [guid]::NewGuid().ToString('N')
$outputPath = Join-Path $liveRoot ('results\frontend-report-09-' + $runId + '.html')
$logPath = Join-Path $liveRoot ('results\frontend-report-09-' + $runId + '.log')
$previousUserProfile = $env:USERPROFILE
try {
    $env:USERPROFILE = Join-Path $liveRoot 'home'
    $timer = [System.Diagnostics.Stopwatch]::StartNew()
    & $exe report --input $inputPath --output $outputPath *> $logPath
    $reportExit = $LASTEXITCODE
    $timer.Stop()
    # 只提取计量行；诊断只另行记录规则分类，不打印原始 HTML 或事实。
    $metrics = Get-Content -LiteralPath $logPath | Where-Object { $_ -match '^phase=(html_generation|material_summary)' }
    $metrics
    [pscustomobject]@{ Exit=$reportExit; Elapsed=$timer.Elapsed; Published=(Test-Path -LiteralPath $outputPath); Log=$logPath }
} finally {
    $env:USERPROFILE = $previousUserProfile
}
```

预期成功时退出 0、发布 HTML 且通过 07/08 内容、安全校验；失败时非零、保留分轮/累计计量和安全分类诊断、没有最终 HTML。完成后若存在真实 HTML，只在隔离目录浏览，不提交、不打印正文或截图。日志中的 raw 内容不作为文档内容复制。

历史真实运行曾因模型返回非 HTML 或校验失败而退出，未发布文件；较早的 2 分钟总限运行也曾达到 timeout。2026-10-05 的初版标准调整后，主线程首次真实复测仍因修复诊断过于笼统而三次失败：耗时 `2m9.936s`，累计 input/completion/visible 为 `37830/43049/28681` tokens，第三次诊断均为“finding set or facts do not match”，未发布 HTML。实现补齐安全字段级诊断后，同一 JSON、DeepSeek Flash 和 10 分钟预算复测成功：第 1 次 `68.424s`（input/completion/visible `6145/20236/9488`），检测到 `sections.achievements.data.model_summary_status` 与材料不符；第 2 次修复 `25.711s`（`15782/10182/9483`），HTML 阶段合计 `1m34.235s`，退出 0，安全发布 HTML，completion usage 均由服务报告。

真实成品仅保存在隔离临时目录：`C:\Users\60429\AppData\Local\Temp\ocr-review-report-live-11fe67599dc746d6add6daab79ec12b8\results\frontend-report-debug-2da3cd4c63bb440297378f51954dd512.html`，大小 38,163 bytes，SHA-256 `CA641A8B2954348872D5FD79EBFBF7FC82BC779A14E2F0B215BB08FFA97CCA5C`。Edge `154.0.4258.53` 实际打开该本地文件并检查：1 条 finding、161 个事实标记；1440px/375px 无横向溢出，筛选可操作、radio 键盘可达、details 可折叠，打印保留全部事实；页面错误、危险 DOM、外联请求均为 0。首屏包含筛选、审查概览和风险统计，正文信息密集但可读，作为允许视觉差异的初版样例，不作为最终样式模板。截图位于 `C:\Users\60429\AppData\Local\Temp\ocr-review-report-live-11fe67599dc746d6add6daab79ec12b8\results\actual-report-viewport-1440.png`，未提交。

## 包级回归限制

```powershell
& $go test ./internal/report ./cmd/opencodereview -count=1 -timeout=180s
& $go test -json ./cmd/opencodereview -count=1 -timeout=45s
& $go test ./cmd/opencodereview -run '^TestReviewE2E_JSONHumanStreamsProgressToStderr$' -count=1 -timeout=45s
git show e66aeaef2c4f554b0f36ed788d47bbc9fab56237:cmd/opencodereview/progress_stream_e2e_test.go | Select-String 'func TestReviewE2E_JSONHumanStreamsProgressToStderr'
```

实际：第一条报告包通过，CLI 全包达到 180 秒包级测试上限；45 秒 JSON 定位运行最后停留在既有 `TestReviewE2E_JSONHumanStreamsProgressToStderr`，堆栈为 `retryTestGit` 的 `exec.Cmd.CombinedOutput`（`retry_fake_llm_test.go:343`，调用自 `progress_stream_e2e_test.go:36`）。该测试在 e66 基线已存在，单独运行通过（6.365 秒）；未确认全包超时根因，也未修改该无关测试。包级 test timeout 与产品 HTML 10 分钟总预算是不同限制，最终集成全量回归由主线程使用更充分的包级时限执行。
