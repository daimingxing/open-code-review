// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
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

func TestEnrichReportSectionsDoesNotInferNonCommitResults(t *testing.T) {
	manifest := &session.RunManifest{Input: session.ManifestInput{Mode: session.InputModeRange}}
	achievements, people, _ := enrichReportSections(t.Context(), nil, t.TempDir(), report.Scope{Mode: session.InputModeRange}, manifest, "", nil, "")
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

func TestKnowledgeSourcesWithoutAccessEvidence(t *testing.T) {
	section := knowledgeSourcesSection()
	if section.Status != report.StatusNotCollected || len(section.Data) != 0 || section.Reason == "" {
		t.Fatalf("migration must not claim knowledge was collected: %+v", section)
	}
}
