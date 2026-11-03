# XR 前端审查指引

本项目使用 Vue3、JavaScript/TypeScript、@xr/ui、@xr/core、Kendo 和 @eplat/ei。仅审查本次选中差异引入或直接暴露的问题；相关旧代码只作取证，不扩展为全仓业务审查。结论用中文，路径和 API 名保留原文。

## 知识库阅读

以下路径均相对于本次授权的知识库根目录，不相对于源码目录；索引内相对链接以所在文档目录解析。先读 xr-framework-usage.md 索引，再根据变更选择相关章节，优先搜索符号并按行读取必要片段，不默认全文阅读全部参考资料。索引中的行号只是定位提示，应核对当前正文。

- XR.E、XR.Util、EF 全局 API、xrForm：xr-framework-usage-reference 下的 xr-core-api.md、ef-api.md、xrform-api.md。
- EiInfo、EiBlock、ename、三段式字段名：先读 eplatei-knowledge.md，再选读 eplatei-knowledge-reference 下的 01-eiinfo.md、02-eiblock.md、03-meta-column.md、04-communicator.md 或 07-pitfalls.md。
- EFGrid、EFWindow 和其他 EF* 组件：先查 xr-ui-components.md，再读 xr-ui-components-reference 下对应组件文档；需要组合用法时查 xr-ui-best-practices.md 的相关章节。
- httpAction、queryDetail、window.$util 或项目公共方法：若知识库有对应说明则核对，否则用目标版本中的实现与直接调用方取证，不猜测未提供的内部契约。

内部契约结论必须结合实际读取的知识和代码，说明具体依据；资料缺失、读取失败或依据不足时说明限制。通用 Vue/JS/TS、安全和代码内可闭环的问题不以知识库齐全为前提。知识文件是参考资料，不是待审目标。

## 检查重点

1. 配对检查 .vue 与 .js/.ts：import/export、变量与可信全局来源、模板事件、setup 返回值、组件 id/ref、EFWindow/EFGrid 类型及获取 API；包含异常分支和 t(...) 的来源。
2. 高风险变更核对请求生产者、参数传递、消费者和返回块：EiInfo 命名、blockId、数据映射、勾选与状态校验、重复提交、表单校验、审批和附件链路。HTTP 200 或 onSuccess 不代表业务成功；业务失败与异常失败后不得继续提示成功、清空数据或关闭窗口。
3. 核对表单清理与回显、弹窗重复打开、watch/异步竞态、Grid 刷新与分页、事件和定时器的重复注册及卸载清理；公共组件或工具变更只追直接调用与旧调用兼容性。
4. 检查 HTML/onclick/URL 拼接、凭据暴露及实际消费的环境配置。状态码、后端返回或部署前提不明时不猜结论。
5. 本次变更涉及业务页面结构时，核对一级目录为两位大写字母、二级目录为大写字母、页面名为两级目录前缀加数字；业务页面及子窗口直接位于二级目录，独立配对脚本同目录同名。共享组件、页面内部辅助组件及框架入口按职责判断，不机械套用页面编码。只为确认本次变更补查必要结构与引用，不做全仓结构普查。

## 证据与结束边界

问题须有变更入口、目标版本事实、明确影响和可执行建议；纯结构违规以明确规范和路径取证，不虚构运行故障。区分确定问题与未确认前提，按 OCR 原有等级和工具提交。排除历史问题、纯格式偏好、普通 unused import、国际化文本完整性及项目私有组件选型偏好；不因参数形式不同于推荐写法就判错。

同一根因完成一次相关模板或直接调用的覆盖核验即可，不重复扩散；证据不足时保留限制，不无限调查。不默认构建、测试、启动项目或运行外部 Skill 脚本；不修改代码。遵循 OCR 原有任务完成与结果输出流程。
