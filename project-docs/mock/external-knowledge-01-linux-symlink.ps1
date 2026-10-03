# 使用 Podman 的 Alpine 容器验证真实目录符号链接不能越出 filesystem MCP 授权根。 # allow-non-english: 仓库约定要求复测文档使用中文
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$podman = (Get-Command podman -ErrorAction Stop).Source
& $podman machine start
if ($LASTEXITCODE -ne 0) {
    throw "podman machine start failed with exit code $LASTEXITCODE."
}

$startInfo = [Diagnostics.ProcessStartInfo]::new()
$startInfo.FileName = $podman
$startInfo.UseShellExecute = $false
$startInfo.RedirectStandardInput = $true
$startInfo.RedirectStandardOutput = $true
$startInfo.RedirectStandardError = $true
@(
    'run', '--rm', '-i', '--entrypoint', 'sh', 'docker.io/library/alpine:3.20', '-lc',
    'apk add --no-cache nodejs npm >/dev/null 2>&1 && mkdir -p /tmp/allowed /tmp/outside && printf secret > /tmp/outside/secret.md && ln -s /tmp/outside /tmp/allowed/escape && npx --yes @modelcontextprotocol/server-filesystem@2026.8.31 /tmp/allowed'
) | ForEach-Object { [void]$startInfo.ArgumentList.Add($_) }

$process = [Diagnostics.Process]::new()
$process.StartInfo = $startInfo
$stderrTask = $null
try {
    if (-not $process.Start()) { throw 'Could not start the Podman MCP process.' }
    $stderrTask = $process.StandardError.ReadToEndAsync()
    $stdinWriter = $process.StandardInput
    $output = $process.StandardOutput

    $initialize = @{
        jsonrpc = '2.0'; id = 1; method = 'initialize'
        params = @{
            protocolVersion = '2024-11-05'
            capabilities = @{}
            clientInfo = @{ name = 'external-knowledge-symlink-probe'; version = '1.0.0' }
        }
    } | ConvertTo-Json -Depth 8 -Compress
    $stdinWriter.WriteLine($initialize)
    $initializeResponse = $output.ReadLineAsync().WaitAsync([TimeSpan]::FromSeconds(90)).GetAwaiter().GetResult() | ConvertFrom-Json
    if ($initializeResponse.id -ne 1 -or -not $initializeResponse.result) {
        throw "Unexpected MCP initialize response: $($initializeResponse | ConvertTo-Json -Compress -Depth 8)"
    }

    $stdinWriter.WriteLine('{"jsonrpc":"2.0","method":"notifications/initialized"}')
    $call = @{
        jsonrpc = '2.0'; id = 2; method = 'tools/call'
        params = @{ name = 'read_text_file'; arguments = @{ path = '/tmp/allowed/escape/secret.md' } }
    } | ConvertTo-Json -Depth 8 -Compress
    $stdinWriter.WriteLine($call)
    $response = $output.ReadLineAsync().WaitAsync([TimeSpan]::FromSeconds(90)).GetAwaiter().GetResult() | ConvertFrom-Json
    if ($response.id -ne 2 -or -not $response.result.isError -or $response.result.content[0].text -notmatch 'symlink target outside allowed directories') {
        throw "Symlink escape was not rejected: $($response | ConvertTo-Json -Compress -Depth 8)"
    }
    $response.result | ConvertTo-Json -Depth 8
}
finally {
    if ($process -and -not $process.HasExited) {
        $process.StandardInput.Close()
        if (-not $process.WaitForExit(10000)) { $process.Kill($true) }
    }
    if ($stderrTask) {
        $stderr = $stderrTask.GetAwaiter().GetResult()
        if ($stderr) { Write-Verbose $stderr }
    }
    $process.Dispose()
}
