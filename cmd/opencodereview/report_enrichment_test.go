// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReviewE2E_CommitReportContainsEvidenceBackedEnrichment(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	startFakeLLM(t, newFakeLLM())
	reportPath := filepath.Join(t.TempDir(), "enriched.json")
	stdout, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("commit review failed: %v\nstderr: %s", err, stderr)
	}
	var native jsonOutput
	if err := json.Unmarshal([]byte(stdout), &native); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var material report.Material
	if err := json.Unmarshal(data, &material); err != nil {
		t.Fatal(err)
	}
	if err := report.ValidateMaterial(material); err != nil {
		t.Fatalf("validate material: %v", err)
	}
	if material.Sections.Achievements.Status != report.StatusProvided || material.Sections.People.Status != report.StatusProvided {
		t.Fatalf("enrichment status = achievements:%q people:%q", material.Sections.Achievements.Status, material.Sections.People.Status)
	}
	if material.Sections.GitStatistics.Status != report.StatusProvided {
		t.Fatalf("single-commit Git statistics status = %q", material.Sections.GitStatistics.Status)
	}
	var achievements commitAchievements
	if err := json.Unmarshal(material.Sections.Achievements.Data, &achievements); err != nil {
		t.Fatal(err)
	}
	if achievements.CommitSHA != material.Scope.ResolvedHead.SHA || len(achievements.Files) == 0 || len(achievements.Modules) == 0 || achievements.SummaryZH == "" {
		t.Fatalf("achievements lack commit/file/module evidence: %+v", achievements)
	}
	if achievements.CollectionElapsedMS < 0 || achievements.CollectionUsageStatus != "not_applicable" || achievements.ModelSummaryStatus != "provided" || achievements.ModelSummaryUsageStatus != "available" || achievements.ModelSummaryInputTokens == 0 || achievements.ModelSummaryOutputTokens == 0 {
		t.Fatalf("material collection time/usage states = %+v", achievements)
	}
	var people peopleMaterial
	if err := json.Unmarshal(material.Sections.People.Data, &people); err != nil {
		t.Fatal(err)
	}
	if people.Author.Name == "" || people.Committer.Name == "" || people.Basis == "" {
		t.Fatalf("author and committer facts were not preserved: %+v", people)
	}
	if material.Sections.KnowledgeSources.Status != report.StatusNotCollected || !strings.Contains(material.Sections.KnowledgeSources.Reason, "未观察到") { // allow-non-english: asserts Chinese not-observed state
		t.Fatalf("knowledge not-observed state = %+v", material.Sections.KnowledgeSources)
	}
	if native.Manifest == nil || material.Review.RunID != native.Manifest.RunID {
		t.Fatalf("material/native run identity diverged")
	}
}

