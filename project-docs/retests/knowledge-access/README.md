# 外部知识访问回归

验证文件 MCP 的授权根、越界拒绝、缺失与部分读取，以及真实模型能否读取指定知识章节并据此审查。探针脚本只连接本地 MCP；完整模型脚本会产生真实模型用量。

## MCP 边界探针

需要 PowerShell 7、Node.js/npm、可访问 npm registry。探针只在系统临时目录创建测试文件：

```powershell
$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) ('ocr-knowledge-probe-' + [guid]::NewGuid().ToString('N'))
$mcp = Join-Path $root 'mcp'
$entry = Join-Path $mcp 'node_modules/@modelcontextprotocol/server-filesystem/dist/index.js'
try {
    New-Item -ItemType Directory -Path $root | Out-Null
    npm install --prefix $mcp --ignore-scripts --no-audit --no-fund '@modelcontextprotocol/server-filesystem@2026.8.31'
    if ($LASTEXITCODE -ne 0 -or !(Test-Path -LiteralPath $entry)) { throw 'filesystem MCP 安装失败' }
    $outside = Join-Path $root 'outside.md'
    Set-Content -LiteralPath $outside -Value 'outside probe' -NoNewline
    & ./project-docs/retests/knowledge-access/probe-boundary.ps1 `
      -McpEntry $entry `
      -TestRoot (Join-Path $root 'authorized') `
      -OutsideFile $outside
    if ($LASTEXITCODE -ne 0) { throw 'MCP 路径边界探针失败' }
    & ./project-docs/retests/knowledge-access/probe-content.ps1 `
      -McpEntry $entry `
      -TestRoot (Join-Path $root 'content')
    if ($LASTEXITCODE -ne 0) { throw 'MCP 内容探针失败' }
} finally {
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
```

Linux 真符号链接检查另需 Podman machine、容器镜像和 npm 网络，入口为 `probe-linux-symlink.ps1`。Windows 普通文件符号链接曾因会话权限不足而未验证；不要把 Junction 结果当作该项通过。

## 真实知识应用

需要 Go、Node.js/npm、可访问 npm registry、已授权的 DeepSeek 配置，以及[资源索引](../../review-resources.md)所列前后端仓库、知识目录和固定提交。它会只读样例仓库、调用真实模型，并在 ACL 保护的临时目录中保存结果：

```powershell
& ./project-docs/retests/knowledge-access/run-live-review.ps1 `
  -GoExecutable 'D:\WorkPlace\toolchains\go1.25.14\go\bin\go.exe'
```

预期两个审查完成，知识来源及版本状态为 `observed`，MCP 调用失败数为 0；是否实际应用知识还须对照 findings 与成功读取记录。复测配置模板和规则分别位于 `configs/`、`rules/`。本次运行证据与未验证限制见[验收档案索引](../archive/README.md)。
