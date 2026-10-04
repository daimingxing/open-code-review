// 工单 09 的离线浏览器复测，只输出计数、交互结果和安全结论，不输出材料正文。 allow-non-english: 中文复测说明
const { chromium } = require('playwright');
const { pathToFileURL } = require('node:url');
const fs = require('node:fs');
const path = require('node:path');

function requireCheck(value, message) {
  if (!value) throw new Error(message);
}

(async () => {
  const root = process.env.OCR_REPORT_BROWSER_ROOT;
  requireCheck(root && fs.existsSync(root), '需要 OCR_REPORT_BROWSER_ROOT 指向报告目录'); // allow-non-english: 中文参数诊断
  const cases = ['single-short', 'single-long', 'multi-short', 'multi-long'];
  const browser = await chromium.launch({
    executablePath: process.env.OCR_EDGE_PATH || 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
    headless: true
  });
  const results = [];
  try {
    for (const name of cases) {
      const file = path.join(root, `${name}.html`);
      requireCheck(fs.existsSync(file), `${name}: 缺少 HTML`); // allow-non-english: 中文复测诊断
      const page = await browser.newPage();
      const errors = [];
      const requests = [];
      page.on('pageerror', error => errors.push(error.message));
      page.on('request', request => requests.push(request.url()));
      const fileURL = pathToFileURL(file).href;
      await page.goto(fileURL, { waitUntil: 'load' });
      const baseline = await page.evaluate(() => ({
        findings: document.querySelectorAll('article[data-finding-id]').length,
        facts: document.querySelectorAll('[data-fact]').length,
        unsafe: document.querySelectorAll('script,link,img,iframe,object,embed,form,svg,video,audio,canvas,dialog,noscript,noembed,noframes,[onclick],[onload],[src]').length,
        externalLinks: [...document.querySelectorAll('[href]')].filter(node => !node.getAttribute('href').startsWith('#')).length
      }));
      requireCheck(baseline.unsafe === 0 && baseline.externalLinks === 0, `${name}: 存在危险 DOM 或外部资源`); // allow-non-english: 中文复测诊断
      const viewports = [];
      for (const width of [1440, 375]) {
        await page.setViewportSize({ width, height: width === 375 ? 812 : 1000 });
        for (const group of ['unit', 'severity', 'category']) {
          await page.locator(`#report-filter-${group}-all`).check();
        }
        const controls = [];
        for (const group of ['unit', 'severity', 'category']) {
          const options = await page.locator(`input[name="report-filter-${group}"]`).all();
          for (let index = 1; index < options.length; index++) {
            await options[index].check();
            const correct = await page.evaluate(({ group, index }) => {
              const attribute = `data-review-${group}-index`;
              const nodes = [...document.querySelectorAll('article[data-finding-id]')];
              return nodes.every(node => (getComputedStyle(node).display !== 'none') === (node.getAttribute(attribute) === String(index - 1)));
            }, { group, index });
            requireCheck(correct, `${name}/${width}/${group}: 筛选与事实归属不一致`); // allow-non-english: 中文复测诊断
          }
          await page.locator(`#report-filter-${group}-all`).check();
          controls.push({ group, options: options.length });
        }
        await page.locator('#report-filter-unit-all').focus();
        await page.keyboard.press('Tab');
        const keyboardRadio = await page.evaluate(() => document.activeElement?.matches('input[type="radio"]'));
        requireCheck(keyboardRadio, `${name}/${width}: Tab 无法访问 radio`); // allow-non-english: 中文复测诊断
        await page.keyboard.press('Space');
        const summary = page.locator('details.finding-details > summary').first();
        let disclosure = '无问题'; // allow-non-english: 中文报告状态
        if (await summary.count()) {
          const details = summary.locator('..');
          await summary.focus();
          const focusVisible = await summary.evaluate(node => node.matches(':focus-visible'));
          await page.keyboard.press('Enter');
          requireCheck(!(await details.evaluate(node => node.open)), `${name}/${width}: Enter 未折叠`); // allow-non-english: 中文复测诊断
          await page.keyboard.press('Space');
          requireCheck(await details.evaluate(node => node.open), `${name}/${width}: Space 未展开`); // allow-non-english: 中文复测诊断
          requireCheck(focusVisible, `${name}/${width}: 缺少键盘聚焦状态`); // allow-non-english: 中文复测诊断
          disclosure = 'Tab/Enter/Space 通过'; // allow-non-english: 中文复测结果
        }
        const dimensions = await page.evaluate(() => ({
          width: innerWidth,
          scrollWidth: document.documentElement.scrollWidth,
          findings: document.querySelectorAll('article[data-finding-id]').length,
          facts: document.querySelectorAll('[data-fact]').length
        }));
        requireCheck(dimensions.scrollWidth <= width && dimensions.findings === baseline.findings && dimensions.facts === baseline.facts, `${name}/${width}: 横溢或事实变动`); // allow-non-english: 中文复测诊断
        await page.screenshot({ path: path.join(root, `${name}-${width}.png`), fullPage: true });
        viewports.push({ ...dimensions, controls, keyboardRadio, disclosure });
      }
      for (const group of ['unit', 'severity', 'category']) await page.locator(`#report-filter-${group}-all`).check();
      await page.locator('details.finding-details').evaluateAll(nodes => nodes.forEach(node => node.removeAttribute('open')));
      await page.emulateMedia({ media: 'print' });
      const print = await page.evaluate(() => ({
        filtersHidden: getComputedStyle(document.querySelector('.report-filters')).display === 'none',
        detailsVisible: [...document.querySelectorAll('.finding-detail-content')].every(node => getComputedStyle(node).display !== 'none'),
        facts: document.querySelectorAll('[data-fact]').length
      }));
      requireCheck(print.filtersHidden && print.detailsVisible && print.facts === baseline.facts, `${name}: 打印遗漏事实`); // allow-non-english: 中文复测诊断
      await page.pdf({ path: path.join(root, `${name}.pdf`), format: 'A4', printBackground: true });
      requireCheck(errors.length === 0 && requests.every(url => url === fileURL), `${name}: 页面错误或外联`); // allow-non-english: 中文复测诊断
      results.push({ name, ...baseline, viewports, print, pageErrors: errors.length, externalRequests: requests.filter(url => url !== fileURL).length });
      await page.close();
    }
    const result = { browser: browser.version(), results };
    fs.writeFileSync(path.join(root, 'browser-summary.json'), JSON.stringify(result, null, 2));
    console.log(JSON.stringify(result, null, 2));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error.message); process.exitCode = 1; });
