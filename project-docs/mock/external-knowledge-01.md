# 外部知识接入与真实效果复测

## 目的与前提

此场景核验原生 OCR 规则是否关联独立知识索引、背景是否仍由原生 `--background-file` 接收、文件 MCP 是否限制在授权知识目录和只读工具，以及真实模型是否按知识证据判断目标提交。真实样例来自[资源索引](../review-resources.md)，样例仓库与 `.ai_knowledge` 可能有用户未提交内容；复测只能读取，不要清理、切换或覆盖它们。

最近执行日期：2026-10-04。实测环境为 Windows 11、PowerShell 7、OCR v1.12.11（a758d9c）、Node.js v22.22.2、`@modelcontextprotocol/server-filesystem` 2026.8.31、DeepSeek `deepseek-flash`。真实模型测试使用样例仓库提交 `fd4fdae1dda41b4a6ad7193218d2585500307349` 与 `ab9d7dc11cc72f6413974992882aa25f8779d9a6`。重新运行前应确认资源索引中的路径和目标提交仍存在，并从自身授权模型配置取得服务凭据；不要把凭据写进仓库或终端输出。

配置模板 [`external-knowledge-01-config.frontend.json`](external-knowledge-01-config.frontend.json) 和 [`external-knowledge-01-config.backend.json`](external-knowledge-01-config.backend.json) 仅包含 MCP 覆盖段，不含凭据。下方 PS7 流程从当前用户的 OCR 配置安全读取既有模型配置，在内存中解析并保留 provider credential，仅将完整配置序列化到当前用户 ACL 保护的唯一系统临时目录；不打印凭据，不复制到仓库或其他长期配置，不修改来源配置。OCR CLI 运行时必须能从临时配置文件读取 provider credential，因此该短期隔离文件是运行所需的临时副本；脚本用 `finally` 精确删除本次创建的临时目录。若你的 OCR 使用自定义配置位置，只需调整 `$sourceConfigPath`。白名单限定为 `read_text_file`、`list_directory`、`search_files`、`get_file_info`、`list_allowed_directories`；filesystem 服务自身还会列出写工具，OCR 配置白名单必须阻止模型调用它们。

## 隔离准备与真实模型命令

以下代码块可在仓库根目录的 PowerShell 7 终端整段执行。前提是 OCR v1.12.11、Node.js/npm、已授权的 DeepSeek 配置，以及资源索引中两个只读真实样例仓库和目标提交存在；npm registry 可访问。它在系统临时目录安装 filesystem MCP，规则文件从仓库模板复制，背景文件写入临时目录。真实仓库与知识库仅通过 MCP 读取。真实模型调用会产生服务用量。

