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

	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestParseWorkspaceStatus(t *testing.T) {
	entries, err := parseWorkspaceStatus([]byte(" M tracked.go\x00A  staged.go\x00?? new.go\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Path != "new.go" || entries[1].IndexStatus != "A" || entries[2].WorktreeStatus != "M" {
		t.Fatalf("workspace entries = %+v", entries)
	}
}

func TestReviewE2E_WorkspaceReportCapturesMixedSnapshot(t *testing.T) {
	repoDir := reportMaterialWorkspaceRepo(t)
	writeReportMaterialFile(t, repoDir, "tracked.go", "package p\n\nfunc tracked() int { return 2 }\n")
	writeReportMaterialFile(t, repoDir, "staged.go", "package p\n\nfunc staged() int { return 1 }\n")
	retryTestGit(t, repoDir, "add", "staged.go")
	if err := os.Remove(filepath.Join(repoDir, "deleted.go")); err != nil {
		t.Fatal(err)
	}
	writeReportMaterialFile(t, repoDir, "new.go", "package p\n\nfunc newValue() int { return 3 }\n")
	startFakeLLM(t, newFakeLLM())
	reportPath := filepath.Join(t.TempDir(), "workspace report.json")
	stdout, stderr, err := runReportReview(t, "--repo", repoDir, "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("workspace review failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}
	var native jsonOutput
	if err := json.Unmarshal([]byte(stdout), &native); err != nil {
		t.Fatalf("decode native result: %v", err)
	}
	data, err := os.ReadFile(reportPath)
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
	if material.Scope.Mode != session.InputModeWorkspace || material.Scope.ResolvedHead.Status != report.StatusNotApplicable {
		t.Fatalf("workspace scope = %+v", material.Scope)
	}
	if material.Scope.SourceArtifact.Status != report.StatusProvided || len(material.Scope.SourceArtifact.Value) != 64 {
		t.Fatalf("workspace source artifact = %+v", material.Scope.SourceArtifact)
	}
	if material.Sections.WorkspaceSnapshot.Status != report.StatusProvided {
		t.Fatalf("workspace snapshot status = %q", material.Sections.WorkspaceSnapshot.Status)
	}
	var snapshot workspaceSnapshot
	if err := json.Unmarshal(material.Sections.WorkspaceSnapshot.Data, &snapshot); err != nil {
		t.Fatalf("decode workspace snapshot: %v", err)
	}
	if snapshot.HeadSHA != material.Scope.ResolvedBase.SHA || snapshot.SourceArtifactSHA256 != material.Scope.SourceArtifact.Value || snapshot.SelectedCount != len(native.Manifest.Coverage.Selected) {
		t.Fatalf("snapshot identity = %+v, native coverage=%d scope=%+v", snapshot, len(native.Manifest.Coverage.Selected), material.Scope)
	}
	statuses := make(map[string]string, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		statuses[entry.Path] = entry.IndexStatus + entry.WorktreeStatus
	}
	for path, want := range map[string]string{"staged.go": "A ", "tracked.go": " M", "deleted.go": " D", "new.go": "??"} {
		if statuses[path] != want {
			t.Fatalf("status for %q = %q, want %q; entries=%+v", path, statuses[path], want, snapshot.Entries)
		}
	}
	if snapshot.SnapshotSHA256 == "" || snapshot.StatusSHA256 == "" || snapshot.CapturedAt.IsZero() {
		t.Fatalf("snapshot identity incomplete: %+v", snapshot)
	}
}

func TestReviewE2E_WorkspaceReportDistinguishesCleanZeroScope(t *testing.T) {
	repoDir := reportMaterialWorkspaceRepo(t)
	startFakeLLM(t, newFakeLLM())
	reportPath := filepath.Join(t.TempDir(), "clean report.json")
	stdout, stderr, err := runReportReview(t, "--repo", repoDir, "--format", "json", "--report", reportPath)
	if err != nil {
		t.Fatalf("clean workspace review failed: %v\nstderr: %s", err, stderr)
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
	var snapshot workspaceSnapshot
	if err := json.Unmarshal(material.Sections.WorkspaceSnapshot.Data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if material.Sections.WorkspaceSnapshot.Status != report.StatusProvided || snapshot.SelectedCount != 0 || len(snapshot.Entries) != 0 || material.Review.Status == session.StateFailed {
		t.Fatalf("clean workspace was not represented as a confirmed zero scope: status=%q snapshot=%+v native=%+v", material.Review.Status, snapshot, native.Manifest)
	}
}

func TestDefaultReportPathUsesWorkspaceLabel(t *testing.T) {
	path := defaultReportPath(filepath.Join(t.TempDir(), "sample repository"), report.Scope{Mode: session.InputModeWorkspace}, timeNowForWorkspaceTest())
	if !strings.Contains(filepath.Base(path), "__workspace__") || !strings.HasSuffix(path, ".report.json") {
		t.Fatalf("workspace report path = %q", path)
	}
}

func timeNowForWorkspaceTest() time.Time {
	return time.Date(2026, 10, 4, 10, 11, 12, 0, time.FixedZone("HKT", 8*60*60))
}

func reportMaterialWorkspaceRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	retryTestGit(t, dir, "init", "-q", "-b", "main")
	writeReportMaterialFile(t, dir, "tracked.go", "package p\n\nfunc tracked() int { return 1 }\n")
	writeReportMaterialFile(t, dir, "deleted.go", "package p\n\nfunc deleted() int { return 1 }\n")
	retryTestGit(t, dir, "add", "tracked.go", "deleted.go")
	retryTestGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}
