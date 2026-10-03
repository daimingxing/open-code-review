# 外部知识接入与真实效果复测

## 目的与前提

此场景核验原生 OCR 规则是否关联独立知识索引、背景是否仍由原生 `--background-file` 接收、文件 MCP 是否限制在授权知识目录和只读工具，以及真实模型是否按知识证据判断目标提交。真实样例来自[资源索引](../review-resources.md)，样例仓库与 `.ai_knowledge` 可能有用户未提交内容；复测只能读取，不要清理、切换或覆盖它们。

最近执行日期：2026-10-04。实测环境为 Windows 11、PowerShell 7、OCR v1.12.11（a758d9c）、Node.js v22.22.2、`@modelcontextprotocol/server-filesystem` 2026.8.31、DeepSeek `deepseek-flash`。真实模型测试使用样例仓库提交 `fd4fdae1dda41b4a6ad7193218d2585500307349` 与 `ab9d7dc11cc72f6413974992882aa25f8779d9a6`。重新运行前应确认资源索引中的路径和目标提交仍存在，并从自身授权模型配置取得服务凭据；不要把凭据写进仓库或终端输出。

配置模板 [`external-knowledge-01-config.frontend.json`](external-knowledge-01-config.frontend.json) 和 [`external-knowledge-01-config.backend.json`](external-knowledge-01-config.backend.json) 仅包含 MCP 覆盖段，不含凭据。将其 `mcp_servers` 合并进隔离用户配置；保留该隔离配置中已有的模型供应商凭据，并将 `command`、filesystem MCP 程序路径及授权目录替换成当前环境的绝对路径。不要改全局 OCR 配置。白名单限定为 `read_text_file`、`list_directory`、`search_files`、`get_file_info`、`list_allowed_directories`；filesystem 服务自身还会列出写工具，OCR 配置白名单必须阻止模型调用它们。

## 真实模型命令

以下为 PowerShell 7 命令。先设置环境变量，指向一个临时隔离的用户配置目录、已安装的 OCR CLI、规则和背景文件；`USERPROFILE` 应仅在当前进程环境生效。隔离配置应从已有用户配置副本建立，再合并对应 MCP 覆盖段，不要输出配置文件内容。

```powershell
$env:USERPROFILE = $isolatedHome
$repoFrontend = 'D:\WorkPlace\longruan_codeReview\jk_web'
$repoBackend = 'D:\WorkPlace\longruan_codeReview\jk'

ocr review --repo $repoFrontend --commit fd4fdae1 --rule $frontendRule --background-file $frontendBackground --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $testRoot 'frontend.json') --timeout 10 --max-tokens-budget 75000
ocr review --repo $repoBackend --commit ab9d7dc1 --rule $backendRule --background-file $backendBackground --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $testRoot 'backend.json') --timeout 10 --max-tokens-budget 120000
```

前端规则样例：[`external-knowledge-01-rule.frontend.json`](external-knowledge-01-rule.frontend.json)。规则路径关联 `.ai_knowledge/xr-framework-usage.md`，要求先读索引并按需读章节，核实 `@eplat/ei` 版本和 `EiBlock.getMappedRows`。原生背景文件提供审查焦点；期望模型引用 `eplatei-knowledge.md` 与 `02-eiblock.md`、版本 `@eplat/ei 2.2.1`，排除已受支持且有空值保护的 API 误报，并能结合当前差异发现实际问题。

后端规则样例：[`external-knowledge-01-rule.backend.json`](external-knowledge-01-rule.backend.json)。规则路径关联 `.ai_knowledge/02-服务调用.md`，要求精确比较负状态约定与 `STATUS_FAILURE` 等值判断。原生背景文件限定核验范围；期望模型实际读取索引/章节、引用文档路径并根据目标提交代码得出结论。仅凭审查结果“complete”或模型自行复述规则不算知识使用证据，必须核对 MCP `read_text_file` 工具调用及其文件路径。

输出目录中可能含代码片段和审查内容，应按项目数据处理；分享结果前检查并移除不需要的内容。清理临时隔离配置和模型输出时，仅删除本次创建且已核实位于系统临时目录的专用目录。

## MCP 路径边界复测

安装文件 MCP 到临时前缀后，可用 [`external-knowledge-01-boundary.ps1`](external-knowledge-01-boundary.ps1) 检查服务端授权根目录、根外和父目录访问。脚本仅在系统临时目录建测试目录和探测文件。参数 `McpEntry` 指向 filesystem MCP 的 `dist/index.js`；`OutsideFile` 必须是测试根之外的临时普通文件；`TestRoot` 必须是系统临时目录内的子目录。

```powershell
$tempRoot = Join-Path ([IO.Path]::GetTempPath()) 'ocr-knowledge-boundary'
$outsideFile = Join-Path ([IO.Path]::GetTempPath()) 'ocr-knowledge-outside.md'
Set-Content -LiteralPath $outsideFile -Value 'boundary probe' -NoNewline
& .\project-docs\mock\external-knowledge-01-boundary.ps1 `
  -McpEntry $mcpEntry `
  -TestRoot (Join-Path $tempRoot 'authorized-root') `
  -OutsideFile $outsideFile