```powershell
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path '.').Path
$repoFrontend = 'D:\WorkPlace\longruan_codeReview\jk_web'
$repoBackend = 'D:\WorkPlace\longruan_codeReview\jk'
$sourceConfigPath = Join-Path $env:USERPROFILE '.opencodereview/config.json'
$runId = [guid]::NewGuid().ToString('N')
$testRoot = Join-Path ([IO.Path]::GetTempPath()) "ocr-external-knowledge-$runId"
$resultDirectory = Join-Path $testRoot 'results'
$isolatedHome = Join-Path $testRoot 'home'
$ocrConfigDirectory = Join-Path $isolatedHome '.opencodereview'
$isolatedConfigPath = Join-Path $ocrConfigDirectory 'config.json'
$mcpRoot = Join-Path $testRoot 'mcp'
$mcpEntry = Join-Path $mcpRoot 'node_modules/@modelcontextprotocol/server-filesystem/dist/index.js'
$frontendKnowledge = Join-Path $repoFrontend '.ai_knowledge'
$backendKnowledge = Join-Path $repoBackend '.ai_knowledge'
$frontendRule = Join-Path $testRoot 'rule.frontend.json'
$backendRule = Join-Path $testRoot 'rule.backend.json'
$frontendConfigTemplate = Join-Path $repoRoot 'project-docs/mock/external-knowledge-01-config.frontend.json'
$backendConfigTemplate = Join-Path $repoRoot 'project-docs/mock/external-knowledge-01-config.backend.json'
$frontendBackground = Join-Path $testRoot 'background.frontend.md'
$backendBackground = Join-Path $testRoot 'background.backend.md'
$oldUserProfile = $env:USERPROFILE
$oldNpmCache = $env:npm_config_cache

foreach ($path in @($repoFrontend, $repoBackend, $frontendKnowledge, $backendKnowledge, $sourceConfigPath)) {
    if (-not (Test-Path -LiteralPath $path)) { throw "Required local input is missing: $path" }
}
git -C $repoFrontend cat-file -e 'fd4fdae1^{commit}'
if ($LASTEXITCODE -ne 0) { throw 'Frontend target commit fd4fdae1 is unavailable.' }
git -C $repoBackend cat-file -e 'ab9d7dc1^{commit}'
if ($LASTEXITCODE -ne 0) { throw 'Backend target commit ab9d7dc1 is unavailable.' }

New-Item -ItemType Directory -Path $testRoot | Out-Null
$identity = [Security.Principal.WindowsIdentity]::GetCurrent().User
$acl = [Security.AccessControl.DirectorySecurity]::new()
$acl.SetAccessRuleProtection($true, $false)
$acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
    $identity, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow'))
Set-Acl -LiteralPath $testRoot -AclObject $acl
New-Item -ItemType Directory -Path $isolatedHome, $ocrConfigDirectory, $mcpRoot, $resultDirectory | Out-Null
Write-Output "Temporary review root (contains the short-lived isolated config): $testRoot"

try {
    # 仅从授权配置读取，不把 provider credential 写入输出或工作区文件。
    $userConfig = Get-Content -LiteralPath $sourceConfigPath -Raw | ConvertFrom-Json -AsHashtable
    $userConfig['mcp_servers'] = @{}

    $env:npm_config_cache = Join-Path $testRoot 'npm-cache'
    npm install --prefix $mcpRoot --ignore-scripts --no-audit --no-fund '@modelcontextprotocol/server-filesystem@2026.8.31'
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $mcpEntry)) { throw 'Installing filesystem MCP failed.' }
    $node = (Get-Command node -ErrorAction Stop).Source

    Copy-Item -LiteralPath (Join-Path $repoRoot 'project-docs/mock/external-knowledge-01-rule.frontend.json') $frontendRule
    Copy-Item -LiteralPath (Join-Path $repoRoot 'project-docs/mock/external-knowledge-01-rule.backend.json') $backendRule
    Set-Content -LiteralPath $frontendBackground -Encoding utf8 -Value @'
审查目标：fd4fdae1。仅按目标提交范围判断。核对 EiBlock.getMappedRows 是否受当前 @eplat/ei 版本支持，并排除知识已证明可用且代码已做空值保护的误报；同时检查变更中的真实并发状态问题。知识必须通过规则指定的独立知识目录读取。
'@
    Set-Content -LiteralPath $backendBackground -Encoding utf8 -Value @'
审查目标：ab9d7dc1。仅按目标提交范围判断。重点核对 XLocalManager.call 包装中的成功状态判断，严格依据独立知识目录中的服务调用章节，不从常识推断状态语义；读取失败或正文不完整时如实说明。
'@

    function Set-IsolatedKnowledgeRoot([string] $knowledgeRoot, [string] $templatePath) {
        $override = Get-Content -LiteralPath $templatePath -Raw | ConvertFrom-Json -AsHashtable
        $knowledgeServer = $override['mcp_servers']['knowledge']
        $knowledgeServer['command'] = $node
        $knowledgeServer['args'] = @($mcpEntry, $knowledgeRoot)
        $userConfig['mcp_servers'] = @{ knowledge = $knowledgeServer }
        $json = ConvertTo-Json -InputObject $userConfig -Depth 100
        [IO.File]::WriteAllText($isolatedConfigPath, $json, [Text.UTF8Encoding]::new($false))
    }

    $env:USERPROFILE = $isolatedHome
    $testRoot
    Set-IsolatedKnowledgeRoot $frontendKnowledge $frontendConfigTemplate
    ocr review --repo $repoFrontend --commit fd4fdae1 --rule $frontendRule --background-file $frontendBackground --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $resultDirectory 'frontend.json') --timeout 10 --max-tokens-budget 75000
    if ($LASTEXITCODE -ne 0) { throw "Frontend OCR review failed with exit code $LASTEXITCODE." }

    Set-IsolatedKnowledgeRoot $backendKnowledge $backendConfigTemplate
    ocr review --repo $repoBackend --commit ab9d7dc1 --rule $backendRule --background-file $backendBackground --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $resultDirectory 'backend-first.json') --timeout 10 --max-tokens-budget 120000
    if ($LASTEXITCODE -ne 0) { throw "Backend first OCR review failed with exit code $LASTEXITCODE." }

    # 保留首次结果；后端定向重试使用显式定义的 $backendRule 和 $backendBackground。
    ocr review --repo $repoBackend --commit ab9d7dc1 --rule $backendRule --background-file $backendBackground --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $resultDirectory 'backend-retry.json') --timeout 10 --max-tokens-budget 180000
    if ($LASTEXITCODE -ne 0) { throw "Backend retry OCR review failed with exit code $LASTEXITCODE." }
}
finally {
    $env:USERPROFILE = $oldUserProfile
    if ($null -eq $oldNpmCache) { Remove-Item Env:npm_config_cache -ErrorAction SilentlyContinue }
    else { $env:npm_config_cache = $oldNpmCache }
    $savedResults = $null
    if ((Test-Path -LiteralPath $resultDirectory) -and (Get-ChildItem -LiteralPath $resultDirectory -File -ErrorAction SilentlyContinue | Select-Object -First 1)) {
        $savedResults = Join-Path ([IO.Path]::GetTempPath()) "ocr-external-knowledge-results-$runId"
        Move-Item -LiteralPath $resultDirectory -Destination $savedResults
    }
    if (Test-Path -LiteralPath $testRoot) { Remove-Item -LiteralPath $testRoot -Recurse -Force }
    if ($savedResults) {
        Write-Output "Review JSON results (may contain repository code): $savedResults"
        Write-Output "After inspection, remove only that directory with: Remove-Item -LiteralPath '$savedResults' -Recurse -Force"
    }
}
```

