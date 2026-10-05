param(
    [Parameter(Mandatory)]
    [string] $GoExecutable
)

$ErrorActionPreference = 'Stop'
if ($PSVersionTable.PSVersion.Major -lt 7) {
    throw 'Run this acceptance script with PowerShell 7.'
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$frontendRepo = 'D:\WorkPlace\longruan_codeReview\jk_web'
$backendRepo = 'D:\WorkPlace\longruan_codeReview\jk'
$sourceConfigPath = Join-Path $env:USERPROFILE '.opencodereview/config.json'
$runId = [guid]::NewGuid().ToString('N')
$runRoot = Join-Path ([IO.Path]::GetTempPath()) "ocr-final-knowledge-$runId"
$resultRoot = Join-Path ([IO.Path]::GetTempPath()) "ocr-final-knowledge-results-$runId"
$isolatedHome = Join-Path $runRoot 'home'
$configDirectory = Join-Path $isolatedHome '.opencodereview'
$configPath = Join-Path $configDirectory 'config.json'
$mcpRoot = Join-Path $runRoot 'mcp'
$mcpEntry = Join-Path $mcpRoot 'node_modules/@modelcontextprotocol/server-filesystem/dist/index.js'
$resultDirectory = Join-Path $runRoot 'results'
$exe = Join-Path $runRoot 'ocr.exe'
$node = (Get-Command node -ErrorAction Stop).Source
$npm = (Get-Command npm -ErrorAction Stop).Source
$oldUserProfile = $env:USERPROFILE
$oldNpmCache = $env:npm_config_cache

function Set-CurrentUserOnlyAcl([string] $Path) {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl = [Security.AccessControl.DirectorySecurity]::new()
    $acl.SetAccessRuleProtection($true, $false)
    $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
        $identity, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow'))
    Set-Acl -LiteralPath $Path -AclObject $acl
}

function Assert-TargetCommit([string] $Repository, [string] $Commit) {
    & git -C $Repository rev-parse --verify "$($Commit)^{commit}" *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Required test commit is unavailable: $Commit"
    }
}

function Set-KnowledgeRoot([string] $KnowledgeRoot, [string] $TemplatePath) {
    $template = Get-Content -LiteralPath $TemplatePath -Raw | ConvertFrom-Json -AsHashtable
    $server = $template.mcp_servers.knowledge
    $server.command = $node
    $server.args = @($mcpEntry, $KnowledgeRoot)
    $sourceConfig.mcp_servers = @{ knowledge = $server }
    [IO.File]::WriteAllText(
        $configPath,
        (ConvertTo-Json -InputObject $sourceConfig -Depth 100),
        [Text.UTF8Encoding]::new($false))
}

