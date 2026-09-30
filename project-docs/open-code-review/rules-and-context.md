# 规则、背景与知识读取

[返回知识库索引](README.md)。本文件说明原生能力和已有验证，审查功能如何使用这些能力见[功能 Spec](../../.scratch/review-report/spec.md)。源码基准为 `578e647`，CLI 和 MCP 的实测范围分别标明。

## 规则加载

`--rule <文件>` 原生接受 JSON；`rules[].path` 匹配代码路径，`rules[].rule` 可以是审查要求正文，也可以直接引用 Markdown 文本文件：

```json
{
  "rules": [
    {
      "path": "src/**/*.{vue,ts,tsx,js,jsx}",
      "rule": "rules/xr-review.md",
      "merge_system_rule": true
    }
  ]
}
```

正文示例见 [XR 规则文件](../examples/xr-review-rule.json)。文件引用由 [规则解析器](../../internal/config/rules/system_rules.go) 处理：

- 支持 `.md`、`.txt`、`.markdown`；只有单行、无空格、扩展名匹配的字符串被识别为路径。引用路径含空格时不能照此方式使用；这不等于 CLI 的 `--rule` 参数路径不允许空格。
- `--rule` 和全局规则中的相对引用以 JSON 所在目录解析；仓库规则以仓库根目录解析，并限制实际路径不能逃出仓库。外部自定义规则可使用绝对 Markdown 路径。
- 引用文件上限为 512 KiB，检查扩展名与符号链接。读取失败会将该条规则正文置空并警告，不能只凭 JSON 加载成功认定规则有效。
- 仅加载直接引用文件的正文，不递归展开正文中的链接，不执行脚本或 Skill。
- 优先级为 `--rule` → 仓库 `.opencodereview/rule.json` → 全局 `~/.opencodereview/rule.json` → 内置规则；每层按声明顺序取首个匹配，不累加多个用户规则。
- 自定义规则默认替换内置规则。`merge_system_rule: true` 合并命中的用户规则与内置规则；这是源码确认的能力，尚未单独完成实际 CLI 合并效果验证。

无需调用模型即可检查命中：

```powershell
ocr rules check --repo "E:\代码仓库" --rule "E:\审查资料\rule.json" "src/views/example.vue"
```

`rules[].path` 只选择适用规则，不限定整次审查范围。`include` / `exclude`、原生扩展名与测试文件过滤、二进制或秘密文件保护、删除文件及差异大小等共同影响选中范围；`include` 不能视为唯一允许集合。集成前使用 `--preview` 检查。

## 业务背景

规则是长期复用的判断标准；背景说明本次需求、预期行为和边界；知识提供框架或业务事实。三者均可影响模型判断，但不能把现有代码自动当作正确需求，也不能将推断写成已确认事实。

```powershell
ocr review --repo "E:\代码仓库" --from main --to feature/example --rule "E:\审查资料\rule.json" --background "本次新增编辑流程；请求失败时应保留表单，不执行成功提示。"
ocr review --repo "E:\代码仓库" --from main --to feature/example --background-file "E:\审查资料\background.md"
```

以上背景只是用法示例。背景文件可组织为目标、预期行为、相关约定与待确认事项。

`--background-file` 优先于 `--background`，两者不自动拼接。[背景加载器](../../cmd/opencodereview/background_file.go) 清洗后超过 2000 字符警告、超过 8000 字符报错；这是该入口的限制，不是模型总上下文上限，也不是 Markdown 规则的文件上限。

背景文件可以来自仓库外，但注入其正文不会授权读取其他文件。背景与规则分别进入 `requirement_background`、`system_rule`；不能推断所有后处理阶段都获得同样资料。无关背景可能在多个审查分组重复增加成本。

## 内置读取与外部知识的区别

| 入口 | 能力边界 |
|---|---|
| Markdown 规则引用 | 加载指定文档全文作为规则，不跟随链接 |
| `--background-file` | 加载指定背景正文，不跟随链接 |
| 工作区模式的 `file_read` | 从仓库内磁盘读取，防止符号链接逃逸 |
| 分支/提交模式的 `file_read` | 从目标 Git 引用读取，磁盘存在不代表目标版本中存在 |
| 已配置的外部 MCP 工具 | 按对应服务权限读取外部资料，独立于代码 Git 范围 |
| 规则里只提到路径或 URL | 仅为模型指引，不自动赋予读取权限或工具 |

[FileReader](../../internal/tool/filereader.go) 决定磁盘或 Git 读取模式；[file_read](../../internal/tool/file_read.go) 一次最多返回 500 行，提供 `IS_TRUNCATED` 和行范围，可继续分段读取。不能把这一能力套用到所有外部文件工具。

其他内置工具包括 `file_find`、`code_search`、`file_read_diff`、`code_comment` 和 `task_done`。它们支持上下文、差异、评论及任务完成，不提供任意本机文件访问，也不自动扩大可评论的代码范围。

