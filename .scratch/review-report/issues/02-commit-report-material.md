# 02：单提交审查生成独立报告材料

Status: ready-for-agent

Blocked by: [01：验收外部知识的选读与实际应用](01-verify-external-knowledge.md)

阶段：2。规格来源：[Spec](../spec.md)；执行约定及依赖总览：[工单索引](../README.md)。关联用户故事：1、2、15、16、17、18、19、20、21、22、31、39、42。

## 交付行为

用户对一次提交运行审查，即可用裸参数或指定路径保存版本化报告 JSON，并继续独立使用原生输出。

## 验收条件

- [x] 从 CLI 验证单提交模式的裸 --report、空格分隔路径、带空格路径、参数顺序和相邻选项；多余位置参数明确报错，不吞掉后续选项，不私自改为必须使用等号。
- [x] 材料包含运行身份、仓库、实际提交与比较起点、带时区时间、最终问题及稳定标识、四级风险、代码位置、证据、建议、覆盖和限制；键与技术标识保持原生语义，原始说明保留，问题具备必要中文展示文本。
- [x] 落地可校验的材料版本与必需内容约束，不按 HTML DOM 建模；后续成果材料未具备时如实标记缺失，不能伪造空成果或宣称已具备完整报告内容。
- [x] 按已确认的仓库、单提交范围与启动时间自动命名，在仓库根目录保存；清理非法字符和过长标签，重名及并发使用排他编号；显式已存在路径、缺失父目录、与原生输出路径冲突时拒绝破坏性保存。
- [x] 不传 --report 时原生行为不变；原生 JSON 和报告 JSON 可分别保存，无须同时启用原生 JSON。材料生成或保存失败不破坏原生结果，返回非零并说明失败阶段。
- [x] 使用临时仓库和可控模型从 CLI 验证零问题、有问题、参数和保存路径；并发创建仅在必要的文件输出边界补最小测试。同步用户可见的参数帮助。
- [x] 在首次材料契约中说明三种审查模式共享的事实、来源和缺失状态，以及后续扩展边界，供 03、04、05 独立实施；不为尚未交付的模式伪造数据或建立无用途的抽象。

## 执行记录（2026-10-04）

- 执行者：Codex 实施智能体。工作树：`D:\WorkPlace\open-code-review-worktrees\review-report-02`；分支：`codex/review-report-02-material`；初始基线：`8c9188a`；开发前合入 `feature-review-report` 的 `4f37ebf` 与最新 `e229220`，保留 merge 历史。初始实现提交：`c9f1b70a39c7d0312d7822f16e7a9d2eb9fc9f26`；Spec 兼容修复提交：`4e707324b7fbdca14d2b63089ed36ce0d5e08a01`；最终证据文档提交随后补入。
- 交付契约：`internal/report/material.go` 定义 `Material` v1 与共享 `Scope`、`Finding`、`Coverage`、六类固定 `Section` 和 `Limitation{source,status,reason}`。原生 manifest 与 findings 是身份、范围、问题和覆盖的事实来源；缺失字段和每个报告分区分别明确区分 `not_collected`、`not_applicable`、`failed` 并带原因。03–05 只需基于各自真实证据填充相应 `Section.Data`/状态和补充限制，不改变 `Finding.SourceContent`、原生身份或解析后的范围事实。
- 缺失项：单提交原生数据不含 Git 统计、工作区快照、成果、人员、实际知识读取来源和结构检查时，报告材料标记为 `not_collected` 或按模式标记 `not_applicable`，没有伪造数据；覆盖失败、跳过、工具失败、仓库/输入摘要缺失、代码证据与建议缺失均进入限制列表。运行或模型服务的真实验收不由本工单替代。
- 文件系统边界：常见支持硬链接的文件系统使用同目录临时文件硬链接原子发布；硬链接不支持时回退到 `O_EXCL` 独占创建和复制，仍拒绝覆盖，但非原子，写入失败可能留下部分文件。实现不会按路径删除失败目标，以免竞态删除并发替换文件；复测记录说明后续检查和处理方式。
- 已运行：聚焦报告测试 `go test ./internal/report ./cmd/opencodereview -run 'Test(ValidateMaterial|WriteMaterial|CopyMaterialExclusively|PathsConflict|ParseReviewFlagsOptionalReportPath|DefaultReportPath|SanitizeReportLabel|ReviewE2E_Report|BuildReportMaterial|MaterialFindings)' -count=1` 通过；OCR finding 回归测试 `go test ./cmd/opencodereview -run 'Test(ParseReviewFlagsOptionalReportPath|BuildReportMaterialRejectsMissingManifest)' -count=1` 通过；全量普通测试 `go test ./... -count=1` 通过；合并 `e229220` 后 `make check` 通过。`make test` 未验证：默认 `CGO_ENABLED=0` 不支持 race，开启 CGO 后环境没有 GCC。命令、前提和结果见[历史复测记录](../../../project-docs/retests/archive/review-report-2026-10/report-material-commit.md)及[当前复测入口](../../../project-docs/retests/report-material/README.md)。
- 提交前 OCR 自审：初次 review 发现原生远程仓库身份摘要缺少 schema 前缀、POSIX 冒号路径误拒、行号词典序和非报告路径重复解析；均已修复并补测试。最终 review 部分完成（5 个文件中 2 条意见、3 个文件因预算未完成，耗时 5m15s）：manifest nil 意见由 `buildReportMaterial` 的显式错误返回处理，新增测试锁定；preview/report 意见因 Cobra 先执行 `Args` 再执行 `RunE`，参数解析已先设置 `reportEnabled`，新增裸参数和显式路径互斥测试锁定。较早的无上限/组预算审查在分发前被 token 预算拒绝，不作为完整审查结论。早期原生输出冲突检查保留，因为原生输出文件在后续范围检查前会打开，删除检查会使同路径报告先截断原生文件；其余结构建议已通过共享 `Scope` revision 消除重复推导。
- 提交后的独立 Spec 复审发现可选原生 finding 标签兼容问题：有效 `LlmComment` 允许 `Category`/`Severity` 缺省，但材料此前因此拒绝整份报告。材料保留空字符串原值，分别提供 `severity_status`/`severity_reason` 与 `category_status`/`category_reason`，缺失状态为 `not_collected` 并说明原因；中文展示使用“未提供”。非空但不支持的标签继续拒绝，避免把未知事实转成低等级或其他类别。回归测试覆盖同时缺省、各字段分别缺省、未知值拒绝、材料保存回读和 schema 校验。
- Standards 复核还发现测试夹具新增英文注释；已改为中文并按仓库扫描器约定标记。此前 Standards/Spec 初审指出的重复 revision 状态推导、无语义转发函数均已修正；主线程其余最终复审意见待补。
- 字段状态契约修复后，`go test ./cmd/opencodereview ./internal/report -count=1` 通过（CLI 152.7s、report 2.2s）；随后补充的展示映射一致性校验及测试通过定向命令 `go test ./cmd/opencodereview ./internal/report -run 'Test(BuildReportMaterialPreservesMissingFindingLabelFacts|ValidateMaterialChecksMissingFindingLabelFacts|ValidateMaterialChecksFindingFactFieldsAndUniqueIDs|ReviewE2E_ReportFindingAndNativeJSONAreIndependent)' -count=1`。`make check` 在一致性校验前一版退出码 0；最终版本 `make english-check` 退出码 0，659 个文件扫描通过。最终源码为 `4e707324b7fbdca14d2b63089ed36ce0d5e08a01`。

