# 规则、背景与知识读取

[返回知识库主文件](README.md)

## 三类输入的职责

| 输入 | 主要用途 | 示例 |
|---|---|---|
| 审查规则 | 长期复用的检查要求 | 批量操作必须检查选中数量与合法状态 |
| 业务背景 | 本次变更的目的和预期行为 | 编辑已处理记录时保留原处理状态 |
| 框架知识 | 理解代码所需的事实 | `EF` 是框架提供的全局对象 |

它们最终都参与模型上下文，但不能互相替代。差异说明代码发生了什么，背景说明应该实现什么；背景越准确，越有助于判断业务错误。没有证据支持“内容越多，效果必然越好”。

## rule 文件

通过 `--rule <文件>` 传入 JSON。最小示例：

```json
{
  "rules": [
    {
      "path": "src/**/*.{vue,ts,tsx,js,jsx}",
      "rule": "检查接口失败后是否继续执行成功路径。结合统一请求封装判断异常是否已有处理，不要无依据要求重复弹窗。"
    }
  ]
}
```

`path` 匹配文件，`rule` 是审查要求，不是规则 Markdown 的文件路径。JSON 字符串中的换行用 `\n` 表示。

每个文件按以下优先级查询规则，首个匹配项生效：

1. `--rule` 指定文件。
2. `<repo>/.opencodereview/rule.json`。
3. `~/.opencodereview/rule.json`。
4. 内置默认规则。

同层按声明顺序匹配，不自动累加所有条目。未命中的文件可以继续匹配其他层级。自定义规则默认替换命中的内置规则。

**版本待验证项**：安装的 Skill 描述了条目字段 `merge_system_rule: true`，用于同时包含命中的用户规则和内置规则。此选项尚未在调研中通过本机 CLI 验证；它不是所有用户规则自动叠加的开关。

检查规则命中无需调用模型：

```powershell
rtk pwsh -Command 'ocr rules check --repo "E:\代码仓库" --rule "E:\审查资料\rule.json" "src/views/example.vue"'
```

路径示例可以用于匹配检查，但 `--repo` 必须指向有效 Git 仓库。

## 文件筛选不等于规则匹配

规则文件还支持 `include` 与 `exclude`。`rules[].path` 决定使用什么要求，不限定整次审查只检查这些文件。

`include` 不是白名单：文档说明它可绕过部分默认测试文件和扩展名过滤。`exclude` 用于排除文件。二进制、秘密路径保护、过大差异、删除文件等也可能影响审查范围；具体过滤顺序以版本为准。先用 `--preview` 检查实际范围。

## background 的传入方式

短背景直接传文本：

```powershell
rtk pwsh -Command 'ocr review --repo "E:\代码仓库" --from "main" --to "feature/example" --background "本次新增事故台账编辑功能。新增记录初始化为 pending，编辑已有记录必须保留原处理状态。接口失败时保留弹窗和输入，不执行成功提示或刷新。" --format json --output "E:\审查结果\result.json"'
```

上例是说明参数用法的假设业务需求，不是任何真实项目已核实的约定。

较长背景写成 Markdown，传入文件路径：

```powershell
rtk pwsh -Command 'ocr review --repo "E:\代码仓库" --from "main" --to "feature/example" --background-file "E:\审查资料\background.md" --rule "E:\审查资料\rule.json" --format json --output "E:\审查结果\result.json"'
```

`--background-file` 读取正文，优先于 `--background`，两者不自动拼接。需要多来源内容时应先形成一份背景文件。背景文件本身可以由 CLI 从仓库外读取；这不意味着模型的仓库读取工具获得了仓库外访问能力。

推荐背景正文结构：

```markdown
# 本次变更背景

## 目标
要解决的业务问题与涉及模块。

## 预期行为
状态、权限、错误处理、数据口径和验收条件。

## 相关框架约定
本次涉及的 API、统一封装及来源版本。

## 边界与待确认事项
刻意保持的行为、例外及尚不明确的要求。
```

安装的官方 Skill 将收集业务背景列为审查前步骤。官方提示词将背景放入 `requirement_background`，规则放入 `system_rule`；文档说明背景用于计划和主审查阶段，不能据此推断所有后处理阶段都获得相同知识。

根据代码推断出的需求应标为推断，不能把现有实现当作正确性的依据。大量无关背景会消耗上下文，并可能在多个审查分组重复增加成本。

## 知识库路径是否可读

规则可以要求“先读知识索引，遇到组件问题再读相应参考文档”。这是对模型使用工具的引导，不是程序自动递归展开所有文档链接。

| 文档位置 | 可依赖的入口 |
|---|---|
| 被审查仓库目标提交中的 Markdown | 用仓库上下文工具读取；是否实际读取需要查看工具记录 |
| 工作目录中临时添加、未进入目标提交的 Markdown | 不能假设范围模式可见 |
| 仓库外的 Markdown | CLI 可通过 `--background-file` 注入该文件正文；其他引用文件不会自动注入 |
| 外部路径或 URL | 仅写入规则不保证有对应读取能力 |

少量框架事实可以直接放进规则或背景。较大的索引式知识库需要验证读取路径、引用文件可见性与上下文预算。

## 工具与实现边界

- `file_read`：读取仓库相对路径的变更后文件内容。
- `file_read_diff`：获取同次变更中其他文件的差异。
- `file_find`：按路径或文件名查找仓库文件。
- `code_search`：在仓库内搜索代码，底层使用 Git 搜索能力。
- `code_comment`：提交带位置和建议的评论。
- `task_done`：明确结束当前审查任务。

上下文读取并不自动扩大评论目标。文档约束主审查围绕分配的文件或分组产生问题，不能把任意项目级分析当作原生行级结果能力。

`--tools` 可修改工具定义及描述；增加新的工具行为需要 Go 侧注册和实现，只有 JSON 不足以提供外部知识检索。规则文件也不等于通用 Skill 执行环境，不自动执行 Skill 中的脚本或多阶段流程。

## 来源

- [审查规则](https://open-codereview.ai/docs/review-rules)
- [CLI 参数](https://open-codereview.ai/docs/cli-reference)
- [主审查提示词源码](https://github.com/alibaba/open-code-review/blob/main/internal/config/template/prompts/main_task_user.md)
- [工具文档源码](https://github.com/alibaba/open-code-review/blob/main/pages/src/content/docs/en/tools.md)
- 安装的官方 Skill，路径和证据范围见主文件。