function Invoke-KnowledgeReview(
    [string] $Name,
    [string] $Repository,
    [string] $KnowledgeRoot,
    [string] $Commit,
    [string] $RuleName,
    [string] $ConfigName,
    [string] $Background,
    [int] $TokenBudget
) {
    $rulePath = Join-Path $runRoot "$RuleName.json"
    $backgroundPath = Join-Path $runRoot "$Name-background.md"
    $nativePath = Join-Path $resultDirectory "$Name-native.json"
    $materialPath = Join-Path $resultDirectory "$Name-report.json"
    $logPath = Join-Path $resultDirectory "$Name.log"
    Copy-Item -LiteralPath (Join-Path $repoRoot "project-docs/mock/$RuleName.json") -Destination $rulePath
    Set-Content -LiteralPath $backgroundPath -Encoding utf8 -Value $Background
    Set-KnowledgeRoot $KnowledgeRoot (Join-Path $repoRoot "project-docs/mock/$ConfigName.json")

    & $exe review --repo $Repository --commit $Commit --rule $rulePath --background-file $backgroundPath `
        --provider deepseek --model deepseek-flash --audience agent --format json `
        --report $materialPath --output $nativePath --timeout 10 --max-tokens-budget $TokenBudget *> $logPath
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        throw "$Name review failed with exit code $exitCode. See the protected result directory."
    }

    $native = Get-Content -LiteralPath $nativePath -Raw | ConvertFrom-Json -AsHashtable
    $material = Get-Content -LiteralPath $materialPath -Raw | ConvertFrom-Json -AsHashtable
    $knowledge = $material.sections.knowledge_sources.data
    $observations = @($knowledge.observations)
    $reads = @($observations | Where-Object { $_.tool -eq 'read_text_file' })
    $comments = @($native.comments)
    $visibleComments = (@($comments | ForEach-Object { $_.content }) -join "`n")
    if ($Name -eq 'backend') {
        $requiredSources = @('02-服务调用.md') # allow-non-english: identifies the required Chinese knowledge chapter
    }
    else {
        $requiredSources = @(
            'eplatei-knowledge.md',
            'eplatei-knowledge-reference/02-eiblock.md',
            'eplatei-knowledge-reference/05-constants-version.md')
    }
    $completeSources = @(
        foreach ($read in $reads | Where-Object { $_.status -eq 'complete' -and $_.source_status -eq 'identified' }) {
            foreach ($source in @($read.sources)) { $source }
        }
    )
    $missingSources = @($requiredSources | Where-Object { $_ -notin $completeSources })

    if ($native.status -ne 'complete' -or $knowledge.status -notin @('observed', 'partial') -or
        $knowledge.knowledge_version_status -notin @('observed', 'partial') -or $missingSources.Count -gt 0) {
        throw "$Name review did not produce complete evidence for every required knowledge source. See the protected result directory."
    }

    if ($Name -eq 'backend') {
        $matching = @($comments | Where-Object {
            $_.path -match 'EiInfoCallUtil\.java' -and $_.content -match 'XLocalManager' -and
            $_.content -match 'STATUS_FAILURE' -and $_.content -match 'negative status'
        })
        if ($matching.Count -eq 0 -or @($reads | Where-Object { $_.sources -contains '02-服务调用.md' }).Count -eq 0) { # allow-non-english: 知识来源使用现有中文文件名
            throw 'Backend review did not connect the service-call chapter read to its expected finding.'
        }
    }
    else {
        $matching = @($comments | Where-Object {
            $_.path -match 'AJXX11\.js' -and $_.content -match 'fetchEnterpriseBaseInfo' -and $_.content -match 'fetchLicenses'
        })
        if ($visibleComments -match 'getMappedRows|EiBlock' -or $matching.Count -eq 0 -or
            @($reads | Where-Object { $_.sources -contains 'eplatei-knowledge.md' }).Count -eq 0 -or
            @($reads | Where-Object { $_.sources -contains 'eplatei-knowledge-reference/02-eiblock.md' }).Count -eq 0) {
            throw 'Frontend review did not exclude the documented API false positive and find the expected changed-code issue.'
        }
    }

    [pscustomobject]@{
        Scenario = $Name
        Status = $native.status
        FindingCount = $comments.Count
        KnowledgeStatus = $knowledge.status
        KnowledgeVersion = $knowledge.knowledge_version_status
        ApplicationStatus = $knowledge.application_status
        CompleteTextReads = $reads.Count
        FailedTextReads = @($reads | Where-Object { $_.status -ne 'complete' }).Count
        MissingRequiredSources = $missingSources
        ExpectedFinding = $matching[0].content
        ReadEvidence = @($reads | ForEach-Object {
            [pscustomobject]@{
                Sources = ($_.sources -join ',')
                Status = $_.status
                Bytes = $_.response_bytes
                SHA256 = $_.sha256
            }
        })
        Log = $logPath
    }
}

foreach ($path in @($frontendRepo, $backendRepo, $sourceConfigPath,
        (Join-Path $frontendRepo '.ai_knowledge'), (Join-Path $backendRepo '.ai_knowledge'))) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Required local input is missing: $path"
    }
}
Assert-TargetCommit $frontendRepo 'fd4fdae1dda41b4a6ad7193218d2585500307349'
Assert-TargetCommit $backendRepo 'ab9d7dc11cc72f6413974992882aa25f8779d9a6'
if (-not (Test-Path -LiteralPath $GoExecutable)) {
    throw "Go executable is unavailable: $GoExecutable"
}

