// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestDefaultReportPathIncludesSanitizedRangeLabelsAndStartTime(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "sample repository")
	started := time.Date(2026, 10, 4, 10, 11, 12, 0, time.FixedZone("HKT", 8*60*60))
	scope := report.Scope{Mode: session.InputModeRange, RequestedFrom: "release/main weird", RequestedHead: "feature:long name"}
	name := filepath.Base(defaultReportPath(repoDir, scope, started))
	if strings.ContainsAny(name, ` <>:"/\\|?*`) {
		t.Fatalf("default range report name contains unsafe characters: %q", name)
	}
	if !strings.HasPrefix(name, "sample-repository__range-release-main-weird-to-feature-long-name__") {
		t.Fatalf("default range report name = %q, want both sanitized branch labels", name)
	}
	if !strings.Contains(name, started.Local().Format("20060102-150405")) || !strings.HasSuffix(name, ".report.json") {
		t.Fatalf("default range report name omits local start time or suffix: %q", name)
	}
}

func TestCollectBranchGitStatisticsUsesResolvedRangeAndDistinctGitActors(t *testing.T) {
	repoDir, refs := reportMaterialForkedRepo(t)
	statistics, err := collectBranchGitStatistics(t.Context(), gitcmd.New(4), repoDir, refs.baseSHA, refs.headSHA)
	if err != nil {
		t.Fatalf("collectBranchGitStatistics: %v", err)
	}
	if statistics.BaseSHA != refs.baseSHA || statistics.HeadSHA != refs.headSHA {
		t.Fatalf("statistics range = %s..%s, want %s..%s", statistics.BaseSHA, statistics.HeadSHA, refs.baseSHA, refs.headSHA)
	}
	if statistics.FinalDiff.PathChanges != 2 || statistics.FinalDiff.TextInsertions != 6 || statistics.FinalDiff.TextDeletions != 0 {
		t.Fatalf("final diff statistics = %+v, want two added text paths and six added lines", statistics.FinalDiff)
	}
	if statistics.CommitCount != 2 || len(statistics.PerCommit) != 2 {
		t.Fatalf("commit count = %d, per_commit=%d; want two", statistics.CommitCount, len(statistics.PerCommit))
	}
	if statistics.PerCommitCumulative.PathChanges != 2 || statistics.PerCommitCumulative.TextInsertions != 6 {
		t.Fatalf("per-commit cumulative statistics = %+v, want two added text paths and six added lines", statistics.PerCommitCumulative)
	}
	first := statistics.PerCommit[0]
	if first.Subject != "feature one" || first.Author.Name != "Feature Author" || first.Committer.Name != "Release Integrator" {
		t.Fatalf("first commit author/committer facts were conflated: %+v", first)
	}
	if first.Author.Email != "feature-author@example.test" || first.Committer.Email != "release-integrator@example.test" {
		t.Fatalf("first commit emails = %q/%q", first.Author.Email, first.Committer.Email)
	}
}

func TestReviewE2E_ReportRangeUsesMergeBaseAndPreservesNativeCoverage(t *testing.T) {
	repoDir, refs := reportMaterialForkedRepo(t)
	startFakeLLM(t, newFakeLLM())
	reportPath := filepath.Join(t.TempDir(), "branch report.json")
	stdout, stderr, err := runReportReview(t,
		"--repo", repoDir,
		"--from", refs.fromRef,
		"--to", refs.toRef,
		"--format", "json",
		"--report", reportPath,
	)
	if err != nil {
		t.Fatalf("range review failed: %v\nstderr: %s", err, stderr)
	}
	var native jsonOutput
	if err := json.Unmarshal([]byte(stdout), &native); err != nil {
		t.Fatalf("decode native result: %v\n%s", err, stdout)
	}
	if native.Manifest == nil || native.Manifest.Input.Mode != session.InputModeRange {
		t.Fatalf("native range manifest missing: %+v", native.Manifest)
	}
	var material report.Material
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &material); err != nil {
		t.Fatalf("decode report material: %v", err)
	}
	if err := report.ValidateMaterial(material); err != nil {
		t.Fatalf("validate report material: %v", err)
	}
	if material.Scope.RequestedFrom != refs.fromRef || material.Scope.RequestedHead != refs.toRef ||
		material.Scope.ResolvedBase.SHA != refs.baseSHA || material.Scope.ResolvedHead.SHA != refs.headSHA ||
		material.Scope.ExactRange != refs.baseSHA+".."+refs.headSHA {
		t.Fatalf("material scope = %+v, want raw refs and exact resolved merge-base range", material.Scope)
	}
	var statistics branchGitStatistics
	if err := json.Unmarshal(material.Sections.GitStatistics.Data, &statistics); err != nil {
		t.Fatalf("decode branch statistics: %v\n%s", err, material.Sections.GitStatistics.Data)
	}
	if material.Sections.GitStatistics.Status != report.StatusProvided || statistics.FinalDiff.PathChanges != 2 || statistics.CommitCount != 2 {
		t.Fatalf("branch statistics status/content = %q/%+v", material.Sections.GitStatistics.Status, statistics)
	}
	if len(native.Manifest.Coverage.Selected) != 2 || len(native.Manifest.Coverage.Completed) != 2 ||
		!reflect.DeepEqual(material.Coverage, native.Manifest.Coverage) {
		t.Fatalf("material coverage diverged from native selected/completed/failed/reused/waived sets: native=%+v material=%+v", native.Manifest.Coverage, material.Coverage)
	}
	if len(material.Limitations) == 0 {
		t.Fatal("the material must retain uncollected sections as limitations")
	}
}