前端规则样例：[`external-knowledge-01-rule.frontend.json`](external-knowledge-01-rule.frontend.json)。规则路径关联 `.ai_knowledge/xr-framework-usage.md`，要求先读索引并按需读章节，核实 `@eplat/ei` 版本和 `EiBlock.getMappedRows`。原生背景文件提供审查焦点；期望模型引用 `eplatei-knowledge.md` 与 `02-eiblock.md`、版本 `@eplat/ei 2.2.1`，排除已受支持且有空值保护的 API 误报，并能结合当前差异发现实际问题。

后端规则样例：[`external-knowledge-01-rule.backend.json`](external-knowledge-01-rule.backend.json)。规则路径关联 `.ai_knowledge/02-服务调用.md`，要求精确比较负状态约定与 `STATUS_FAILURE` 等值判断。原生背景文件限定核验范围；期望模型实际读取索引/章节、引用文档路径并根据目标提交代码得出结论。仅凭审查结果“complete”或模型自行复述规则不算知识使用证据，必须核对 MCP `read_text_file` 工具调用及其文件路径。

结果 JSON 移到 ACL 仅授予当前 Windows 用户的独立临时结果目录，供命令结束后检查；它可能含代码片段和审查内容，应按项目数据处理。输出中会给出结果目录和精确清理命令。隔离配置、依赖、规则与背景文件在 `finally` 中删除；若进程被强制终止导致清理未运行，先核对输出的唯一临时根路径确属本次运行，再精确删除该目录。不要递归清理系统临时目录中的其他内容。

## MCP 路径边界复测

