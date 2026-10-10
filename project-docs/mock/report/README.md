# 报告功能人工验收

这里验收的是当前仓库源码编出的 CLI，不是全局安装的官方 NPM `ocr`。PATH 上的 `ocr` 可以保留，但后面步骤一律调用 `$cli` 指向的本地文件，也不运行仓库里的测试脚本。

## 前提

- Windows、PowerShell 7，以及已加入用户 PATH 的全局 Go、GNU Make 和 Git（含 `C:\Program Files\Git\usr\bin`，以便 `make` 找到 `sh.exe`）。命令在仓库根目录、新开的终端中执行；`go version`、`make --version` 和 `Get-Command sh` 都能打印再开始。
- 本机已经配置可用的模型服务。官方 `ocr` 写过的用户配置可以继续用（同一份 `.opencodereview/config.json`）。尚未配置时，用刚构建的本地文件跑 `config provider` 和 `config model`。
- 准备一个要审查的 Git 仓库，并确认当前账号有权读取它和使用模型服务。

## 1. 用仓库命令打包

`make build` 把当前源码编成 `dist/opencodereview.exe`。这是项目自己的开发构建，不是 npm 包。编完后把该文件记到 `$cli`，以后验收只调用它。

```powershell
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Force -Path .\dist | Out-Null
make build
if ($LASTEXITCODE -ne 0) { throw 'CLI 构建失败' }

$cli = (Resolve-Path .\dist\opencodereview.exe).Path
& $cli --version
& $cli report --help
```

`--version` 和 `report --help` 都能正常返回，才进入下一步。不要执行 `ocr`，那是官方 npm 包。

## 2. 配置模型服务（首次需要）

已经用官方 `ocr` 配好模型的，可以跳过本步。否则使用刚构建的本地文件配置：

```powershell
& $cli config provider
& $cli config model
```

这两个命令会把配置保存到用户目录的 `.opencodereview/config.json`。也可以使用非交互命令，例如：

```powershell
& $cli config set provider anthropic
& $cli config set model claude-opus-4-6
& $cli config set providers.anthropic.api_key $env:ANTHROPIC_API_KEY
```

不要把真实密钥写入仓库或提交到 Git。

## 2.1. 配置服务和知识库目录（首次需要）

`@modelcontextprotocol/server-filesystem` 是 MCP 官方项目提供的本地文件服务，让 CLI 通过工具读取指定目录中的知识文档；它本身不审查代码。当前使用全局 npm 安装，CLI 在审查时自动启动、调用并关闭服务，不需要手动启动或另开终端。需要已加入 PATH 的 Node.js 和 npm，安装时能访问 npm registry。

先安装服务，再检查安装文件是否存在：

```powershell
npm install -g '@modelcontextprotocol/server-filesystem@2026.8.31'
if ($LASTEXITCODE -ne 0) { throw '文件 MCP 安装失败' }

Test-Path 'C:\Users\60429\AppData\Roaming\npm\node_modules\@modelcontextprotocol\server-filesystem\dist\index.js'
```

预期返回 `True`。下面的路径按当前验收机器填写；如果返回 `False` 或在其他机器操作，用以下命令查询实际 npm 全局目录和 Node.js 路径，再替换配置命令中的对应值。服务脚本位于 npm 全局目录下的 `@modelcontextprotocol/server-filesystem/dist/index.js`。

```powershell
npm root -g
(Get-Command node -ErrorAction Stop).Source
```

可以直接使用第 1 步构建的 `$cli` 配置：

```powershell
& $cli config set mcp_servers.jk_web_knowledge.type stdio

& $cli config set mcp_servers.jk_web_knowledge.command 'C:/nvm4w/nodejs/node.exe'

& $cli config set mcp_servers.jk_web_knowledge.args '["C:/Users/60429/AppData/Roaming/npm/node_modules/@modelcontextprotocol/server-filesystem/dist/index.js","D:/WorkPlace/longruan_codeReview/jk_web/.ai_knowledge"]'

& $cli config set mcp_servers.jk_web_knowledge.tools '["read_text_file","list_directory","search_files","get_file_info","list_allowed_directories"]'
```

`jk_web_knowledge` 是这项服务的名称；`command` 指定 Node.js；`args` 的第一个路径是服务脚本，**第二个路径就是允许读取的知识库目录**，这里为 `jk_web/.ai_knowledge`。其他项目可以使用不同服务名称和对应知识目录。`tools` 必须保持上述非空读取白名单，空列表会开放全部工具；只读工具限制由 OCR 注册边界执行。

命令会将配置保存到用户目录的 `.opencodereview/config.json`，保留已有模型和其他服务配置；同名服务的这些字段会被更新。该文件由本地 CLI 和官方 OCR 共用，以后无需重复配置；安装位置或知识目录改变时重新设置对应字段。