func TestReviewE2E_RangeWithoutReportKeepsNativeBehavior(t *testing.T) {
	repoDir, refs := reportMaterialForkedRepo(t)
	startFakeLLM(t, newFakeLLM())
	stdout, stderr, err := runReportReview(t,
		"--repo", repoDir,
		"--from", refs.fromRef,
		"--to", refs.toRef,
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("native range review failed: %v\nstderr: %s", err, stderr)
	}
	var native jsonOutput
	if err := json.Unmarshal([]byte(stdout), &native); err != nil || native.Manifest == nil || native.Manifest.Input.Mode != session.InputModeRange {
		t.Fatalf("native range output changed: decode=%v manifest=%+v", err, native.Manifest)
	}
	paths, err := filepath.Glob(filepath.Join(repoDir, "*.report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("review without --report unexpectedly created materials: %v", paths)
	}
}

type reportMaterialBranchRefs struct {
	fromRef string
	toRef   string
	baseSHA string
	headSHA string
}

func reportMaterialForkedRepo(t *testing.T) (string, reportMaterialBranchRefs) {
	t.Helper()
	repoDir := reportMaterialTestRepo(t)
	baseSHA := reportMaterialGitOutput(t, repoDir, "rev-parse", "HEAD")
	retryTestGit(t, repoDir, "checkout", "-qb", "feature/report")
	writeReportMaterialFile(t, repoDir, "feature-one.go", "package p\n\nfunc featureOne() int { return 1 }\n")
	retryTestGit(t, repoDir, "add", "feature-one.go")
	reportMaterialGitCommitAs(t, repoDir, "Feature Author", "feature-author@example.test", "Release Integrator", "release-integrator@example.test", "feature one")
	writeReportMaterialFile(t, repoDir, "feature-two.go", "package p\n\nfunc featureTwo() int { return 2 }\n")
	retryTestGit(t, repoDir, "add", "feature-two.go")
	reportMaterialGitCommitAs(t, repoDir, "Second Feature Author", "second-feature@example.test", "Second Integrator", "second-integrator@example.test", "feature two")
	headSHA := reportMaterialGitOutput(t, repoDir, "rev-parse", "HEAD")
	retryTestGit(t, repoDir, "checkout", "-qb", "release/main", baseSHA)
	writeReportMaterialFile(t, repoDir, "release-only.go", "package p\n\nfunc releaseOnly() int { return 3 }\n")
	retryTestGit(t, repoDir, "add", "release-only.go")
	reportMaterialGitCommitAs(t, repoDir, "Release Author", "release-author@example.test", "Release Committer", "release-committer@example.test", "release only")
	mergeBase := reportMaterialGitOutput(t, repoDir, "merge-base", "release/main", "feature/report")
	if mergeBase != baseSHA {
		t.Fatalf("fixture merge-base = %s, want %s", mergeBase, baseSHA)
	}
	return repoDir, reportMaterialBranchRefs{fromRef: "release/main", toRef: "feature/report", baseSHA: mergeBase, headSHA: headSHA}
}

func writeReportMaterialFile(t *testing.T, repoDir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func reportMaterialGitCommitAs(t *testing.T, repoDir, authorName, authorEmail, committerName, committerEmail, subject string) {
	t.Helper()
	cmd := exec.Command("git", "-c", "commit.gpgsign=false", "commit", "-q", "-m", subject)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_COMMITTER_NAME="+committerName,
		"GIT_COMMITTER_EMAIL="+committerEmail,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit %q: %v\n%s", subject, err, output)
	}
}

func reportMaterialGitOutput(t *testing.T, repoDir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
