# 临时启动文件 MCP 并验证授权根目录、只读工具白名单和路径边界。 # allow-non-english: 仓库约定要求复测文档使用中文
# 运行前设置 $McpEntry 为 @modelcontextprotocol/server-filesystem 的 dist/index.js。 # allow-non-english: 仓库约定要求复测文档使用中文
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string] $McpEntry,
    [Parameter(Mandatory = $true)] [string] $TestRoot,
    [Parameter(Mandatory = $true)] [string] $OutsideFile
)

$ErrorActionPreference = 'Stop'
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$testPath = [IO.Path]::GetFullPath($TestRoot)
if (-not $testPath.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw "TestRoot must be under the system temporary directory: $tempRoot"
}

$linkPath = Join-Path $testPath '__external-knowledge-boundary-link'
$linkAvailable = $false
$junctionPath = Join-Path $testPath '__external-knowledge-boundary-junction'
$junctionTarget = Join-Path (Split-Path -Parent $testPath) ("__ocr-junction-target-" + [guid]::NewGuid().ToString('N'))
$junctionAvailable = $false
try {
    New-Item -ItemType Directory -Path $testPath -Force | Out-Null
    New-Item -ItemType Directory -Path $junctionTarget | Out-Null
    $junctionFile = Join-Path $junctionTarget 'junction-outside.md'
    Set-Content -LiteralPath $junctionFile -Value 'junction boundary probe' -NoNewline
    try {
        New-Item -ItemType SymbolicLink -Path $linkPath -Target $OutsideFile | Out-Null
        $linkAvailable = $true
    }
    catch {
        Write-Warning "符号链接测试未执行：当前 Windows 会话没有创建符号链接所需权限。" # allow-non-english: 如实报告当前环境下未执行的测试
    }
    try {
        New-Item -ItemType Junction -Path $junctionPath -Target $junctionTarget | Out-Null
        $junctionAvailable = $true
    }
    catch {
        Write-Warning "Junction 测试未执行：无法创建目录联接。" # allow-non-english: 如实报告当前环境下未执行的测试
    }
    $node = (Get-Command node).Source.Replace('\', '/')
    $entry = (Resolve-Path -LiteralPath $McpEntry).Path.Replace('\', '/')
    $root = (Resolve-Path -LiteralPath $testPath).Path.Replace('\', '/')
    $parent = (Resolve-Path -LiteralPath (Join-Path $testPath '..')).Path.Replace('\', '/')
    $outside = (Resolve-Path -LiteralPath $OutsideFile).Path.Replace('\', '/')
    $link = $linkPath.Replace('\', '/')
    $junctionFilePath = (Join-Path $junctionPath 'junction-outside.md').Replace('\', '/')
    $linkFlag = if ($linkAvailable) { 'true' } else { 'false' }
    $junctionFlag = if ($junctionAvailable) { 'true' } else { 'false' }
    $script = @"
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
const transport = new StdioClientTransport({ command: '$node', args: ['$entry', '$root'] });
const client = new Client({ name: 'external-knowledge-boundary', version: '1.0.0' }, { capabilities: {} });
await client.connect(transport);
const result = { tools: (await client.listTools()).tools.map(tool => tool.name) };
result.allowed = await client.callTool({ name: 'list_allowed_directories', arguments: {} });
for (const [label, path] of Object.entries({ parent: '$parent', outside: '$outside', symlink: '$link', junction: '$junctionFilePath' })) {
  try { result[label] = await client.callTool({ name: 'read_text_file', arguments: { path } }); }
  catch (error) { result[label] = { error: String(error?.message ?? error) }; }
}
if (!$linkFlag) result.symlink = { skipped: '符号链接创建权限不足' }; // allow-non-english: 如实报告当前环境下未执行的测试
if (!$junctionFlag) result.junction = { skipped: 'Junction 创建失败' }; // allow-non-english: 如实报告当前环境下未执行的测试
console.log(JSON.stringify(result));
await client.close();
"@
    Push-Location (Split-Path -Parent $entry)
    try {
        node --input-type=module -e $script
    }
    finally {
        Pop-Location
    }
}
finally {
    if ($linkAvailable -and (Test-Path -LiteralPath $linkPath)) {
        [IO.File]::Delete($linkPath)
    }
    if ($junctionAvailable -and (Test-Path -LiteralPath $junctionPath)) {
        [IO.Directory]::Delete($junctionPath)
    }
    if (Test-Path -LiteralPath $junctionTarget) {
        [IO.Directory]::Delete($junctionTarget, $true)
    }
}
