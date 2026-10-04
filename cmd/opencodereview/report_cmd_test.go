// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestReportCommandRendersExistingMaterialAndRejectsAlteredModelFacts(t *testing.T) {
	setTestHome(t, t.TempDir())
	responses := make(chan string, 4)
	requests := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- request
		content := <-responses
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "report-test", "model": "fake-report-model",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": content},
			}},
			"usage": map[string]any{"prompt_tokens": 123, "completion_tokens": 456, "total_tokens": 579},
		})
	}))
	defer server.Close()
	t.Setenv("OCR_LLM_URL", server.URL+"/v1")
	t.Setenv("OCR_LLM_TOKEN", "report-test-token")
	t.Setenv("OCR_LLM_MODEL", "fake-report-model")
	t.Setenv("OCR_LLM_PROTOCOL", "openai")

	material := reportTestLongMaterial()
	input := writeReportInput(t, material)
	out := filepath.Join(t.TempDir(), "four-risks.html")
	responses <- reportHTMLFixture(material)
	stdout, stderr, err := executeReportCommand([]string{"--input", input, "--output", out})
	if err != nil {
		t.Fatalf("report command failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, out) || !strings.Contains(stderr, "input_tokens=123 output_tokens=456 usage=reported") {
		t.Fatalf("command diagnostics are incomplete: stdout=%q stderr=%q", stdout, stderr)
	}
	request := <-requests
	if _, ok := request["tools"]; ok {
		t.Fatal("report request must not expose tools")
	}
	messages, ok := request["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("report request messages = %#v, want only template and report material", request["messages"])
	}
	templateMessage, ok := messages[0].(map[string]any)
	if !ok || templateMessage["role"] != "system" || !strings.Contains(fmt.Sprint(templateMessage["content"]), "data-fact") {
		t.Fatalf("report request is missing the template's fact-binding instructions: %#v", messages[0])
	}
	materialJSON, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	userMessage, ok := messages[1].(map[string]any)
	if !ok || userMessage["role"] != "user" || userMessage["content"] != string(materialJSON) {
		t.Fatalf("model did not receive the existing report material as its only user input: %#v", messages[1])
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if evidencePath := os.Getenv("OCR_REPORT_HTML_EVIDENCE_FILE"); evidencePath != "" {
		if err := os.WriteFile(evidencePath, data, 0o600); err != nil {
			t.Fatalf("write browser evidence HTML: %v", err)
		}
	}
	if err := report.ValidateHTMLDocument(string(data), material); err != nil {
		t.Fatalf("saved HTML failed fact and safety validation: %v", err)
	}
	for _, severity := range []string{"critical", "high", "medium", "low"} {
		if !strings.Contains(string(data), `data-severity="`+severity+`"`) {
			t.Errorf("saved HTML omitted %s findings", severity)
		}
	}
	if !strings.Contains(string(data), "not collected") || !strings.Contains(string(data), `return secret &lt;&amp; token`) {
		t.Fatal("partial state, knowledge gap, or safely escaped evidence was omitted")
	}

	zeroMaterial := reportTestMaterial(false)
	zeroInput := writeReportInput(t, zeroMaterial)
	zeroOutput := filepath.Join(t.TempDir(), "zero.html")
	responses <- reportHTMLFixture(zeroMaterial)
	if _, stderr, err := executeReportCommand([]string{"--input", zeroInput, "--output", zeroOutput}); err != nil {
		t.Fatalf("zero-finding report failed: %v\nstderr: %s", err, stderr)
	}
	<-requests
	zeroHTML, err := os.ReadFile(zeroOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(zeroHTML), `data-stat="finding-count">0`) {
		t.Fatal("zero-finding report did not show a computed zero")
	}

	badOutput := filepath.Join(t.TempDir(), "altered.html")
	criticalCount := 0
	for _, finding := range material.Findings {
		if finding.Severity == "critical" {
			criticalCount++
		}
	}
	responses <- strings.Replace(reportHTMLFixture(material), fmt.Sprintf(`data-stat="risk-critical">%d`, criticalCount), fmt.Sprintf(`data-stat="risk-critical">%d`, criticalCount+1), 1)
	if _, stderr, err := executeReportCommand([]string{"--input", input, "--output", badOutput}); err == nil || !strings.Contains(stderr+err.Error(), "statistic") {
		t.Fatalf("altered model statistics were accepted: err=%v stderr=%s", err, stderr)
	}
	<-requests
	if _, err := os.Stat(badOutput); !os.IsNotExist(err) {
		t.Fatalf("invalid model HTML left a final file: %v", err)
	}
	badPeopleOutput := filepath.Join(t.TempDir(), "invented-people.html")
	peopleSection := `<section data-section="people">`
	forgedPeople := strings.Replace(reportHTMLFixture(material), `</section><section data-section="governance"`, `<p>Avery made 42 commits.</p></section><section data-section="governance"`, 1)
	if !strings.Contains(forgedPeople, peopleSection) {
		t.Fatal("fixture is missing the people section")
	}
	responses <- forgedPeople
	_, badPeopleStderr, err := executeReportCommand([]string{"--input", input, "--output", badPeopleOutput})
	if err == nil || !strings.Contains(badPeopleStderr+err.Error(), "unverified narrative") {
		t.Fatalf("invented people details were accepted: err=%v stderr=%s", err, badPeopleStderr)
	}
	<-requests
	if _, err := os.Stat(badPeopleOutput); !os.IsNotExist(err) {
		t.Fatalf("invented facts left a final HTML file: %v", err)
	}
}

func TestReportCommandRejectsInvalidInputAndTemplateBeforeModelCall(t *testing.T) {
	cmd := newReportCommand()
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly once") {
		t.Fatalf("missing input error = %v", err)
	}
	input := filepath.Join(t.TempDir(), "native.json")
	if err := os.WriteFile(input, []byte(`{"comments":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd = newReportCommand()
	cmd.SetArgs([]string{"--input", input, "--template", "unknown"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown report template") {
		t.Fatalf("unknown template error = %v", err)
	}
	cmd = newReportCommand()
	cmd.SetArgs([]string{"--input", input})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("native JSON input error = %v", err)
	}
}

func TestReportCommandUsesDateDefaultOutputWithoutOverwriting(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeReportCompletion(w, reportHTMLFixture(material), "stop")
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)

	dir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	cmd := newReportCommand()
	cmd.SetArgs(nil)
	cmd.SetContext(context.Background())
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	fixedDate := time.Date(2026, 10, 4, 10, 11, 12, 0, time.Local)
	if err := runHTMLReport(cmd, reportOptions{inputs: []string{input}, template: report.DefaultHTMLTemplate, generatedAt: func() time.Time { return fixedDate }}); err != nil {
		t.Fatalf("default output failed: %v\nstderr: %s", err, stderr.String())
	}
	defaultPath := filepath.Join(dir, "report-2026-10-04.html")
	if _, err := os.Stat(defaultPath); err != nil {
		t.Fatalf("default date output missing: %v", err)
	}
	if err := runHTMLReport(cmd, reportOptions{inputs: []string{input}, template: report.DefaultHTMLTemplate, generatedAt: func() time.Time { return fixedDate }}); err != nil {
		t.Fatalf("automatic numbered output failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "report-2026-10-04(1).html")); err != nil {
		t.Fatalf("automatic output did not avoid overwrite: %v", err)
	}
}

func TestReportCommandBoundsInputBeforeModelRequest(t *testing.T) {
	setTestHome(t, t.TempDir())
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeReportCompletion(w, "", "stop")
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)

	material := reportTestMaterial(false)
	material.Repository.Name = strings.Repeat("repository-name ", 35_000)
	input := writeReportInput(t, material)
	_, stderr, err := executeReportCommand([]string{"--input", input, "--output", filepath.Join(t.TempDir(), "must-not-exist.html")})
	if err == nil || !strings.Contains(err.Error(), "tokens") {
		t.Fatalf("oversized model input was accepted: err=%v stderr=%s", err, stderr)
	}
	if requests.Load() != 0 {
		t.Fatalf("input over the token budget reached the model %d times", requests.Load())
	}
}

func TestReportCommandRejectsTimeoutAndTruncatedModelOutput(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)

	t.Run("timeout", func(t *testing.T) {
		started := make(chan struct{}, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			started <- struct{}{}
			time.Sleep(1500 * time.Millisecond)
			writeReportCompletion(w, reportHTMLFixture(material), "stop")
		}))
		defer server.Close()
		configureReportLLM(t, server.URL)
		t.Setenv("OCR_LLM_TIMEOUT", "1")
		out := filepath.Join(t.TempDir(), "timeout.html")
		_, stderr, err := executeReportCommand([]string{"--input", input, "--output", out})
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout") && !strings.Contains(strings.ToLower(err.Error()), "deadline") {
			t.Fatalf("timed-out request was accepted: err=%v stderr=%s", err, stderr)
		}
		select {
		case <-started:
		default:
			t.Fatal("timeout test never reached the model endpoint")
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("timeout left a final HTML file: %v", err)
		}
		if !strings.Contains(stderr, "elapsed=") || !strings.Contains(stderr, "usage=estimated") {
			t.Fatalf("timeout diagnostics missing: %s", stderr)
		}
	})

	t.Run("truncated output", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeReportCompletion(w, reportHTMLFixture(material), "length")
		}))
		defer server.Close()
		configureReportLLM(t, server.URL)
		out := filepath.Join(t.TempDir(), "truncated.html")
		_, stderr, err := executeReportCommand([]string{"--input", input, "--output", out})
		if err == nil || !strings.Contains(err.Error(), "token limit") {
			t.Fatalf("truncated model output was accepted: err=%v stderr=%s", err, stderr)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("truncated output left a final HTML file: %v", err)
		}
	})
}

func executeReportCommand(args []string) (string, string, error) {
	cmd := newReportCommand()
	cmd.SetArgs(args)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func configureReportLLM(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("OCR_LLM_URL", baseURL+"/v1")
	t.Setenv("OCR_LLM_TOKEN", "report-test-token")
	t.Setenv("OCR_LLM_MODEL", "fake-report-model")
	t.Setenv("OCR_LLM_PROTOCOL", "openai")
}

func writeReportCompletion(w http.ResponseWriter, content, finishReason string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "report-test", "model": "fake-report-model",
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": finishReason,
			"message": map[string]any{"role": "assistant", "content": content},
		}},
		"usage": map[string]any{"prompt_tokens": 123, "completion_tokens": 456, "total_tokens": 579},
	})
}

func reportTestMaterial(withFindings bool) report.Material {
	now := time.Date(2026, 10, 4, 10, 11, 12, 0, time.FixedZone("HKT", 8*60*60))
	material := report.Material{
		SchemaVersion: report.MaterialSchemaVersion,
		Review: report.ReviewMetadata{
			RunID: "report-run-07", StartedAt: now, CompletedAt: now.Add(time.Second),
			ElapsedMS: 1000, Status: session.StatePartial, Provider: "test", Model: "review-model",
		},
		Repository: report.Repository{Name: "sample", Identity: report.Fact{Status: report.StatusNotCollected, Reason: "repository identity not collected"}},
		Scope: report.Scope{
			Mode: session.InputModeCommit, RequestedHead: "HEAD",
			ResolvedHead:   report.Revision{Status: report.StatusProvided, SHA: strings.Repeat("a", 40)},
			ResolvedBase:   report.Revision{Status: report.StatusProvided, SHA: strings.Repeat("b", 40)},
			ExactRange:     strings.Repeat("b", 40) + ".." + strings.Repeat("a", 40),
			SourceArtifact: report.Fact{Status: report.StatusNotApplicable, Reason: "commit input has no source artifact"},
		},
		Findings: []report.Finding{},
		Coverage: session.Coverage{
			Selected: []session.CoverageItem{}, Completed: []session.CoverageItem{}, Reused: []session.CoverageItem{},
			Failed: []session.CoverageItem{}, Waived: []session.CoverageItem{},
		},
		Sections: report.MaterialSections{
			GitStatistics:     report.Section{Status: report.StatusNotCollected, Reason: "git statistics not collected"},
			WorkspaceSnapshot: report.Section{Status: report.StatusNotApplicable, Reason: "not a workspace review"},
			Achievements:      report.Section{Status: report.StatusNotCollected, Reason: "achievements not collected"},
			People:            report.Section{Status: report.StatusNotCollected, Reason: "people not collected"},
			KnowledgeSources:  report.Section{Status: report.StatusNotCollected, Reason: "knowledge sources not collected"},
			StructuralChecks:  report.Section{Status: report.StatusNotCollected, Reason: "structural checks not collected"},
		},
		Limitations: []report.Limitation{{Source: "knowledge_sources", Status: report.StatusNotCollected, Reason: "knowledge sources not collected"}},
	}
	if !withFindings {
		material.Review.Status = session.StateComplete
		material.Limitations = []report.Limitation{}
		return material
	}
	itemID := strings.Repeat("c", 64)
	material.Coverage.Selected = []session.CoverageItem{{ItemID: itemID, Path: "src/a.go"}}
	material.Coverage.Failed = []session.CoverageItem{{ItemID: itemID, Path: "src/a.go", Classification: session.FailureProvider, Reason: "model request failed"}}
	for index, severity := range []string{"critical", "high", "medium", "low"} {
		labels := map[string]string{"critical": "\u4e25\u91cd", "high": "\u9ad8", "medium": "\u4e2d", "low": "\u4f4e"}
		finding := report.Finding{
			ID: "sha256:" + strings.Repeat(string(rune('1'+index)), 64), Path: fmt.Sprintf("src/%c.go", 'a'+index),
			StartLine: index + 1, EndLine: index + 1, Severity: severity, SeverityStatus: report.StatusProvided,
			Category: "bug", CategoryStatus: report.StatusProvided, SourceContent: "return secret <& token",
			Display:        report.FindingDisplay{SummaryZH: "\u4e0d\u7a33\u5b9a\u7ed3\u679c", SeverityZH: labels[severity], CategoryZH: "\u7f3a\u9677"},
			Evidence:       report.FindingEvidence{Status: report.StatusProvided, Code: "return secret <& token"},
			Recommendation: report.FindingRecommendation{Status: report.StatusProvided, Code: "validate the value"},
		}
		if index > 0 {
			finding.Evidence = report.FindingEvidence{Status: report.StatusNotCollected, Reason: "independent evidence not collected"}
			finding.Recommendation = report.FindingRecommendation{Status: report.StatusNotCollected, Reason: "recommendation not collected"}
		}
		material.Findings = append(material.Findings, finding)
	}
	return material
}

func reportTestLongMaterial() report.Material {
	material := reportTestMaterial(true)
	base := append([]report.Finding(nil), material.Findings...)
	for index := len(base); index < 60; index++ {
		finding := base[index%len(base)]
		finding.ID = "sha256:" + fmt.Sprintf("%064x", index+100)
		finding.Path = fmt.Sprintf("src/long-%02d.go", index)
		finding.StartLine = index + 1
		finding.EndLine = index + 2
		finding.SourceContent = strings.Repeat(fmt.Sprintf("line %d: return secret <& token. ", index), 8)
		material.Findings = append(material.Findings, finding)
	}
	return material
}

func writeReportInput(t *testing.T, material report.Material) string {
	t.Helper()
	data, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "input.report.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func reportHTMLFixture(material report.Material) string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>审查报告</title></head><body>`) // allow-non-english: fixture is the model's Chinese HTML response
	fmt.Fprintf(&builder, `<main data-review-status="%s" data-run-id="%s">`, html.EscapeString(string(material.Review.Status)), html.EscapeString(material.Review.RunID))
	sections := []struct{ id, title string }{
		{"overview", "\u62a5\u544a\u6982\u89c8"}, {"quality-coverage", "\u8d28\u91cf\u4e0e\u8986\u76d6"},
		{"finding-details", "\u95ee\u9898\u660e\u7ec6"}, {"changes", "\u4ed3\u5e93\u53d8\u66f4"},
		{"achievements", "\u5de5\u4f5c\u6210\u679c"}, {"people", "\u4eba\u5458\u660e\u7ec6"},
		{"governance", "\u9879\u76ee\u7ed3\u6784\u68c0\u67e5"}, {"limitations", "\u9650\u5236\u4e0e\u672a\u786e\u8ba4\u4e8b\u9879"},
		{"sources", "\u6750\u6599\u6765\u6e90"},
	}
	facts := reportTestMaterialFacts(material)
	for _, section := range sections {
		fmt.Fprintf(&builder, `<section data-section="%s"><h2>%s</h2>`, section.id, section.title)
		appendReportMaterialFacts(&builder, facts, section.id)
		if section.id == "quality-coverage" {
			appendReportStatistics(&builder, material)
		}
		if section.id == "finding-details" {
			for _, finding := range material.Findings {
				appendReportFinding(&builder, finding)
			}
		}
		builder.WriteString(`</section>`)
	}
	builder.WriteString(`</main></body></html>`)
	return builder.String()
}

func reportTestMaterialFacts(material report.Material) map[string]string {
	data, err := json.Marshal(material)
	if err != nil {
		panic(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		panic(err)
	}
	facts := make(map[string]string)
	var collect func(string, any)
	collect = func(path string, value any) {
		switch current := value.(type) {
		case map[string]any:
			for key, child := range current {
				if path == "" && key == "findings" {
					continue
				}
				name := key
				if path != "" {
					name = path + "." + key
				}
				collect(name, child)
			}
		case []any:
			for index, child := range current {
				collect(fmt.Sprintf("%s[%d]", path, index), child)
			}
		case string:
			facts[path] = current
		case json.Number:
			facts[path] = current.String()
		case bool:
			facts[path] = fmt.Sprint(current)
		}
	}
	collect("", root)
	return facts
}

func appendReportMaterialFacts(builder *strings.Builder, facts map[string]string, section string) {
	keys := make([]string, 0, len(facts))
	for key := range facts {
		if reportTestFactAllowedInSection(section, key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := reportTestFactDisplay(key, facts[key])
		fmt.Fprintf(builder, `<div class="fact-row"><strong class="fact-label">%s</strong><span data-fact="%s">%s</span></div>`, reportTestFactLabel(key), html.EscapeString(key), html.EscapeString(value))
	}
}

func reportTestFactLabel(key string) string {
	labels := map[string]string{
		"repository.name": "仓库", "repository.identity.status": "仓库标识", "review.status": "审查状态", "scope.mode": "审查范围",
		"review.started_at": "开始时间", "review.completed_at": "结束时间", "review.elapsed_ms": "耗时", "review.provider": "提供方", "review.model": "模型",
		"scope.requested_from": "基准提交", "scope.requested_head": "目标提交", "scope.exact_range": "实际范围", "scope.source_artifact.status": "来源",
		"sections.structural_checks.status": "项目结构检查", "schema_version": "材料版本", "review.run_id": "运行标识",
	}
	if label, ok := labels[key]; ok {
		return label
	}
	if strings.HasSuffix(key, ".status") {
		return "状态"
	}
	if strings.Contains(key, "reason") {
		return "原因"
	}
	if strings.HasPrefix(key, "coverage.") {
		return "审查覆盖"
	}
	if strings.HasPrefix(key, "sections.people.") {
		return "人员"
	}
	if strings.HasPrefix(key, "sections.achievements.") {
		return "成果"
	}
	if strings.HasPrefix(key, "sections.knowledge_sources.") {
		return "知识来源"
	}
	if strings.HasPrefix(key, "limitations[") {
		return "限制"
	}
	return "材料事实"
}

func reportTestFactAllowedInSection(section, key string) bool {
	prefixes := map[string][]string{
		"overview":         {"review.", "repository.", "scope.", "run_failure."},
		"quality-coverage": {"coverage."},
		"changes":          {"sections.git_statistics.", "sections.workspace_snapshot."},
		"achievements":     {"sections.achievements."},
		"people":           {"sections.people."},
		"governance":       {"sections.structural_checks."},
		"limitations":      {"limitations["},
		"sources":          {"schema_version", "review.run_id", "sections.knowledge_sources."},
	}
	for _, prefix := range prefixes[section] {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func reportTestFactDisplay(key, value string) string {
	if !strings.HasSuffix(key, ".status") {
		return value
	}
	labels := map[string]string{
		"provided": "\u5df2\u63d0\u4f9b", "not_collected": "\u672a\u63d0\u4f9b",
		"not_applicable": "\u4e0d\u9002\u7528", "failed": "\u5931\u8d25",
		"complete": "\u5df2\u5b8c\u6210", "partial": "\u90e8\u5206\u5b8c\u6210", "skipped": "\u5df2\u8df3\u8fc7",
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}

func appendReportStatistics(builder *strings.Builder, material report.Material) {
	stats := []struct {
		name  string
		count int
	}{
		{"finding-count", len(material.Findings)},
		{"risk-critical", 0}, {"risk-high", 0}, {"risk-medium", 0}, {"risk-low", 0},
		{"coverage-selected", len(material.Coverage.Selected)}, {"coverage-completed", len(material.Coverage.Completed)},
		{"coverage-failed", len(material.Coverage.Failed)}, {"coverage-skipped", len(material.Coverage.Waived)},
		{"coverage-reused", len(material.Coverage.Reused)},
	}
	for _, finding := range material.Findings {
		for index, severity := range []string{"critical", "high", "medium", "low"} {
			if finding.Severity == severity {
				stats[index+1].count++
			}
		}
	}
	for _, stat := range stats {
		labels := map[string]string{"finding-count": "问题数量", "risk-critical": "严重", "risk-high": "高", "risk-medium": "中", "risk-low": "低", "coverage-selected": "选中", "coverage-completed": "完成", "coverage-failed": "失败", "coverage-skipped": "跳过", "coverage-reused": "复用"}
		fmt.Fprintf(builder, `<div class="report-statistic"><strong class="fact-label">%s</strong><output data-stat="%s">%d</output></div>`, labels[stat.name], stat.name, stat.count)
	}
}

func appendReportFinding(builder *strings.Builder, finding report.Finding) {
	fmt.Fprintf(builder, `<article data-finding-id="%s" data-severity="%s" data-category="%s" data-path="%s" data-start-line="%d" data-end-line="%d">`, html.EscapeString(finding.ID), html.EscapeString(finding.Severity), html.EscapeString(finding.Category), html.EscapeString(finding.Path), finding.StartLine, finding.EndLine)
	facts := []struct{ name, value string }{
		{"summary_zh", finding.Display.SummaryZH}, {"severity_zh", finding.Display.SeverityZH},
		{"category_zh", finding.Display.CategoryZH}, {"source_content", finding.SourceContent},
		{"evidence_status", string(finding.Evidence.Status)}, {"recommendation_status", string(finding.Recommendation.Status)},
	}
	if finding.Evidence.Status == report.StatusProvided {
		facts = append(facts, struct{ name, value string }{"evidence_code", finding.Evidence.Code})
	} else {
		facts = append(facts, struct{ name, value string }{"evidence_reason", finding.Evidence.Reason})
	}
	if finding.Recommendation.Status == report.StatusProvided {
		facts = append(facts, struct{ name, value string }{"recommendation_code", finding.Recommendation.Code})
	} else {
		facts = append(facts, struct{ name, value string }{"recommendation_reason", finding.Recommendation.Reason})
	}
	for _, fact := range facts {
		fmt.Fprintf(builder, `<span data-fact="%s">%s</span>`, fact.name, html.EscapeString(fact.value))
	}
	builder.WriteString(`</article>`)
}