`--tools` 修改工具定义不等于实现新的工具执行行为。OCR 原生有 MCP 客户端；[initMCPClients](../../cmd/opencodereview/review_cmd.go) 接入服务，[RegisterAll](../../internal/mcp/provider.go) 注册工具，无须先扩展 rule 解析器。MCP 初始化失败会警告并继续审查，不能依赖规则提示实现程序级强制中断。

## 已验证的文件 MCP 接入

外部服务 `@modelcontextprotocol/server-filesystem@2026.8.31` 以 Node.js 本地标准输入输出进程运行，由 OCR 启动和关闭，无须部署远程后台。服务本身支持读写；以下配置通过 OCR 非空工具白名单只注册读取类工具。路径为占位值，需按实际安装位置替换。

```json
{
  "mcp_servers": {
    "knowledge": {
      "command": "C:/path/to/node.exe",
      "args": ["D:/tools/node_modules/@modelcontextprotocol/server-filesystem/dist/index.js", "D:/knowledge"],
      "tools": ["read_text_file", "list_directory", "search_files", "get_file_info", "list_allowed_directories"]
    }
  }
}
```

- `args` 指定允许访问的知识目录；rule 中的索引路径不能扩大该范围。
- `tools` 为空表示不筛选，会开放全部可注册工具；不能用空列表表达禁止访问。
- 只读限制成立于 OCR 工具注册边界，不是操作系统只读挂载；服务进程仍具有启动用户的系统权限。
- `search_files` 是路径模式搜索，不是正文关键词或语义检索。
- `read_text_file` 支持全文、头部或尾部，不支持任意中间行范围。已有索引可引导分册读取，长篇正文仍需控制返回规模。
- 绝对 Node.js 与脚本路径可避免依赖 Windows npm 包装脚本的启动方式。

## 验证记录

外部知识样例来自 `D:/WorkPlace/xc-imms/xcmpms-imms-f/.ai_knowledge`。以下是已完成的验证，不代表真实模型效果已验收：

| 日期 | 方法 | 结果与边界 |
|---|---|---|
| 2026-09-29 | 原生 `ocr rules check` | 仓库外 JSON 命中 `Custom (--rule)`；直接引用外部 `eplatei-knowledge.md` 能加载索引正文。只验证规则加载，不包含模型审查。 |
| 2026-09-29 | 本地链接与 Git 只读检查 | Ei、XR UI 索引链接的分册存在；目录被忽略且当时 HEAD 不含资料，因此不能通过分支/提交模式的原生 `file_read` 读取。 |
| 2026-09-30 | 原生 OCR、临时仓库、隔离配置、本地模型替身及上述 MCP 服务 | 索引和 `eplatei-knowledge-reference/01-eiinfo.md` 读取成功，目录外读取被拒绝；模型工具列表没有写入、编辑、移动或创建目录工具，退出码 0、状态 `complete`。 |
| 2026-09-30 | 直接 MCP 调用 | 父目录读取也被拒绝。 |

测试未修改正常用户配置、业务知识库或产品源码；未调用付费模型。模型替身按预设步骤请求工具，证明接入与执行链路可用，不证明真实模型会选对章节、正确应用知识或提高审查质量；这些留给真实模型验收。

## 上下文成本

2026-09-30 对上述知识目录只读测量，使用 `js-tiktoken@1.0.21` 的 `cl100k_base` 编码，与基准源码的 [CountTokens](../../internal/llm/client.go) 默认计数方式一致。

| 指标 | 结果 |
|---|---|
| 文件与正文规模 | 52 个文件，255,901 字节（约 249.9 KiB），204,476 个字符 |
| 逐文件 Token 合计 | 75,507 |
| 加来源标题合并后的 Token | 76,273 |
| 最大两份文档 | `xr-ui-best-practices.md` 22,026；`project-public-components.md` 12,843 Token |
| Ei 与 XR UI 两份索引 | 分别 1,132、1,194，共 2,326 Token |

供应商实际分词和计费可能不同。约 7.6 万 Token 相当于 128,000 窗口的 59.6%，或 200,000 窗口的 38.1%，尚未包括代码、规则、工具结果和输出空间。

基准模板 `MAX_TOKENS` 为 200,000，可由配置覆盖，但不代表供应商支持同样窗口。[checkPromptBudget](../../internal/agent/agent.go) 对主审查消息执行 80% 预算检查；规则进入各组的初始提示，初始两条消息属于 [压缩保留区域](../../internal/llmloop/compression.go)，不能依赖历史压缩自动移除大段知识。

拆分文件后仍全部注入不减少输入。文件可加载、窗口可容纳与模型有效使用资料是不同问题，质量影响需要真实模型评估。

## 来源

- [规则解析器](../../internal/config/rules/system_rules.go)、[主审查提示词](../../internal/config/template/prompts/main_task_user.md)
- [审查规则文档](https://open-codereview.ai/docs/review-rules)、[CLI 文档](https://open-codereview.ai/docs/cli-reference)
- [Filesystem 服务说明](https://github.com/modelcontextprotocol/servers/tree/main/src/filesystem)：运行验证版本为 npm `2026.8.31`，在线主分支可能继续变化。
- 命令帮助、安装的官方 Skill 及上述本地测试；版本范围见 [索引](README.md)。
