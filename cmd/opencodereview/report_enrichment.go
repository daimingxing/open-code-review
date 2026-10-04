// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

type commitMaterial struct {
	SHA       string
	Parents   []string
	Subject   string
	Author    gitPerson
	Committer gitPerson
	Files     []commitFileChange
}

type gitPerson struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
}

type commitFileChange struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

type commitAchievements struct {
	CommitSHA               string             `json:"commit_sha"`
	Subject                 string             `json:"subject"`
	SummaryZH               string             `json:"summary_zh"`
	Files                   []commitFileChange `json:"files"`
	Modules                 []moduleChange     `json:"modules"`
	Evidence                []string           `json:"evidence"`
	ModelSummarySource      string             `json:"model_summary_source"`
	ModelSummaryStatus      string             `json:"model_summary_status"`
	ModelSummary            string             `json:"model_summary,omitempty"`
	ModelSummaryUsageStatus string             `json:"model_summary_usage_status"`
	ModelSummaryElapsedMS   int64              `json:"model_summary_elapsed_ms,omitempty"`
	ModelSummaryReasonZH    string             `json:"model_summary_reason_zh"`
	CollectionElapsedMS     int64              `json:"collection_elapsed_ms"`
	CollectionUsageStatus   string             `json:"collection_usage_status"`
	CollectionUsageReasonZH string             `json:"collection_usage_reason_zh"`
}

type moduleChange struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
}

type peopleMaterial struct {
	CommitSHA string    `json:"commit_sha"`
	Subject   string    `json:"subject"`
	Author    gitPerson `json:"author"`
	Committer gitPerson `json:"committer"`
	Basis     string    `json:"basis"`
}

type knowledgeMaterial struct {
	Status            string           `json:"status"`
	RuleConfigSHA256  string           `json:"rule_config_sha256,omitempty"`
	KnowledgeVersion  string           `json:"knowledge_version_status"`
	ApplicationStatus string           `json:"application_status"`
	ToolCalls         map[string]int64 `json:"tool_calls,omitempty"`
	FailedTools       []string         `json:"failed_tools,omitempty"`
	ReasonZH          string           `json:"reason_zh"`
}

func enrichReportSections(
	ctx context.Context,
	runner *gitcmd.Runner,
	repoDir string,
	scope report.Scope,
	manifest *session.RunManifest,
	toolCalls map[string]int64,
	toolFailures []llmloop.ToolFailureDetail,
	projectSummary string,
) (report.Section, report.Section, report.Section) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	achievements := notCollectedSection("本次审查尚未采集有依据的提交成果") // allow-non-english: report JSON requires Chinese missing-value reason
	people := notCollectedSection("本次审查尚未采集 Git 作者与提交者资料")  // allow-non-english: report JSON requires Chinese missing-value reason
	if scope.Mode == session.InputModeCommit && manifest != nil && scope.ResolvedHead.Status == report.StatusProvided {
		started := time.Now()
		material, err := collectCommitMaterial(ctx, runner, repoDir, scope.ResolvedBase.SHA, scope.ResolvedHead.SHA)
		if err != nil {
			failed := report.Section{Status: report.StatusFailed, Reason: "Git 提交事实采集失败，未推断成果或人员"} // allow-non-english: report JSON requires Chinese failure reason
			achievements, people = failed, failed
		} else {
			achievements = commitAchievementsSection(material, projectSummary, time.Since(started))
			people = peopleSection(material)
		}
	}
	knowledge := knowledgeSourcesSection(manifest, toolCalls, toolFailures)
	return achievements, people, knowledge
}

