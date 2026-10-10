[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string] $MaterialPath,
    [string] $GoExecutable
)

$ErrorActionPreference = 'Stop'
if ($PSVersionTable.PSVersion.Major -lt 7) {
    throw '需要 PowerShell 7。'
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..')).Path
$inputPath = (Resolve-Path -LiteralPath $MaterialPath).Path
if ([string]::IsNullOrWhiteSpace($GoExecutable)) {
    $GoExecutable = (Get-Command go -ErrorAction Stop).Source
}
if (-not (Test-Path -LiteralPath $GoExecutable -PathType Leaf)) {
    throw "找不到 Go 可执行文件：$GoExecutable"
}

$runRoot = Join-Path ([IO.Path]::GetTempPath()) ('ocr-report-manual-' + [guid]::NewGuid().ToString('N'))
$exe = Join-Path $runRoot 'opencodereview.exe'
$output = Join-Path $runRoot 'report.html'
New-Item -ItemType Directory -Path $runRoot | Out-Null
try {
    & $GoExecutable build -C $repoRoot -o $exe ./cmd/opencodereview
    if ($LASTEXITCODE -ne 0) {
        throw '本地 CLI 构建失败。'
    }

    & $exe report --input $inputPath --output $output
    if ($LASTEXITCODE -ne 0) {
        throw 'HTML 报告生成失败；请检查 CLI 诊断和模型服务状态。'
    }

    Start-Process -FilePath $output
    Write-Output "报告 HTML：$output"
}
catch {
    if (Test-Path -LiteralPath $runRoot) {
        Remove-Item -LiteralPath $runRoot -Recurse -Force
    }
    throw
}