New-Item -ItemType Directory -Path $runRoot, $resultRoot | Out-Null
Set-CurrentUserOnlyAcl $runRoot
Set-CurrentUserOnlyAcl $resultRoot
New-Item -ItemType Directory -Path $isolatedHome, $configDirectory, $mcpRoot, $resultDirectory | Out-Null
Set-CurrentUserOnlyAcl $resultDirectory

try {
    $sourceConfig = Get-Content -LiteralPath $sourceConfigPath -Raw | ConvertFrom-Json -AsHashtable
    $env:npm_config_cache = Join-Path $runRoot 'npm-cache'
    & $GoExecutable build -C $repoRoot -o $exe ./cmd/opencodereview
    if ($LASTEXITCODE -ne 0) {
        throw 'Building OCR from the current source tree failed.'
    }
    & $npm install --prefix $mcpRoot --ignore-scripts --no-audit --no-fund '@modelcontextprotocol/server-filesystem@2026.8.31' `
        *> (Join-Path $runRoot 'npm.log')
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $mcpEntry)) {
        throw 'Installing the pinned filesystem MCP failed.'
    }

    $env:USERPROFILE = $isolatedHome
    $frontendKnowledge = Join-Path $frontendRepo '.ai_knowledge'
    $backendKnowledge = Join-Path $backendRepo '.ai_knowledge'
    $frontendBackground = '审查目标：fd4fdae1。仅按目标提交范围判断。核对 EiBlock.getMappedRows 是否受当前 @eplat/ei 版本支持，并排除知识已证明可用且代码已做空值保护的误报；同时检查变更中的真实并发状态问题。知识必须通过规则指定的独立知识目录读取。' # allow-non-english: 模型测试背景沿用工单中文验收要求
    $backendBackground = '审查目标：ab9d7dc1。仅按目标提交范围判断。重点核对 XLocalManager.call 包装中的成功状态判断，严格依据独立知识目录中的服务调用章节，不从常识推断状态语义；读取失败或正文不完整时如实说明。' # allow-non-english: 模型测试背景沿用工单中文验收要求
    Invoke-KnowledgeReview 'frontend' $frontendRepo $frontendKnowledge 'fd4fdae1' `
        'external-knowledge-01-rule.frontend' 'external-knowledge-01-config.frontend' $frontendBackground 150000
    Invoke-KnowledgeReview 'backend' $backendRepo $backendKnowledge 'ab9d7dc1' `
        'external-knowledge-01-rule.backend' 'external-knowledge-01-config.backend' $backendBackground 180000
}
finally {
    $env:USERPROFILE = $oldUserProfile
    if ($null -eq $oldNpmCache) {
        Remove-Item Env:npm_config_cache -ErrorAction SilentlyContinue
    }
    else {
        $env:npm_config_cache = $oldNpmCache
    }

    if (Test-Path -LiteralPath $resultDirectory) {
        Move-Item -LiteralPath $resultDirectory -Destination (Join-Path $resultRoot 'results')
    }
    $tempPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    $fullRunRoot = [IO.Path]::GetFullPath($runRoot)
    if (-not $fullRunRoot.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path -Leaf $fullRunRoot) -notmatch '^ocr-final-knowledge-[0-9a-f]{32}$') {
        throw "Refusing to clean an unexpected temporary path: $fullRunRoot"
    }
    if (Test-Path -LiteralPath $fullRunRoot) {
        Remove-Item -LiteralPath $fullRunRoot -Recurse -Force
    }
    Write-Output "Protected native/report JSON and logs: $(Join-Path $resultRoot 'results')"
    Write-Output "After inspection, remove only the generated result directory with: Remove-Item -LiteralPath '$(Join-Path $resultRoot 'results')' -Recurse -Force"
}