func TestReviewE2E_CommitWithoutReportKeepsNativeBehavior(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	srv := newFakeLLM()
	startFakeLLM(t, srv)
	stdout, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json")
	if err != nil {
		t.Fatalf("native commit review failed: %v\nstderr: %s", err, stderr)
	}
	var native jsonOutput
	if err := json.Unmarshal([]byte(stdout), &native); err != nil || native.Manifest == nil || native.Manifest.Input.Mode != session.InputModeCommit {
		t.Fatalf("native commit output changed: decode=%v manifest=%+v", err, native.Manifest)
	}
	paths, err := filepath.Glob(filepath.Join(repoDir, "*.report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("review without --report unexpectedly created material: %v", paths)
	}
	if got := srv.summaryCallCount(); got != 0 {
		t.Fatalf("native review made %d report summary calls", got)
	}
}

func TestReviewE2E_ReportRecordsActualMCPContentVersionChanges(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	srv := newFakeLLM()
	srv.summaryDelay = 2 * time.Second
	startFakeLLM(t, srv)
	knowledgePath := filepath.Join(t.TempDir(), "knowledge.md")
	if err := os.WriteFile(knowledgePath, []byte("OLD_KNOWLEDGE"), 0o600); err != nil {
		t.Fatal(err)
	}
	callLog := configureReportFilesystemMCP(t, filepath.Dir(knowledgePath), true)
	srv.setKnowledgePath(knowledgePath, false)
	reportPath := filepath.Join(t.TempDir(), "enriched.json")
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("commit review failed: %v\nstderr: %s", err, stderr)
	}
	material := readReportMaterial(t, reportPath)
	assertFilesystemMCPCalls(t, callLog, "2")
	var achievements commitAchievements
	if err := json.Unmarshal(material.Sections.Achievements.Data, &achievements); err != nil {
		t.Fatal(err)
	}
	if achievements.ModelSummaryStatus != "provided" || achievements.ModelSummaryUsageStatus != "available" || achievements.ModelSummaryInputTokens != 31 || achievements.ModelSummaryOutputTokens != 17 || len(achievements.ModuleSummaries) == 0 || len(achievements.RepresentativeChanges) == 0 {
		t.Fatalf("Git evidence summary or independent model usage is missing: %+v", achievements)
	}
	if achievements.ModelSummaryElapsedMS < 1900 || achievements.CollectionElapsedMS >= achievements.ModelSummaryElapsedMS {
		t.Fatalf("Git collection time includes delayed model summary: collection=%dms model=%dms", achievements.CollectionElapsedMS, achievements.ModelSummaryElapsedMS)
	}
	if len(material.Sections.KnowledgeSources.Data) == 0 {
		t.Fatalf("MCP knowledge read was not observed: status=%q reason=%q model reads=%d summary calls=%d stderr=%s", material.Sections.KnowledgeSources.Status, material.Sections.KnowledgeSources.Reason, srv.knowledgeReadCount(), srv.summaryCallCount(), stderr)
	}
	var knowledge map[string]any
	if err := json.Unmarshal(material.Sections.KnowledgeSources.Data, &knowledge); err != nil {
		t.Fatal(err)
	}
	if knowledge["status"] != "partial" || knowledge["changed"] != true {
		t.Fatalf("knowledge observations did not disclose a changed source: %s", material.Sections.KnowledgeSources.Data)
	}
	observations, ok := knowledge["observations"].([]any)
	if !ok || len(observations) != 2 {
		t.Fatalf("knowledge observations = %#v, want two MCP reads", knowledge["observations"])
	}
	first, second := observations[0].(map[string]any), observations[1].(map[string]any)
	if first["source"] != "knowledge.md" || first["sha256"] == second["sha256"] || first["status"] != "complete" {
		t.Fatalf("actual MCP source/version evidence = %#v / %#v", first, second)
	}
	changes, ok := knowledge["version_changes"].([]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("same-run content change was not recorded: %#v", knowledge["version_changes"])
	}
	change := changes[0].(map[string]any)
	if change["previous_sha256"] != first["sha256"] || change["current_sha256"] != second["sha256"] {
		t.Fatalf("recorded version transition does not match returned hashes: %#v", change)
	}
	serialized, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), filepath.Dir(knowledgePath)) {
		t.Fatalf("knowledge root absolute path leaked into report material: %s", serialized)
	}
	if _, leaked := first["content"]; leaked {
		t.Fatalf("knowledge body was persisted in report material: %#v", first)
	}
}

func TestReviewE2E_ReportMarksRestrictedMCPReadPartial(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	srv := newFakeLLM()
	startFakeLLM(t, srv)
	knowledgePath := filepath.Join(t.TempDir(), "knowledge.md")
	if err := os.WriteFile(knowledgePath, []byte("FIRST_LINE\nSECOND_LINE"), 0o600); err != nil {
		t.Fatal(err)
	}
	callLog := configureReportFilesystemMCP(t, filepath.Dir(knowledgePath), false)
	srv.setKnowledgePath(knowledgePath, true)
	reportPath := filepath.Join(t.TempDir(), "enriched.json")
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("commit review failed: %v\nstderr: %s", err, stderr)
	}
	material := readReportMaterial(t, reportPath)
	assertFilesystemMCPCalls(t, callLog, "1")
	if len(material.Sections.KnowledgeSources.Data) == 0 {
		t.Fatalf("restricted MCP read was not observed: status=%q reason=%q model reads=%d summary calls=%d stderr=%s", material.Sections.KnowledgeSources.Status, material.Sections.KnowledgeSources.Reason, srv.knowledgeReadCount(), srv.summaryCallCount(), stderr)
	}
	var knowledge map[string]any
	if err := json.Unmarshal(material.Sections.KnowledgeSources.Data, &knowledge); err != nil {
		t.Fatal(err)
	}
	observations, _ := knowledge["observations"].([]any)
	if knowledge["status"] != "partial" || len(observations) != 1 || observations[0].(map[string]any)["status"] != "partial" {
		t.Fatalf("restricted read was not marked partial: %s", material.Sections.KnowledgeSources.Data)
	}
	if knowledge["knowledge_version_status"] != "partial" {
		t.Fatalf("restricted response was reported as a complete version: %s", material.Sections.KnowledgeSources.Data)
	}
}

