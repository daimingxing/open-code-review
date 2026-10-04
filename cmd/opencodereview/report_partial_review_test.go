// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

type reportReviewModeFixture struct {
	name      string
	inputMode string
	args      []string
}

var reportReviewModes = []reportReviewModeFixture{
	{name: "commit", inputMode: session.InputModeCommit, args: []string{"--commit", "HEAD"}},
	{name: "range", inputMode: session.InputModeRange, args: []string{"--from", "HEAD~1", "--to", "HEAD"}},
	{name: "workspace", inputMode: session.InputModeWorkspace},
}

func reportReviewRepoForMode(t *testing.T, mode reportReviewModeFixture) string {
	t.Helper()
	repoDir := retryTestRepo(t)
	if mode.inputMode == session.InputModeWorkspace {
		for marker, name := range markers {
			path := filepath.Join(repoDir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, []byte("\n// workspace "+marker+"\n")...)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return repoDir
}

func reportReviewArgs(repoDir, nativePath, materialPath string, mode reportReviewModeFixture) []string {
	args := []string{"--repo", repoDir, "--format", "json", "--output", nativePath, "--report", materialPath, "--no-filter"}
	return append(args, mode.args...)
}

func readNativeReview(t *testing.T, path string) jsonOutput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read native output: %v", err)
	}
	var native jsonOutput
	if err := json.Unmarshal(data, &native); err != nil {
		t.Fatalf("decode native output: %v\n%s", err, data)
	}
	if native.Manifest == nil {
		t.Fatalf("native output has no run manifest: %s", data)
	}
	return native
}

func TestReviewE2E_ReportPartialAndFailedOutcomesAcrossModes(t *testing.T) {
	for _, mode := range reportReviewModes {
		for _, outcome := range []string{"partial", "failed"} {
			t.Run(mode.name+"/"+outcome, func(t *testing.T) {
				repoDir := reportReviewRepoForMode(t, mode)
				srv := newFakeLLM()
				srv.includeFinding = true
				if outcome == "partial" {
					srv.hardFail["b.go"] = true
				} else {
					srv.failAll()
				}
				startFakeLLM(t, srv)

				nativePath := filepath.Join(t.TempDir(), "native.json")
				materialPath := filepath.Join(t.TempDir(), "review.report.json")
				args := reportReviewArgs(repoDir, nativePath, materialPath, mode)
				_, stderr, runErr := runReportReview(t, args...)
				native := readNativeReview(t, nativePath)
				material := readReportMaterial(t, materialPath)

				if !reflect.DeepEqual(native.Manifest.Coverage, material.Coverage) {
					t.Fatalf("native/report coverage diverged: native=%+v report=%+v", native.Manifest.Coverage, material.Coverage)
				}
				if native.Manifest.RunID != material.Review.RunID || native.Status != string(material.Review.Status) {
					t.Fatalf("native/report identity or status diverged: native=%q/%q report=%q/%q", native.Manifest.RunID, native.Status, material.Review.RunID, material.Review.Status)
				}
				if material.Scope.Mode != mode.inputMode {
					t.Fatalf("report mode = %q, want %q", material.Scope.Mode, mode.inputMode)
				}

				switch outcome {
				case "partial":
					if runErr != nil || native.Status != string(session.StatePartial) || material.Review.Status != session.StatePartial {
						t.Fatalf("partial run error/status = %v/%q/%q; stderr: %s", runErr, native.Status, material.Review.Status, stderr)
					}
					if len(native.Manifest.Coverage.Completed) == 0 || len(native.Manifest.Coverage.Failed) != 1 || len(material.Findings) != len(native.Comments) {
						t.Fatalf("partial result lost usable findings or failure coverage: native comments=%d coverage=%+v report findings=%d", len(native.Comments), native.Manifest.Coverage, len(material.Findings))
					}
					failed := native.Manifest.Coverage.Failed[0]
					if failed.Path != "b.go" || failed.Classification == "" {
						t.Fatalf("partial failure lacks path/classification: %+v", failed)
					}
					foundFailureLimitation := false
					for _, limitation := range material.Limitations {
						if limitation.Source == "coverage.failed."+failed.ItemID && limitation.Status == report.StatusFailed && limitation.Reason != "" {
							foundFailureLimitation = true
						}
					}
					if !foundFailureLimitation {
						t.Fatalf("report did not explain failed review item %q: %+v", failed.ItemID, material.Limitations)
					}
				case "failed":
					if runErr == nil || !strings.Contains(runErr.Error(), "review failed") {
						t.Fatalf("fully failed run must return a diagnostic error, got %v; stderr: %s", runErr, stderr)
					}
					if native.Status != string(session.StateFailed) || material.Review.Status != session.StateFailed || len(native.Comments) != 0 || len(material.Findings) != 0 || len(native.Manifest.Coverage.Failed) == 0 {
						t.Fatalf("fully failed run looks successful or lost diagnostics: native status=%q comments=%d coverage=%+v report status=%q findings=%d", native.Status, len(native.Comments), native.Manifest.Coverage, material.Review.Status, len(material.Findings))
					}
				}
			})
		}
	}
}

