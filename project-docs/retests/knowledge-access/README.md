# 外部知识访问回归

## 原生知识库常用样本

真实模型功能复测使用[资源索引](../../review-resources.md#原生知识库常用验收样本)中的前端仓库、知识库和 `dev-1010...dev-1016`，不默认重复高成本的 `dev-1023...dev-1030`。可复用背景为 [xr-review-background.md](xr-review-background.md)，来源和提取边界见资源索引。自行测试显式使用 `--effort low`，只验证功能链路，不以审查效果或知识应用质量作为通过条件，不为改善效果提高 effort 或反复付费运行。

`review --knowledge-dir` 已实现，契约见[Spec](../../../.scratch/native-knowledge/spec.md)。`--background-file` 本身不会开放知识目录；需要显式传入知识参数。

在本项目根目录执行，使用构建后的本地 CLI 和已配置的模型：

```powershell
$cli = Join-Path (Get-Location) 'dist/opencodereview.exe'
$background = Join-Path (Get-Location) 'project-docs/retests/knowledge-access/xr-review-background.md'
& $cli review `
  --repo 'D:\WorkPlace\longruan_codeReview\jk_web' `
  --from dev-1010 --to dev-1016 `
  --knowledge-dir 'D:\WorkPlace\longruan_codeReview\jk_web\.ai_knowledge' `
  --background-file $background `
  --effort low --concurrency 1 --max-tokens-budget 200000 --no-filter `
  --report
```

这是付费模型功能测试，需主动执行。预算是累计输入加输出的 token 数（含缓存输入），并非货币额度或绝对硬上限；在途和收尾请求可能超出，触及预算允许只完成部分审查。记录实际覆盖与限制，不把部分结果当作完整验收。新实现应能通过原生工具按索引查阅真实知识，`--report` 生成报告 JSON；不在本命令内生成 HTML。

## 当前状态与回归

2026-11-03：已移除旧文件 MCP 专用的知识来源采集及活动探针、配置和真实模型脚本；不再要求安装 @modelcontextprotocol/server-filesystem。通用 MCP 与参数反馈继续保留，可调用其他外部服务。

原生知识读取、查找、搜索、恢复校验及报告来源记录已经接通；未配置或未访问知识时明确保留未采集说明，查找、搜索与实际读取分别记录。已有报告 JSON 仍可用于 HTML 生成。旧文件 MCP 的读取证据只作为[历史档案](../archive/README.md)，不作为当前安装或验收步骤。

## 2026-11-03 开发验证与完整命令

开发基线为文档提交 `6bd0f66`，实现位于 `feature/native-knowledge`。以下验证使用本机 Go 1.27.2、PowerShell 7；真实模型为已配置的 `deepseek/deepseek-flash`。全部模型审查命令显式使用 low effort；未修改用户模型凭据、知识库或业务仓库文件。

### 自动化与构建

以下是本轮实际使用的测试命令；定向命令在修复后有重跑。测试使用隔离配置、本地 HTTP 可控模型和临时 Git 仓库，不调用付费审查模型。仓库根目录执行：

```powershell
$env:PATH = 'C:\Program Files\Git\usr\bin;' + $env:PATH
$env:LC_ALL = 'C'
make test PACKAGES='./internal/knowledge ./internal/tool ./internal/session ./internal/agent ./cmd/opencodereview'
```

该命令因 `CGO_ENABLED=0` 且未安装 C 编译器，无法启用 `-race`。继续用以下普通测试覆盖功能，不把它写成 race 已通过：

```powershell
go test ./internal/knowledge ./internal/tool ./internal/session ./internal/agent ./cmd/opencodereview -run 'Test(ReadLines|FileFind|GitGrep|Knowledge|ReviewE2E_ReportBarePath)' -count=1
go test ./cmd/opencodereview ./internal/tool -run '^TestKnowledge' -count=1 -timeout 180s
go test ./internal/agent -run '^TestKnowledge' -count=1 -timeout 90s
go test ./internal/knowledge ./internal/tool -run 'TestDirectoryValidationAndResume|TestKnowledgeReadBoundsFailuresAndEscape' -v -count=1
go test ./cmd/opencodereview -run '^TestKnowledge' -count=1 -timeout 180s
go test ./internal/knowledge ./internal/tool ./internal/session ./internal/agent ./internal/report ./cmd/opencodereview -count=1 -timeout 10m
go test ./... -count=1 -timeout 10m
go test ./cmd/opencodereview -run '^TestKnowledgeMaterialSurvivesBudgetStop$|^TestReviewE2E_CommitReportContainsEvidenceBackedEnrichment$' -v -count=1 -timeout 120s
go test ./cmd/opencodereview -run '^TestKnowledgeMaterialSurvivesBudgetStop$' -v -count=1 -timeout 90s
go test ./internal/tool ./cmd/opencodereview -run '^TestKnowledge|^TestFileFind|^TestReviewE2E_CommitReportContainsEvidenceBackedEnrichment$' -count=1 -timeout 180s
go test ./internal/knowledge ./internal/tool ./internal/agent ./cmd/opencodereview -run '^TestKnowledge|^TestDirectoryValidationAndResume|^TestFileFind' -count=1 -timeout 180s
go test ./internal/knowledge ./internal/tool ./internal/agent ./cmd/opencodereview -run '^TestKnowledge|^TestDirectoryValidationAndResume|^TestFileFind|^TestReviewE2E_CommitReportContainsEvidenceBackedEnrichment$' -count=1 -timeout 180s
go test ./internal/session ./cmd/opencodereview -run '^TestKnowledge|^TestFinalize|^TestSession|^TestReviewE2E_CommitReportContainsEvidenceBackedEnrichment$' -count=1 -timeout 180s
make license-add
make check
make english-check
go mod tidy
go vet ./...
make build
git diff --check
```

全仓库普通回归共 27 个包通过，其中 CLI 包约 307 秒。之后修正普通源码查找的知识目录遍历及来源细节，相关工具、知识模块、共享压缩和 CLI 定向回归再次通过。覆盖如下行为：

- 工作区、分支、单提交分别配合仓库内外知识目录，共六组真实 CLI 场景；查找、搜索忽略文件、按行读取均断言实际工具结果。
- 知识来自磁盘，源码来自对应版本；仓库内知识变更不计入审查覆盖。恢复保留访问依据，配置或已读内容变化时拒绝无提示复用。
- 中文及空格路径、相对目录、空目录、无效输入、500 行截断及继续读取；明确请求 500 行与被上限截断分别记录。
- 未授权读取、查找、搜索及实际链接逃逸；本机使用 Windows 目录联接验证，不以字符串前缀测试代替。
- 实际触发共享压缩后继续读取，初始知识区块和通用用途仍保留。
- 报告材料保留部分读取、真实失败、资料变化与恢复依据；预算停止后保留已读取来源；旧报告和 HTML 草稿编辑交付继续回归。

许可证检查通过，新增源码没有引入额外英文检查问题；`make check` 仍被历史人工样例的 31 行非英文内容阻断。格式化由本轮实际修改的 Go 文件执行 `gofmt -w` 补足，`go mod tidy`、`go vet ./...`、构建和差异检查分别通过；依赖文件没有变化。没有扩大整目录语言豁免或修改历史样例来绕过检查。

### 真实 XR 命令与结果

先只读确认比较范围：

```powershell
git -C 'D:/WorkPlace/longruan_codeReview/jk_web' rev-parse dev-1010 dev-1016
git -C 'D:/WorkPlace/longruan_codeReview/jk_web' merge-base dev-1010 dev-1016
git -C 'D:/WorkPlace/longruan_codeReview/jk_web' diff --shortstat dev-1010...dev-1016
```

确认的完整提交见[资源索引](../../review-resources.md#原生知识库常用验收样本)，三点差异仍为 15 文件、444 行新增、148 行删除。下面是本次成功发起的原样功能命令；历史记录不加上后来建议的 `--no-filter`。复跑请使用新的临时目录名，保留已有结果。

```powershell
$nativeTestDir = Join-Path $env:TEMP 'ocr-native-knowledge-20261103'
New-Item -ItemType Directory -Path $nativeTestDir -Force | Out-Null
$nativeMaterial = Join-Path $nativeTestDir 'xr.report.json'
$nativeResult = Join-Path $nativeTestDir 'xr.native.json'
$nativeLog = Join-Path $nativeTestDir 'xr-review.log'
$nativeBackground = (Resolve-Path './project-docs/retests/knowledge-access/xr-review-background.md').Path
& ./dist/opencodereview.exe review `
  --repo 'D:/WorkPlace/longruan_codeReview/jk_web' `
  --from dev-1010 --to dev-1016 `
  --knowledge-dir 'D:/WorkPlace/longruan_codeReview/jk_web/.ai_knowledge' `
  --background-file $nativeBackground `
  --effort low --concurrency 1 --max-tokens-budget 200000 `
  --format json --output $nativeResult --report=$nativeMaterial *> $nativeLog
$LASTEXITCODE
```

业务样本只进行一次实际付费审查运行，退出码 0，状态为 `partial`；OCR 选择 13 个目标，覆盖清单实际完成 5 个、失败或未完成 8 个，不将原生摘要中的 `files_reviewed=13` 解释为全部完成。已保存原生结果和报告 JSON，未再调用模型生成 HTML。另有下方单独披露的预提交分组请求。

| 观察项 | 实测结果 |
|---|---|
| 审查耗时 | 485,329 ms，约 8 分 5 秒 |
| 累计输入 / 输出 | 252,067 / 19,412 tokens；合计 271,479，输入含缓存读取 186,624 |
| 模型请求 | 17：分组 1、计划 1、主循环 13、评论过滤 2；请求失败 0 |
| 工具调用 | 23：读取 7、搜索 9、查找 5、评论 2；工具失败及返回 Error 的记录均为 0 |
| 同组完全相同调用 | 0 次重复 |
| 实际知识使用 | 两次显式知识内容搜索均无匹配；模型没有读取知识索引或正文，材料记录 `reading_status=not_read`、`application_status=not_verified` |
| 完成边界 | 达到累计预算后进入原有收尾流程并跳过后续范围；不是完整审查通过 |

知识查找、搜索、读取和局部读取的功能由可控模型 CLI 回归验证；本次真实模型未遵循索引阅读指引，是必须保留的观察，不能写成知识已正确应用。按照用户决定，本阶段不为了改善模型行为提高 effort 或重复付费审查。

该真实运行使用细节修正前的开发构建，构建版本标识为 `v0.0.0-6bd0f66`，不是最终提交已逐字运行真实模型的证明。实测后修正了普通源码查找的额外知识遍历，并补充搜索来源和截断证据细节；最终代码通过相关可控回归和重新构建，未为这些确定性修正再次付费审查。

### 报错、额外开销与修复归属

| 问题 | 归属与处理 |
|---|---|
| 真实命令初次使用仓库外 background 相对路径，解析到业务仓库后不存在 | 命令准备错误，发生在模型请求前，退出码 1；改为绝对路径后只发起一次真实运行，没有为此消耗模型请求 |
| 普通源码查找也枚举知识目录，实测发生 5 次，查找调用合计约 15 秒 | 本次开发引入的额外开销；已限定只有显式知识 `directory` 才遍历，普通查找仅保留源码范围；目录路由使用纯路径判断，避免逐个源码路径触发磁盘祖先查验；补充防回归测试并通过 |
| 状态类型比较、Windows 路径 JSON 转义、重复 `--report` 参数、预算夹具小于分组预估、按紧凑 JSON 断言格式化材料 | 开发测试代码及夹具问题，均在本地可控模型回归中修正；不属于真实模型运行的大量工具错误 |
| 缺省知识来源说明改成英文，破坏原有中文报告断言 | 本次开发引入的兼容问题；改为复用原有缺省说明，原测试及后续全仓库普通回归通过 |
| 预算 200,000，实际累计 271,479 tokens；低 effort 仍用了约 8 分钟 | 原有预算检查、在途/收尾及过滤语义和模型源码探索造成；预算不是绝对硬上限。本次不改分组、预算或循环机制，后续纯功能实测建议省略评论过滤并进一步缩小范围 |
| `make test` 无法启用 race；`make check` 命中历史 31 行样例 | 本机环境与已有检查阻碍，分别保留普通回归结果及静态检查结果，不冒称全部标准命令通过 |

核对依据为本机临时目录下的原生 JSON、报告 JSON、会话 `f693c42e-0c76-464f-be1a-dcfc56c4a7e1` 及测试日志。真实材料、会话正文、模型凭据和临时日志不入库。

### 预提交命令

按项目约定调用 CLI 审查，限制预算，不将分组成功或零评论当作代码审查通过：

```powershell
& ./dist/opencodereview.exe review --audience agent --effort low --concurrency 1 --max-tokens-budget 1 --background 'Native knowledge implementation: explicit read-only directory, native tool routing, code Git versions, task knowledge block, resume evidence, and report material compatibility. Check the current implementation diff.'
```

该调用耗时 14 秒，分组请求累计消耗 1,371 tokens（输入 612、输出 759），随后预算阻止主审查，退出码 1，工具调用为 0；不是完整代码审查。业务样本与该预提交调用合计记录 272,850 tokens，不等同于货币费用。本轮还在文档提交前执行过同样 low effort、预算为 1 的文档审查命令，因没有选中代码而跳过，消耗为 0。确定性验证、人工阅读代码及发现问题后的修复证据如上，不以预算截断的模型审查替代。
