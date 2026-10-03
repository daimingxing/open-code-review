# 使用真实 filesystem MCP 核验文档缺失与按 head 参数读取部分正文的工具响应。
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string] $McpEntry,
    [Parameter(Mandatory = $true)] [string] $TestRoot
)

$ErrorActionPreference = 'Stop'
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$testPath = [IO.Path]::GetFullPath($TestRoot)
if (-not $testPath.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw "TestRoot must be under the system temporary directory: $tempRoot"
}

$partialPath = Join-Path $testPath 'partial.md'
New-Item -ItemType Directory -Path $testPath -Force | Out-Null
Set-Content -LiteralPath $partialPath -Value "evidence-first`nevidence-second`nevidence-third" -NoNewline

try {
    $node = (Get-Command node).Source.Replace('\', '/')
    $entry = (Resolve-Path -LiteralPath $McpEntry).Path.Replace('\', '/')
    $root = (Resolve-Path -LiteralPath $testPath).Path.Replace('\', '/')
    $partial = (Resolve-Path -LiteralPath $partialPath).Path.Replace('\', '/')
    $missing = (Join-Path $testPath 'missing.md').Replace('\', '/')
    $script = @"
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
const transport = new StdioClientTransport({ command: '$node', args: ['$entry', '$root'] });
const client = new Client({ name: 'external-knowledge-content-probe', version: '1.0.0' }, { capabilities: {} });
await client.connect(transport);
const allowed = await client.callTool({ name: 'list_allowed_directories', arguments: {} });
const missing = await client.callTool({ name: 'read_text_file', arguments: { path: '$missing' } });
const partial = await client.callTool({ name: 'read_text_file', arguments: { path: '$partial', head: 1 } });
console.log(JSON.stringify({ allowed, missing, partial }));
await client.close();
"@
    Push-Location (Split-Path -Parent $entry)
    try {
        node --input-type=module -e $script
        if ($LASTEXITCODE -ne 0) {
            throw "MCP content probe failed with exit code $LASTEXITCODE."
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    [IO.File]::Delete($partialPath)
}