func TestReviewE2E_ReportSkippedIsNotAnEmptySuccessfulReview(t *testing.T) {
	repoDir := retryTestRepo(t)
	startFakeLLM(t, newFakeLLM())
	nativePath := filepath.Join(t.TempDir(), "native.json")
	materialPath := filepath.Join(t.TempDir(), "review.report.json")
	args := []string{"--repo", repoDir, "--format", "json", "--output", nativePath, "--report", materialPath, "--exclude", "*.go"}
	_, stderr, err := runReportReview(t, args...)
	if err != nil {
		t.Fatalf("empty workspace review failed: %v\nstderr: %s", err, stderr)
	}
	native := readNativeReview(t, nativePath)
	material := readReportMaterial(t, materialPath)
	if native.Status != string(session.StateSkipped) || material.Review.Status != session.StateSkipped || len(native.Manifest.Coverage.Selected) != 0 || len(native.Comments) != 0 || len(material.Findings) != 0 {
		t.Fatalf("skipped review was represented as another outcome: native=%q manifest=%+v comments=%d report=%q findings=%d", native.Status, native.Manifest.Coverage, len(native.Comments), material.Review.Status, len(material.Findings))
	}
}

func TestReviewE2E_ReportResumePreservesIdentityAndScope(t *testing.T) {
	for _, mode := range reportReviewModes[:2] {
		t.Run(mode.name, func(t *testing.T) {
			repoDir := reportReviewRepoForMode(t, mode)
			srv := newFakeLLM()
			srv.includeFinding = true
			srv.hardFail["b.go"] = true
			startFakeLLM(t, srv)

			parentNativePath := filepath.Join(t.TempDir(), "parent-native.json")
			parentMaterialPath := filepath.Join(t.TempDir(), "parent.report.json")
			parentArgs := reportReviewArgs(repoDir, parentNativePath, parentMaterialPath, mode)
			_, stderr, err := runReportReview(t, parentArgs...)
			if err != nil {
				t.Fatalf("parent partial review failed unexpectedly: %v\nstderr: %s", err, stderr)
			}
			parent := readNativeReview(t, parentNativePath)
			parentMaterial := readReportMaterial(t, parentMaterialPath)
			if parent.Status != string(session.StatePartial) || parentMaterial.Review.RunID != parent.Manifest.RunID || parentMaterial.Scope.Mode != mode.inputMode || parentMaterial.Scope.SourceArtifact.Value != parent.Manifest.Input.SourceArtifactSHA256 {
				t.Fatalf("parent identity/status mismatch: native=%q/%q report=%q", parent.Manifest.RunID, parent.Status, parentMaterial.Review.RunID)
			}

			srv.mu.Lock()
			srv.hardFail["b.go"] = false
			srv.mu.Unlock()
			childNativePath := filepath.Join(t.TempDir(), "child-native.json")
			childMaterialPath := filepath.Join(t.TempDir(), "child.report.json")
			childArgs := reportReviewArgs(repoDir, childNativePath, childMaterialPath, mode)
			childArgs = append(childArgs, "--resume", parent.Manifest.RunID)
			_, stderr, err = runReportReview(t, childArgs...)
			if err != nil {
				t.Fatalf("resumed review failed: %v\nstderr: %s", err, stderr)
			}
			child := readNativeReview(t, childNativePath)
			childMaterial := readReportMaterial(t, childMaterialPath)
			if child.Manifest.RunID == parent.Manifest.RunID || child.Manifest.ParentRunID != parent.Manifest.RunID || childMaterial.Review.RunID != child.Manifest.RunID {
				t.Fatalf("resumed run identity is not linked to parent: parent=%q child=%+v report=%q", parent.Manifest.RunID, child.Manifest, childMaterial.Review.RunID)
			}
			if child.Status != string(session.StateComplete) || childMaterial.Review.Status != session.StateComplete || len(child.Manifest.Coverage.Reused) == 0 || len(child.Manifest.Coverage.Failed) != 0 {
				t.Fatalf("resumed coverage/status is inconsistent: native=%q %+v report=%q", child.Status, child.Manifest.Coverage, childMaterial.Review.Status)
			}
			if !reflect.DeepEqual(child.Manifest.Coverage, childMaterial.Coverage) {
				t.Fatalf("resumed native/report coverage diverged: native=%+v report=%+v", child.Manifest.Coverage, childMaterial.Coverage)
			}
			if child.Manifest.Input.ResolvedBase != parent.Manifest.Input.ResolvedBase || child.Manifest.Input.ResolvedHead != parent.Manifest.Input.ResolvedHead || child.Manifest.Input.ExactRange != parent.Manifest.Input.ExactRange || child.Manifest.Input.SourceArtifactSHA256 != parent.Manifest.Input.SourceArtifactSHA256 {
				t.Fatalf("resume changed the reviewed input identity: parent=%+v child=%+v", parent.Manifest.Input, child.Manifest.Input)
			}
			if childMaterial.Scope.ResolvedBase.SHA != child.Manifest.Input.ResolvedBase || childMaterial.Scope.ResolvedHead.SHA != child.Manifest.Input.ResolvedHead || childMaterial.Scope.ExactRange != child.Manifest.Input.ExactRange {
				t.Fatalf("resumed report scope does not match native input: report=%+v native=%+v", childMaterial.Scope, child.Manifest.Input)
			}
			if childMaterial.Scope.Mode != mode.inputMode || childMaterial.Scope.SourceArtifact.Value != child.Manifest.Input.SourceArtifactSHA256 {
				t.Fatalf("resumed report mode or source identity does not match native manifest: report repo=%+v scope=%+v native repo=%+v input=%+v", childMaterial.Repository, childMaterial.Scope, child.Manifest.Repository, child.Manifest.Input)
			}
			if child.Manifest.Repository.IdentitySHA256 == "" && childMaterial.Repository.Identity.Status != report.StatusNotCollected {
				t.Fatalf("report invented a repository identity absent from native manifest: %+v", childMaterial.Repository.Identity)
			}
		})
	}
}

func TestReviewE2E_ReportDoesNotAddWorkspaceResume(t *testing.T) {
	startFakeLLM(t, newFakeLLM())
	repoDir := retryTestRepo(t)
	materialPath := filepath.Join(t.TempDir(), "workspace.report.json")
	_, stderr, err := runReportReview(t, "--repo", repoDir, "--resume", "workspace-session", "--report", materialPath)
	if err == nil || !strings.Contains(err.Error(), "workspace resume is not supported") {
		t.Fatalf("workspace resume should remain unsupported, got %v; stderr: %s", err, stderr)
	}
	if _, statErr := os.Stat(materialPath); !os.IsNotExist(statErr) {
		t.Fatalf("unsupported workspace resume left report material: stat error=%v", statErr)
	}
}