安装文件 MCP 到临时前缀后，可用 [`external-knowledge-01-boundary.ps1`](external-knowledge-01-boundary.ps1) 检查服务端授权根目录、根外和父目录访问。脚本仅在系统临时目录建测试目录和探测文件。参数 `McpEntry` 指向 filesystem MCP 的 `dist/index.js`；`OutsideFile` 必须是测试根之外的临时普通文件；`TestRoot` 必须是系统临时目录内的子目录。

```powershell
$ErrorActionPreference = 'Stop'
$runRoot = Join-Path ([IO.Path]::GetTempPath()) ('ocr-knowledge-boundary-' + [guid]::NewGuid().ToString('N'))
$mcpRoot = Join-Path $runRoot 'mcp'
$mcpEntry = Join-Path $mcpRoot 'node_modules/@modelcontextprotocol/server-filesystem/dist/index.js'
$tempRoot = Join-Path $runRoot 'authorized'
$outsideFile = Join-Path $runRoot 'boundary-outside.md'
$npmCache = Join-Path $runRoot 'npm-cache'
$oldNpmCache = $env:npm_config_cache
try {
    New-Item -ItemType Directory -Path $runRoot | Out-Null
    $env:npm_config_cache = $npmCache
    npm install --prefix $mcpRoot --ignore-scripts --no-audit --no-fund '@modelcontextprotocol/server-filesystem@2026.8.31'
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $mcpEntry)) { throw 'Installing filesystem MCP failed.' }
    Set-Content -LiteralPath $outsideFile -Value 'boundary probe' -NoNewline
    & .\project-docs\mock\external-knowledge-01-boundary.ps1 `
      -McpEntry $mcpEntry `
      -TestRoot (Join-Path $tempRoot 'authorized-root') `
      -OutsideFile $outsideFile
    if ($LASTEXITCODE -ne 0) { throw "Boundary probe failed with exit code $LASTEXITCODE." }
}
finally {
    if ($null -eq $oldNpmCache) { Remove-Item Env:npm_config_cache -ErrorAction SilentlyContinue }
    else { $env:npm_config_cache = $oldNpmCache }
    if (Test-Path -LiteralPath $runRoot) { Remove-Item -LiteralPath $runRoot -Recurse -Force }
}
```

通过条件：`list_allowed_directories` 只列出测试根；父目录和根外文件读取返回 `Access denied`；指向授权根外目标的文件符号链接或目录 Junction 也必须拒绝。此脚本连接 MCP 服务端，因此服务端 `listTools` 会展示它实现的全部工具；是否只向模型提供只读能力须另核对 OCR 会话中实际调用工具名与隔离配置白名单。Windows 的 Junction 是目录联接，不等同于文件符号链接，需分别记录。

Linux 真符号链接复测使用 [external-knowledge-01-linux-symlink.ps1](external-knowledge-01-linux-symlink.ps1)，前提是 PowerShell 7、Podman machine、网络可访问 Alpine package repository 与 npm registry。脚本启动 `docker.io/library/alpine:3.20`，用 `apk` 安装 Node.js/npm，通过 `npx` 获取 `@modelcontextprotocol/server-filesystem@2026.8.31`，在容器中建立 `/tmp/allowed/escape -> /tmp/outside` 并请求读取链接下的文件。2026-10-04 实测返回 `isError: true`，正文 `Access denied - symlink target outside allowed directories: /tmp/outside/secret.md not in /tmp/allowed`。Windows 当前会话创建普通文件符号链接提示需要管理员权限，仍未在 Windows 本机验证；Linux 真实符号链接和 Windows Junction 分别验证，不互相替代。测试容器使用 `--rm`，退出后移除；主线程在测试后停止 Podman machine。

### 缺失文档与部分正文探针

下列命令使用真实 filesystem MCP，但仅处理临时目录中的探针文件，不调用模型：

```powershell
$ErrorActionPreference = 'Stop'
$runRoot = Join-Path ([IO.Path]::GetTempPath()) ('ocr-knowledge-content-' + [guid]::NewGuid().ToString('N'))
$mcpRoot = Join-Path $runRoot 'mcp'
$mcpEntry = Join-Path $mcpRoot 'node_modules/@modelcontextprotocol/server-filesystem/dist/index.js'
$npmCache = Join-Path $runRoot 'npm-cache'
$oldNpmCache = $env:npm_config_cache
try {
    New-Item -ItemType Directory -Path $runRoot | Out-Null
    $env:npm_config_cache = $npmCache
    npm install --prefix $mcpRoot --ignore-scripts --no-audit --no-fund '@modelcontextprotocol/server-filesystem@2026.8.31'
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $mcpEntry)) { throw 'Installing filesystem MCP failed.' }
    & .\project-docs\mock\external-knowledge-01-content.ps1 `
      -McpEntry $mcpEntry `
      -TestRoot (Join-Path $runRoot 'authorized-root')
    if ($LASTEXITCODE -ne 0) { throw "Content probe failed with exit code $LASTEXITCODE." }
}
finally {
    if ($null -eq $oldNpmCache) { Remove-Item Env:npm_config_cache -ErrorAction SilentlyContinue }
    else { $env:npm_config_cache = $oldNpmCache }
    if (Test-Path -LiteralPath $runRoot) { Remove-Item -LiteralPath $runRoot -Recurse -Force }
}
```

2026-10-04，Node.js v22.22.2、`@modelcontextprotocol/server-filesystem` 2026.8.31 实测：授权列表只含探针根；读取不存在的 `missing.md` 返回 `isError: true` 和 `ENOENT`；`read_text_file` 对三行 `partial.md` 指定 `head: 1` 时仅返回 `evidence-first`，不返回后两行，也没有声称内容完整的元数据。脚本在 `finally` 中删除探针正文文件。

下面代码块独立准备临时 Git 仓库、缺失/部分知识文档和私有隔离 OCR 配置，然后分别验证部分正文与服务不可用。所有变量在块内定义；只在你已有 DeepSeek provider credential 的情况下运行。模型调用可能产生费用。

```powershell
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path '.').Path
$backendConfigTemplate = Join-Path $repoRoot 'project-docs/mock/external-knowledge-01-config.backend.json'
$sourceConfigPath = Join-Path $env:USERPROFILE '.opencodereview/config.json'
$oldUserProfile = $env:USERPROFILE
$oldNpmCache = $env:npm_config_cache
$failureRoot = Join-Path ([IO.Path]::GetTempPath()) ('ocr-knowledge-failure-' + [guid]::NewGuid().ToString('N'))
$isolatedHome = Join-Path $failureRoot 'home'
$configDirectory = Join-Path $isolatedHome '.opencodereview'
$configPath = Join-Path $configDirectory 'config.json'
$mcpRoot = Join-Path $failureRoot 'mcp'
$knowledgeRoot = Join-Path $failureRoot 'knowledge'
$fixtureRepo = Join-Path $failureRoot 'repo'
$outputDirectory = Join-Path $failureRoot 'output'
$mcpEntry = Join-Path $mcpRoot 'node_modules/@modelcontextprotocol/server-filesystem/dist/index.js'
$rulePath = Join-Path $failureRoot 'rule.json'
$node = (Get-Command node -ErrorAction Stop).Source
$npmCache = Join-Path $failureRoot 'npm-cache'
$mcpTools = @('read_text_file', 'list_directory', 'search_files', 'get_file_info', 'list_allowed_directories')

