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
	"github.com/alibaba/open-code-review/internal/llmloop"
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
	if achievements.CollectionElapsedMS < 0 || achievements.CollectionUsageStatus != "not_applicable" || achievements.ModelSummaryUsageStatus != "not_collected" {
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
	startFakeLLM(t, newFakeLLM())
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
}

func TestKnowledgeSourcesSectionDistinguishesObservedPartialAndFailure(t *testing.T) {
	manifest := &session.RunManifest{Execution: session.ManifestExecution{RuleConfigSHA256: strings.Repeat("a", 64)}}
	success := knowledgeSourcesSection(manifest, map[string]int64{"read_file": 2}, nil)
	if success.Status != report.StatusProvided {
		t.Fatalf("observed knowledge status = %q", success.Status)
	}
	var successData knowledgeMaterial
	if err := json.Unmarshal(success.Data, &successData); err != nil || successData.Status != "observed" || successData.KnowledgeVersion != "not_observed" || successData.ApplicationStatus != "not_observed" {
		t.Fatalf("observed knowledge data = %+v, err=%v", successData, err)
	}
	failures := []llmloop.ToolFailureDetail{{ToolName: "read_file"}}
	partial := knowledgeSourcesSection(manifest, map[string]int64{"read_file": 2}, failures)
	if partial.Status != report.StatusFailed {
		t.Fatalf("partial knowledge status = %q", partial.Status)
	}
	var partialData knowledgeMaterial
	if err := json.Unmarshal(partial.Data, &partialData); err != nil || partialData.Status != "partial" || len(partialData.FailedTools) != 1 {
		t.Fatalf("partial knowledge data = %+v, err=%v", partialData, err)
	}
	failure := knowledgeSourcesSection(manifest, nil, failures)
	if failure.Status != report.StatusFailed {
		t.Fatalf("failed knowledge status = %q", failure.Status)
	}
	attemptFailure := knowledgeSourcesSection(manifest, map[string]int64{"read_file": 1}, failures)
	if attemptFailure.Status != report.StatusFailed {
		t.Fatalf("all failed knowledge attempts status = %q, want failed", attemptFailure.Status)
	}
}

func TestEnrichReportSectionsDoesNotInferNonCommitResults(t *testing.T) {
	manifest := &session.RunManifest{Input: session.ManifestInput{Mode: session.InputModeRange}}
	achievements, people, _ := enrichReportSections(t.Context(), nil, t.TempDir(), report.Scope{Mode: session.InputModeRange}, manifest, nil, nil, "")
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
}
