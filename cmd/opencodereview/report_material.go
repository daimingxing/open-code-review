// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/spf13/cobra"
)

const bareReportValue = "\x00"

func reviewArgsValidator(opts *reviewOptions) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if opts.reportPath == bareReportValue {
			opts.reportEnabled = true
			opts.reportPath = ""
		} else if cmd.Flags().Changed("report") {
			opts.reportEnabled = true
		}
		if len(args) == 0 {
			return nil
		}
		if !opts.reportEnabled || opts.reportPath != "" || len(args) != 1 {
			return fmt.Errorf("unexpected positional arguments %q; --report accepts at most one path", args)
		}
		opts.reportPath = args[0]
		return nil
	}
}

func defaultReportPath(repoDir string, scope report.Scope, startedAt time.Time) string {
	repoLabel := sanitizeReportLabel(filepath.Base(filepath.Clean(repoDir)), 48)
	var scopeLabel string
	switch scope.Mode {
	case session.InputModeCommit:
		scopeLabel = "commit-" + scope.RequestedHead
	case session.InputModeRange:
		scopeLabel = "range-" + scope.RequestedFrom + "-to-" + scope.RequestedHead
	case session.InputModeWorkspace:
		scopeLabel = "workspace"
	default:
		scopeLabel = "review"
	}
	scopeLabel = sanitizeReportLabel(scopeLabel, 64)
	name := fmt.Sprintf("%s__%s__%s.report.json", repoLabel, scopeLabel, startedAt.Local().Format("20060102-150405"))
	return filepath.Join(repoDir, name)
}

func requestedReportScope(opts reviewOptions) report.Scope {
	scope := report.Scope{Mode: session.InputModeWorkspace}
	switch {
	case opts.commit != "":
		scope.Mode = session.InputModeCommit
		scope.RequestedHead = opts.commit
	case opts.from != "" && opts.to != "":
		scope.Mode = session.InputModeRange
		scope.RequestedFrom = opts.from
		scope.RequestedHead = opts.to
	}
	return scope
}

func sanitizeReportLabel(value string, maxRunes int) string {
	var builder strings.Builder
	lastHyphen := false
	runeCount := 0
	for _, char := range strings.TrimSpace(value) {
		if unicode.IsControl(char) || unicode.IsSpace(char) || strings.ContainsRune(`<>:"/\\|?*`, char) {
			if !lastHyphen {
				builder.WriteByte('-')
			}
			lastHyphen = true
		} else {
			builder.WriteRune(char)
			lastHyphen = char == '-'
		}
		runeCount++
		if runeCount >= maxRunes {
			break
		}
	}
	label := strings.Trim(builder.String(), " .-")
	if label == "" {
		return "unknown"
	}
	return label
}