func collectCommitMaterial(ctx context.Context, runner *gitcmd.Runner, repoDir, baseSHA, sha string) (commitMaterial, error) {
	if runner == nil {
		return commitMaterial{}, fmt.Errorf("git runner is unavailable")
	}
	metadata, err := runner.Output(ctx, repoDir, "show", "-s", "--format=%H%x00%P%x00%an%x00%ae%x00%cn%x00%ce%x00%aI%x00%cI%x00%s", "--end-of-options", sha)
	if err != nil {
		return commitMaterial{}, err
	}
	fields := strings.Split(strings.TrimSuffix(string(metadata), "\n"), "\x00")
	if len(fields) != 9 {
		return commitMaterial{}, fmt.Errorf("Git commit metadata is incomplete")
	}
	authorAt, err := time.Parse(time.RFC3339, fields[6])
	if err != nil {
		return commitMaterial{}, err
	}
	committerAt, err := time.Parse(time.RFC3339, fields[7])
	if err != nil {
		return commitMaterial{}, err
	}
	var filesOutput []byte
	if baseSHA != "" {
		filesOutput, err = runner.Output(ctx, repoDir, "diff", "--name-status", "-z", "--no-renames", "--end-of-options", baseSHA, sha, "--")
	} else {
		filesOutput, err = runner.Output(ctx, repoDir, "diff-tree", "--root", "--no-commit-id", "--name-status", "-z", "--no-renames", "-r", "--end-of-options", sha)
	}
	if err != nil {
		return commitMaterial{}, err
	}
	files, err := parseCommitFiles(filesOutput)
	if err != nil {
		return commitMaterial{}, err
	}
	return commitMaterial{
		SHA: fields[0], Parents: strings.Fields(fields[1]), Subject: fields[8],
		Author:    gitPerson{Name: fields[2], Email: fields[3], Timestamp: authorAt},
		Committer: gitPerson{Name: fields[4], Email: fields[5], Timestamp: committerAt}, Files: files,
	}, nil
}

