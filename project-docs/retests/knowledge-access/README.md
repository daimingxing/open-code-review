# 外部知识访问回归

## 原生知识库迁移的常用样本（待实现）

真实效果复测使用[资源索引](../../review-resources.md#原生知识库常用验收样本)中的前端仓库、知识库和 `dev-1010...dev-1016`，不默认重复高成本的 `dev-1023...dev-1030`。可复用背景为 [xr-review-background.md](xr-review-background.md)，来源和提取边界见资源索引。

下面的 `--knowledge-dir` 尚未实现，供[新 Spec](../../../.scratch/native-knowledge/spec.md)开发完成后使用；`--background-file` 本身已有支持。当前仅写 background 不会开放知识目录。

在本项目根目录执行，使用构建后的本地 CLI 和已配置的模型：

```powershell
$cli = Join-Path (Get-Location) 'dist/opencodereview.exe'
$background = Join-Path (Get-Location) 'project-docs/retests/knowledge-access/xr-review-background.md'
& $cli review `
  --repo 'D:\WorkPlace\longruan_codeReview\jk_web' `
  --from dev-1010 --to dev-1016 `
  --knowledge-dir 'D:\WorkPlace\longruan_codeReview\jk_web\.ai_knowledge' `
  --background-file $background `
  --effort low --concurrency 1 --max-tokens-budget 1000000 `
  --report
```

这是付费模型效果测试，需主动执行。预算是累计输入加输出的 token 数（含缓存输入），并非货币额度或绝对硬上限；在途和收尾请求可能超出，触及预算允许只完成部分审查。记录实际覆盖与限制，不把部分结果当作完整验收。新实现应能通过原生工具按索引查阅真实知识，`--report` 生成报告 JSON；不在本命令内生成 HTML。

## 过渡状态与回归

2026-11-03：已移除旧文件 MCP 专用的知识来源采集及活动探针、配置和真实模型脚本；不再要求安装 @modelcontextprotocol/server-filesystem。通用 MCP 与参数反馈继续保留，可调用其他外部服务。

当前尚未实现原生知识参数，报告的 knowledge_sources 明确为 not_collected；已有报告 JSON 仍可用于 HTML 生成。既有读取证据只作为[历史档案](../archive/README.md)，不作为当前安装或验收步骤。
