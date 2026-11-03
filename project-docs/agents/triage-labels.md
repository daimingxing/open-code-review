# 分诊状态

工程技能中的五个分诊角色映射为以下状态名称，写入本地实施任务的 `Status:` 字段。

| 技能中的角色 | 本地状态名称 | 含义 |
|---|---|---|
| `needs-triage` | `needs-triage` | 待评估 |
| `needs-info` | `needs-info` | 待补充信息 |
| `ready-for-agent` | `ready-for-agent` | 需求明确，可由智能体实施 |
| `ready-for-human` | `ready-for-human` | 需人工实施 |
| `wontfix` | `wontfix` | 不处理 |

技能要求应用分诊标签时，更新对应任务文件的 `Status:` 字段。新建且尚未评估的实施任务使用 `needs-triage`。

探索型子任务的执行状态遵循 [本地工作事项](issue-tracker.md) 中的探索型工作约定。
