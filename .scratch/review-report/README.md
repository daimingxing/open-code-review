# 审查与报告工单

日期：2026-10-04。拆分已获用户确认，9 张工单已发布；01 已验收并集成至 `feature-review-report`，02–09 尚待实施与验收。需求与最终验收以 [Spec](spec.md) 为准，本索引只维护执行顺序和工单入口。

由主线程协调多个实施智能体完成整项功能时，读取[实施协调计划](implementation-plan.md)，按其中的派发、审查、集成及交接方式推进。

## 执行顺序

| 工单 | 阶段 | 前置工单 | 交付 |
|---|---|---|---|
| [01](issues/01-verify-external-knowledge.md) | 1 | 无 | 验收外部知识的选读与实际应用 |
| [02](issues/02-commit-report-material.md) | 2 | 01 | 单提交审查生成独立报告材料 |
| [03](issues/03-branch-report-material.md) | 2 | 02 | 分支比较生成准确范围的报告材料 |
| [04](issues/04-workspace-report-material.md) | 2 | 02 | 工作区审查生成真实快照的报告材料 |
| [05](issues/05-enrich-report-material.md) | 2 | 02 | 补齐成果、人员与知识来源材料 |
| [06](issues/06-partial-review-material.md) | 2 | 03、04、05 | 交付不完整审查材料与失败恢复行为 |
| [07](issues/07-single-html-report.md) | 3 | 06 | 单份报告材料生成中文 HTML |
| [08](issues/08-multi-input-html-report.md) | 3 | 07 | 多份报告材料汇总为一份 HTML |
| [09](issues/09-report-experience-and-reliability.md) | 3 | 08 | 报告阅读体验与长报告可靠性 |

01 验收知识选读与应用后，进入 02–06 的报告材料阶段；02 后的 03、04、05 可以并行。06 整合并验收三种模式后进入 HTML 阶段：07 打通单份生成，08 支持多份汇总，09 完善阅读体验与长报告可靠性。完整功能以所有工单验收完成为准。

## 共同执行约定

- 真实项目效果验收使用[长期资源索引](../../project-docs/review-resources.md)中的仓库与资料，不各自维护路径副本；开发中可复用的命令和实际结果记录到[复测命令目录](../../project-docs/mock/README.md)，工单链接具体场景。
- 仅启动前置工单均已验收完成的任务。Status 表示分诊状态，ready-for-agent 不表示其依赖已完成或功能已交付；执行结论与证据记录在对应工单，讨论写入 Comments。
- 每张工单自行交付可验证行为、相应测试与必要使用说明。06 和 09 各有新的用户行为，不是替前序工单统一补测试。
- 输入校验、事实边界、安全展示及防覆盖随首次可用路径交付。07 可暂不具备 09 的完整阅读交互，但不得放宽安全或真实性要求。
- 自动验收从 CLI 观察参数、退出状态、stdout/stderr 和文件内容，使用临时仓库、隔离配置及可控模型；知识实际应用必须另有真实模型证据。执行仓库已有的必要检查，不按私有实现结构或固定模型措辞断言。
- 按[术语表](../../GLOSSARY.md)命名；涉及架构边界时读取[审查与报告分离](../../project-docs/adr/0001-separate-review-and-report.md)、[外部知识读取](../../project-docs/adr/0002-external-knowledge-access.md)、[模型生成 HTML](../../project-docs/adr/0003-model-generated-html.md)。实现遵循[项目级设计原则](../../project-docs/project-design.md)。