func TestReviewE2E_ReportKnowledgeFailurePreservesCodeFinding(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	srv := newFakeLLM()
	srv.includeFinding = true
	srv.findingPath = "main.go"
	startFakeLLM(t, srv)
	knowledgePath := filepath.Join(t.TempDir(), "missing.md")
	configureReportFilesystemMCP(t, filepath.Dir(knowledgePath), false)
	srv.setKnowledgePath(knowledgePath, false)
	reportPath := filepath.Join(t.TempDir(), "report.json")
	stdout, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath, "--no-filter")
	if err != nil {
		t.Fatalf("knowledge read failure should not fail code review: %v\nstderr: %s", err, stderr)
	}
	var native jsonOutput
	if err := json.Unmarshal([]byte(stdout), &native); err != nil {
		t.Fatalf("decode native output: %v\n%s", err, stdout)
	}
	material := readReportMaterial(t, reportPath)
	if native.Manifest == nil || native.Status != string(session.StateComplete) || len(native.Comments) == 0 || len(material.Findings) != len(native.Comments) {
		t.Fatalf("knowledge failure erased or hid supported code finding: native status=%q comments=%d material status=%q findings=%d", native.Status, len(native.Comments), material.Review.Status, len(material.Findings))
	}
	var knowledge knowledgeMaterial
	if err := json.Unmarshal(material.Sections.KnowledgeSources.Data, &knowledge); err != nil {
		t.Fatalf("decode knowledge evidence: %v", err)
	}
	if knowledge.Status != "failed" || knowledge.ApplicationStatus != "not_observed" || len(knowledge.Observations) != 1 || knowledge.Observations[0].Status != "failed" {
		t.Fatalf("missing knowledge was not reported as unavailable and unapplied: %+v", knowledge)
	}
}

func TestReviewE2E_ReportSummaryFailureKeepsGitEvidenceAndUsage(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	srv := newFakeLLM()
	srv.failSummary = true
	startFakeLLM(t, srv)
	reportPath := filepath.Join(t.TempDir(), "enriched.json")
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("native review should survive enrichment failure: %v\nstderr: %s", err, stderr)
	}
	material := readReportMaterial(t, reportPath)
	var achievements commitAchievements
	if err := json.Unmarshal(material.Sections.Achievements.Data, &achievements); err != nil {
		t.Fatal(err)
	}
	if material.Sections.Achievements.Status != report.StatusFailed || achievements.ModelSummaryStatus != "failed" || achievements.ModelSummaryUsageStatus != "not_available" || len(achievements.Files) == 0 {
		t.Fatalf("summary failure lost facts or failure accounting: status=%s data=%+v", material.Sections.Achievements.Status, achievements)
	}
}

func TestReviewE2E_ReportRejectsUnsupportedAchievementReferences(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	srv := newFakeLLM()
	srv.invalidSummary = true
	startFakeLLM(t, srv)
	reportPath := filepath.Join(t.TempDir(), "enriched.json")
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("native review should survive rejected summary: %v\nstderr: %s", err, stderr)
	}
	material := readReportMaterial(t, reportPath)
	var achievements commitAchievements
	if err := json.Unmarshal(material.Sections.Achievements.Data, &achievements); err != nil {
		t.Fatal(err)
	}
	if material.Sections.Achievements.Status != report.StatusFailed || achievements.ModelSummaryStatus != "rejected" || len(achievements.Files) == 0 || achievements.ModelSummary != "" || len(achievements.RepresentativeChanges) != 0 {
		t.Fatalf("unsupported model references were accepted or Git facts were lost: status=%s data=%+v", material.Sections.Achievements.Status, achievements)
	}
}

