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

	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/report"
)

func TestKnowledgeMaterialCLIReadFailureChangeAndResume(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure-and-resume", true: "changed-after-read"}[changed], func(t *testing.T) {
			setTestHome(t, t.TempDir())
			repo, kb := knowledgeTestRepo(t, true)
			path := filepath.Join(kb, "contract.md")
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
				switch rounds.Add(1) {
				case 1:
					writeReportToolCalls(w, 10, 5, reportToolCall("missing", "file_read", map[string]any{"file_path": filepath.Join(kb, "missing.md")}))
				case 2:
					if !strings.Contains(lastKnowledgeTool(req), "not found") {
						t.Error("missing read was not fed back")
					}
					writeReportToolCalls(w, 10, 5, reportToolCall("read", "file_read", map[string]any{"file_path": path, "start_line": 2, "end_line": 2}))
				case 3:
					if !strings.Contains(lastKnowledgeTool(req), "2|disk-contract") {
						t.Error("no real partial read")
					}
					if changed {
						if err := os.WriteFile(path, []byte("changed during review\n"), 0600); err != nil {
							t.Error(err)
						}
					}
					writeReportToolCalls(w, 10, 5, reportToolCall("done", "task_done", map[string]any{"state": "DONE"}))
				default:
					t.Error("unexpected model round during reuse")
					writeReportToolCalls(w, 10, 5, reportToolCall("done", "task_done", map[string]any{"state": "DONE"}))
				}
			}))
			defer server.Close()
			configureReportLLM(t, server.URL)
			materialPath := filepath.Join(t.TempDir(), "material.json")
			args := []string{"--repo", repo, "--commit", "HEAD", "--knowledge-dir", kb, "--effort", "low", "--concurrency", "1", "--no-filter", "--format", "json", "--report", materialPath}
			stdout, stderr, err := runReportReview(t, args...)
			if err != nil {
				t.Fatalf("review: %v %s", err, stderr)
			}
			readMaterial := func(path string) report.Material {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				material, err := report.DecodeMaterial(data)
				if err != nil {
					t.Fatal(err)
				}
				return material
			}
			material := readMaterial(materialPath)
			section := material.Sections.KnowledgeSources
			if section.Status != report.StatusFailed || section.Reason == "" {
				t.Fatalf("missing limitation: %+v", section)
			}
			var data struct {
				Accesses    []knowledge.Access `json:"accesses"`
				Application string             `json:"application_status"`
			}
			if err := json.Unmarshal(section.Data, &data); err != nil {
				t.Fatal(err)
			}
			if len(data.Accesses) != 2 || data.Accesses[0].Status != "failed" || !data.Accesses[1].Partial || data.Accesses[1].StartLine != 2 || data.Accesses[1].EndLine != 2 || data.Accesses[1].SHA256 == "" || data.Application != "not_verified" || data.Accesses[1].Changed != changed {
				t.Fatalf("incorrect source facts: %+v", data)
			}
			if changed {
				return
			}
			var output jsonOutput
			if err := json.Unmarshal([]byte(stdout), &output); err != nil {
				t.Fatal(err)
			}
			resumedPath := filepath.Join(t.TempDir(), "resumed.json")
			resumedArgs := append([]string{}, args...)
			resumedArgs[len(resumedArgs)-1] = resumedPath
			_, stderr, err = runReportReview(t, append(resumedArgs, "--resume", output.Manifest.RunID)...)
			if err != nil {
				t.Fatalf("resume report: %v %s", err, stderr)
			}
			resumed := readMaterial(resumedPath)
			if err := json.Unmarshal(resumed.Sections.KnowledgeSources.Data, &data); err != nil {
				t.Fatal(err)
			}
			if len(data.Accesses) != 2 || !data.Accesses[0].Inherited || !data.Accesses[1].Inherited || rounds.Load() != 3 {
				t.Fatalf("lost/repeated source reads on resume: %+v rounds=%d", data, rounds.Load())
			}
		})
	}
}

func TestKnowledgeMaterialDistinguishesConfigurationAndLookup(t *testing.T) {
	base, err := knowledge.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if section := nativeKnowledgeSection(nil); section.Status != report.StatusNotCollected {
		t.Fatalf("unconfigured: %+v", section)
	}
	if section := nativeKnowledgeSection(base); section.Status != report.StatusNotCollected {
		t.Fatalf("unused directory: %+v", section)
	}
	base.Record(knowledge.Access{Operation: "file_find", Path: base.Root, Status: "listed", Partial: true})
	section := nativeKnowledgeSection(base)
	if section.Status != report.StatusProvided || !strings.Contains(string(section.Data), `"reading_status":"not_read"`) {
		t.Fatalf("lookup is not a document read: %+v", section)
	}
}