New-Item -ItemType Directory -Path $failureRoot | Out-Null
$identity = [Security.Principal.WindowsIdentity]::GetCurrent().User
$acl = [Security.AccessControl.DirectorySecurity]::new()
$acl.SetAccessRuleProtection($true, $false)
$acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
    $identity, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow'))
Set-Acl -LiteralPath $failureRoot -AclObject $acl
New-Item -ItemType Directory -Path $isolatedHome, $configDirectory, $mcpRoot, $knowledgeRoot, $fixtureRepo, $outputDirectory | Out-Null
Write-Output "Temporary failure-probe root (contains the short-lived isolated config): $failureRoot"

try {
    $userConfig = Get-Content -LiteralPath $sourceConfigPath -Raw | ConvertFrom-Json -AsHashtable
    $userConfig['mcp_servers'] = @{}
    $env:npm_config_cache = $npmCache
    npm install --prefix $mcpRoot --ignore-scripts --no-audit --no-fund '@modelcontextprotocol/server-filesystem@2026.8.31'
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $mcpEntry)) { throw 'Installing filesystem MCP failed.' }

    Set-Content -LiteralPath (Join-Path $knowledgeRoot 'index.md') -Encoding utf8 -Value @'
# 临时知识索引
- missing-chapter.md：预期但不存在的文档。
- partial.md：刻意要求只读取一行的部分正文。
'@
    Set-Content -LiteralPath (Join-Path $knowledgeRoot 'partial.md') -Encoding utf8 -NoNewline -Value "evidence-first`nevidence-second`nevidence-third"
    Set-Content -LiteralPath (Join-Path $fixtureRepo 'probe.txt') -Encoding utf8 -Value 'initial fixture'
    git -C $fixtureRepo init
    git -C $fixtureRepo config user.name 'OCR fixture'
    git -C $fixtureRepo config user.email 'ocr-fixture@example.invalid'
    git -C $fixtureRepo add probe.txt
    git -C $fixtureRepo commit -m 'fixture baseline'
    New-Item -ItemType Directory -Path (Join-Path $fixtureRepo 'src') | Out-Null
    Set-Content -LiteralPath (Join-Path $fixtureRepo 'src/probe.js') -Encoding utf8 -Value 'function readValue(value) { return value.trim(); }'
    git -C $fixtureRepo add src/probe.js
    git -C $fixtureRepo commit -m 'fixture review target'

    $probeRuleText = @"
