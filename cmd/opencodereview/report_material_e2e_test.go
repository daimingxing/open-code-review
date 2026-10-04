// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

func reportMaterialTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	retryTestGit(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package p\n\nfunc value() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	retryTestGit(t, dir, "add", "main.go")
	retryTestGit(t, dir, "commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package p\n\nfunc value() int { return 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	retryTestGit(t, dir, "add", "main.go")
	retryTestGit(t, dir, "commit", "-q", "-m", "change")
	return dir
}

func runReportReview(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout string
	var runErr error
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			runErr = runReview(args)
		})
	})
	return stdout, stderr, runErr
}

func TestReviewE2E_ReportBarePathIncludesZeroFindingsAndNativeOutput(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	startFakeLLM(t, newFakeLLM())

	stdout, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--report")
	if err != nil {
		t.Fatalf("review failed: %v\nstderr: %s", err, stderr)
	}
	var native jsonOutput
	if err := json.Unmarshal([]byte(stdout), &native); err != nil {
		t.Fatalf("decode native output: %v\n%s", err, stdout)
	}
	if native.Manifest == nil || native.Status != string(session.StateComplete) || len(native.Comments) != 0 {
		t.Fatalf("native result changed: status=%q manifest=%v comments=%d", native.Status, native.Manifest != nil, len(native.Comments))
	}
	paths, err := filepath.Glob(filepath.Join(repoDir, "*.report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || !strings.Contains(filepath.Base(paths[0]), "__commit-HEAD__") {
		t.Fatalf("automatic report paths = %v, want one commit-HEAD file in repo root", paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var material report.Material
	if err := json.Unmarshal(data, &material); err != nil {
		t.Fatalf("decode report material: %v\n%s", err, data)
	}
	if err := report.ValidateMaterial(material); err != nil {
		t.Fatalf("report material validation: %v", err)
	}
	if material.Review.RunID != native.Manifest.RunID || material.Review.Status != session.StateComplete {
		t.Fatalf("report review identity/status = %q/%q, native = %q/%q", material.Review.RunID, material.Review.Status, native.Manifest.RunID, native.Status)
	}
	if material.Scope.ResolvedHead.SHA != native.Manifest.Input.ResolvedHead || material.Scope.ResolvedBase.SHA != native.Manifest.Input.ResolvedBase {
		t.Fatalf("report scope does not preserve native resolved refs: %+v vs %+v", material.Scope, native.Manifest.Input)
	}
	if material.Findings == nil || len(material.Findings) != 0 {
		t.Fatalf("zero findings must be an explicit empty array, got %#v", material.Findings)
	}
	if !strings.Contains(string(data), `"knowledge_sources"`) || !strings.Contains(string(data), `"limitations"`) || strings.Contains(string(data), repoDir) {
		t.Fatalf("report material omitted limitation facts or leaked an absolute repository path: %s", data)
	}
}

func TestReviewE2E_ReportFindingAndNativeJSONAreIndependent(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	srv := newFakeLLM()
	srv.includeFinding = true
	srv.findingPath = "main.go"
	startFakeLLM(t, srv)
	reportDir := filepath.Join(repoDir, "reports")
	if err := os.Mkdir(reportDir, 0o700); err != nil {
		t.Fatal(err)
	}
	nativePath := filepath.Join(repoDir, "native.json")
	reportPath := filepath.Join(reportDir, "final review.json")
	_, stderr, err := runReportReview(t, "--report", reportPath, "--format", "json", "--output", nativePath,
		"--repo", repoDir, "--commit", "HEAD", "--no-filter")
	if err != nil {
		t.Fatalf("review failed: %v\nstderr: %s", err, stderr)
	}
	nativeData, err := os.ReadFile(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	var native jsonOutput
	if err := json.Unmarshal(nativeData, &native); err != nil {
		t.Fatalf("decode native result: %v\n%s", err, nativeData)
	}
	if native.Manifest == nil || len(native.Comments) == 0 {
		t.Fatalf("native output should preserve model finding: manifest=%v comments=%d", native.Manifest != nil, len(native.Comments))
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var material report.Material
	if err := json.Unmarshal(data, &material); err != nil {
		t.Fatalf("decode report material: %v\n%s", err, data)
	}
	if err := report.ValidateMaterial(material); err != nil {
		t.Fatalf("report material validation: %v", err)
	}
	if len(material.Findings) != len(native.Comments) {
		t.Fatalf("material findings=%d, native comments=%d", len(material.Findings), len(native.Comments))
	}
	finding := material.Findings[0]
	if !strings.HasPrefix(finding.ID, "sha256:") || finding.Severity != "high" || finding.Category != "bug" || finding.Path != "main.go" {
		t.Fatalf("finding native identity fields are wrong: %+v", finding)
	}
	if finding.SourceContent != native.Comments[0].Content || finding.Evidence.Code != "return 2" || finding.Recommendation.Code != "return 1" {
		t.Fatalf("finding source/evidence/recommendation were not preserved: %+v", finding)
	}
	if !strings.Contains(finding.Display.SummaryZH, "发现高级缺陷问题") || strings.Contains(string(data), `"thinking"`) { // allow-non-english: asserts required Chinese display output
		t.Fatalf("Chinese display summary or private-field exclusion failed: %s", data)
	}
}

func TestReviewE2E_ReportSaveFailurePreservesNativeOutput(t *testing.T) {
	for _, mode := range reportReviewModes {
		t.Run(mode.name, func(t *testing.T) {
			repoDir := reportReviewRepoForMode(t, mode)
			startFakeLLM(t, newFakeLLM())
			nativePath := filepath.Join(t.TempDir(), "native.json")
			reportPath := filepath.Join(t.TempDir(), "existing.report.json")
			if err := os.WriteFile(reportPath, []byte("keep existing report"), 0o600); err != nil {
				t.Fatal(err)
			}
			args := reportReviewArgs(repoDir, nativePath, reportPath, mode)
			_, stderr, err := runReportReview(t, args...)
			if err == nil || !strings.Contains(err.Error(), "save report material") {
				t.Fatalf("review error = %v, want save report material failure\nstderr: %s", err, stderr)
			}
			native := readNativeReview(t, nativePath)
			if native.Status != string(session.StateComplete) {
				t.Fatalf("native result status = %q, want preserved complete result", native.Status)
			}
			kept, readErr := os.ReadFile(reportPath)
			if readErr != nil || string(kept) != "keep existing report" {
				t.Fatalf("existing explicit report changed: content=%q error=%v", kept, readErr)
			}
		})
	}
}

func TestReviewE2E_ReportAndNativeOutputConflictPreservesExistingFile(t *testing.T) {
	repoDir := reportMaterialTestRepo(t)
	srv := newFakeLLM()
	startFakeLLM(t, srv)
	sharedPath := filepath.Join(repoDir, "shared.json")
	if err := os.WriteFile(sharedPath, []byte("keep native output"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--commit", "HEAD", "--format", "json", "--output", sharedPath, "--report", sharedPath)
	if err == nil || !strings.Contains(err.Error(), "conflicts with --output") {
		t.Fatalf("review error = %v, want output path conflict\nstderr: %s", err, stderr)
	}
	kept, readErr := os.ReadFile(sharedPath)
	if readErr != nil || string(kept) != "keep native output" {
		t.Fatalf("conflicting output file changed: content=%q error=%v", kept, readErr)
	}
	srv.mu.Lock()
	calls := srv.groupingCalls + len(srv.attemptsByFile)
	srv.mu.Unlock()
	if calls != 0 {
		t.Fatalf("review model was called before rejecting conflicting paths: %d calls", calls)
	}
}

func TestBuildReportMaterialCarriesPartialAndFailedLimitations(t *testing.T) {
	started := time.Now().Add(-time.Second)
	base := strings.Repeat("b", 40)
	head := strings.Repeat("a", 40)
	manifest := &session.RunManifest{
		RunID:         "partial-run",
		TerminalState: session.StatePartial,
		Repository:    session.ManifestRepository{IdentitySHA256: strings.Repeat("d", 64)},
		Input: session.ManifestInput{
			Mode: session.InputModeCommit, RequestedHead: "HEAD", ResolvedBase: base, ResolvedHead: head,
			ExactRange: base + ".." + head, SourceArtifactSHA256: strings.Repeat("c", 64),
		},
		Coverage: session.Coverage{
			Selected:  []session.CoverageItem{{ItemID: strings.Repeat("1", 64)}, {ItemID: strings.Repeat("2", 64)}},
			Completed: []session.CoverageItem{{ItemID: strings.Repeat("1", 64)}},
			Reused:    []session.CoverageItem{},
			Failed:    []session.CoverageItem{{ItemID: strings.Repeat("2", 64), Classification: session.FailureProvider, Reason: "provider request failed"}},
			Waived:    []session.CoverageItem{},
		},
		ElapsedMS: 100,
	}
	comments := []model.LlmComment{{Path: "main.go", Content: "finding", ExistingCode: "return 2", Severity: "high", Category: "bug"}}
	toolFailures := []llmloop.ToolFailureDetail{{ToolCallNumber: 1, ToolName: "read_file", Arguments: "sensitive input", Error: "raw detail"}}
	material, err := buildReportMaterial(manifest, "repo", comments, started, started.Add(time.Second), "fake", "fake-model", toolFailures, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.ValidateMaterial(material); err != nil {
		t.Fatalf("partial material invalid: %v", err)
	}
	seen := make(map[string]report.Limitation)
	for _, limitation := range material.Limitations {
		seen[limitation.Source] = limitation
	}
	for _, source := range []string{"coverage.failed." + strings.Repeat("2", 64), "tool_failure.1", "sections.knowledge_sources"} {
		if _, ok := seen[source]; !ok {
			t.Errorf("partial material omitted limitation %q", source)
		}
	}
	for _, limitation := range material.Limitations {
		if strings.Contains(limitation.Reason, "sensitive input") || strings.Contains(limitation.Reason, "raw detail") {
			t.Errorf("tool arguments or raw error leaked into limitation: %+v", limitation)
		}
	}

	manifest.RunID = "failed-run"
	manifest.TerminalState = session.StateFailed
	manifest.RunFailure = &session.RunFailure{Classification: session.RunFailureTimeout, Reason: "overall timeout"}
	failed, err := buildReportMaterial(manifest, "repo", nil, started, started.Add(time.Second), "fake", "fake-model", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.ValidateMaterial(failed); err != nil {
		t.Fatalf("failed material invalid: %v", err)
	}
	if failed.Review.Status != session.StateFailed || len(failed.Limitations) == 0 || failed.Limitations[0].Source != "run_failure" {
		t.Fatalf("failed material lost run-level limitation: %+v", failed.Limitations)
	}
}

func TestBuildReportMaterialPreservesEveryReviewModeScope(t *testing.T) {
	started := time.Now().Add(-time.Second)
	base := strings.Repeat("b", 40)
	head := strings.Repeat("a", 40)
	cases := []struct {
		name      string
		input     session.ManifestInput
		wantBase  report.Status
		wantHead  report.Status
		wantRange string
	}{
		{
			name: "commit",
			input: session.ManifestInput{Mode: session.InputModeCommit, RequestedHead: "HEAD", ResolvedBase: base, ResolvedHead: head,
				ExactRange: base + ".." + head, SourceArtifactSHA256: strings.Repeat("c", 64)},
			wantBase: report.StatusProvided, wantHead: report.StatusProvided, wantRange: base + ".." + head,
		},
		{
			name: "range",
			input: session.ManifestInput{Mode: session.InputModeRange, RequestedFrom: "main", RequestedHead: "feature", ResolvedBase: base, ResolvedHead: head,
				ExactRange: base + ".." + head, SourceArtifactSHA256: strings.Repeat("c", 64)},
			wantBase: report.StatusProvided, wantHead: report.StatusProvided, wantRange: base + ".." + head,
		},
		{
			name:     "workspace",
			input:    session.ManifestInput{Mode: session.InputModeWorkspace, ResolvedBase: base, SourceArtifactSHA256: strings.Repeat("c", 64)},
			wantBase: report.StatusProvided, wantHead: report.StatusNotApplicable,
		},
		{
			name:     "root commit",
			input:    session.ManifestInput{Mode: session.InputModeCommit, RequestedHead: "HEAD", ResolvedHead: head, SourceArtifactSHA256: strings.Repeat("c", 64)},
			wantBase: report.StatusNotApplicable, wantHead: report.StatusProvided,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := &session.RunManifest{
				RunID: "scope-run", TerminalState: session.StateComplete,
				Repository: session.ManifestRepository{IdentitySHA256: strings.Repeat("d", 64)},
				Input:      tc.input,
				Coverage: session.Coverage{
					Selected: []session.CoverageItem{}, Completed: []session.CoverageItem{}, Reused: []session.CoverageItem{},
					Failed: []session.CoverageItem{}, Waived: []session.CoverageItem{},
				},
			}
			material, err := buildReportMaterial(manifest, "repo", nil, started, started.Add(time.Second), "fake", "fake-model", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := report.ValidateMaterial(material); err != nil {
				t.Fatalf("report material validation: %v", err)
			}
			if material.Repository.Identity.Value != "sha256:"+strings.Repeat("d", 64) {
				t.Fatalf("repository identity = %q, want normalized native SHA-256", material.Repository.Identity.Value)
			}
			if material.Scope.Mode != tc.input.Mode || material.Scope.ResolvedBase.Status != tc.wantBase || material.Scope.ResolvedHead.Status != tc.wantHead || material.Scope.ExactRange != tc.wantRange {
				t.Fatalf("scope = %+v, want mode=%s base=%s head=%s range=%q", material.Scope, tc.input.Mode, tc.wantBase, tc.wantHead, tc.wantRange)
			}
		})
	}
}

func TestReportFindingIdentityChangesWithScope(t *testing.T) {
	identity := report.FindingIdentity{RepositoryName: "repo", Mode: session.InputModeCommit, ResolvedHead: strings.Repeat("a", 40), Path: "main.go", SourceContent: "finding"}
	first := report.NewFindingID(identity)
	identity.ResolvedHead = strings.Repeat("b", 40)
	if first == report.NewFindingID(identity) {
		t.Fatal("finding identity did not include resolved scope")
	}
}

func TestMaterialFindingsSortsLineNumbersNumerically(t *testing.T) {
	manifest := &session.RunManifest{Input: session.ManifestInput{Mode: session.InputModeCommit, ResolvedHead: strings.Repeat("a", 40)}}
	findings, err := materialFindings(manifest, "repo", []model.LlmComment{
		{Path: "main.go", StartLine: 10, EndLine: 10, Severity: "high", Category: "bug", Content: "later"},
		{Path: "main.go", StartLine: 9, EndLine: 9, Severity: "high", Category: "bug", Content: "earlier"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 || findings[0].StartLine != 9 || findings[1].StartLine != 10 {
		t.Fatalf("finding line order = %+v, want 9 before 10", findings)
	}
}

func TestBuildReportMaterialPreservesMissingFindingLabelFacts(t *testing.T) {
	base := strings.Repeat("b", 40)
	head := strings.Repeat("a", 40)
	manifest := &session.RunManifest{
		RunID:         "missing-labels-run",
		TerminalState: session.StateComplete,
		Repository:    session.ManifestRepository{IdentitySHA256: strings.Repeat("d", 64)},
		Input: session.ManifestInput{
			Mode: session.InputModeCommit, RequestedHead: "HEAD", ResolvedBase: base, ResolvedHead: head,
			ExactRange: base + ".." + head, SourceArtifactSHA256: strings.Repeat("c", 64),
		},
		Coverage: session.Coverage{
			Selected: []session.CoverageItem{}, Completed: []session.CoverageItem{}, Reused: []session.CoverageItem{},
			Failed: []session.CoverageItem{}, Waived: []session.CoverageItem{},
		},
	}
	started := time.Now()
	cases := []struct {
		name               string
		severity           string
		category           string
		wantSeverityStatus report.Status
		wantSeverityReason string
		wantCategoryStatus report.Status
		wantCategoryReason string
		wantSeverityZH     string
		wantCategoryZH     string
		wantSummaryPart    string
		wantErr            bool
	}{
		{name: "both missing", wantSeverityStatus: report.StatusNotCollected, wantSeverityReason: "原生审查结果未提供问题等级", wantCategoryStatus: report.StatusNotCollected, wantCategoryReason: "原生审查结果未提供问题类别", wantSeverityZH: "未提供", wantCategoryZH: "未提供", wantSummaryPart: "等级未提供类别未提供"}, // allow-non-english: fixture exercises missing finding labels
		{name: "severity missing", category: "bug", wantSeverityStatus: report.StatusNotCollected, wantSeverityReason: "原生审查结果未提供问题等级", wantCategoryStatus: report.StatusProvided, wantCategoryReason: "", wantCategoryZH: "缺陷", wantSeverityZH: "未提供", wantSummaryPart: "等级未提供缺陷"}, // allow-non-english: fixture exercises missing finding labels
		{name: "category missing", severity: "high", wantSeverityStatus: report.StatusProvided, wantSeverityReason: "", wantCategoryStatus: report.StatusNotCollected, wantCategoryReason: "原生审查结果未提供问题类别", wantSeverityZH: "高", wantCategoryZH: "未提供", wantSummaryPart: "高级类别未提供"}, // allow-non-english: fixture exercises missing finding labels
		{name: "unknown labels", severity: "urgent", category: "unknown", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const sourceContent = "Original model finding text"
			material, err := buildReportMaterial(manifest, "repo", []model.LlmComment{{
				Path: "main.go", StartLine: 3, EndLine: 3, Severity: tc.severity, Category: tc.category, Content: sourceContent,
			}}, started, started.Add(time.Second), "fake", "fake-model", nil, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatal("build material accepted unsupported non-empty labels")
				}
				return
			}
			if err != nil {
				t.Fatalf("build material: %v", err)
			}
			finding := material.Findings[0]
			if finding.Severity != tc.severity || finding.Category != tc.category || finding.SeverityStatus != tc.wantSeverityStatus || finding.SeverityReason != tc.wantSeverityReason || finding.CategoryStatus != tc.wantCategoryStatus || finding.CategoryReason != tc.wantCategoryReason {
				t.Fatalf("material label facts = severity(%q,%q,%q) category(%q,%q,%q)", finding.Severity, finding.SeverityStatus, finding.SeverityReason, finding.Category, finding.CategoryStatus, finding.CategoryReason)
			}
			if finding.Display.SeverityZH != tc.wantSeverityZH || finding.Display.CategoryZH != tc.wantCategoryZH || !strings.Contains(finding.Display.SummaryZH, tc.wantSummaryPart) || finding.SourceContent != sourceContent {
				t.Fatalf("material display/source = %+v / %q", finding.Display, finding.SourceContent)
			}
			target := filepath.Join(t.TempDir(), "finding.report.json")
			if _, err := report.WriteMaterial(target, false, material); err != nil {
				t.Fatalf("save normalized material: %v", err)
			}
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			var saved report.Material
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatalf("decode saved material: %v", err)
			}
			if err := report.ValidateMaterial(saved); err != nil {
				t.Fatalf("saved material validation: %v", err)
			}
			if saved.Findings[0].Severity != tc.severity || saved.Findings[0].Category != tc.category || saved.Findings[0].SeverityStatus != tc.wantSeverityStatus || saved.Findings[0].SeverityReason != tc.wantSeverityReason || saved.Findings[0].CategoryStatus != tc.wantCategoryStatus || saved.Findings[0].CategoryReason != tc.wantCategoryReason {
				t.Fatalf("saved label facts = severity(%q,%q) category(%q,%q)", saved.Findings[0].Severity, saved.Findings[0].SeverityStatus, saved.Findings[0].Category, saved.Findings[0].CategoryStatus)
			}
		})
	}
}

func TestDefaultReportPathSanitizesRangeRefs(t *testing.T) {
	got := defaultReportPath("repo", requestedReportScope(reviewOptions{from: "release:1", to: "feature/x"}), time.Now())
	if strings.Contains(filepath.Base(got), ":") || !strings.Contains(filepath.Base(got), "range-release-1-to-feature-x") {
		t.Fatalf("unsafe or incomplete range label: %s", filepath.Base(got))
	}
}