func parseCommitFiles(raw []byte) ([]commitFileChange, error) {
	parts := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	if len(parts) == 1 && parts[0] == "" {
		return []commitFileChange{}, nil
	}
	if len(parts)%2 != 0 {
		return nil, fmt.Errorf("Git file change records are incomplete")
	}
	files := make([]commitFileChange, 0, len(parts)/2)
	for i := 0; i < len(parts); i += 2 {
		if parts[i] == "" || parts[i+1] == "" {
			return nil, fmt.Errorf("Git file change record is empty")
		}
		files = append(files, commitFileChange{Status: parts[i], Path: filepath.ToSlash(parts[i+1])})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func commitAchievementsSection(material commitMaterial, projectSummary string, elapsed time.Duration) report.Section {
	modules := map[string][]string{}
	for _, file := range material.Files {
		path := filepath.ToSlash(file.Path)
		module := "."
		if strings.Contains(path, "/") {
			module = strings.Split(path, "/")[0]
		}
		if module == "" {
			module = "."
		}
		modules[module] = append(modules[module], path)
	}
	moduleChanges := make([]moduleChange, 0, len(modules))
	for name, files := range modules {
		sort.Strings(files)
		moduleChanges = append(moduleChanges, moduleChange{Name: name, Files: files})
	}
	sort.Slice(moduleChanges, func(i, j int) bool { return moduleChanges[i].Name < moduleChanges[j].Name })
	summarySource, summaryStatus, summaryReason, summaryText := "not_available", "not_provided", "当前审查路径未提供独立的模型成果归纳；材料仅保留 Git 提交事实，不据此推断成果。", "" // allow-non-english: report JSON requires Chinese missing-value reason
	if strings.TrimSpace(projectSummary) != "" {
		summarySource, summaryStatus, summaryReason, summaryText = "agent.project_summary", "provided", "成果归纳来自原生 Agent ProjectSummary；未额外重写其内容。", projectSummary // allow-non-english: report JSON requires Chinese source boundary
	}
	data, err := json.Marshal(commitAchievements{
		CommitSHA: material.SHA, Subject: material.Subject,
		SummaryZH: "该提交包含 " + strconv.Itoa(len(material.Files)) + " 个文件变更；以下内容仅依据 Git 提交事实。", // allow-non-english: report JSON requires Chinese user-facing summary
		Files:     material.Files, Modules: moduleChanges,
		Evidence:           []string{"git show commit metadata", "git diff-tree --name-status"},
		ModelSummarySource: summarySource, ModelSummaryStatus: summaryStatus,
		ModelSummary:            summaryText,
		ModelSummaryUsageStatus: "not_collected", ModelSummaryReasonZH: summaryReason,
		CollectionElapsedMS: elapsed.Milliseconds(), CollectionUsageStatus: "not_applicable",
		CollectionUsageReasonZH: "成果与人员材料由 Git 事实整理，未额外调用模型；模型用量不适用。", // allow-non-english: report JSON requires Chinese usage boundary
	})
	if err != nil {
		return report.Section{Status: report.StatusFailed, Reason: "提交成果事实无法编码"} // allow-non-english: report JSON requires Chinese failure reason
	}
	return report.Section{Status: report.StatusProvided, Data: data}
}

func peopleSection(material commitMaterial) report.Section {
	data, err := json.Marshal(peopleMaterial{CommitSHA: material.SHA, Subject: material.Subject, Author: material.Author, Committer: material.Committer, Basis: "Git commit metadata；不代表推送者、工时或缺陷责任"}) // allow-non-english: report JSON requires Chinese people-data boundary
	if err != nil {
		return report.Section{Status: report.StatusFailed, Reason: "Git 人员事实无法编码"} // allow-non-english: report JSON requires Chinese failure reason
	}
	return report.Section{Status: report.StatusProvided, Data: data}
}

func knowledgeSourcesSection(manifest *session.RunManifest, toolCalls map[string]int64, failures []llmloop.ToolFailureDetail) report.Section {
	if manifest == nil {
		return notCollectedSection("原生 manifest 不可用，未能确认规则或知识来源") // allow-non-english: report JSON requires Chinese missing-value reason
	}
	mcpCalls := make(map[string]int64)
	failedCounts := make(map[string]int64)
	for _, failure := range failures {
		if !isNativeReviewTool(failure.ToolName) {
			failedCounts[failure.ToolName]++
		}
	}
	for name, count := range toolCalls {
		if !isNativeReviewTool(name) {
			if successful := count - failedCounts[name]; successful > 0 {
				mcpCalls[name] = successful
			}
		}
	}
	failed := make([]string, 0)
	for _, failure := range failures {
		if !isNativeReviewTool(failure.ToolName) {
			failed = append(failed, failure.ToolName)
		}
	}
	sort.Strings(failed)
	if len(mcpCalls) == 0 && len(failed) == 0 {
		return notCollectedSection("本次运行未观察到外部知识工具读取；规则哈希不等于知识正文已读取") // allow-non-english: report JSON requires Chinese not-observed reason
	}
	status := "observed"
	versionStatus := "not_observed"
	applicationStatus := "not_observed"
	reason := "已观察到非原生工具调用；无法确认其是否读取了知识资料，且未记录正文版本或内容摘要，不能证明知识已正确应用" // allow-non-english: report JSON requires Chinese knowledge boundary
	sectionStatus := report.StatusProvided
	if len(failed) > 0 && len(mcpCalls) > 0 {
		status, reason, sectionStatus = "partial", "部分非原生工具调用失败；成功调用仅表示已观察到工具返回，不证明知识正文版本或应用结果", report.StatusFailed // allow-non-english: report JSON requires Chinese failure reason
	} else if len(failed) > 0 {
		status, reason, sectionStatus = "failed", "非原生工具调用全部失败，未能确认知识正文版本或应用结果", report.StatusFailed // allow-non-english: report JSON requires Chinese failure reason
	}
	data, err := json.Marshal(knowledgeMaterial{Status: status, RuleConfigSHA256: manifest.Execution.RuleConfigSHA256, KnowledgeVersion: versionStatus, ApplicationStatus: applicationStatus, ToolCalls: mcpCalls, FailedTools: failed, ReasonZH: reason})
	if err != nil {
		return report.Section{Status: report.StatusFailed, Reason: "知识来源事实无法编码"} // allow-non-english: report JSON requires Chinese failure reason
	}
	return report.Section{Status: sectionStatus, Reason: func() string {
		if sectionStatus == report.StatusFailed {
			return reason
		}
		return ""
	}(), Data: data}
}

func isNativeReviewTool(name string) bool {
	switch name {
	case "file_read", "file_find", "file_read_diff", "code_search", "code_comment":
		return true
	default:
		return false
	}
}
