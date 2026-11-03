// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/report"
)

func TestKnowledgeMaterialSurvivesBudgetStop(t *testing.T) {
	setTestHome(t, t.TempDir())
	repo, kb := knowledgeTestRepo(t, false)
	var rounds atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			http.Error(w, "decode", 400)
			return
		}
		if !reportRequestHasTools(req) {
			writeReportText(w, 10, 5, "Summary: value changed\nIssues\nNone")
			return
		}
		if rounds.Add(1) == 1 {
			writeReportToolCalls(w, 50001, 5, reportToolCall("read", "file_read", map[string]any{"file_path": filepath.Join(kb, "contract.md"), "start_line": 2, "end_line": 2}))
			return
		}
		if !strings.Contains(reportRequestText(req), "disk-contract") {
			t.Error("budget stop lost the completed tool result")
		}
		writeReportToolCalls(w, 10, 5, reportToolCall("done", "task_done", map[string]any{"state": "FAILED"}))
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	path := filepath.Join(t.TempDir(), "partial.report.json")
	_, stderr, err := runReportReview(t, "--repo", repo, "--commit", "HEAD", "--knowledge-dir", kb, "--effort", "low", "--concurrency", "1", "--max-tokens-budget", "50000", "--no-filter", "--report="+path)
	if err == nil {
		t.Fatal("incomplete budget stop should fail when no file was covered")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("partial material lost: %v %s %v", readErr, stderr, err)
	}
	material, err := report.DecodeMaterial(data)
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Accesses []struct {
			StartLine int `json:"start_line"`
		} `json:"accesses"`
	}
	if err := json.Unmarshal(material.Sections.KnowledgeSources.Data, &evidence); err != nil {
		t.Fatal(err)
	}
	if material.Sections.KnowledgeSources.Status != report.StatusProvided || len(evidence.Accesses) != 1 || evidence.Accesses[0].StartLine != 2 || rounds.Load() != 2 {
		t.Fatalf("partial evidence missing or loop did not stop: %+v rounds=%d", material.Sections.KnowledgeSources, rounds.Load())
	}
}
