# 工单 08：多份中文 HTML 报告复测

日期：2026-10-04。该记录覆盖工单 08 的多份材料聚合、审查单元归属、可信人员身份、多单元事实校验、安全保存和离线浏览器检查。CLI 使用进程内受控 OpenAI 兼容服务；本工单没有调用真实模型或外部知识服务。

## 测试目的与前提

- 在工单工作树的 PowerShell 7 中执行，源码分支为 `codex/review-report-08-multi`；本次差异以 `97e1cf80abb0871b78425c357c85842417d00d52` 为基线。Go 使用 `D:\WorkPlace\toolchains\go1.25.14\go\bin\go.exe`（Go 1.25.14）。
- CLI 测试以受控 HTTP 模型响应生成三单元报告：包含同仓库不同/重叠范围、另一仓库、完整与部分状态、不同仓库的重复问题 ID、一个邮箱一致的作者别名对、两条无邮箱同名身份。测试另验证重排、不同路径重复材料在模型请求前拒绝、没有合并 JSON、失败不发布以及后续成功重试。
- 第一单元含一条 `SeverityStatus=not_collected` 的真实 finding；HTML 必须保留该问题并显示单独的“未提供等级数量”汇总事实，不能把它记为任一已知等级或丢弃。
- HTML 使用已安装的 Edge `154.0.4258.53` 与工作区捆绑的 Playwright 1.62.1（通过 `NODE_PATH` 加载），以 `file:` URL 直接打开，不访问网络服务。
- 以下临时 HTML 与截图包含合成数据，不是实际代码审查报告；不得作为真实审查结果分享。

## 复测命令

先运行受控 CLI 测试并保留两个输入顺序的 HTML，再运行报告包及 CLI 聚焦集合：

```powershell
Set-Location 'C:\Users\60429\.codex\worktrees\review-report-08-multi\open-code-review'
$go = 'D:\WorkPlace\toolchains\go1.25.14\go\bin\go.exe'
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('ocr-report-08-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempRoot | Out-Null
$env:OCR_MULTI_REPORT_HTML_EVIDENCE_FILE = Join-Path $tempRoot 'ordered.html'
$env:OCR_MULTI_REPORT_HTML_REORDERED_EVIDENCE_FILE = Join-Path $tempRoot 'reordered.html'

& $go test ./cmd/opencodereview -run '^TestReportCommandMultiInputPreservesOrderAndRejectsDuplicateMaterialBeforeRequest$' -count=1 -timeout=75s
if ($LASTEXITCODE -ne 0) { throw '多输入受控 CLI 场景失败' }
& $go test ./internal/report -count=1 -timeout=75s
if ($LASTEXITCODE -ne 0) { throw '报告材料和 HTML 校验包测试失败' }
& $go test ./cmd/opencodereview -run '^TestReportCommand' -count=1 -timeout=120s
if ($LASTEXITCODE -ne 0) { throw '报告命令聚焦测试失败' }
git diff --check
if ($LASTEXITCODE -ne 0) { throw '差异空白检查失败' }
```

用 Edge 检查两个顺序的文档、页面归属、人员事实、离线行为和窄屏：

