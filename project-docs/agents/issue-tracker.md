# 本地工作事项

本项目的需求、任务与讨论使用本地 Markdown 文件管理。技能要求“发布到问题跟踪系统”时，按以下约定写入本地文件；要求“获取任务”时，读取指定文件。

## 文件组织

- 每项功能使用独立目录：`.scratch/<feature>/`，其中 `<feature>` 为简短英文标识。
- 需求说明：`.scratch/<feature>/spec.md`。
- 实施任务：`.scratch/<feature>/issues/<NN>-<slug>.md`，每个任务一个文件，编号从 `01` 开始。
- 分诊状态放在任务文件顶部附近的 `Status:` 字段中，名称见 [分诊状态约定](triage-labels.md)。
- 讨论按时间追加到文件末尾的 `## Comments` 下。
- 引用任务优先使用文件路径；仅提供编号时，在已确定的功能目录内解析，存在歧义时先确定所属功能。

## 探索型工作

`wayfinder` 使用以下本地约定管理探索过程：

- 探索地图：`.scratch/<effort>/map.md`，包含 `Notes`、`Decisions-so-far` 和 `Fog` 部分。
- 子任务：`.scratch/<effort>/issues/<NN>-<slug>.md`，编号从 `01` 开始，正文记录待解决问题。
- `Type:` 字段取值为 `research`、`prototype`、`grilling` 或 `task`。
- 这些子任务的 `Status:` 使用 `open`、`claimed`、`resolved`，表示探索执行状态，与实施任务的分诊状态分开使用。
- 依赖以文件顶部附近的 `Blocked by: NN, NN` 表示，编号在同一探索目录内解析；依赖全部为 `resolved` 后才可开始。
- 按编号选择首个 `open` 且依赖已解决的任务，开始前将状态写为 `claimed`。
- 完成后在 `## Answer` 下追加结果，将状态改为 `resolved`，并在地图的 `Decisions-so-far` 中追加结论摘要和任务文件链接。