$knowledgeRoot/index.md
Before reaching any code conclusion, call the configured knowledge MCP to list its authorized root and read index.md. Then attempt to read missing-chapter.md and report its actual failure without claiming its contents. Read partial.md with head=1; state that only the returned line was read and do not claim full chapter verification. Base code review comments only on source evidence and state the knowledge limitations.
"@
    $ruleObject = @{ rules = @(@{ path = 'src/probe.js'; rule = $probeRuleText }) }
    [IO.File]::WriteAllText($rulePath, (ConvertTo-Json $ruleObject -Depth 10), [Text.UTF8Encoding]::new($false))

    function Write-FailureProbeConfig([string] $command, [string[]] $arguments) {
        $override = Get-Content -LiteralPath $backendConfigTemplate -Raw | ConvertFrom-Json -AsHashtable
        $knowledgeServer = $override['mcp_servers']['knowledge']
        $knowledgeServer['command'] = $command
        $knowledgeServer['args'] = $arguments
        $userConfig['mcp_servers'] = @{ knowledge = $knowledgeServer }
        [IO.File]::WriteAllText($configPath, (ConvertTo-Json $userConfig -Depth 100), [Text.UTF8Encoding]::new($false))
    }

    $env:USERPROFILE = $isolatedHome
    Write-FailureProbeConfig $node @($mcpEntry, $knowledgeRoot)
    ocr review --repo $fixtureRepo --rule $rulePath --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $outputDirectory 'missing-partial.json') --timeout 10 --max-tokens-budget 60000
    if ($LASTEXITCODE -ne 0) { throw "Missing/partial knowledge review failed with exit code $LASTEXITCODE." }

    Write-FailureProbeConfig (Join-Path $failureRoot 'missing-filesystem-mcp.exe') @()
    ocr review --repo $fixtureRepo --rule $rulePath --provider deepseek --model deepseek-flash --audience agent --format json --output (Join-Path $outputDirectory 'mcp-unavailable.json') --timeout 10 --max-tokens-budget 60000
    if ($LASTEXITCODE -ne 0) { throw "Unavailable MCP review failed with exit code $LASTEXITCODE." }
}
finally {
    $env:USERPROFILE = $oldUserProfile
    if ($null -eq $oldNpmCache) { Remove-Item Env:npm_config_cache -ErrorAction SilentlyContinue }
    else { $env:npm_config_cache = $oldNpmCache }
    $savedFailureResults = $null
    if ((Test-Path -LiteralPath $outputDirectory) -and (Get-ChildItem -LiteralPath $outputDirectory -File -ErrorAction SilentlyContinue | Select-Object -First 1)) {
        $savedFailureResults = Join-Path ([IO.Path]::GetTempPath()) ("ocr-knowledge-failure-results-" + [IO.Path]::GetFileName($failureRoot))
        Move-Item -LiteralPath $outputDirectory -Destination $savedFailureResults
    }
    if (Test-Path -LiteralPath $failureRoot) { Remove-Item -LiteralPath $failureRoot -Recurse -Force }
    if ($savedFailureResults) {
        Write-Output "Review JSON results (may contain fixture source): $savedFailureResults"
        Write-Output "After inspection, remove only that directory with: Remove-Item -LiteralPath '$savedFailureResults' -Recurse -Force"
    }
}
```

若进程被强制终止，先确认它仍是刚输出的 `$failureRoot` 且位于系统临时目录，再执行 `Remove-Item -LiteralPath $failureRoot -Recurse -Force`；正常完成时隔离配置自动删除，唯独保留待人工检查的结果 JSON。

2026-10-04 实际退出码为 0，OCR 状态 `complete`，10 次调用。工具调用记录含索引读取、缺失章节的 `ENOENT` 响应，以及仅 `head: 1` 的部分章节读取。模型明确说缺失章节未读、只取得部分正文，不能声称了解缺失章节或已核验完整知识；结论限于已读证据。这里的 `complete` 仅表示本次代码审查覆盖完成，不表示知识读取完整。

### MCP 服务不可用

在隔离配置中把 MCP 命令改成不存在的临时可执行文件（不得改全局配置），再执行相同临时仓库的真实模型审查。2026-10-04 OCR v1.12.11 输出原始警告 `failed to start MCP server "knowledge"` / `The system cannot find the file specified`，并继续审查；OCR manifest 仍为 `complete`，6 次工具调用中 1 次原生 `file_read` 失败，失败原因保留在 `failure_details`。DeepSeek 模型说明 MCP 不可用、索引/章节均未读，随后原生 `file_read` 也因路径不在仓库授权范围失败；没有声称读过知识，结论仅根据临时仓库实际代码。该结果同样说明 `complete` 不是知识验收状态。

### 小型 Markdown 直接注入

原生 `--rule` 规则字段可直接承载小型 Markdown 正文，无需知识服务或新规则协议。以临时规则 JSON 关联 `src/probe.js` 并执行 `ocr rules check --repo . --rule $scratchRule src/probe.js`，2026-10-04 输出 `Source: Custom (--rule)`，并逐行显示正文中的 `# 隔离验收知识`、`getMappedRows` 版本说明和限制语句。探针规则 JSON 位于系统临时目录并在命令结束时删除。此方式适合短而稳定的知识；较大知识仍用规则关联索引和文件 MCP 按需读取。