func TestReviewE2E_ReportRecordsRemoteMCPReadWithUnknownSource(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	srv := newFakeLLM()
	startFakeLLM(t, srv)
	server := mcp.NewServer(&mcp.Implementation{Name: "report-remote", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "read_text_file", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "REMOTE_BODY"}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	remote := httptest.NewServer(handler)
	t.Cleanup(remote.Close)
	requestedPath := filepath.Join(t.TempDir(), "private", "knowledge.md")
	remoteRoot := filepath.Dir(requestedPath)
	configureReportMCP(t, map[string]MCPServerConfig{"remote-knowledge": {Type: "remote", URL: remote.URL, Args: []string{remoteRoot}, Env: []string{"OCR_REMOTE_ROOT=" + remoteRoot}, Tools: []string{"read_text_file"}}})
	srv.setKnowledgePath(requestedPath, false)
	reportPath := filepath.Join(t.TempDir(), "enriched.json")
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("review with remote MCP failed: %v\nstderr: %s", err, stderr)
	}
	material := readReportMaterial(t, reportPath)
	var knowledge knowledgeMaterial
	if err := json.Unmarshal(material.Sections.KnowledgeSources.Data, &knowledge); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("REMOTE_BODY"))
	if material.Sections.KnowledgeSources.Status != report.StatusFailed || knowledge.Status != "partial" || knowledge.KnowledgeVersion != "source_unknown" || len(knowledge.Observations) != 1 {
		t.Fatalf("remote source provenance was discarded or overclaimed: status=%s knowledge=%+v reason=%s", material.Sections.KnowledgeSources.Status, knowledge, material.Sections.KnowledgeSources.Reason)
	}
	observation := knowledge.Observations[0]
	if observation.SourceStatus != "unknown" || observation.Status != "complete" || observation.SHA256 != hex.EncodeToString(digest[:]) || observation.ResponseBytes != len("REMOTE_BODY") || observation.Source != "" || len(observation.Sources) != 0 {
		t.Fatalf("remote MCP return evidence = %+v", observation)
	}
	serialized, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), filepath.Dir(requestedPath)) || strings.Contains(string(serialized), "REMOTE_BODY") {
		t.Fatal("report leaked an unmapped local path or MCP response body")
	}
}

func TestReviewE2E_ReportRecordsMCPServerStartFailure(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	startFakeLLM(t, newFakeLLM())
	configureReportMCP(t, map[string]MCPServerConfig{"knowledge": {Command: filepath.Join(t.TempDir(), "missing-mcp.exe"), Tools: []string{"read_text_file"}}})
	reportPath := filepath.Join(t.TempDir(), "enriched.json")
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("review should survive MCP startup failure: %v\nstderr: %s", err, stderr)
	}
	material := readReportMaterial(t, reportPath)
	var knowledge knowledgeMaterial
	if err := json.Unmarshal(material.Sections.KnowledgeSources.Data, &knowledge); err != nil {
		t.Fatal(err)
	}
	if material.Sections.KnowledgeSources.Status != report.StatusFailed || knowledge.Status != "failed" || len(knowledge.Observations) != 1 || knowledge.Observations[0].Failure != "start_failed" || knowledge.Observations[0].Tool != "server_unavailable" {
		t.Fatalf("MCP startup failure was not recorded: status=%s data=%+v stderr=%s", material.Sections.KnowledgeSources.Status, knowledge, stderr)
	}
	serialized, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "missing-mcp.exe") {
		t.Fatal("report leaked MCP command/path from its failure evidence")
	}
}

func TestReviewE2E_ReportDoesNotRecordIssueMCPFailureAsKnowledgeFailure(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	startFakeLLM(t, newFakeLLM())
	configureReportMCP(t, map[string]MCPServerConfig{"issues": {Type: "remote", Tools: []string{"list_issues"}}})
	reportPath := filepath.Join(t.TempDir(), "enriched.json")
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("review should survive unavailable issue MCP: %v\nstderr: %s", err, stderr)
	}
	material := readReportMaterial(t, reportPath)
	if material.Sections.KnowledgeSources.Status != report.StatusNotCollected {
		t.Fatalf("unavailable issue MCP was misattributed to knowledge sources: status=%s reason=%s", material.Sections.KnowledgeSources.Status, material.Sections.KnowledgeSources.Reason)
	}
}

