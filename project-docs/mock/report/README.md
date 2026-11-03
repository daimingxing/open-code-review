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

## 2.1. 知识接入迁移状态

旧文件知识 MCP 专用采集已清理，无需安装 @modelcontextprotocol/server-filesystem。新的 --knowledge-dir 尚未实现；当前可继续验收审查与报告，知识来源标记为未采集，不代表已完成知识核对。计划中的知识验收及可复用背景见[知识回归入口](../../retests/knowledge-access/README.md)。

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

`report` 通过与 review 相同的智能体循环生成 HTML：模型按需读取报告 JSON、模板和样式，用 `write_html` 创建完整草稿，再按需用 `read_html` 查看、`edit_html` 局部修订；上下文压缩后仍可继续编辑。草稿更新不等于最终发布，只有明确结束任务后才保存报告。样式文件只是参考，应用不会再插入样式或控件。提示词建议优先依据这些材料、避免重复审查源码，但这不是源码访问拦截。首轮不解析或校验 HTML 内容、脚本或样式；循环未完成时不会发布报告。失败时用同一份 JSON 再跑即可。浏览器应能离线打开 HTML；排版和措辞允许差异。

运行信息中的 `review_elapsed` 是输入材料已有的审查耗时之和；`html_generation elapsed` 才是当前智能体生成阶段的经过时间。输入/输出 token 是各轮请求累计值，不是当前上下文大小；缓存读取、写入用量只有服务提供时才显示，未提供和部分提供分别标记，不据此推算费用。局部编辑能避免不必要的全文输出，但不保证模型每次都选择它。

多份审查合成一份 HTML 时，重复 `--input`：

```powershell
& $cli report --input 'D:\WorkPlace\your-project\frontend.report.json' --input 'D:\WorkPlace\your-project\backend.report.json' --output 'D:\WorkPlace\your-project\combined.html'
```

验收时记录使用的 CLI 构建提交、模型服务、审查命令、输出文件和实际观察结果；开发回归与历史证据见 [`project-docs/retests/`](../../retests/README.md)。
