// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

func collectFindingCommitters(runner *gitcmd.Runner, repoDir string, scope report.Scope, findings []report.Finding) []report.Limitation {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var limitations []report.Limitation
	for i := range findings {
		finding := &findings[i]
		finding.CommitterNames = []string{}
		reason := ""
		status := report.StatusNotCollected
		switch {
		case scope.Mode == session.InputModeWorkspace:
			reason = "工作区问题未关联历史提交人，避免把未提交代码归于 HEAD 提交人" // allow-non-english: 报告材料说明未知提交人
		case scope.ResolvedHead.Status != report.StatusProvided || runner == nil:
			reason = "缺少已固定的目标提交或 Git 采集能力，未确定问题提交人" // allow-non-english: 报告材料说明未知提交人
		case finding.StartLine <= 0 || finding.EndLine < finding.StartLine:
			reason = "问题没有有效代码行定位，未确定提交人" // allow-non-english: 报告材料说明未知提交人
		case !filepath.IsLocal(filepath.FromSlash(finding.Path)):
			reason = "问题路径不在仓库内，未采集提交人" // allow-non-english: 报告材料说明未知提交人
		default:
			// 只按审查时固定版本的定位行采集提交者；不使用当前 HEAD 或推断组外代码归属。 // allow-non-english: 用户要求中文代码注释
			output, err := runner.Output(ctx, repoDir, "blame", "--line-porcelain", "-L", fmt.Sprintf("%d,%d", finding.StartLine, finding.EndLine), scope.ResolvedHead.SHA, "--", finding.Path)
			if err != nil {
				status = report.StatusFailed
				reason = "Git 无法读取目标版本的问题定位行，提交人未确定；可能缺少文件、行号或历史资料" // allow-non-english: 报告材料不复制可能含本机路径的 Git 错误
				break
			}
			names := map[string]bool{}
			for _, line := range strings.Split(string(output), "\n") {
				if name, ok := strings.CutPrefix(line, "committer "); ok && strings.TrimSpace(name) != "" {
					names[strings.TrimSpace(name)] = true
				}
			}
			for name := range names {
				finding.CommitterNames = append(finding.CommitterNames, name)
			}
			sort.Strings(finding.CommitterNames)
			if len(finding.CommitterNames) == 0 {
				reason = "Git 未提供问题定位行的提交人姓名" // allow-non-english: 报告材料说明未知提交人
			}
		}
		if reason != "" {
			limitations = append(limitations, report.Limitation{Source: "findings." + finding.ID + ".committer_names", Status: status, Reason: reason})
		}
	}
	return limitations
}