func buildReportMaterial(
	manifest *session.RunManifest,
	repoDir string,
	comments []model.LlmComment,
	startedAt, completedAt time.Time,
	provider, modelName string,
	toolFailures []llmloop.ToolFailureDetail,
) (report.Material, error) {
	if manifest == nil {
		return report.Material{}, fmt.Errorf("native run manifest is unavailable")
	}
	repositoryIdentity := report.Fact{Status: report.StatusNotCollected, Reason: "原生 manifest 未提供仓库身份 SHA-256"} // allow-non-english: report JSON requires Chinese user-facing facts
	if manifest.Repository.IdentitySHA256 != "" {
		repositoryIdentity = report.Fact{Status: report.StatusProvided, Value: "sha256:" + manifest.Repository.IdentitySHA256}
	}
	sourceArtifact := report.Fact{Status: report.StatusNotCollected, Reason: "原生 manifest 未提供输入工件 SHA-256"} // allow-non-english: report JSON requires Chinese user-facing facts
	if manifest.Input.SourceArtifactSHA256 != "" {
		sourceArtifact = report.Fact{Status: report.StatusProvided, Value: manifest.Input.SourceArtifactSHA256}
	}

	base, err := materialRevision("scope.resolved_base", manifest.Input.ResolvedBase, manifest, false)
	if err != nil {
		return report.Material{}, err
	}
	head, err := materialRevision("scope.resolved_head", manifest.Input.ResolvedHead, manifest, true)
	if err != nil {
		return report.Material{}, err
	}
	scope := report.Scope{
		Mode:           manifest.Input.Mode,
		RequestedFrom:  manifest.Input.RequestedFrom,
		RequestedHead:  manifest.Input.RequestedHead,
		ResolvedBase:   base,
		ResolvedHead:   head,
		ExactRange:     manifest.Input.ExactRange,
		SourceArtifact: sourceArtifact,
	}
	repoName := filepath.Base(filepath.Clean(repoDir))
	findings, err := materialFindings(manifest, repoName, comments)
	if err != nil {
		return report.Material{}, err
	}
	sections := report.MaterialSections{
		GitStatistics:     notCollectedSection("本次审查尚未计算 Git 统计数据"),          // allow-non-english: report JSON requires Chinese user-facing facts
		WorkspaceSnapshot: notCollectedSection("本次审查尚未采集工作区状态统计"),            // allow-non-english: report JSON requires Chinese user-facing facts
		Achievements:      notCollectedSection("本次审查尚未采集模块或成果依据"),            // allow-non-english: report JSON requires Chinese user-facing facts
		People:            notCollectedSection("本次审查尚未采集人员贡献依据"),             // allow-non-english: report JSON requires Chinese user-facing facts
		KnowledgeSources:  notCollectedSection("本次审查 manifest 未记录实际读取的知识来源"), // allow-non-english: report JSON requires Chinese user-facing facts
		StructuralChecks:  notCollectedSection("本次审查尚未采集结构检查结果"),             // allow-non-english: report JSON requires Chinese user-facing facts
	}
	if manifest.Input.Mode != session.InputModeWorkspace {
		sections.WorkspaceSnapshot = report.Section{Status: report.StatusNotApplicable, Reason: "本次输入不是工作区审查"} // allow-non-english: report JSON requires Chinese user-facing facts
	}
	material := report.Material{
		SchemaVersion: report.MaterialSchemaVersion,
		Review: report.ReviewMetadata{
			RunID:       manifest.RunID,
			StartedAt:   startedAt,
			CompletedAt: completedAt,
			ElapsedMS:   manifest.ElapsedMS,
			Status:      manifest.TerminalState,
			OCRVersion:  manifest.Execution.OCRVersion,
			Provider:    provider,
			Model:       modelName,
		},
		Repository:  report.Repository{Name: repoName, Identity: repositoryIdentity},
		Scope:       scope,
		Findings:    findings,
		Coverage:    manifest.Coverage,
		RunFailure:  manifest.RunFailure,
		Sections:    sections,
		Limitations: materialLimitations(manifest, scope, sections, findings, toolFailures),
	}
	return material, nil
}

