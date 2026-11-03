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
	"github.com/alibaba/open-code-review/internal/llm"
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
	Diff      string
	Evidence  []string
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
	CommitSHA                string                 `json:"commit_sha"`
	Subject                  string                 `json:"subject"`
	SummaryZH                string                 `json:"summary_zh"`
	Files                    []commitFileChange     `json:"files"`
	Modules                  []moduleChange         `json:"modules"`
	Evidence                 []string               `json:"evidence"`
	ModelSummarySource       string                 `json:"model_summary_source"`
	ModelSummaryStatus       string                 `json:"model_summary_status"`
	ModelSummary             string                 `json:"model_summary,omitempty"`
	ModuleSummaries          []moduleSummary        `json:"module_summaries"`
	RepresentativeChanges    []representativeChange `json:"representative_changes"`
	ModelSummaryUsageStatus  string                 `json:"model_summary_usage_status"`
	ModelSummaryElapsedMS    int64                  `json:"model_summary_elapsed_ms,omitempty"`
	ModelSummaryInputTokens  int64                  `json:"model_summary_input_tokens,omitempty"`
	ModelSummaryOutputTokens int64                  `json:"model_summary_output_tokens,omitempty"`
	ModelSummaryReasonZH     string                 `json:"model_summary_reason_zh"`
	CollectionElapsedMS      int64                  `json:"collection_elapsed_ms"`
	CollectionUsageStatus    string                 `json:"collection_usage_status"`
	CollectionUsageReasonZH  string                 `json:"collection_usage_reason_zh"`
}

type moduleChange struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
}

type moduleSummary struct {
	Name      string `json:"name"`
	SummaryZH string `json:"summary_zh"`
}

type representativeChange struct {
	Path      string `json:"path"`
	SummaryZH string `json:"summary_zh"`
}

type peopleMaterial struct {
	CommitSHA string    `json:"commit_sha"`
	Subject   string    `json:"subject"`
	Author    gitPerson `json:"author"`
	Committer gitPerson `json:"committer"`
	Basis     string    `json:"basis"`
}

func enrichReportSections(
	ctx context.Context,
	runner *gitcmd.Runner,
	repoDir string,
	scope report.Scope,
	manifest *session.RunManifest,
	projectSummary string,
	summaryClient llm.LLMClient,
	modelName string,
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
		collectionElapsed := time.Since(started)
		if err != nil {
			failed := report.Section{Status: report.StatusFailed, Reason: "Git 提交事实采集失败，未推断成果或人员"} // allow-non-english: report JSON requires Chinese failure reason
			achievements, people = failed, failed
		} else {
			summary := summarizeCommitMaterial(ctx, summaryClient, modelName, material, projectSummary)
			achievements = commitAchievementsSection(material, projectSummary, collectionElapsed, summary)
			people = peopleSection(material)
		}
	}
	knowledge := knowledgeSourcesSection()
	return achievements, people, knowledge
}