func TestReviewE2E_ReportRecordsMissingConfiguredKnowledgeTool(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "report-missing-tool", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "list_directory", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "directory entries"}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	remote := httptest.NewServer(handler)
	t.Cleanup(remote.Close)

	for _, tc := range []struct {
		name          string
		tools         []string
		wantStatus    report.Status
		wantKnowledge bool
	}{
		{name: "configured knowledge read is missing", tools: []string{"read_text_file"}, wantStatus: report.StatusFailed, wantKnowledge: true},
		{name: "issue tools do not imply knowledge reads", tools: []string{"list_issues"}, wantStatus: report.StatusNotCollected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			startFakeLLM(t, newFakeLLM())
			configureReportMCP(t, map[string]MCPServerConfig{"remote": {Type: "remote", URL: remote.URL, Tools: tc.tools}})
			reportPath := filepath.Join(t.TempDir(), "enriched.json")
			_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
			if err != nil {
				t.Fatalf("review with an unavailable configured MCP tool failed: %v\nstderr: %s", err, stderr)
			}
			material := readReportMaterial(t, reportPath)
			if material.Sections.KnowledgeSources.Status != tc.wantStatus {
				t.Fatalf("knowledge status = %s, want %s; reason=%s", material.Sections.KnowledgeSources.Status, tc.wantStatus, material.Sections.KnowledgeSources.Reason)
			}
			if tc.wantKnowledge {
				var knowledge knowledgeMaterial
				if err := json.Unmarshal(material.Sections.KnowledgeSources.Data, &knowledge); err != nil {
					t.Fatal(err)
				}
				if len(knowledge.Observations) != 1 || knowledge.Observations[0].Tool != "read_text_file" || knowledge.Observations[0].Failure != "tool_unavailable" {
					t.Fatalf("missing configured knowledge tool evidence = %+v", knowledge.Observations)
				}
			}
		})
	}
}

func TestReviewE2E_ReportSkipsSummaryForEmptyCommit(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	retryTestGit(t, repoDir, "commit", "--allow-empty", "-q", "-m", "empty")
	srv := newFakeLLM()
	startFakeLLM(t, srv)
	reportPath := filepath.Join(t.TempDir(), "enriched.json")
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("empty-commit review failed: %v\nstderr: %s", err, stderr)
	}
	material := readReportMaterial(t, reportPath)
	var achievements commitAchievements
	if err := json.Unmarshal(material.Sections.Achievements.Data, &achievements); err != nil {
		t.Fatal(err)
	}
	if srv.summaryCallCount() != 0 || material.Sections.Achievements.Status != report.StatusProvided || len(achievements.Files) != 0 || len(achievements.Modules) != 0 || achievements.ModelSummaryStatus != "not_applicable" || achievements.ModelSummaryUsageStatus != "not_applicable" || !strings.Contains(achievements.ModelSummary, "没有文件差异") { // allow-non-english: asserts the Chinese no-change result in report material
		t.Fatalf("empty commit was treated as a model summarization failure: calls=%d section=%s data=%+v", srv.summaryCallCount(), material.Sections.Achievements.Status, achievements)
	}
}

func readReportMaterial(t *testing.T, path string) report.Material {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var material report.Material
	if err := json.Unmarshal(data, &material); err != nil {
		t.Fatalf("decode report material: %v", err)
	}
	if err := report.ValidateMaterial(material); err != nil {
		t.Fatalf("validate report material: %v", err)
	}
	return material
}

func configureReportFilesystemMCP(t *testing.T, root string, mutateAfterFirstRead bool) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	callLog := filepath.Join(t.TempDir(), "successful-reads")
	entry := MCPServerConfig{
		Command: executable,
		Args:    []string{"-test.run=^TestReportFilesystemMCPProcess$"},
		Env:     []string{"OCR_REPORT_MCP_ROOT=" + root, "OCR_REPORT_MCP_CALL_LOG=" + callLog, fmt.Sprintf("OCR_REPORT_MCP_MUTATE=%t", mutateAfterFirstRead)},
		Tools:   []string{"read_text_file"},
	}
	configureReportMCP(t, map[string]MCPServerConfig{"knowledge": entry})
	return callLog
}