## 实际结果与限制

| 检查 | 结果 |
|---|---|
| 前端真实模型知识读取和误报排除 | 通过：DeepSeek Flash / OCR v1.12.11 读取 `eplatei-knowledge.md` 和 `02-eiblock.md`，依据 `@eplat/ei 2.2.1` 排除 `getMappedRows` 误报。目标差异为 `b401174f8d40f94d693da6401b2627fb23c8734d..fd4fdae1dda41b4a6ad7193218d2585500307349`；模型另检出 `fetchEnterpriseBaseInfo` 与 `fetchLicenses` 并发更新丢失 `businessLicenseObj.rid` 的问题。9 次工具调用无失败；模型用量 87,533 tokens，超过 75,000 预算并跳过第二轮，因此只代表已完成的首轮证据。 |
| 后端真实模型知识依赖问题 | 通过（第二次运行）：目标差异为 `35b22d0ad6de8e2dbfdae9f0e9d6e5188b6fe70e..ab9d7dc11cc72f6413974992882aa25f8779d9a6`；明确要求先经 MCP 读取索引和章节后，模型执行 21 次工具调用，其中 `list_allowed_directories` 2 次、`list_directory` 2 次、`read_text_file` 4 次。它引用 `02-服务调用.md` 的“01 本地服务调用(XLocalManager)”规则 `outInfo.getStatus() < 0`，并以目标提交中的 `== EiConstant.STATUS_FAILURE` 检出可能漏掉其他负状态的高优先级问题。 |
| 后端首次运行行为 | 首次规则虽要求读取知识，但 16 次调用仅为原生代码搜索/读取/评论工具，没有知识 MCP 调用；输出 complete 但只报告风格意见。隔离 MCP 配置正确且白名单非空。将规则强化为“在分析前必须成功调用 MCP 列目录、读索引和章节；读取失败须诚实说明”后，第二次运行实际调用知识 MCP 并产生知识依赖结论。故当前复测规则保留该明确顺序；首次结果仍作为模型遵循原规则不充分的证据。 |
| 只读能力 | 通过：前后端真实审查的实际调用均为 OCR 原生读取/检索/评论工具及 MCP 的 `list_allowed_directories`、`list_directory`、`read_text_file`；没有 MCP 写工具调用，隔离配置白名单仅含五个只读工具。filesystem 服务端 `listTools` 本身仍列有写工具，因此应以 OCR 配置白名单和会话实际调用共同判定模型权限。 |
| 父目录与根外路径 | 通过：filesystem 2026.8.31 对两种 `read_text_file` 请求均返回 `Access denied`；唯一授权目录是测试根。 |
| 符号链接越界 | 通过：Podman Alpine 3.20 中创建真实目录符号链接，filesystem 2026.8.31 返回 `isError: true` 和 `Access denied - symlink target outside allowed directories: /tmp/outside/secret.md not in /tmp/allowed`。Windows 当前会话创建普通文件符号链接提示需要管理员权限，故 Windows 本机该能力未验；Windows Junction 已另行成功创建且越界读取拒绝。 |
| 文档缺失与部分正文 | 真实 filesystem MCP 返回缺失文件 `ENOENT` 与 `isError: true`；`head: 1` 仅返回首行。DeepSeek 真实模型根据这两种响应明确说明知识缺失/不完整，没有声称已读或完整核验；审查 `complete` 仅指代码文件覆盖。 |
| MCP 服务不可用 | OCR 发出 MCP 启动失败原始警告并继续审查；模型明确未读知识且引用失败证据，没有伪称知识结论。输出状态 `complete` 仍仅为代码覆盖状态。 |
| 小型 Markdown 直接注入 | 原生规则检查输出临时规则中的多行 Markdown 正文，确认文字直接注入规则内容；无需 MCP。 |
| 历史提交读取 | 通过：两个 OCR manifest 均为 `mode=commit`，实际解析到上述完整提交范围，而不是读取工作树快照。 |

## 2026-10-04 执行命令补充

实际执行时后端知识规则先尝试一次、发现未调用知识 MCP，随后按上述后端定向重试要求补跑。前端退出码 0，9 次工具调用无失败；后端首次退出码 0、16 次工具调用但没有知识 MCP 调用，不算效果通过。后端定向重试退出码 0，OCR 状态 `complete`，21 次工具调用、失败 0 次、总用量 177,397 tokens，其中有 4 次知识正文读取。它报告 1 条高优先级知识依赖问题及两条低优先级意见。检查 JSON 时应核对 `tool_calls` 中实际知识路径、`comments` 中结论与 `manifest` 中目标提交范围；状态 `complete` 单独不构成知识应用证据。隔离运行根在本次执行后清理，若需复核应重跑上述完整准备流程并在脚本结束前查看本地产物。