func collectCommitMaterial(ctx context.Context, runner *gitcmd.Runner, repoDir, baseSHA, sha string) (commitMaterial, error) {
	if runner == nil {
		return commitMaterial{}, fmt.Errorf("git runner is unavailable")
	}
	metadataArgs := []string{"show", "-s", "--format=%H%x00%P%x00%an%x00%ae%x00%cn%x00%ce%x00%aI%x00%cI%x00%s", "--end-of-options", sha}
	metadata, err := runner.Output(ctx, repoDir, metadataArgs...)
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
	var filesArgs []string
	if baseSHA != "" {
		filesArgs = []string{"diff", "--name-status", "-z", "--no-renames", "--end-of-options", baseSHA, sha, "--"}
	} else {
		filesArgs = []string{"diff-tree", "--root", "--no-commit-id", "--name-status", "-z", "--no-renames", "-r", "--end-of-options", sha}
	}
	filesOutput, err = runner.Output(ctx, repoDir, filesArgs...)
	if err != nil {
		return commitMaterial{}, err
	}
	files, err := parseCommitFiles(filesOutput)
	if err != nil {
		return commitMaterial{}, err
	}
	diffArgs := []string{"diff", "--no-ext-diff", "--no-color", "--no-renames", "--unified=3", "--end-of-options", baseSHA, sha, "--"}
	if baseSHA == "" {
		diffArgs = []string{"show", "--root", "--format=", "--no-ext-diff", "--no-color", "--no-renames", "--unified=3", "--end-of-options", sha, "--"}
	}
	diffOutput, err := runner.Output(ctx, repoDir, diffArgs...)
	if err != nil {
		return commitMaterial{}, err
	}
	return commitMaterial{
		SHA: fields[0], Parents: strings.Fields(fields[1]), Subject: fields[8],
		Author:    gitPerson{Name: fields[2], Email: fields[3], Timestamp: authorAt},
		Committer: gitPerson{Name: fields[4], Email: fields[5], Timestamp: committerAt}, Files: files,
		Diff:     string(diffOutput),
		Evidence: []string{"git " + strings.Join(metadataArgs, " "), "git " + strings.Join(filesArgs, " "), "git " + strings.Join(diffArgs, " ")},
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

func commitAchievementsSection(material commitMaterial, projectSummary string, elapsed time.Duration, summary commitSummaryResult) report.Section {
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
	summarySource, summaryStatus, summaryReason, summaryText := "model.git_diff", summary.status, summary.reason, summary.text
	if summary.status == "not_applicable" {
		summarySource = "git.first_parent_diff"
	}
	sectionStatus := report.StatusProvided
	if summary.status != "not_applicable" && strings.TrimSpace(projectSummary) != "" {
		summarySource, summaryStatus, summaryReason, summaryText = "agent.project_summary", "provided", "成果归纳复用原生 Agent ProjectSummary；未额外改写。", projectSummary // allow-non-english: report JSON requires Chinese source boundary
		summary = commitSummaryResult{status: "provided", usageStatus: "not_applicable"}
	}
	if summary.status == "failed" || summary.status == "rejected" {
		sectionStatus = report.StatusFailed
	}
	data, err := json.Marshal(commitAchievements{
		CommitSHA: material.SHA, Subject: material.Subject,
		SummaryZH: "该提交包含 " + strconv.Itoa(len(material.Files)) + " 个文件变更；以下内容仅依据 Git 提交事实。", // allow-non-english: report JSON requires Chinese user-facing summary
		Files:     material.Files, Modules: moduleChanges,
		Evidence:           material.Evidence,
		ModelSummarySource: summarySource, ModelSummaryStatus: summaryStatus,
		ModelSummary: summaryText, ModuleSummaries: summary.modules, RepresentativeChanges: summary.changes,
		ModelSummaryUsageStatus: summary.usageStatus, ModelSummaryElapsedMS: summary.elapsed.Milliseconds(),
		ModelSummaryInputTokens: summary.inputTokens, ModelSummaryOutputTokens: summary.outputTokens, ModelSummaryReasonZH: summaryReason,
		CollectionElapsedMS: elapsed.Milliseconds(), CollectionUsageStatus: "not_applicable",
		CollectionUsageReasonZH: "Git 事实采集独立计时；模型成果归纳单独记录耗时与用量。", // allow-non-english: report JSON requires Chinese usage boundary
	})
	if err != nil {
		return report.Section{Status: report.StatusFailed, Reason: "提交成果事实无法编码"} // allow-non-english: report JSON requires Chinese failure reason
	}
	section := report.Section{Status: sectionStatus, Data: data}
	if sectionStatus == report.StatusFailed {
		section.Reason = summary.reason
	}
	return section
}

const maxCommitSummaryDiffBytes = 96 * 1024

type commitSummaryResult struct {
	status       string
	usageStatus  string
	text         string
	reason       string
	modules      []moduleSummary
	changes      []representativeChange
	inputTokens  int64
	outputTokens int64
	elapsed      time.Duration
}

type commitSummaryResponse struct {
	SummaryZH             string                 `json:"summary_zh"`
	Modules               []moduleSummary        `json:"modules"`
	RepresentativeChanges []representativeChange `json:"representative_changes"`
}

func summarizeCommitMaterial(ctx context.Context, client llm.LLMClient, modelName string, material commitMaterial, projectSummary string) commitSummaryResult {
	if len(material.Files) == 0 {
		return commitSummaryResult{
			status: "not_applicable", usageStatus: "not_applicable",
			text:   "该提交相对第一父提交没有文件差异，因此没有可归纳的提交成果。", // allow-non-english: user-facing Chinese no-change summary
			reason: "零文件差异直接依据 Git 事实记录，未调用模型。",      // allow-non-english: user-facing Chinese evidence boundary
		}
	}
	if strings.TrimSpace(projectSummary) != "" {
		return commitSummaryResult{status: "provided", usageStatus: "not_applicable"}
	}
	if client == nil {
		return commitSummaryResult{status: "failed", usageStatus: "not_available", reason: "报告成果归纳模型不可用，保留 Git 提交事实"} // allow-non-english: report JSON requires Chinese failure reason
	}
	started := time.Now()
	callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	patch := material.Diff
	diffTruncated := len(patch) > maxCommitSummaryDiffBytes
	if diffTruncated {
		patch = patch[:maxCommitSummaryDiffBytes]
		patch = strings.ToValidUTF8(patch, "")
	}
	input, err := json.Marshal(map[string]any{
		"commit_sha":        material.SHA,
		"subject":           material.Subject,
		"files":             material.Files,
		"modules":           moduleNames(material.Files),
		"diff_first_parent": patch,
		"diff_truncated":    diffTruncated,
	})
	if err != nil {
		return commitSummaryResult{status: "failed", usageStatus: "not_available", elapsed: time.Since(started), reason: "Git 事实无法编码为成果归纳输入"} // allow-non-english: report JSON requires Chinese failure reason
	}
	resp, err := client.CompletionsWithCtx(callCtx, llm.ChatRequest{
		Model: modelName,
		Messages: []llm.Message{
			llm.NewTextMessage("system", "OCR_REPORT_ENRICHMENT: Summarize only this commit's first-parent Git evidence. The input contains no issue, request, finding, or business-goal context. Do not invent goals, outcomes, affected systems, or files. Preserve technical identifiers. Return one JSON object with summary_zh, modules [{name, summary_zh}], and representative_changes [{path, summary_zh}]. Use only module names and paths present in the input. If evidence is insufficient, say so in Chinese."),
			llm.NewTextMessage("user", string(input)),
		},
		MaxTokens: 900,
	})
	result := commitSummaryResult{status: "failed", usageStatus: "not_available", elapsed: time.Since(started), reason: "模型成果归纳失败，保留 Git 提交事实"} // allow-non-english: report JSON requires Chinese failure reason
	if err != nil {
		return result
	}
	result.elapsed = time.Since(started)
	if resp.Usage != nil {
		result.usageStatus = "available"
		result.inputTokens = resp.Usage.PromptTokens
		result.outputTokens = resp.Usage.CompletionTokens
	} else {
		result.usageStatus = "not_reported"
	}
	var decoded commitSummaryResponse
	if err := json.Unmarshal([]byte(resp.Content()), &decoded); err != nil || !validCommitSummary(decoded, material.Files) {
		result.status = "rejected"
		result.reason = "模型成果归纳引用缺少 Git 证据或格式无效，已丢弃归纳并保留提交事实" // allow-non-english: report JSON requires Chinese rejection reason
		return result
	}
	result.status = "provided"
	result.reason = ""
	result.text = decoded.SummaryZH
	result.modules = decoded.Modules
	result.changes = decoded.RepresentativeChanges
	if diffTruncated {
		result.reason = "模型归纳仅使用有大小上限的差异前缀；请勿视为完整变更摘要" // allow-non-english: report JSON requires Chinese evidence limitation
	}
	return result
}

func moduleNames(files []commitFileChange) []string {
	modules := make(map[string]struct{})
	for _, file := range files {
		path := filepath.ToSlash(file.Path)
		module := "."
		if strings.Contains(path, "/") {
			module = strings.Split(path, "/")[0]
		}
		modules[module] = struct{}{}
	}
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validCommitSummary(summary commitSummaryResponse, files []commitFileChange) bool {
	if strings.TrimSpace(summary.SummaryZH) == "" || len(summary.Modules) == 0 || len(summary.RepresentativeChanges) == 0 {
		return false
	}
	modules := make(map[string]struct{})
	paths := make(map[string]struct{}, len(files))
	for _, name := range moduleNames(files) {
		modules[name] = struct{}{}
	}
	for _, file := range files {
		paths[filepath.ToSlash(file.Path)] = struct{}{}
	}
	for _, module := range summary.Modules {
		if _, ok := modules[module.Name]; !ok || strings.TrimSpace(module.SummaryZH) == "" {
			return false
		}
	}
	for _, change := range summary.RepresentativeChanges {
		if _, ok := paths[filepath.ToSlash(change.Path)]; !ok || strings.TrimSpace(change.SummaryZH) == "" {
			return false
		}
	}
	return true
}

func peopleSection(material commitMaterial) report.Section {
	data, err := json.Marshal(peopleMaterial{CommitSHA: material.SHA, Subject: material.Subject, Author: material.Author, Committer: material.Committer, Basis: "Git commit metadata；不代表推送者、工时或缺陷责任"}) // allow-non-english: report JSON requires Chinese people-data boundary
	if err != nil {
		return report.Section{Status: report.StatusFailed, Reason: "Git 人员事实无法编码"} // allow-non-english: report JSON requires Chinese failure reason
	}
	return report.Section{Status: report.StatusProvided, Data: data}
}

func knowledgeSourcesSection() report.Section {
	return notCollectedSection("未观察到知识来源：未配置原生知识库，或调用方未提供知识访问事实") // allow-non-english: report explains absent knowledge evidence
}