Remove-Item -LiteralPath $outsideFile -Force
```

通过条件：`list_allowed_directories` 只列出测试根；父目录和根外文件读取返回 `Access denied`；若当前 Windows 权限允许创建符号链接，指向根外文件的链接也必须拒绝。此脚本连接 MCP 服务端，因此服务端 `listTools` 会展示它实现的全部工具；是否只向模型提供只读能力须另核对 OCR 会话中实际调用工具名与隔离配置白名单。

## 实际结果与限制

| 检查 | 结果 |
|---|---|
| 前端真实模型知识读取和误报排除 | 通过：DeepSeek Flash / OCR v1.12.11 读取 `eplatei-knowledge.md` 和 `02-eiblock.md`，依据 `@eplat/ei 2.2.1` 排除 `getMappedRows` 误报。目标差异为 `b401174f8d40f94d693da6401b2627fb23c8734d..fd4fdae1dda41b4a6ad7193218d2585500307349`；模型另检出 `fetchEnterpriseBaseInfo` 与 `fetchLicenses` 并发更新丢失 `businessLicenseObj.rid` 的问题。9 次工具调用无失败；模型用量 87,533 tokens，超过 75,000 预算并跳过第二轮，因此只代表已完成的首轮证据。 |
| 后端真实模型知识依赖问题 | 通过（第二次运行）：目标差异为 `35b22d0ad6de8e2dbfdae9f0e9d6e5188b6fe70e..ab9d7dc11cc72f6413974992882aa25f8779d9a6`；明确要求先经 MCP 读取索引和章节后，模型执行 21 次工具调用，其中 `list_allowed_directories` 2 次、`list_directory` 2 次、`read_text_file` 4 次。它引用 `02-服务调用.md` 的“01 本地服务调用(XLocalManager)”规则 `outInfo.getStatus() < 0`，并以目标提交中的 `== EiConstant.STATUS_FAILURE` 检出可能漏掉其他负状态的高优先级问题。 |
| 后端首次运行行为 | 首次规则虽要求读取知识，但 16 次调用仅为原生代码搜索/读取/评论工具，没有知识 MCP 调用；输出 complete 但只报告风格意见。隔离 MCP 配置正确且白名单非空。将规则强化为“在分析前必须成功调用 MCP 列目录、读索引和章节；读取失败须诚实说明”后，第二次运行实际调用知识 MCP 并产生知识依赖结论。故当前复测规则保留该明确顺序；首次结果仍作为模型遵循原规则不充分的证据。 |
| 只读能力 | 通过：前后端真实审查的实际调用均为 OCR 原生读取/检索/评论工具及 MCP 的 `list_allowed_directories`、`list_directory`、`read_text_file`；没有 MCP 写工具调用，隔离配置白名单仅含五个只读工具。filesystem 服务端 `listTools` 本身仍列有写工具，因此应以 OCR 配置白名单和会话实际调用共同判定模型权限。 |
| 父目录与根外路径 | 通过：filesystem 2026.8.31 对两种 `read_text_file` 请求均返回 `Access denied`；唯一授权目录是测试根。 |
| 符号链接越界 | 未验收：Windows 会话创建符号链接返回需 Administrator privilege；脚本保留为可在有权限环境重跑的检查，不把跳过当通过。 |
| 缺失/不可用/部分正文与 Markdown 直接注入后备 | 未运行：本轮真实模型与边界验收未覆盖这些失败场景，不标记通过。 |
| 历史提交读取 | 通过：两个 OCR manifest 均为 `mode=commit`，实际解析到上述完整提交范围，而不是读取工作树快照。 |

## 2026-10-04 执行命令补充

本次隔离运行时把全局配置副本放在系统临时目录，仅在副本中加入一个知识 MCP；前后端审查分别运行，以便每次 MCP 授权只指向对应仓库 `.ai_knowledge`。隔离目录是本机临时路径，命令主体如下；具体 `$frontendRule`、`$backendRule` 与背景文件内容见已保存样例和上文，不包含密钥。

```powershell
$env:USERPROFILE = $isolatedHome
ocr review --repo 'D:\WorkPlace\longruan_codeReview\jk_web' --commit fd4fdae1 --rule $frontendRule --background-file $frontendBackground --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $testRoot 'frontend.json') --timeout 10 --max-tokens-budget 75000
ocr review --repo 'D:\WorkPlace\longruan_codeReview\jk' --commit ab9d7dc1 --rule $backendRule --background-file $backendBackground --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $testRoot 'backend.json') --timeout 10 --max-tokens-budget 120000
```

前两条命令退出码均为 0，OCR manifest 状态均为 `complete`。前端报告 1 条高优先级缺陷并排除知识确认的 API 误报；后端首次只报告 1 条低优先级风格意见且未读知识，不能算效果通过。

首次运行后使用强化的 [`external-knowledge-01-rule.backend.json`](external-knowledge-01-rule.backend.json) 按以下命令重试，结果文件应另存以保留首次证据：

```powershell
ocr review --repo 'D:\WorkPlace\longruan_codeReview\jk' --commit ab9d7dc1 --rule $rule --background-file $background --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $testRoot 'backend-retry.json') --timeout 10 --max-tokens-budget 180000
```

第二次运行退出码为 0，OCR manifest 状态为 `complete`，21 次工具调用、失败 0 次、总用量 177,397 tokens；其中有 4 次知识正文读取。它报告 1 条高优先级知识依赖问题及两条低优先级意见。知识 MCP 调用路径、实际章节、提交范围和代码结论可在命令输出 JSON 的 `tool_calls`、`comments` 与 `manifest` 字段核验。两次后端运行均保留在隔离临时目录，供本机复核后清理。