func materialLimitations(
	manifest *session.RunManifest,
	scope report.Scope,
	sections report.MaterialSections,
	findings []report.Finding,
	toolFailures []llmloop.ToolFailureDetail,
) []report.Limitation {
	limitations := make([]report.Limitation, 0)
	add := func(source string, status report.Status, reason string) {
		if status != report.StatusProvided {
			limitations = append(limitations, report.Limitation{Source: source, Status: status, Reason: reason})
		}
	}
	if manifest.RunFailure != nil {
		reason := manifest.RunFailure.Reason
		if reason == "" {
			reason = "原生 manifest 记录了运行级失败，但没有提供具体原因" // allow-non-english: report JSON requires Chinese limitation reasons
		}
		add("run_failure", report.StatusFailed, reason)
	}
	for _, item := range manifest.Coverage.Failed {
		reason := item.Reason
		if reason == "" {
			reason = "原生 coverage 将该审查项标记为失败，但没有提供具体原因" // allow-non-english: report JSON requires Chinese limitation reasons
		}
		add("coverage.failed."+item.ItemID, report.StatusFailed, reason)
	}
	for _, item := range manifest.Coverage.Waived {
		reason := item.Reason
		if reason == "" {
			reason = "原生 coverage 将该审查项标记为未审查" // allow-non-english: report JSON requires Chinese limitation reasons
		}
		add("coverage.waived."+item.ItemID, report.StatusNotCollected, reason)
	}
	if manifest.Repository.IdentitySHA256 == "" {
		add("repository.identity", report.StatusNotCollected, "原生 manifest 未提供仓库身份 SHA-256") // allow-non-english: report JSON requires Chinese limitation reasons
	}
	if manifest.Input.SourceArtifactSHA256 == "" {
		add("scope.source_artifact", report.StatusNotCollected, "原生 manifest 未提供输入工件 SHA-256") // allow-non-english: report JSON requires Chinese limitation reasons
	}
	addRevisionLimitation(&limitations, "scope.resolved_base", scope.ResolvedBase)
	addRevisionLimitation(&limitations, "scope.resolved_head", scope.ResolvedHead)
	sectionsByName := []struct {
		name    string
		section report.Section
	}{
		{"git_statistics", sections.GitStatistics},
		{"workspace_snapshot", sections.WorkspaceSnapshot},
		{"achievements", sections.Achievements},
		{"people", sections.People},
		{"knowledge_sources", sections.KnowledgeSources},
		{"structural_checks", sections.StructuralChecks},
	}
	for _, entry := range sectionsByName {
		add("sections."+entry.name, entry.section.Status, entry.section.Reason)
	}
	for _, finding := range findings {
		add("findings."+finding.ID+".evidence", finding.Evidence.Status, finding.Evidence.Reason)
		add("findings."+finding.ID+".recommendation", finding.Recommendation.Status, finding.Recommendation.Reason)
	}
	for index, failure := range toolFailures {
		reason := fmt.Sprintf("OCR 工具 %q 调用失败；为避免复制原始参数与响应，错误详情未写入材料", failure.ToolName) // allow-non-english: report JSON requires Chinese limitation reasons
		add(fmt.Sprintf("tool_failure.%d", index+1), report.StatusFailed, reason)
	}
	return limitations
}

func addRevisionLimitation(limitations *[]report.Limitation, source string, revision report.Revision) {
	if revision.Status != report.StatusProvided {
		*limitations = append(*limitations, report.Limitation{Source: source, Status: revision.Status, Reason: revision.Reason})
	}
}

func materialRevision(name, sha string, manifest *session.RunManifest, isHead bool) (report.Revision, error) {
	if sha != "" {
		return report.Revision{Status: report.StatusProvided, SHA: sha}, nil
	}
	mode := manifest.Input.Mode
	if mode == session.InputModeWorkspace && isHead {
		return report.Revision{Status: report.StatusNotApplicable, Reason: "工作区审查没有固定的目标提交"}, nil // allow-non-english: report JSON requires Chinese scope facts
	}
	if mode == session.InputModeWorkspace && !isHead && !inputResolutionFailed(manifest) {
		return report.Revision{Status: report.StatusNotApplicable, Reason: "仓库没有 HEAD 提交"}, nil // allow-non-english: report JSON requires Chinese scope facts
	}
	if mode == session.InputModeCommit && !isHead && manifest.Input.ResolvedHead != "" && manifest.Input.ExactRange == "" && !inputResolutionFailed(manifest) {
		return report.Revision{Status: report.StatusNotApplicable, Reason: "目标提交没有父提交，原生审查从空树比较"}, nil // allow-non-english: report JSON requires Chinese scope facts
	}
	if manifest.TerminalState != session.StateFailed {
		return report.Revision{}, fmt.Errorf("%s is missing from a non-failed native manifest", name)
	}
	reason := "本次运行未能解析该提交引用" // allow-non-english: report JSON requires Chinese scope facts
	if manifest.RunFailure != nil && manifest.RunFailure.Reason != "" {
		reason = manifest.RunFailure.Reason
	}
	return report.Revision{Status: report.StatusFailed, Reason: reason}, nil
}

func notCollectedSection(reason string) report.Section {
	return report.Section{Status: report.StatusNotCollected, Reason: reason}
}

func inputResolutionFailed(manifest *session.RunManifest) bool {
	return manifest.RunFailure != nil && manifest.RunFailure.Classification == session.RunFailureInput
}

