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

## 2.1. 指定知识库

本项目已实现 `review --knowledge-dir`，由原生工具按需读取指定目录中的磁盘资料，无需安装文件 MCP。`rules-and-context.md` 是[使用说明](../../open-code-review/rules-and-context.md)，不是知识库配置文件；真正授权访问的是命令中的 `--knowledge-dir`。只在规则或背景里写知识路径不会开放访问。

以下先用后端仓库；前后端知识库地址统一维护在[审查资源](../../review-resources.md)。命令继续使用第 1 步的 `$cli`：

```powershell
$repo = 'D:\WorkPlace\longruan_codeReview\jk'
$knowledgeDir = Join-Path $repo '.ai_knowledge'
Get-Item -LiteralPath $knowledgeDir
```

每次审查只能指定一个知识目录，建议使用绝对路径。目录内可以包含未跟踪或被 Git 忽略的资料，不要求资料存在于被审查的提交中；指定目录中的知识文件不作为代码审查目标。目录无效会在模型调用前报错。授权不保证模型一定读取或正确应用资料，验收时还需核对实际访问记录。

## 2.2. 指定审查规则

`--rule` 接受规则 JSON，不直接接受 Skill 目录或任意 Markdown。下面在临时目录生成一份前后端均可使用的最小规则；实际使用时将规则正文换成项目标准，或将 `$reviewRule` 指向已有规则 JSON：

```powershell
$reviewRule = Join-Path $env:TEMP 'ocr-acceptance-rule.json'
@'
{
  "rules": [
    {
      "path": "**/*",
      "rule": "仅审查本次变更引入或直接暴露的问题，结论使用中文。涉及项目框架或业务契约时，先查阅已授权知识库的索引或目录，再按需读取相关文档，并结合目标版本源码取证。资料缺失或依据不足时说明限制，不猜测内部契约；不默认遍历全部知识正文。",
      "merge_system_rule": true
    }
  ]
}
'@ | Set-Content -LiteralPath $reviewRule -Encoding utf8
```

`path` 决定哪些代码使用这条规则，不限定审查文件范围。`merge_system_rule: true` 保留内置审查标准并合并本条要求；不设置时，自定义规则默认替换内置规则。需要维护较长规则时，可将 `rule` 改为相对于 JSON 所在目录的 `.md` 文件路径；正文中的链接不会自动展开，也不会执行 Skill 脚本。详细约束见[规则加载](../../open-code-review/rules-and-context.md#规则加载)。

本次需求可另外用 `--background` 或 `--background-file` 提供；它们不替代审查规则或知识授权。前端可复用的背景及适用边界见[审查资源](../../review-resources.md)。

## 2.3. 调用模型前检查

先确认示例代码命中自定义规则，再预览当前提交的审查范围；以下两条命令不调用模型：

```powershell
& $cli rules check --repo $repo --rule $reviewRule 'jkdbms-sp/src/main/java/com/baosight/jkdbms/aj/ww/service/ServiceAJWW02.java'
& $cli review --repo $repo --commit HEAD --knowledge-dir $knowledgeDir --rule $reviewRule --preview
```

规则检查应显示来源为 `Custom (--rule)`，并包含自定义正文与内置标准。预览范围合理后再运行下一步；首次验收优先选单提交或少量文件的分支范围，避免直接对数百个文件发起完整审查。

## 3. 真实生成报告材料

下面沿用第 2 步准备的仓库、知识目录和规则，审查当前最新提交；这是真实模型调用，不是仓库里的测试。

```powershell
& $cli review --repo $repo --commit HEAD --knowledge-dir $knowledgeDir --rule $reviewRule --report
```

`--report` 不带路径时，材料会写到被审查仓库根目录，文件名类似 `your-project__commit-HEAD__20261010-153000.report.json`。命令结束时会给出这份文件的路径，下一步直接用它。

要自己指定材料位置：

```powershell
& $cli review --repo $repo --commit HEAD --knowledge-dir $knowledgeDir --rule $reviewRule --report (Join-Path $repo 'review.report.json')
```

也可以把 `--report` 指向已有目录，程序会在目录内自动命名。新目录请以路径分隔符结尾。文件名的非 JSON 后缀会替换为 `.json`，重名自动编号，不覆盖已有文件。程序在模型调用前检查保存位置；指定位置不可用时告警并回退到仓库根目录，成功后打印实际文件路径。

比较两个分支时改用 `--from` / `--to`，不要和 `--commit` 一起用。以下是资源索引中的前端小样本，先预览，再按需执行真实审查；切换仓库时同步切换知识目录：

```powershell
$repo = 'D:\WorkPlace\longruan_codeReview\jk_web'
$knowledgeDir = Join-Path $repo '.ai_knowledge'
& $cli review --repo $repo --from dev-1010 --to dev-1016 --knowledge-dir $knowledgeDir --rule $reviewRule --preview
& $cli review --repo $repo --from dev-1010 --to dev-1016 --knowledge-dir $knowledgeDir --rule $reviewRule --report
```

保存 JSON 成功不等于审查完整完成。检查 `review.status`、`coverage.completed/failed` 和 `limitations`：`partial` 表示仅部分完成，`task_done` 只表示某组某轮结束。知识访问事实在 `sections.knowledge_sources` 中，需区分查找、搜索和实际读取；未采集或未读取不能当作知识核对通过。完整验收应确认所有选中目标均有完成覆盖，并核对需要的知识是否实际读取。

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
