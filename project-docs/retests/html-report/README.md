# HTML 报告回归

自动化测试检查智能体循环交付、真实暂存材料读取、共享压缩后继续完成、提前停止不发布、失败不覆盖和多份输入顺序。本阶段必跑验收看工具执行、退出状态、文件交付和清理，不以固定 DOM、筛选器、CSS 或脚本禁令判定通过。

首轮不因 HTML 内容、脚本或样式检查拒绝交付；提示词建议优先读取报告 JSON 与模板/样式，但不会增加源码路径权限拦截。样式文件是参考资源，应用不会再插入样式或控件。历史 01–09 验收证据仍在[验收档案索引](../archive/README.md)，本目录只保留可复跑入口。

浏览器可用于人工打开真实产物观察排版，不把未确定的展示形式写成通过条件。`verify-browser-report.cjs` 仍保留工单 09 的旧版控件断言，只适用于当时档案中的合成 HTML，不是当前必跑入口。

在 PowerShell 7、已加入 PATH 的全局 Go 1.25+ 环境中，从仓库根目录运行：

```powershell
$ErrorActionPreference = 'Stop'

go test ./internal/report ./internal/tool -count=1 -timeout=90s
if ($LASTEXITCODE -ne 0) { throw 'report/tool package tests failed' }
go test ./cmd/opencodereview -run '^(TestReportCommandAgentLoopReadsMaterialsWritesHTMLAndFinishes|TestReportCommandDoesNotPublishWhenLoopStopsEarly|TestReportCommandCompressesContextThenCompletes|TestStageReportResourcesCleansUpAfterWriteFailure|TestReportCommandMultiInputPreservesOrderAndRejectsDuplicateBeforeModel|TestReportCommandMultiInputCanRetryAfterModelFailure|TestReportCommandFailsWhenHTMLNeverWritten|TestReportCommandRejectsExistingExplicitOutputBeforeLoop)$' -count=1 -timeout=180s
if ($LASTEXITCODE -ne 0) { throw 'HTML agent-loop CLI tests failed' }
```

预期报告包测试和 CLI 智能体循环测试成功。本测试使用可控模型替身，不调用真实模型，也不证明视觉质量或浏览器安全性。
