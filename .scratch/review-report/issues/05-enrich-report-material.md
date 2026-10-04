# 05：补齐成果、人员与知识来源材料

Status: ready-for-agent

Blocked by: [02：单提交审查生成独立报告材料](02-commit-report-material.md)

阶段：2。规格来源：[Spec](../spec.md)；执行约定及依赖总览：[工单索引](../README.md)。关联用户故事：11、12、14、21、22、33、34、35、41。

## 交付行为

先在单提交路径交付有证据的成果、人员、知识来源与中文材料，并以共享报告材料契约供其他模式复用；三种模式的整合在 06 完成。

## 验收条件

- [x] 在审查及材料阶段取得模块成果、代表变更、相关文件和提交依据，不让后续报告模型仅凭问题列表推造成果；没有证据的内容明确缺失。
- [x] 保存人员工作明细和关联依据，区分作者、提交者与未知身份；不把行数作为工时绩效或把修改者作为缺陷责任人。
- [x] 记录规则与实际读取知识的版本或内容摘要，区分读取成功、失败、未观察到及部分读取；同次审查使用一致版本，或检测并披露资料变动。
- [x] 为报告准备必要中文说明并保留原始英文说明、代码与 API 标识；结构检查未执行时记录未提供，不增加全仓库扫描，治理项和未确认事项不冒充确认缺陷。
- [x] 在实际 Git 统计、模型成果归纳和原生审查事实之间保留来源；同一问题沿用稳定标识，未知不转为零。
- [x] 材料整理记录独立耗时、模型用量和限制，避免重复整理；通过单提交 CLI 样例验证材料完整性，并覆盖不支持的成果推断、知识版本变化及归纳失败。复用 02 确定的材料契约，不要求分支或工作区工单先完成。

## 执行记录（2026-10-04）

- 执行者：主线程 Codex（原实施上下文长时间未提交后接续）。工作树：`D:\WorkPlace\open-code-review-worktrees\review-report-05-enrich`；分支：`codex/review-report-05-enrich`；基线：`8251970`（已集成工单 04）。
- 交付范围：单提交模式从 `git show` 和 `git diff-tree --name-status -z` 取得提交、文件、模块、作者和提交者事实；成果摘要明确仅依据 Git 事实。规则哈希与外部工具调用按成功、部分失败、失败和未观察标记知识来源；成功工具调用不被表述为知识正文已正确应用。非提交模式成果和人员保持 `not_collected`，结构检查仍为 `not_collected`，未增加全仓扫描或责任推断。
- 已运行：`go test ./cmd/opencodereview -run 'Test(ReviewE2E_CommitReportContainsEvidenceBackedEnrichment|KnowledgeSourcesSectionDistinguishesObservedPartialAndFailure|EnrichReportSectionsDoesNotInferNonCommitResults)' -count=1`，3 个测试通过。复测命令见[成果、人员和知识来源复测记录](../../../project-docs/mock/review-report-enrichment-05.md)。
- 独立 Spec 审查发现并促成三项修复：MCP/外部工具失败计数会同时存在于总调用计数中，现按失败数扣除后区分全部失败与部分失败；merge commit 文件现按原生 first-parent 基线统计；补齐材料整理耗时和模型用量适用状态。非原生工具调用不再被断言为知识读取，只作为观察到的工具活动，知识正文版本与正确应用仍标 `not_observed`。新增 merge first-parent、全部失败真实计数及不传 `--report` 原生兼容测试。
- 已验证：定向测试 5 个通过；修复后全量普通测试 `go test ./... -count=1` 通过（5137 个测试/25 包），`go vet ./...` 和 `make check` 通过（665 个源文件英文检查、license、gofmt、tidy 和 vet）。材料保留原生 `ProjectSummary`（如有）及来源/未采集状态。独立 Standards 复核、修复后 Spec 复核待完成；race 仍受 Windows CGO/GCC 前提限制。

