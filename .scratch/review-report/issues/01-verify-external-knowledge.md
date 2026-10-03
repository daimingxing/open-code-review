# 01：验收外部知识的选读与实际应用

Status: ready-for-agent

Blocked by: 无，可立即开始

阶段：1。规格来源：[Spec](../spec.md)；执行约定及依赖总览：[工单索引](../README.md)。关联用户故事：3、4、5、6、7、8、9、10、11、12、13、14、42。

## 交付行为

通过原生规则与现成文件 MCP 完成真实模型审查，证明知识既可按需读取，也能用于检出问题和排除误报。

真实项目验收使用[长期资源索引](../../../project-docs/review-resources.md)，以前后端对应的审查 Skill 和知识库准备规则与样例；复测命令、前提和实际结果按[复测记录约定](../../../project-docs/mock/README.md)长期保存。

## 验收条件

- [x] 提供可复现的隔离配置与前后端样例；规则声明独立知识索引，业务背景沿用原生入口，不新增规则协议或知识命令参数。
- [x] 真实模型样例分别覆盖必须依靠框架知识才能判断的问题、应排除的误报；核验索引与章节读取记录、版本、代码证据和结论，记录模型与服务版本，不用替身结果代替效果验收。
- [x] 使用现成文件 MCP 的授权根目录及非空只读工具白名单；验证目录外、父目录与符号链接越界拒绝，模型不获得写工具；历史代码仍按目标提交读取。（Linux 真实目录符号链接及 Windows Junction 分别实测拒绝；Windows 本机普通文件符号链接因权限限制未验证。）
- [x] 文档缺失、服务不可用、只返回部分正文时不伪造已读或全部完成，保留有证据的结果并说明限制；验证小型 Markdown 直接注入的后备方式。
- [x] 将配置方式、预期结果和复测方法保存为可复用验收资料。只有基础接入确有缺口时才评估最小改动，不把本工单扩大为开发新的 MCP 服务。

## 执行记录（2026-10-04）

- 执行者：Codex 实施智能体。
- 工作树：`D:\WorkPlace\open-code-review-worktrees\review-report-01`；分支：`codex/review-report-01-knowledge`；基线：`d50f4dc2502edc510e2673b495ad4ac99871eedb`（`feature-review-report`）；实施提交：`877b0ae3f49bc53d8bebac6ecc0e14920811b3c9`、`d4bf93ba052dc851fd72e4380d4033cd0446d2c7`、`fcc92b631fe47fb5d5e549c83205ded0da7b0838`；复测可重现性审查修复提交：`c8ef8d04b70d7b2969666f98a48d702f1a8fbd7b`；功能集成提交：`7a931112c8254c5417a5545ef8156c7bfe518a31`。
- 环境：Windows 11、PowerShell 7、OCR v1.12.11（a758d9c）、Node.js v22.22.2、filesystem MCP 2026.8.31、DeepSeek `deepseek-flash`。真实仓库及 `.ai_knowledge` 只读使用，未改其文件、忽略配置或未提交内容。复测流程只从当前用户授权的 OCR 配置读取模型设置，在当前用户 ACL 保护的系统临时目录短时写入隔离副本；凭据不回显、不提交，完成后清理。未修改源配置。详见[复测命令与实际结果](../../../project-docs/mock/external-knowledge-01.md)、前后端 MCP 模板和规则样例。

### 真实模型证据

- 前端正/负样例命令：`ocr review --repo D:\WorkPlace\longruan_codeReview\jk_web --commit fd4fdae1 --rule $frontendRule --background-file $frontendBackground --provider deepseek --model deepseek-flash --audience agent --format json --output $testRoot\frontend.json --timeout 10 --max-tokens-budget 75000`。退出码 0，报告 `complete`；实际读索引 `eplatei-knowledge.md` 和章节 `02-eiblock.md`，确认 `@eplat/ei 2.2.1` 支持 `getMappedRows` 且代码有空值保护，排除该误报；另检出 `fetchEnterpriseBaseInfo` 与 `fetchLicenses` 并发覆盖 `businessLicenseObj.rid` 的问题。工具调用 9 次、失败 0 次；87,533 tokens 超过预算，第二轮被跳过，结论限于已完成轮次。
- 后端首次正例命令同样使用 `$backendRule` 和 `$backendBackground`，目标提交 `ab9d7dc1`，预算 120,000。退出码 0、16 次工具调用，但未出现 MCP 调用，只得到风格意见。检查确认隔离 MCP 指向正确，问题是模型没有按原规则主动走知识读取；本次保留这次失败证据，不算验收通过。
- 强化规则明确要求“列目录、读索引、读章节成功后方可分析”。可复跑的后端定向重试使用完整准备流程中定义的 `$backendRule` 和 `$backendBackground`，以 180,000 token 预算单独保存到 `backend-retry.json`；具体命令见复测资料。退出码 0，manifest 为 `complete`；21 次工具调用、失败 0 次，其中知识 MCP `list_allowed_directories` 2 次、`list_directory` 2 次、`read_text_file` 4 次。模型引用 `02-服务调用.md` 的“01 本地服务调用(XLocalManager)”规则 `outInfo.getStatus() < 0`，对照目标提交的 `outInfo.getStatus() == EiConstant.STATUS_FAILURE` 检出其他负状态可能被当作成功处理的高优先级问题；还报告两条低优先级意见。目标代码范围为 `35b22d0ad6de8e2dbfdae9f0e9d6e5188b6fe70e..ab9d7dc11cc72f6413974992882aa25f8779d9a6`。
- 两个模型场景均使用原生规则文件的知识索引路径和原生 `--background-file`；使用 `--commit`，manifest 显示准确的目标提交范围，没有从工作树样例文件推断目标差异。