func materialFindings(manifest *session.RunManifest, repoName string, comments []model.LlmComment) ([]report.Finding, error) {
	comments = append([]model.LlmComment(nil), comments...)
	sort.SliceStable(comments, func(i, j int) bool {
		a, b := comments[i], comments[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		if a.EndLine != b.EndLine {
			return a.EndLine < b.EndLine
		}
		return strings.Join([]string{a.Severity, a.Category, a.Content, a.ExistingCode, a.SuggestionCode}, "\x00") <
			strings.Join([]string{b.Severity, b.Category, b.Content, b.ExistingCode, b.SuggestionCode}, "\x00")
	})
	findings := make([]report.Finding, 0, len(comments))
	identity := manifest.Repository.IdentitySHA256
	duplicates := make(map[string]int, len(comments))
	for _, comment := range comments {
		repoPath := strings.ReplaceAll(comment.Path, "\\", "/")
		repoPath = path.Clean(repoPath)
		if repoPath == "." {
			repoPath = ""
		}
		comment.Path = repoPath
		findingIdentity := report.FindingIdentity{
			RepositoryIdentity: identity,
			RepositoryName:     repoName,
			Mode:               manifest.Input.Mode,
			ResolvedHead:       manifest.Input.ResolvedHead,
			Path:               comment.Path,
			StartLine:          comment.StartLine,
			EndLine:            comment.EndLine,
			Severity:           comment.Severity,
			Category:           comment.Category,
			SourceContent:      comment.Content,
		}
		baseID := report.NewFindingID(findingIdentity)
		duplicates[baseID]++
		findingIdentity.DuplicateOrdinal = duplicates[baseID]
		severityZH, categoryZH := findingLabels(comment.Severity, comment.Category)
		if severityZH == "" || categoryZH == "" {
			return nil, fmt.Errorf("finding %s has an unsupported native severity or category", baseID)
		}
		position := "行号未解析" // allow-non-english: report JSON requires Chinese position text
		if comment.StartLine > 0 {
			position = fmt.Sprintf("第 %d 行", comment.StartLine) // allow-non-english: report JSON requires Chinese position text
			if comment.EndLine > comment.StartLine {
				position = fmt.Sprintf("第 %d-%d 行", comment.StartLine, comment.EndLine) // allow-non-english: report JSON requires Chinese position text
			}
		}
		evidenceStatus, evidenceCode, evidenceReason := materialCode(comment.ExistingCode, "原生审查结果未提供相关代码片段")                     // allow-non-english: report JSON requires Chinese limitation reasons
		recommendationStatus, recommendationCode, recommendationReason := materialCode(comment.SuggestionCode, "原生审查结果未提供修复代码建议") // allow-non-english: report JSON requires Chinese limitation reasons
		finding := report.Finding{
			ID:            report.NewFindingID(findingIdentity),
			Path:          comment.Path,
			StartLine:     comment.StartLine,
			EndLine:       comment.EndLine,
			Severity:      comment.Severity,
			Category:      comment.Category,
			SourceContent: comment.Content,
			Display: report.FindingDisplay{
				SummaryZH:  fmt.Sprintf("%s 的%s发现%s级%s问题。原始审查意见：%s", comment.Path, position, severityZH, categoryZH, comment.Content), // allow-non-english: report JSON requires Chinese finding display text
				SeverityZH: severityZH,
				CategoryZH: categoryZH,
			},
			Evidence:       report.FindingEvidence{Status: evidenceStatus, Code: evidenceCode, Reason: evidenceReason},
			Recommendation: report.FindingRecommendation{Status: recommendationStatus, Code: recommendationCode, Reason: recommendationReason},
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func materialCode(value, unavailableReason string) (report.Status, string, string) {
	if strings.TrimSpace(value) == "" {
		return report.StatusNotCollected, "", unavailableReason
	}
	return report.StatusProvided, value, ""
}

func findingLabels(severity, category string) (string, string) {
	severityLabels := map[string]string{"critical": "严重", "high": "高", "medium": "中", "low": "低"} // allow-non-english: report JSON requires Chinese labels
	categoryLabels := map[string]string{
		"bug": "缺陷", "security": "安全", "performance": "性能", "maintainability": "可维护性", // allow-non-english: report JSON requires Chinese labels
		"test": "测试", "style": "风格", "documentation": "文档", "other": "其他", // allow-non-english: report JSON requires Chinese labels
	}
	return severityLabels[severity], categoryLabels[category]
}