```powershell
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
    const cases = [
      { name: 'ordered', file: process.env.OCR_MULTI_REPORT_HTML_EVIDENCE_FILE, expected: ['multi-partial-first', 'multi-complete-overlap', 'multi-partial-other'] },
      { name: 'reordered', file: process.env.OCR_MULTI_REPORT_HTML_REORDERED_EVIDENCE_FILE, expected: ['multi-partial-other', 'multi-complete-overlap', 'multi-partial-first'] }
    ];
    const results = [];
    for (const testCase of cases) {
      const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
      const pageErrors = [];
      const requests = [];
      page.on('pageerror', error => pageErrors.push(error.message));
      page.on('request', request => requests.push(request.url()));
      await page.goto(pathToFileURL(testCase.file).href, { waitUntil: 'load' });
      const desktop = await page.evaluate(() => {
        const units = [...document.querySelectorAll('section[data-section="overview"] > div[data-review-unit-id]')];
        const findings = [...document.querySelectorAll('section[data-section="finding-details"] [data-finding-id]')];
        const findingOwners = new Map();
        for (const finding of findings) {
          const id = finding.getAttribute('data-finding-id');
          const owners = findingOwners.get(id) || new Set();
          owners.add(finding.getAttribute('data-review-unit-id'));
          findingOwners.set(id, owners);
        }
        const peopleByIndex = new Map();
        for (const fact of document.querySelectorAll('section[data-section="people"] [data-fact^="people["]')) {
          const match = fact.getAttribute('data-fact').match(/^people\[(\d+)\]\.(.+)$/);
          if (!match) continue;
          const [, index, property] = match;
          const person = peopleByIndex.get(index) || {};
          person[property] = fact.textContent.trim();
          peopleByIndex.set(index, person);
        }
        const people = [...peopleByIndex.values()];
        return {
          title: document.title,
          lang: document.documentElement.lang,
          sections: document.querySelectorAll('main section[data-section]').length,
          facts: document.querySelectorAll('[data-fact]').length,
          units: units.map(unit => ({
            id: unit.getAttribute('data-review-unit-id'),
            runId: unit.querySelector('[data-fact$=".material.review.run_id"]')?.textContent.trim(),
            label: unit.querySelector('.fact-row .fact-label')?.textContent.trim(),
            divider: getComputedStyle(unit).borderBottomWidth
          })),
          findings: findings.length,
          duplicateFindingIDs: [...findingOwners.entries()].filter(([, owners]) => owners.size > 1).map(([id, owners]) => ({ id, owners: [...owners] })),
          unknownFinding: (() => {
            const finding = findings.find(item => item.getAttribute('data-severity') === '' && item.querySelector('[data-fact="severity_zh"]')?.textContent.trim() === '未提供');
            return finding ? { id: finding.getAttribute('data-finding-id'), unit: finding.getAttribute('data-review-unit-id'), summary: finding.querySelector('[data-fact="summary_zh"]')?.textContent.trim() } : null;
          })(),
          unknownSeverity: (() => {
            const fact = document.querySelector('[data-fact="summary.findings_by_severity.not_collected"]');
            return fact ? { value: fact.textContent.trim(), label: fact.closest('.fact-row')?.querySelector('.fact-label')?.textContent.trim() } : null;
          })(),
          people,
          unsafeElements: document.querySelectorAll('script,link,img,iframe,object,embed,form,svg,video,audio,canvas,details,dialog,noscript,noembed,noframes').length,
          scrollWidth: document.documentElement.scrollWidth,
          innerWidth
        };
      });
      if (JSON.stringify(desktop.units.map(unit => unit.runId)) !== JSON.stringify(testCase.expected)) throw new Error(`${testCase.name}: input order mismatch`);
      if (desktop.sections !== 9 || desktop.units.length !== 3 || desktop.units.some(unit => unit.label !== '运行标识' || unit.divider !== '1px')) throw new Error(`${testCase.name}: unit grouping/heading mismatch`);
      if (desktop.facts !== 258 || desktop.unknownSeverity?.value !== '1' || desktop.unknownSeverity?.label !== '未提供等级数量' || desktop.unsafeElements !== 0 || desktop.scrollWidth !== desktop.innerWidth) throw new Error(`${testCase.name}: facts, unknown severity, safety, or desktop width mismatch`);
      const unknownUnit = desktop.units.find(unit => unit.runId === 'multi-partial-first');
      if (!desktop.unknownFinding || desktop.unknownFinding.unit !== unknownUnit.id) throw new Error(`${testCase.name}: unknown-severity finding was omitted or assigned to the wrong unit`);
      if (desktop.findings !== 8 || desktop.duplicateFindingIDs.length !== 4 || desktop.duplicateFindingIDs.some(item => item.owners.length !== 2)) throw new Error(`${testCase.name}: finding set or colliding finding ownership mismatch`);
      const alice = desktop.people.find(person => {
        const variants = new Set(Object.entries(person).filter(([key]) => key.startsWith('name_variants[')).map(([, value]) => value));
        return person.identity_status === 'email_verified' && variants.has('Alice Chen') && variants.has('A. Chen');
      });
      const unverifiedSams = desktop.people.filter(person => person.identity_status === 'unit_scoped' && person.display_name === 'Sam Lee');
      if (!alice || !alice['contributions[1].review_unit_id'] || unverifiedSams.length !== 2) throw new Error(`${testCase.name}: person identity grouping mismatch`);
      await page.screenshot({ path: path.join(path.dirname(testCase.file), `${testCase.name}-desktop-viewport.png`) });
      await page.screenshot({ path: path.join(path.dirname(testCase.file), `${testCase.name}-desktop.png`), fullPage: true });
      await page.setViewportSize({ width: 375, height: 812 });
      const mobile = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, innerWidth, findings: document.querySelectorAll('section[data-section="finding-details"] [data-finding-id]').length, facts: document.querySelectorAll('[data-fact]').length }));
      if (mobile.scrollWidth !== mobile.innerWidth || mobile.findings !== desktop.findings || mobile.facts !== desktop.facts) throw new Error(`${testCase.name}: mobile content mismatch`);
      await page.screenshot({ path: path.join(path.dirname(testCase.file), `${testCase.name}-mobile.png`), fullPage: true });
      if (pageErrors.length !== 0 || requests.some(url => !url.startsWith('file:'))) throw new Error(`${testCase.name}: page error or non-local request`);
      results.push({ name: testCase.name, desktop, mobile, pageErrors, requests });
      await page.close();
    }
    console.log(JSON.stringify(results, null, 2));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error.stack || String(error)); process.exitCode = 1; });
'@
& $node -e $browserScript
if ($LASTEXITCODE -ne 0) { throw 'Edge 多单元报告检查失败' }
```

