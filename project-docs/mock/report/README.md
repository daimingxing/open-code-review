# 报告 HTML 人工验收

用本地构建的二次开发 CLI，将已有报告材料 JSON 生成为 HTML，并在浏览器中查看。这里不调用官方 NPM `ocr` 命令。生成 HTML 需要本机已经配置可用的模型服务；输入 JSON 可由本工具的 `review --report` 生成。

默认输入是本目录的 [`sample-material.json`](sample-material.json)，它只用于观察 HTML 生成效果，内容为虚构的仓库、统计和问题，不证明真实审查或知识应用通过。也可换成自己用本地 CLI `review --report <路径>` 生成的材料 JSON。然后在仓库根目录使用 PowerShell 7：

```powershell
& ./project-docs/mock/report/run-report.ps1 `
  -MaterialPath './project-docs/mock/report/sample-material.json' `
  -GoExecutable 'D:\WorkPlace\toolchains\go1.25.14\go\bin\go.exe'
```

脚本会从当前工作树构建 `cmd/opencodereview`，调用本地程序的 `report --input ... --output ...`，并打开新生成的 HTML。JSON 不会改写；输出放在系统临时目录。预期浏览器能离线打开报告，必要章节、问题与基础统计可见。真实模型生成过程可能因服务限流、超时或内容校验失败而返回错误；失败不会产出可验收的成功报告。
