// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
)

func knowledgeTestRepo(t *testing.T, inside bool) (string, string) {
	t.Helper()
	repo := t.TempDir()
	kb := filepath.Join(t.TempDir(), "knowledge space \u77e5\u8bc6")
	if inside {
		kb = filepath.Join(repo, "knowledge space \u77e5\u8bc6")
	}
	if err := os.MkdirAll(kb, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	retryTestGit(t, repo, "init", "-q", "-b", "main")
	write(filepath.Join(repo, "main.go"), "package p\nfunc value() int { return 1 }\n")
	if inside {
		write(filepath.Join(kb, "context.go"), "package context\nconst Version = 1\n")
	}
	retryTestGit(t, repo, "add", ".")
	retryTestGit(t, repo, "commit", "-q", "-m", "base")
	write(filepath.Join(repo, "main.go"), "package p\nfunc value() int { return 2 }\n")
	if inside {
		write(filepath.Join(kb, "context.go"), "package context\nconst Version = 2\n")
	}
	retryTestGit(t, repo, "add", ".")
	retryTestGit(t, repo, "commit", "-q", "-m", "target")
	write(filepath.Join(repo, "main.go"), "package p\nfunc value() int { return 3 }\n")
	write(filepath.Join(kb, ".gitignore"), "*.md\n")
	write(filepath.Join(kb, "contract.md"), "index-marker\ndisk-contract\nend-marker\n")
	return repo, kb
}

func lastKnowledgeTool(request map[string]any) string {
	messages, _ := request["messages"].([]any)
	for i := len(messages) - 1; i >= 0; i-- {
		m, _ := messages[i].(map[string]any)
		if m["role"] == "tool" {
			return fmt.Sprint(m["content"])
		}
	}
	return ""
}

func TestKnowledgeCLIReadsSearchesAndPreservesVersion(t *testing.T) {
	for _, mode := range []string{"workspace", "range", "commit"} {
		for _, inside := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/inside=%v", mode, inside), func(t *testing.T) {
				setTestHome(t, t.TempDir())
				repo, kb := knowledgeTestRepo(t, inside)
				var rounds atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request map[string]any
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						http.Error(w, "decode", 400)
						return
					}
					for _, msg := range request["messages"].([]any) {
						m := msg.(map[string]any)
						if m["role"] == "system" && strings.Contains(fmt.Sprint(m["content"]), kb) {
							t.Error("knowledge path leaked into system message")
						}
					}
					if !reportRequestHasTools(request) {
						writeReportText(w, 10, 5, "Summary: changed value\nIssues\nNone\n")
						return
					}
					if !strings.Contains(reportRequestText(request), "<knowledge_bases>") {
						t.Error("missing knowledge task block")
					}
					n := rounds.Add(1)
					switch n {
					case 1:
						writeReportToolCalls(w, 10, 5, reportToolCall("find", "file_find", map[string]any{"directory": kb, "query_name": "contract"}))
					case 2:
						if !strings.Contains(lastKnowledgeTool(request), "contract.md") {
							t.Errorf("real find failed: %s", lastKnowledgeTool(request))
						}
						writeReportToolCalls(w, 10, 5, reportToolCall("search", "code_search", map[string]any{"directory": kb, "search_text": "disk-contract", "file_patterns": []string{"*.md"}}))
					case 3:
						result := lastKnowledgeTool(request)
						if !strings.Contains(result, "2|disk-contract") || !strings.Contains(result, filepath.ToSlash(kb)) {
							t.Errorf("real ignored-file search failed: %s", result)
						}
						writeReportToolCalls(w, 10, 5, reportToolCall("read", "file_read", map[string]any{"file_path": filepath.Join(kb, "contract.md"), "start_line": 2, "end_line": 2}))
					case 4:
						result := lastKnowledgeTool(request)
						if !strings.Contains(result, "LINE_RANGE: 2-2") || !strings.Contains(result, "2|disk-contract") || strings.Contains(result, "1|index-marker") {
							t.Errorf("line read failed: %s", result)
						}
						writeReportToolCalls(w, 10, 5, reportToolCall("source", "file_read", map[string]any{"file_path": "main.go"}))
					case 5:
						value := "2"
						if mode == "workspace" {
							value = "3"
						}
						if !strings.Contains(lastKnowledgeTool(request), "return "+value) {
							t.Errorf("wrong source version: %s", lastKnowledgeTool(request))
						}
						writeReportToolCalls(w, 10, 5, reportToolCall("done", "task_done", map[string]any{"state": "DONE"}))
					default:
						t.Error("unexpected extra model round")
						writeReportToolCalls(w, 10, 5, reportToolCall("done", "task_done", map[string]any{"state": "DONE"}))
					}
				}))
				defer server.Close()
				configureReportLLM(t, server.URL)
				args := []string{"--repo", repo, "--knowledge-dir", kb, "--effort", "low", "--concurrency", "1", "--no-filter", "--format", "json"}
				if mode == "range" {
					args = append(args, "--from", "HEAD~1", "--to", "HEAD")
				}
				if mode == "commit" {
					args = append(args, "--commit", "HEAD")
				}
				stdout, stderr, err := runReportReview(t, args...)
				if err != nil {
					t.Fatalf("CLI failed: %v\n%s", err, stderr)
				}
				var output jsonOutput
				if err := json.Unmarshal([]byte(stdout), &output); err != nil {
					t.Fatal(err)
				}
				if output.Manifest == nil || len(output.Manifest.Coverage.Selected) != 1 || output.Status != string(session.StateComplete) {
					t.Fatalf("wrong code coverage/status: %s", stdout)
				}
				state, err := session.LoadReviewResumeState(repo, output.Manifest.RunID)
				if err != nil || state.KnowledgeRoot != kb || len(state.KnowledgeAccesses) != 3 {
					t.Fatalf("knowledge persistence: %+v, %v", state, err)
				}
				if len(state.KnowledgeAccesses[0].Paths) != 1 || len(state.KnowledgeAccesses[1].Matches) != 1 || state.KnowledgeAccesses[1].Matches[0].Line != 2 || state.KnowledgeAccesses[1].ResultSHA256 == "" {
					t.Fatalf("lookup/search provenance missing: %+v", state.KnowledgeAccesses)
				}
				if mode != "workspace" {
					_, stderr, err = runReportReview(t, append(args, "--resume", output.Manifest.RunID)...)
					if err != nil || rounds.Load() != 5 {
						t.Fatalf("unchanged resume: %v %s rounds=%d", err, stderr, rounds.Load())
					}
					if err := os.WriteFile(filepath.Join(kb, "contract.md"), []byte("changed\n"), 0600); err != nil {
						t.Fatal(err)
					}
					_, _, err = runReportReview(t, append(args, "--resume", output.Manifest.RunID)...)
					if err == nil || !strings.Contains(err.Error(), "knowledge source") {
						t.Fatalf("changed knowledge resume accepted: %v", err)
					}
				}
			})
		}
	}
}

func TestKnowledgeCLIInvalidDirectoryBeforeModel(t *testing.T) {
	repo := reportMaterialTestRepo(t)
	for _, dir := range []string{filepath.Join(repo, "missing"), filepath.Join(repo, "main.go")} {
		_, _, err := runReportReview(t, "--repo", repo, "--knowledge-dir", dir, "--effort", "low")
		if err == nil || !strings.Contains(err.Error(), "knowledge directory") {
			t.Fatalf("invalid directory %s accepted: %v", dir, err)
		}
	}
}