## 结果判定

- 三个输入顺序及逆序必须与概览运行标识一致；重复路径别名应在模型请求前失败且不产生输出。finding ID 重复时，每条记录必须留在各自来源单元，不能按裸 ID 丢弃或合并。
- 页面包含九类章节、完整材料事实和单元统计；重叠范围仍按审查单元计数，不显示跨单元去重总量。未采集严重等级的问题仍在 finding 明细中出现，汇总使用单独的 `not_collected` 桶。可信邮箱身份带两个单元贡献与两个名称变体；相同显示名但无邮箱的记录保持单元范围。
- Edge 1440px 和 375px 页面均无横向溢出、页面异常或外部请求；只有打开的 HTML `file:` URL 可出现在请求记录中。保存 HTML 通过同目录临时文件、校验后硬链接发布；显式路径不覆盖，校验或发布失败不留下最终文件。

## 验证记录

- 2026-10-04 修正身份/未知等级 reviewer findings 后，在相同分支与基线上重跑：`go test ./internal/report -run '^(TestNewMultiReportInputKeepsEmailLocalPartCaseDistinct|TestNewMultiReportInputCountsMissingSeverityExplicitly)$' -count=1 -v` 退出码 0；最终 `go test ./internal/report -count=1 -timeout=75s` 退出码 0（2.106s），`go test ./cmd/opencodereview -run '^TestReportCommand' -count=1 -timeout=120s` 退出码 0（4.787s）。专用多输入 CLI 加入未知等级 finding 后退出码 0（2.492s）；Edge 验收及 `git diff --check` 均通过。
- 2026-10-04，使用本次受控 CLI 生成的两个 HTML 由 Edge `154.0.4258.53` 实际打开：各 9 章、8 条问题记录、258 个事实；输入顺序/逆序正确，四个相同问题 ID 均保留两条且归属不同审查单元。`not_collected` 汇总显示为“未提供等级数量：1”，对应的空严重性 finding 仍显示“未提供”及问题摘要，并留在 `multi-partial-first` 单元。两种顺序下可信邮箱的 Alice/A. Chen 是一个已验证身份并有两份单元贡献；无邮箱的两条 Sam Lee 记录保持分开。桌面 `scrollWidth=1440`，375px 屏幕 `scrollWidth=375`，仍保留完整问题和事实。页面错误、危险元素及外部网络请求均为 0；请求记录只有本地 HTML `file:` URL。
- 最新浏览器截图保存在运行临时目录 `C:\Users\60429\AppData\Local\Temp\ocr-report-08-N8Ywl4\`：顺序版/逆序版各有桌面视口、完整页面与窄屏截图；另有 `not-collected-summary.png` 和 `unknown-severity-finding.png`。已实际查看汇总行及 finding 截图，固定标签、数值和问题材料均清楚可读。
- 本工单没有进行真实模型调用；真实 DeepSeek 的 07 格式失败记录及待 09 复测要求见[单份 HTML 历史记录](html-report-single.md)。
- 临时 HTML 与截图不进入 Git；复测结束后仅在核实目标路径后按需删除该目录。
- 提交前检查：9 个改动 Go 文件的 SPDX 与版权头核验通过；这些文件 `gofmt -d` 无输出；`git diff --check` 通过。`go run scripts/verify-english-only.go` 退出码 1，仅报告基线已有的 `internal/report/html_test.go:38` 中文测试夹具；本工单新增 Go 源未被命中。绝对路径 Git Bash `C:\Program Files\Git\bin\bash.exe` 已解决 worktree `.git` 解析问题，但全仓 `scripts/verify-license.sh` 运行超过 60 秒仍无输出，随后按约定取消；因此 license-check 记为未验证，不能视为通过。
- 独立 reviewer `review_ticket_08` 已完成针对初审两项问题的定向复核：未知严重等级明确计入 `not_collected` 且 finding 保留，邮箱仅折叠 domain 大小写并区分 local-part 大小写；结论通过、无阻塞。
