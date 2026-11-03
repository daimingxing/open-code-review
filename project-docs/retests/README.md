# 开发复测

本目录按可重复的行为场景组织开发回归、复现与排障命令。这里的记录说明所测版本、依赖、输入、通过条件和实际结果；不作为用户首次体验产品的入口。

## 场景

| 场景 | 用途 |
|---|---|
| [报告材料](report-material/README.md) | 单提交、分支和工作区材料，完整性及失败恢复 |
| [HTML 报告](html-report/README.md) | 智能体循环交付、材料读取、压缩继续、失败恢复和多份输入顺序 |
| [外部知识访问](knowledge-access/README.md) | 原生知识接入迁移状态、常用样本与可复用审查背景 |
| [历史验收档案](archive/README.md) | 本次功能集成时的逐次测试结果、失败尝试和审查证据 |

面向用户的简明验收步骤与真实输入从 [`project-docs/mock/`](../mock/README.md) 开始。归档中的命令反映当时的分支和工作树；当前复测应使用上方场景入口。

## 2026-11-03 上游同步

清理提交 dcfaab2 在 feature-review-report 完成，经规范与回归风险、Spec 符合度两项独立子智能体审查及修复后合入 dev。dev-sync 以用户已更新的 main（b52b53b，包含 v1.13.4 及其后两个提交）为上游基础，再合入 dev；不改写 main 或分支历史。

唯一文本冲突为 go.mod，保留上游 Go 1.26.10、x/sys 0.48.0 和 x/net 0.60.0，并保留本项目直接使用的 jsonschema 依赖。共享循环保留官方收尾完成和预算警告语义，报告测试覆盖收尾完成后交付及未完成不发布；MCP 工具列表分页修复完整保留。

本机 Go 1.27.2、CGO_ENABLED=0：make test 无法启用 race，使用 LC_ALL=C 的普通 Go 全仓库测试补充回归；go vet ./... 和 make build 通过。默认依赖代理不可达，本次命令临时使用 goproxy.cn，未更改全局 Go 配置。delegate 的 Bash 测试初次命中 WindowsApps/bash 失败，将 Git/usr/bin 放到本次命令 PATH 前端后整包重跑通过。

make check 的许可证检查通过，英文检查仍由既有 archive/manual-sample 的 31 行中文样例阻断，未为同步扩大源码语言豁免。未运行编辑器前端构建或真实项目完整付费审查；新知识目录参数尚未实现。

同步回归最终结果：全仓库 26 个包完成，除 delegate 的 Bash 环境问题外均通过；delegate 在临时 PATH 指向 Git Bash 后整包通过。原始 JSON 日志位于本机临时目录 ocr-dev-sync-regression.jsonl，不入库。同步结果另经子智能体核对共享循环、MCP 分页及模型客户端改动，未发现阻塞问题。未启用 race，且全仓库英文检查的历史样例限制仍存在。