func configureReportMCP(t *testing.T, servers map[string]MCPServerConfig) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(Config{MCPServers: servers})
	if err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(home, ".opencodereview")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFilesystemMCPCalls(t *testing.T, callLog, want string) {
	t.Helper()
	data, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read filesystem MCP call log: %v", err)
	}
	if strings.TrimSpace(string(data)) != want {
		t.Fatalf("filesystem MCP successful body reads = %q, want %s", strings.TrimSpace(string(data)), want)
	}
}

func TestReportFilesystemMCPProcess(t *testing.T) {
	root := os.Getenv("OCR_REPORT_MCP_ROOT")
	if root == "" {
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "report-test-filesystem", Version: "1"}, nil)
	var calls int
	server.AddTool(&mcp.Tool{Name: "read_text_file", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path string `json:"path"`
			Head int    `json:"head"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		path, err := filepath.Abs(args.Path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "outside test root"}}}, nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		calls++
		if args.Head > 0 {
			lines := strings.Split(string(data), "\n")
			if len(lines) > args.Head {
				data = []byte(strings.Join(lines[:args.Head], "\n"))
			}
		}
		if calls == 1 && os.Getenv("OCR_REPORT_MCP_MUTATE") == "true" {
			if err := os.WriteFile(path, []byte("NEW_KNOWLEDGE"), 0o600); err != nil {
				return nil, err
			}
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		t.Fatal(err)
	}
	if callLog := os.Getenv("OCR_REPORT_MCP_CALL_LOG"); callLog != "" {
		if err := os.WriteFile(callLog, []byte(fmt.Sprintf("%d", calls)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	os.Exit(0)
}

func TestKnowledgeSourcesSectionDistinguishesObservedPartialAndFailure(t *testing.T) {
	manifest := &session.RunManifest{Execution: session.ManifestExecution{RuleConfigSHA256: strings.Repeat("a", 64)}}
	noObservation := knowledgeSourcesSection(manifest, nil)
	if noObservation.Status != report.StatusNotCollected {
		t.Fatalf("no actual MCP return was counted as a read: %q", noObservation.Status)
	}
	success := knowledgeSourcesSection(manifest, []knowledgeToolObservation{{Tool: "read_text_file", Source: "knowledge.md", RequestScope: "full", Status: "complete", SHA256: strings.Repeat("b", 64), ResponseBytes: 4}})
	if success.Status != report.StatusProvided {
		t.Fatalf("observed knowledge status = %q", success.Status)
	}
	var successData knowledgeMaterial
	if err := json.Unmarshal(success.Data, &successData); err != nil || successData.Status != "observed" || successData.KnowledgeVersion != "observed" || successData.ApplicationStatus != "not_observed" {
		t.Fatalf("observed knowledge data = %+v, err=%v", successData, err)
	}
	partial := knowledgeSourcesSection(manifest, []knowledgeToolObservation{
		{Tool: "read_text_file", Source: "knowledge.md", RequestScope: `{"head":1}`, Status: "partial", SHA256: strings.Repeat("c", 64), ResponseBytes: 4},
		{Tool: "read_text_file", Source: "knowledge.md", RequestScope: "full", Status: "failed", Failure: "mcp_error"},
	})
	if partial.Status != report.StatusFailed {
		t.Fatalf("partial knowledge status = %q", partial.Status)
	}
	var partialData knowledgeMaterial
	if err := json.Unmarshal(partial.Data, &partialData); err != nil || partialData.Status != "partial" || partialData.KnowledgeVersion != "partial" || len(partialData.Observations) != 2 {
		t.Fatalf("partial knowledge data = %+v, err=%v", partialData, err)
	}
	failure := knowledgeSourcesSection(manifest, []knowledgeToolObservation{{Tool: "read_text_file", Source: "missing.md", Status: "failed", Failure: "mcp_error"}})
	if failure.Status != report.StatusFailed {
		t.Fatalf("failed knowledge status = %q", failure.Status)
	}
	unknownSource := knowledgeSourcesSection(manifest, []knowledgeToolObservation{{Tool: "lookup", Status: "not_observed"}})
	if unknownSource.Status != report.StatusNotCollected {
		t.Fatalf("unidentified tool result status = %q, want not collected", unknownSource.Status)
	}
}

func TestKnowledgeObservationRecorderFiltersDirectoryToolsAndNormalizesBatchSources(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "docs", "rules.md")
	secondPath := filepath.Join(root, "README.md")
	if err := os.MkdirAll(filepath.Dir(firstPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(firstPath, []byte("rules"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("readme"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := newKnowledgeObservationRecorder(&Config{MCPServers: map[string]MCPServerConfig{"knowledge": {Args: []string{root}}}}, t.TempDir())
	// allow-non-english: the fixture separates repoDir from the absolute configured root to test relative root resolution
	recorder.observe("knowledge", "list_directory", map[string]any{"path": root}, "directory entries", nil, false)
	if got := recorder.snapshot(); len(got) != 0 {
		t.Fatalf("directory listing became knowledge body evidence: %+v", got)
	}
	recorder.observe("knowledge", "read_multiple_files", map[string]any{"paths": []any{firstPath, secondPath}}, "batched response", nil, false)
	observations := recorder.snapshot()
	if len(observations) != 1 {
		t.Fatalf("batch read observations = %+v", observations)
	}
	want := []string{"docs/rules.md", "README.md"}
	if strings.Join(observations[0].Sources, ",") != strings.Join(want, ",") || strings.Contains(observations[0].Source, root) {
		t.Fatalf("batch sources were not normalized: %+v", observations[0])
	}
}

func TestKnowledgeObservationRecorderResolvesRelativeRootsFromRepoDir(t *testing.T) {
	repoDir := t.TempDir()
	root := filepath.Join(repoDir, "knowledge")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "rules.md")
	if err := os.WriteFile(file, []byte("rules"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := newKnowledgeObservationRecorder(&Config{MCPServers: map[string]MCPServerConfig{"knowledge": {Args: []string{"knowledge"}}}}, repoDir)
	recorder.observe("knowledge", "read_text_file", map[string]any{"path": file}, "rules", nil, false)
	observations := recorder.snapshot()
	if len(observations) != 1 || observations[0].Source != "rules.md" || observations[0].Status != "complete" {
		t.Fatalf("relative MCP root was not resolved from repoDir: %+v", observations)
	}
}

func TestKnowledgeBatchSourceOrderDoesNotCreateVersionChanges(t *testing.T) {
	repoDir := t.TempDir()
	root := filepath.Join(repoDir, "knowledge")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.md")
	secondPath := filepath.Join(root, "b.md")
	recorder := newKnowledgeObservationRecorder(&Config{MCPServers: map[string]MCPServerConfig{"knowledge": {Args: []string{root}}}}, repoDir)
	recorder.observe("knowledge", "read_multiple_files", map[string]any{"paths": []any{firstPath, secondPath}}, "a content\nb content", nil, false)
	recorder.observe("knowledge", "read_multiple_files", map[string]any{"paths": []any{secondPath, firstPath}}, "b content\na content", nil, false)
	observations := recorder.snapshot()
	if len(observations) != 2 || observations[0].Sources[0] != "a.md" || observations[1].Sources[0] != "b.md" {
		t.Fatalf("batch source request order was lost: %+v", observations)
	}
	section := knowledgeSourcesSection(&session.RunManifest{}, observations)
	var data knowledgeMaterial
	if err := json.Unmarshal(section.Data, &data); err != nil {
		t.Fatal(err)
	}
	if section.Status != report.StatusProvided || data.Changed || len(data.VersionChanges) != 0 {
		t.Fatalf("path order alone was reported as a version change: status=%s data=%+v", section.Status, data)
	}
}

func TestKnowledgeSourceUnknownKeepsSuccessfulResponseDigest(t *testing.T) {
	requestedPath := filepath.Join(t.TempDir(), "knowledge.md")
	recorder := newKnowledgeObservationRecorder(&Config{MCPServers: map[string]MCPServerConfig{"remote": {Type: "remote"}}}, t.TempDir())
	recorder.observe("remote", "read_text_file", map[string]any{"path": requestedPath}, "returned body", nil, false)
	observations := recorder.snapshot()
	section := knowledgeSourcesSection(&session.RunManifest{}, observations)
	var data knowledgeMaterial
	if err := json.Unmarshal(section.Data, &data); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("returned body"))
	if section.Status != report.StatusFailed || len(data.Observations) != 1 || data.KnowledgeVersion != "source_unknown" || data.Observations[0].SourceStatus != "unknown" || data.Observations[0].Status != "complete" || data.Observations[0].SHA256 != hex.EncodeToString(digest[:]) || data.Observations[0].Source != "" {
		t.Fatalf("successful response with unknown source was lost or overclaimed: status=%s data=%+v", section.Status, data)
	}
}

func TestKnowledgeObservationRecorderCapturesMCPIsErrorWithoutChangingBodyHash(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rules.md")
	recorder := newKnowledgeObservationRecorder(&Config{MCPServers: map[string]MCPServerConfig{"knowledge": {Args: []string{root}}}}, t.TempDir())
	recorder.observe("knowledge", "read_text_file", map[string]any{"path": path}, "actual MCP error body", nil, true)
	observations := recorder.snapshot()
	digest := sha256.Sum256([]byte("actual MCP error body"))
	if len(observations) != 1 || observations[0].Status != "failed" || observations[0].Failure != "mcp_error" || observations[0].SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("MCP IsError was not recorded against actual response text: %+v", observations)
	}
}

func TestEnrichReportSectionsDoesNotInferNonCommitResults(t *testing.T) {
	manifest := &session.RunManifest{Input: session.ManifestInput{Mode: session.InputModeRange}}
	achievements, people, _ := enrichReportSections(t.Context(), nil, t.TempDir(), report.Scope{Mode: session.InputModeRange}, manifest, nil, "", nil, nil, "")
	if achievements.Status != report.StatusNotCollected || people.Status != report.StatusNotCollected {
		t.Fatalf("non-commit enrichment should remain uncollected: %q/%q", achievements.Status, people.Status)
	}
}

func TestCollectCommitMaterialUsesFirstParentForMergeCommit(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	base := reportMaterialGitOutput(t, repoDir, "rev-parse", "HEAD")
	retryTestGit(t, repoDir, "checkout", "-qb", "feature", base)
	writeReportMaterialFile(t, repoDir, "feature.go", "package p\n\nfunc feature() int { return 3 }\n")
	retryTestGit(t, repoDir, "add", "feature.go")
	reportMaterialGitCommitAs(t, repoDir, "Feature Author", "feature@example.test", "Feature Committer", "feature-committer@example.test", "feature")
	feature := reportMaterialGitOutput(t, repoDir, "rev-parse", "HEAD")
	retryTestGit(t, repoDir, "checkout", "main")
	writeReportMaterialFile(t, repoDir, "main-only.go", "package p\n\nfunc mainOnly() int { return 4 }\n")
	retryTestGit(t, repoDir, "add", "main-only.go")
	reportMaterialGitCommitAs(t, repoDir, "Main Author", "main@example.test", "Main Committer", "main-committer@example.test", "main change")
	retryTestGit(t, repoDir, "merge", "--no-ff", "-m", "merge feature", "feature")
	merge := reportMaterialGitOutput(t, repoDir, "rev-parse", "HEAD")
	parents := strings.Fields(reportMaterialGitOutput(t, repoDir, "rev-list", "--parents", "-n", "1", merge))
	if len(parents) != 3 || parents[1] == base || parents[2] != feature {
		t.Fatalf("merge parents = %v", parents)
	}
	material, err := collectCommitMaterial(t.Context(), gitcmd.New(2), repoDir, parents[1], merge)
	if err != nil {
		t.Fatal(err)
	}
	if len(material.Files) != 1 || material.Files[0].Path != "feature.go" {
		t.Fatalf("merge first-parent files = %+v, want feature.go", material.Files)
	}
	wantFileCommand := "git diff --name-status -z --no-renames --end-of-options " + parents[1] + " " + merge + " --"
	if len(material.Evidence) != 3 || material.Evidence[1] != wantFileCommand {
		t.Fatalf("evidence does not name the actual first-parent path command: %+v", material.Evidence)
	}
}