成功执行上述四条命令后，配置文件中会包含以下 `mcp_servers` 内容。下方只展示 MCP 部分，实际文件还保留你的模型等配置：

```json
{
  "mcp_servers": {
    "jk_web_knowledge": {
      "type": "stdio",
      "command": "C:/nvm4w/nodejs/node.exe",
      "args": [
        "C:/Users/60429/AppData/Roaming/npm/node_modules/@modelcontextprotocol/server-filesystem/dist/index.js",
        "D:/WorkPlace/longruan_codeReview/jk_web/.ai_knowledge"
      ],
      "tools": [
        "read_text_file",
        "list_directory",
        "search_files",
        "get_file_info",
        "list_allowed_directories"
      ]
    }
  }
}
```

也可以选择手动配置，用下面的命令打开文件，将 `mcp_servers` 加入现有 JSON 最外层；已有 `mcp_servers` 时，在其中加入或更新 `jk_web_knowledge` 即可。保留模型及其他服务配置，相邻 JSON 配置项之间要有逗号。手动填写与上述命令效果相同，两种方式任选其一。

```powershell
notepad "$env:USERPROFILE\.opencodereview\config.json"
```

服务配置决定允许读取哪里，审查规则或背景决定何时读取、读什么。先使用下面的产品命令尝试真实读取；背景正文用于首次验证阅读链路，正式审查可改用已整理的 `--rule` 规则。

```powershell
& $cli review `
  --repo 'D:\WorkPlace\longruan_codeReview\jk_web' `
  --commit HEAD `
  --background '请先通过 jk_web_knowledge 文件 MCP 读取 D:/WorkPlace/longruan_codeReview/jk_web/.ai_knowledge/xr-framework-usage.md 索引，再根据本次变更读取相关章节。涉及 XR/EiInfo/EF* 契约时结合知识库核对；读取失败或依据不足时明确说明限制。' `
  --report
```

本次命令审查最新提交；要验证框架知识实际应用，应选择包含相关代码变更的提交或分支。命令结束后使用输出提示的 `*.report.json` 路径查看实际读取记录：

```powershell
$materialPath = '这里填终端显示的完整.report.json路径'
$material = Get-Content -Raw -Encoding utf8 $materialPath | ConvertFrom-Json
$material.sections.knowledge_sources | ConvertTo-Json -Depth 20
```

预期 `data.observations` 中出现实际读取的索引和相关章节，成功返回的记录有来源、摘要及字节数。没有读取时为 `not_collected`；失败或部分读取会附限制原因，不能仅凭退出码 0 判定接入成功。知识 `application_status` 当前为 `not_observed`，须结合章节内容和实际问题人工判断是否正确应用。确认读取后继续第 4 步，用这份报告材料生成 HTML。

## 3. 真实生成报告材料

把 `--repo` 换成你要审查的 Git 仓库。下面审查该仓库当前最新提交；这是真实模型调用，不是仓库里的测试。

```powershell
& $cli review --repo 'D:\WorkPlace\your-project' --commit HEAD --report
```

`--report` 不带路径时，材料会写到被审查仓库根目录，文件名类似 `your-project__commit-HEAD__20261010-153000.report.json`。命令结束时会给出这份文件的路径，下一步直接用它。

要自己指定材料位置：

```powershell
& $cli review --repo 'D:\WorkPlace\your-project' --commit HEAD --report 'D:\WorkPlace\your-project\review.report.json'
```

比较两个分支时改用 `--from` / `--to`，不要和 `--commit` 一起用：

```powershell
& $cli review --repo 'D:\WorkPlace\your-project' --from main --to feature --report
```

## 4. 直接生成并查看 HTML

把 `--input` 换成上一步得到的 `*.report.json`，`--output` 换成你想打开的 HTML 路径：

```powershell
& $cli report --input 'D:\WorkPlace\your-project\review.report.json' --output 'D:\WorkPlace\your-project\review.html'
Start-Process 'D:\WorkPlace\your-project\review.html'
```

`report` 只读取报告材料，不会重新审查代码。生成最多约 10 分钟；失败时用同一份 JSON 再跑一次即可。浏览器应能离线打开 HTML，并看到预设章节、发现的问题和基础工作统计；首版允许模型在排版和措辞上存在差异。

多份审查合成一份 HTML 时，重复 `--input`：

```powershell
& $cli report --input 'D:\WorkPlace\your-project\frontend.report.json' --input 'D:\WorkPlace\your-project\backend.report.json' --output 'D:\WorkPlace\your-project\combined.html'
```

验收时记录使用的 CLI 构建提交、模型服务、审查命令、输出文件和实际观察结果；开发回归与历史证据见 [`project-docs/retests/`](../../retests/README.md)。