### 边界、未运行项与限制

- 文件 MCP 边界脚本在临时目录实际运行。`list_allowed_directories` 仅返回测试根；父目录和根外普通文件读取均返回 `Access denied`。MCP 服务端自身列举了写工具，但隔离 OCR 配置白名单仅五项只读工具，两个真实会话未调用 MCP 写工具。真实模型仅获得了知识目录工具，没有授权工作区目录或写操作。
- 边界复测：父目录与根外普通文件读取均被拒绝，`list_allowed_directories` 仅返回隔离测试根；授权目录下只读 MCP 白名单非空，真实模型会话未调用写工具。Linux Alpine 3.20 容器中创建真实目录符号链接，MCP 拒绝读取并返回 `Access denied - symlink target outside allowed directories: /tmp/outside/secret.md not in /tmp/allowed`；Windows Junction 也拒绝越界读取。Windows 本机普通文件符号链接因当前权限不足未验证，两种平台测试分别记录于复测说明。
- 失败语义与后备：真实 filesystem MCP 对不存在文档返回 `ENOENT`，对 `head: 1` 返回部分正文；DeepSeek 真实模型明确说明缺失/部分知识且没有声称已完整阅读。将隔离 MCP 命令改为不存在的可执行文件后，OCR 原始警告记录服务启动失败并继续审查，模型说明索引/章节未读，输出仍为 `complete` 但只代表代码覆盖；另用原生 `ocr rules check` 验证短 Markdown 正文直接进入自定义规则。
- 已运行：前后端真实模型审查（后端第一次未读取、定向重试成功读取）、MCP 父目录/根外/Junction 检查、缺失/部分正文工具探针、缺失/部分正文与服务不可用真实模型检查、Markdown 直接注入、目标提交 manifest 核验、JSON 解析、`git diff --check` 和 OCR 预览。完整 OCR 自审因预算预检估算超限未派发，记为未验证而非失败或通过。Go 源码测试不属于本工单改动范围；主线程基线 `go test ./...` 的 5,057 项通过，`make test` race 路径因缺少 gcc/CGO 未运行成功；Go 下载阻塞的旧记录已更正。
- 实际第二次后端读取证明，明确的先读知识规则能让当前服务/模型调用读取知识；首次未读取则说明仅说“按需读取”不足以保证行为。后续接入应保留明确读取要求，并把失败/部分读取状态如实呈现。未发现需要在本工单新增 MCP 服务或产品代码的基础接入缺口。
- 初次执行后的清理：真实样例仓库未被写入，现有未提交文件保持原状。执行策略拒绝了递归删除整棵临时隔离目录的尝试；随后按系统临时目录路径校验，精确删除了含继承凭据的隔离 `config.json`，确认文件不存在。依照交接要求，其他临时目录内容保留；提交中不包含临时日志或密钥。后续可复测流程已改为删除含临时凭据的隔离根，仅把结果 JSON 移至另一个当前用户 ACL 受限的临时目录，并提供该目录的精确清理命令。
- 独立复核曾发现复测文档缺少变量、隔离配置准备步骤和重试参数定义（P2）；实施者在 `c8ef8d0` 补齐完整 PS7 准备流程、受保护配置生命周期及结果清理命令。独立复核确认该问题已关闭；未重复真实模型调用。主线程对复测文档的四个 PowerShell 代码块再次运行 `Parser.ParseInput`，全部通过。复测结果 JSON 保留在当前用户 ACL 限制的独立临时目录，文档给出精确清理命令；该目录由执行者在检查后删除。

