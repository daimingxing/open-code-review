// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)


func TestReportCommandAgentLoopReadsMaterialsWritesHTMLAndFinishes(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestLongMaterial()
	input := writeReportInput(t, material)
	var calls atomic.Int32
	requests := make(chan map[string]any, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- request
		n := calls.Add(1)
		switch n {
		case 1:
			if _, ok := request["tools"]; !ok {
				http.Error(w, "tools missing", http.StatusBadRequest)
				return
			}
			writeReportToolCalls(w, 100, 20, reportToolCall("call_read", "file_read", map[string]any{"file_path": "ignored.json"}))
		case 2:
			writeReportToolCalls(w, 120, 40,
				reportToolCall("call_write", "write_html", map[string]any{"html": reportHTMLFixture(material)}),
				reportToolCall("call_done", "task_done", map[string]any{"state": "DONE"}),
			)
		default:
			http.Error(w, "unexpected extra request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	out := filepath.Join(t.TempDir(), "agent-loop.html")
	stdout, stderr, err := executeReportCommand([]string{"--input", input, "--output", out})
	if err != nil {
		t.Fatalf("report command failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, out) || !strings.Contains(stderr, "phase=html_generation") || !strings.Contains(stderr, "usage=reported") {
		t.Fatalf("diagnostics incomplete: stdout=%q stderr=%q", stdout, stderr)
	}
	first := <-requests
	messages := first["messages"].([]any)
	if len(messages) < 2 {
		t.Fatalf("initial messages = %#v", messages)
	}
	system := fmt.Sprint(messages[0].(map[string]any)["content"])
	user := fmt.Sprint(messages[1].(map[string]any)["content"])
	if strings.Contains(user, `"schema_version"`) {
		t.Fatalf("initial user message stuffed report JSON: %s", user)
	}
	if !strings.Contains(user, "input.report.json") || !strings.Contains(user, "template.md") || !strings.Contains(user, "styles.css") {
		t.Fatalf("initial user message missing staged resource paths: %s", user)
	}
	if !strings.Contains(system, "Prefer the staged report JSON") || !strings.Contains(system, "Avoid re-reading") {
		t.Fatalf("system prompt missing material-first guidance: %s", system)
	}
	toolNames := reportToolNames(first["tools"])
	for _, name := range []string{"file_read", "file_find", "code_search", "write_html", "task_done"} {
		if !slices.Contains(toolNames, name) {
			t.Fatalf("tools = %v, missing %s", toolNames, name)
		}
	}
	if slices.Contains(toolNames, "code_comment") {
		t.Fatalf("report exposed code_comment: %v", toolNames)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	writeReportBrowserEvidence(t, "OCR_REPORT_HTML_EVIDENCE_FILE", data)
	if !strings.Contains(string(data), `data-severity="critical"`) {
		t.Fatal("saved HTML omitted expected finding markers from the draft")
	}

	zero := reportTestMaterial(false)
	zeroInput := writeReportInput(t, zero)
	zeroOut := filepath.Join(t.TempDir(), "zero.html")
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeReportToolCalls(w, 10, 10,
			reportToolCall("w", "write_html", map[string]any{"html": reportHTMLFixture(zero)}),
			reportToolCall("d", "task_done", map[string]any{"state": "DONE"}),
		)
	}))
	defer server2.Close()
	configureReportLLM(t, server2.URL)
	if _, stderr, err := executeReportCommand([]string{"--input", zeroInput, "--output", zeroOut}); err != nil {
		t.Fatalf("zero-finding report failed: %v\nstderr: %s", err, stderr)
	}
	zeroHTML, err := os.ReadFile(zeroOut)
	if err != nil {
		t.Fatal(err)
	}
	writeReportBrowserEvidence(t, "OCR_REPORT_HTML_SHORT_EVIDENCE_FILE", zeroHTML)

	scripted := "<!doctype html><html lang=\"zh-CN\"><head><title>x</title><style>body{}</style></head><body><main><script>alert(1)</script><p>free form</p></main></body></html>"
	scriptOut := filepath.Join(t.TempDir(), "script.html")
	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeReportToolCalls(w, 10, 10,
			reportToolCall("w", "write_html", map[string]any{"html": scripted}),
			reportToolCall("d", "task_done", map[string]any{"state": "DONE"}),
		)
	}))
	defer server3.Close()
	configureReportLLM(t, server3.URL)
	if _, _, err := executeReportCommand([]string{"--input", zeroInput, "--output", scriptOut}); err != nil {
		t.Fatalf("script-containing HTML was rejected: %v", err)
	}
	got, err := os.ReadFile(scriptOut)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "<script>alert(1)</script>") {
		t.Fatal("delivered HTML was altered or rejected for script content")
	}
}

func TestReportCommandFailsWhenHTMLNeverWritten(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeReportToolCalls(w, 5, 5, reportToolCall("d", "task_done", map[string]any{"state": "DONE"}))
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	out := filepath.Join(t.TempDir(), "missing.html")
	_, stderr, err := executeReportCommand([]string{"--input", input, "--output", out})
	if err == nil || !strings.Contains(err.Error(), "write_html was never called") {
		t.Fatalf("error = %v stderr=%s, want missing write_html", err, stderr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("missing write published a file: %v", err)
	}
}

func TestReportCommandModelFailureDoesNotPublish(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)
	secret := `C:\Users\private\request-body bearer=do-not-print`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"error":{"message":%q}}`, secret)
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	out := filepath.Join(t.TempDir(), "provider-fail.html")
	stdout, stderr, err := executeReportCommand([]string{"--input", input, "--output", out})
	if err == nil {
		t.Fatal("provider failure was accepted")
	}
	if strings.Contains(stdout+stderr+err.Error(), secret) || strings.Contains(stdout+stderr, "request-body") {
		t.Fatalf("provider body leaked: stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("failed generation published HTML: %v", err)
	}
}

func TestReportCommandStopsAfterCallerCancellation(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		cancel()
		writeReportToolCalls(w, 5, 5,
			reportToolCall("w", "write_html", map[string]any{"html": reportHTMLFixture(material)}),
			reportToolCall("d", "task_done", map[string]any{"state": "DONE"}),
		)
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	output := filepath.Join(t.TempDir(), "canceled.html")
	cmd := newReportCommand()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--input", input, "--output", output})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if calls.Load() != 1 {
		t.Fatalf("cancellation retried the model: calls=%d stderr=%s", calls.Load(), stderr.String())
	}
}

func TestReportCommandRejectsExistingExplicitOutputBeforeLoop(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)
	out := filepath.Join(t.TempDir(), "exists.html")
	if err := os.WriteFile(out, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeReportToolCalls(w, 5, 5, reportToolCall("d", "task_done", map[string]any{"state": "DONE"}))
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	_, _, err := executeReportCommand([]string{"--input", input, "--output", out})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("model was called despite existing output: %d", calls.Load())
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != "keep" {
		t.Fatalf("existing file was modified: %q err=%v", got, err)
	}
}

func TestReportCommandMultiInputPreservesOrderAndRejectsDuplicateBeforeModel(t *testing.T) {
	setTestHome(t, t.TempDir())
	var calls atomic.Int32
	requests := make(chan map[string]any, 4)
	first := reportTestMaterial(true)
	first.Review.RunID = "multi-partial-first"
	first.Repository.Name = "shared-repository"
	first.Scope.ExactRange = strings.Repeat("b", 40) + ".." + strings.Repeat("a", 40)
	first.Findings[0].Severity = ""
	first.Findings[0].SeverityStatus = report.StatusNotCollected
	first.Findings[0].SeverityReason = "severity not collected"
	first.Findings[0].Display.SeverityZH = "\u672a\u63d0\u4f9b"
	first = withReportTestPeople(first, "Alice Chen", "alice@example.com", "Sam Lee", "")
	second := reportTestMaterial(false)
	second.Review.RunID = "multi-complete-overlap"
	second.Repository.Name = "shared-repository"
	second.Scope.ResolvedHead.SHA = strings.Repeat("c", 40)
	second.Scope.ExactRange = strings.Repeat("b", 40) + ".." + second.Scope.ResolvedHead.SHA
	third := reportTestMaterial(true)
	third.Review.RunID = "multi-partial-other"
	third.Repository.Name = "another-repository"
	third.Scope.ResolvedHead.SHA = strings.Repeat("d", 40)
	third.Scope.ExactRange = strings.Repeat("b", 40) + ".." + third.Scope.ResolvedHead.SHA
	third = withReportTestPeople(third, "A. Chen", "alice@example.com", "Sam Lee", "")
	paths := []string{writeReportInput(t, first), writeReportInput(t, second), writeReportInput(t, third)}
	ordered, err := report.NewMultiReportInput([]report.Material{first, second, third})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := report.NewMultiReportInput([]report.Material{third, second, first})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- request
		n := int(calls.Add(1))
		htmlDoc := reportMultiHTMLFixture(ordered)
		if n > 1 {
			htmlDoc = reportMultiHTMLFixture(reordered)
		}
		writeReportToolCalls(w, 80, 40,
			reportToolCall("w", "write_html", map[string]any{"html": htmlDoc}),
			reportToolCall("d", "task_done", map[string]any{"state": "DONE"}),
		)
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	outDir := t.TempDir()
	firstOutput := filepath.Join(outDir, "multi-first.html")
	if _, stderr, err := executeReportCommand([]string{"--input", paths[0], "--input", paths[1], "--input", paths[2], "--output", firstOutput}); err != nil {
		t.Fatalf("multi-input report failed: %v\nstderr: %s", err, stderr)
	}
	firstReq := <-requests
	user := fmt.Sprint(firstReq["messages"].([]any)[1].(map[string]any)["content"])
	if !strings.Contains(user, "input-1.report.json") || !strings.Contains(user, "input-2.report.json") || !strings.Contains(user, "input-3.report.json") {
		t.Fatalf("ordered prompt missing staged inputs: %s", user)
	}
	pos1, pos2, pos3 := strings.Index(user, "input-1.report.json"), strings.Index(user, "input-2.report.json"), strings.Index(user, "input-3.report.json")
	if !(pos1 < pos2 && pos2 < pos3) {
		t.Fatalf("input paths are not in order: %s", user)
	}
	firstHTML, err := os.ReadFile(firstOutput)
	if err != nil {
		t.Fatal(err)
	}
	writeMultiBrowserEvidence(t, "OCR_MULTI_REPORT_HTML_EVIDENCE_FILE", firstHTML)
	if !strings.Contains(string(firstHTML), first.Review.RunID) || !strings.Contains(string(firstHTML), third.Review.RunID) {
		t.Fatal("multi HTML omitted review unit run ids")
	}

	secondOutput := filepath.Join(outDir, "multi-reordered.html")
	if _, stderr, err := executeReportCommand([]string{"--input", paths[2], "--input", paths[1], "--input", paths[0], "--output", secondOutput}); err != nil {
		t.Fatalf("reordered multi-input report failed: %v\nstderr: %s", err, stderr)
	}
	secondHTML, err := os.ReadFile(secondOutput)
	if err != nil {
		t.Fatal(err)
	}
	writeMultiBrowserEvidence(t, "OCR_MULTI_REPORT_HTML_REORDERED_EVIDENCE_FILE", secondHTML)
	<-requests

	alias := filepath.Join(t.TempDir(), "same-material-different-name.json")
	materialBytes, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alias, materialBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeDuplicate := calls.Load()
	duplicateOutput := filepath.Join(outDir, "duplicate.html")
	if _, _, err := executeReportCommand([]string{"--input", paths[0], "--input", alias, "--output", duplicateOutput}); err == nil || !strings.Contains(err.Error(), "run_id") {
		t.Fatalf("duplicate material was accepted: %v", err)
	}
	if calls.Load() != beforeDuplicate {
		t.Fatalf("duplicate input reached the model")
	}
}

func TestReportCommandMultiInputCanRetryAfterModelFailure(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	material.Review.RunID = "retry-unit-one"
	second := reportTestMaterial(true)
	second.Review.RunID = "retry-unit-two"
	paths := []string{writeReportInput(t, material), writeReportInput(t, second)}
	input, err := report.NewMultiReportInput([]report.Material{material, second})
	if err != nil {
		t.Fatal(err)
	}
	failing := atomic.Bool{}
	failing.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"message":"temporary"}}`))
			return
		}
		writeReportToolCalls(w, 20, 20,
			reportToolCall("w", "write_html", map[string]any{"html": reportMultiHTMLFixture(input)}),
			reportToolCall("d", "task_done", map[string]any{"state": "DONE"}),
		)
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	out := filepath.Join(t.TempDir(), "retry.html")
	args := []string{"--input", paths[0], "--input", paths[1], "--output", out}
	if _, _, err := executeReportCommand(args); err == nil {
		t.Fatal("first failing invocation succeeded")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("failed run left output: %v", err)
	}
	failing.Store(false)
	if _, stderr, err := executeReportCommand(args); err != nil {
		t.Fatalf("retry failed: %v\nstderr: %s", err, stderr)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	writeReportBrowserEvidence(t, "OCR_REPORT_HTML_SHORT_MULTI_EVIDENCE_FILE", data)
}

func TestReportCommandWritesLongMultiBrowserEvidence(t *testing.T) {
	evidencePath := os.Getenv("OCR_REPORT_HTML_LONG_MULTI_EVIDENCE_FILE")
	if evidencePath == "" {
		t.Skip("browser evidence path is not configured")
	}
	setTestHome(t, t.TempDir())
	first := reportTestLongMaterial()
	first.Review.RunID = "long-multi-frontend"
	first.Repository.Name = "frontend-repository"
	second := reportTestMaterial(false)
	second.Review.RunID = "long-multi-backend"
	second.Repository.Name = "backend-repository"
	paths := []string{writeReportInput(t, first), writeReportInput(t, second)}
	input, err := report.NewMultiReportInput([]report.Material{first, second})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeReportToolCalls(w, 50, 50,
			reportToolCall("w", "write_html", map[string]any{"html": reportMultiHTMLFixture(input)}),
			reportToolCall("d", "task_done", map[string]any{"state": "DONE"}),
		)
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	target := filepath.Join(t.TempDir(), "long-multi.html")
	if _, stderr, err := executeReportCommand([]string{"--input", paths[0], "--input", paths[1], "--output", target}); err != nil {
		t.Fatalf("long multi-input report failed: %v\nstderr: %s", err, stderr)
	}
	document, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	writeReportBrowserEvidence(t, "OCR_REPORT_HTML_LONG_MULTI_EVIDENCE_FILE", document)
}

func TestReportCommandRejectsInvalidInputAndTemplateBeforeModelCall(t *testing.T) {
	cmd := newReportCommand()
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "at least once") {
		t.Fatalf("missing input error = %v", err)
	}
	input := filepath.Join(t.TempDir(), "native.json")
	if err := os.WriteFile(input, []byte(`{"comments":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd = newReportCommand()
	cmd.SetArgs([]string{"--input", input, "--template", "unknown"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown report template") {
		t.Fatalf("unknown template error = %v", err)
	}
	cmd = newReportCommand()
	cmd.SetArgs([]string{"--input", input})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("native JSON input error = %v", err)
	}
}

func TestReportCommandUsesDateDefaultOutputWithoutOverwriting(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeReportToolCalls(w, 10, 10,
			reportToolCall("w", "write_html", map[string]any{"html": reportHTMLFixture(material)}),
			reportToolCall("d", "task_done", map[string]any{"state": "DONE"}),
		)
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	dir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	cmd := newReportCommand()
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	fixedDate := time.Date(2026, 10, 4, 10, 11, 12, 0, time.Local)
	if err := runHTMLReport(cmd, reportOptions{inputs: []string{input}, template: report.DefaultHTMLTemplate, generatedAt: func() time.Time { return fixedDate }}); err != nil {
		t.Fatalf("default output failed: %v\nstderr: %s", err, stderr.String())
	}
	defaultPath := filepath.Join(dir, "report-2026-10-04.html")
	if _, err := os.Stat(defaultPath); err != nil {
		t.Fatalf("default date output missing: %v", err)
	}
	if err := runHTMLReport(cmd, reportOptions{inputs: []string{input}, template: report.DefaultHTMLTemplate, generatedAt: func() time.Time { return fixedDate }}); err != nil {
		t.Fatalf("automatic numbered output failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "report-2026-10-04(1).html")); err != nil {
		t.Fatalf("automatic output did not avoid overwrite: %v", err)
	}
}

func TestReportCommandReusesSharedTemplateCompressionSettings(t *testing.T) {
	tpl, err := template.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if tpl.MaxTokens <= 0 {
		t.Fatalf("shared MaxTokens unset: %d", tpl.MaxTokens)
	}
	if len(tpl.MemoryCompressionTask.Messages) == 0 {
		t.Fatal("shared memory compression task missing")
	}
	if tpl.MaxToolRequestTimes <= 0 {
		t.Fatalf("shared MaxToolRequestTimes unset: %d", tpl.MaxToolRequestTimes)
	}
	cmd := newReportCommand()
	if strings.Contains(cmd.Long, "3 attempts") || strings.Contains(cmd.Long, "128,000") {
		t.Fatalf("command help still describes removed report-specific budgets: %s", cmd.Long)
	}
}

func executeReportCommand(args []string) (string, string, error) {
	cmd := newReportCommand()
	cmd.SetArgs(args)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func configureReportLLM(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("OCR_LLM_URL", baseURL+"/v1")
	t.Setenv("OCR_LLM_TOKEN", "report-test-token")
	t.Setenv("OCR_LLM_MODEL", "fake-report-model")
	t.Setenv("OCR_LLM_PROTOCOL", "openai")
}

func reportToolCall(id, name string, args map[string]any) map[string]any {
	payload, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return map[string]any{
		"id": id, "type": "function",
		"function": map[string]any{"name": name, "arguments": string(payload)},
	}
}

func writeReportToolCalls(w http.ResponseWriter, inputTokens, outputTokens int, calls ...map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "report-test", "model": "fake-report-model",
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": "tool_calls",
			"message": map[string]any{"role": "assistant", "content": nil, "tool_calls": calls},
		}},
		"usage": map[string]any{"prompt_tokens": inputTokens, "completion_tokens": outputTokens, "total_tokens": inputTokens + outputTokens},
	})
}

func reportToolNames(raw any) []string {
	tools, ok := raw.([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fn, ok := tool["function"].(map[string]any)
		if !ok {
			continue
		}
		names = append(names, fmt.Sprint(fn["name"]))
	}
	return names
}

func reportTestMaterial(withFindings bool) report.Material {
	now := time.Date(2026, 10, 4, 10, 11, 12, 0, time.FixedZone("HKT", 8*60*60))
	material := report.Material{
		SchemaVersion: report.MaterialSchemaVersion,
		Review: report.ReviewMetadata{
			RunID: "report-run-07", StartedAt: now, CompletedAt: now.Add(time.Second),
			ElapsedMS: 1000, Status: session.StatePartial, Provider: "test", Model: "review-model",
		},
		Repository: report.Repository{Name: "sample", Identity: report.Fact{Status: report.StatusNotCollected, Reason: "repository identity not collected"}},
		Scope: report.Scope{
			Mode: session.InputModeCommit, RequestedHead: "HEAD",
			ResolvedHead:   report.Revision{Status: report.StatusProvided, SHA: strings.Repeat("a", 40)},
			ResolvedBase:   report.Revision{Status: report.StatusProvided, SHA: strings.Repeat("b", 40)},
			ExactRange:     strings.Repeat("b", 40) + ".." + strings.Repeat("a", 40),
			SourceArtifact: report.Fact{Status: report.StatusNotApplicable, Reason: "commit input has no source artifact"},
		},
		Findings: []report.Finding{},
		Coverage: session.Coverage{
			Selected: []session.CoverageItem{}, Completed: []session.CoverageItem{}, Reused: []session.CoverageItem{},
			Failed: []session.CoverageItem{}, Waived: []session.CoverageItem{},
		},
		Sections: report.MaterialSections{
			GitStatistics:     report.Section{Status: report.StatusNotCollected, Reason: "git statistics not collected"},
			WorkspaceSnapshot: report.Section{Status: report.StatusNotApplicable, Reason: "not a workspace review"},
			Achievements:      report.Section{Status: report.StatusNotCollected, Reason: "achievements not collected"},
			People:            report.Section{Status: report.StatusNotCollected, Reason: "people not collected"},
			KnowledgeSources:  report.Section{Status: report.StatusNotCollected, Reason: "knowledge sources not collected"},
			StructuralChecks:  report.Section{Status: report.StatusNotCollected, Reason: "structural checks not collected"},
		},
		Limitations: []report.Limitation{{Source: "knowledge_sources", Status: report.StatusNotCollected, Reason: "knowledge sources not collected"}},
	}
	if !withFindings {
		material.Review.Status = session.StateComplete
		material.Limitations = []report.Limitation{}
		return material
	}
	itemID := strings.Repeat("c", 64)
	material.Coverage.Selected = []session.CoverageItem{{ItemID: itemID, Path: "src/a.go"}}
	material.Coverage.Failed = []session.CoverageItem{{ItemID: itemID, Path: "src/a.go", Classification: session.FailureProvider, Reason: "model request failed"}}
	for index, severity := range []string{"critical", "high", "medium", "low"} {
		labels := map[string]string{"critical": "\u4e25\u91cd", "high": "\u9ad8", "medium": "\u4e2d", "low": "\u4f4e"}
		finding := report.Finding{
			ID: "sha256:" + strings.Repeat(string(rune('1'+index)), 64), Path: fmt.Sprintf("src/%c.go", 'a'+index),
			StartLine: index + 1, EndLine: index + 1, Severity: severity, SeverityStatus: report.StatusProvided,
			Category: "bug", CategoryStatus: report.StatusProvided, SourceContent: "return secret <& token",
			Display:        report.FindingDisplay{SummaryZH: "\u4e0d\u7a33\u5b9a\u7ed3\u679c", SeverityZH: labels[severity], CategoryZH: "\u7f3a\u9677"},
			Evidence:       report.FindingEvidence{Status: report.StatusProvided, Code: "return secret <& token"},
			Recommendation: report.FindingRecommendation{Status: report.StatusProvided, Code: "validate the value"},
		}
		if index > 0 {
			finding.Evidence = report.FindingEvidence{Status: report.StatusNotCollected, Reason: "independent evidence not collected"}
			finding.Recommendation = report.FindingRecommendation{Status: report.StatusNotCollected, Reason: "recommendation not collected"}
		}
		material.Findings = append(material.Findings, finding)
	}
	return material
}

func reportTestLongMaterial() report.Material {
	material := reportTestMaterial(true)
	base := append([]report.Finding(nil), material.Findings...)
	for index := len(base); index < 60; index++ {
		finding := base[index%len(base)]
		finding.ID = "sha256:" + fmt.Sprintf("%064x", index+100)
		finding.Path = fmt.Sprintf("src/long-%02d.go", index)
		finding.StartLine = index + 1
		finding.EndLine = index + 2
		finding.SourceContent = strings.Repeat(fmt.Sprintf("line %d: return secret <& token. ", index), 8)
		material.Findings = append(material.Findings, finding)
	}
	return material
}

func writeReportInput(t *testing.T, material report.Material) string {
	t.Helper()
	data, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "input.report.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func reportHTMLFixture(material report.Material) string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>审查报告</title></head><body>`) // allow-non-english: fixture is the model's Chinese HTML response
	fmt.Fprintf(&builder, `<main data-review-status="%s" data-run-id="%s">`, html.EscapeString(string(material.Review.Status)), html.EscapeString(material.Review.RunID))
	sections := []struct{ id, title string }{
		{"overview", "\u62a5\u544a\u6982\u89c8"}, {"quality-coverage", "\u8d28\u91cf\u4e0e\u8986\u76d6"},
		{"finding-details", "\u95ee\u9898\u660e\u7ec6"}, {"changes", "\u4ed3\u5e93\u53d8\u66f4"},
		{"achievements", "\u5de5\u4f5c\u6210\u679c"}, {"people", "\u4eba\u5458\u660e\u7ec6"},
		{"governance", "\u9879\u76ee\u7ed3\u6784\u68c0\u67e5"}, {"limitations", "\u9650\u5236\u4e0e\u672a\u786e\u8ba4\u4e8b\u9879"},
		{"sources", "\u6750\u6599\u6765\u6e90"},
	}
	facts := reportTestMaterialFacts(material)
	for _, section := range sections {
		fmt.Fprintf(&builder, `<section data-section="%s"><h2>%s</h2>`, section.id, section.title)
		appendReportMaterialFacts(&builder, facts, section.id)
		if section.id == "quality-coverage" {
			appendReportStatistics(&builder, material)
		}
		if section.id == "finding-details" {
			for _, finding := range material.Findings {
				appendReportFinding(&builder, finding)
			}
		}
		builder.WriteString("<p>\u672c\u8282\u5185\u5bb9\u89c1\u8f93\u5165\u6750\u6599</p>")
		builder.WriteString(`</section>`)
	}
	builder.WriteString(`</main></body></html>`)
	return builder.String()
}

const (
	reportMultiTestTitle      = "\u5ba1\u67e5\u62a5\u544a"
	reportMultiTestRunIDLabel = "\u8fd0\u884c\u6807\u8bc6"
)

func reportMultiHTMLFixture(input report.MultiReportInput) string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>` + reportMultiTestTitle + `</title></head><body>`)
	fmt.Fprintf(&builder, `<main data-report-kind="multi-unit" data-review-unit-count="%d">`, len(input.ReviewUnits))
	sections := []struct{ id, title string }{
		{"overview", "\u62a5\u544a\u6982\u89c8"}, {"quality-coverage", "\u8d28\u91cf\u4e0e\u8986\u76d6"},
		{"finding-details", "\u95ee\u9898\u660e\u7ec6"}, {"changes", "\u4ed3\u5e93\u53d8\u66f4"},
		{"achievements", "\u5de5\u4f5c\u6210\u679c"}, {"people", "\u4eba\u5458\u660e\u7ec6"},
		{"governance", "\u9879\u76ee\u7ed3\u6784\u68c0\u67e5"}, {"limitations", "\u9650\u5236\u4e0e\u672a\u786e\u8ba4\u4e8b\u9879"},
		{"sources", "\u6750\u6599\u6765\u6e90"},
	}
	aggregateFacts := reportMultiAggregateFacts(input)
	for _, section := range sections {
		fmt.Fprintf(&builder, `<section data-section="%s"><h2>%s</h2>`, section.id, section.title)
		for unitIndex, unit := range input.ReviewUnits {
			fmt.Fprintf(&builder, `<div data-review-unit-id="%s">`, html.EscapeString(unit.ID))
			materialFacts := reportTestMaterialFacts(unit.Material)
			keys := make([]string, 0, len(materialFacts))
			for key := range materialFacts {
				if reportTestFactAllowedInSection(section.id, key) {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			if section.id == "overview" {
				sort.SliceStable(keys, func(left, right int) bool {
					return keys[left] == "review.run_id" && keys[right] != "review.run_id"
				})
			}
			for _, key := range keys {
				factName := fmt.Sprintf("review_units[%d].material.%s", unitIndex, key)
				value := reportTestFactDisplay(key, materialFacts[key])
				label := reportTestFactLabel(key)
				if section.id == "overview" && key == "review.run_id" {
					label = reportMultiTestRunIDLabel
				}
				fmt.Fprintf(&builder, `<div class="fact-row"><strong class="fact-label">%s</strong><span data-fact="%s">%s</span></div>`, label, html.EscapeString(factName), html.EscapeString(value))
			}
			if section.id == "quality-coverage" {
				builder.WriteString(`<div class="report-statistics">`)
				appendReportStatistics(&builder, unit.Material)
				builder.WriteString(`</div>`)
			}
			if section.id == "finding-details" {
				for _, finding := range unit.Material.Findings {
					var article strings.Builder
					appendReportFinding(&article, finding)
					builder.WriteString(strings.Replace(article.String(), `<article data-finding-id=`, `<article data-review-unit-id="`+html.EscapeString(unit.ID)+`" data-finding-id=`, 1))
				}
			}
			builder.WriteString(`</div>`)
		}
		keys := make([]string, 0, len(aggregateFacts))
		for key := range aggregateFacts {
			if reportMultiAggregateFactSection(key) == section.id {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&builder, `<div class="fact-row"><strong class="fact-label">%s</strong><span data-fact="%s">%s</span></div>`, reportMultiAggregateFactLabel(key), html.EscapeString(key), html.EscapeString(aggregateFacts[key]))
		}
		builder.WriteString("<p>\u672c\u8282\u5185\u5bb9\u89c1\u8f93\u5165\u6750\u6599</p>")
		builder.WriteString(`</section>`)
	}
	builder.WriteString(`</main></body></html>`)
	return builder.String()
}

func reportMultiAggregateFacts(input report.MultiReportInput) map[string]string {
	data, err := json.Marshal(struct {
		Summary report.MultiReportSummary  `json:"summary"`
		People  []report.MultiReportPerson `json:"people"`
	}{Summary: input.Summary, People: input.People})
	if err != nil {
		panic(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		panic(err)
	}
	facts := make(map[string]string)
	var collect func(string, any)
	collect = func(path string, value any) {
		switch current := value.(type) {
		case map[string]any:
			for key, child := range current {
				name := key
				if path != "" {
					name = path + "." + key
				}
				collect(name, child)
			}
		case []any:
			for index, child := range current {
				collect(fmt.Sprintf("%s[%d]", path, index), child)
			}
		case string:
			facts[path] = current
		case json.Number:
			facts[path] = current.String()
		}
	}
	collect("", root)
	return facts
}

func reportMultiAggregateFactSection(name string) string {
	if strings.HasPrefix(name, "summary.findings_by_severity.") {
		return "quality-coverage"
	}
	if strings.HasPrefix(name, "summary.") {
		return "overview"
	}
	if strings.HasPrefix(name, "people[") {
		return "people"
	}
	return ""
}

func reportMultiAggregateFactLabel(name string) string {
	if strings.HasPrefix(name, "people[") {
		return "\u4eba\u5458"
	}
	switch name {
	case "summary.finding_record_count":
		return "\u95ee\u9898\u6570\u91cf"
	case "summary.findings_by_severity.critical":
		return "\u4e25\u91cd"
	case "summary.findings_by_severity.high":
		return "\u9ad8"
	case "summary.findings_by_severity.medium":
		return "\u4e2d"
	case "summary.findings_by_severity.low":
		return "\u4f4e"
	case "summary.findings_by_severity.not_collected":
		return "\u672a\u63d0\u4f9b\u7b49\u7ea7\u6570\u91cf"
	default:
		return "\u6750\u6599\u4e8b\u5b9e"
	}
}

func writeMultiBrowserEvidence(t *testing.T, envName string, data []byte) {
	t.Helper()
	writeReportBrowserEvidence(t, envName, data)
}

func writeReportBrowserEvidence(t *testing.T, envName string, data []byte) {
	t.Helper()
	path := os.Getenv(envName)
	if path == "" {
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write browser evidence %s: %v", envName, err)
	}
}

func withReportTestPeople(material report.Material, authorName, authorEmail, committerName, committerEmail string) report.Material {
	data, err := json.Marshal(struct {
		CommitSHA string `json:"commit_sha"`
		Subject   string `json:"subject"`
		Author    struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		Committer struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"committer"`
		Basis string `json:"basis"`
	}{
		CommitSHA: "abc123",
		Subject:   "Review report test change",
		Author: struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}{Name: authorName, Email: authorEmail},
		Committer: struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}{Name: committerName, Email: committerEmail},
		Basis: "Git commit metadata",
	})
	if err != nil {
		panic(err)
	}
	material.Sections.People = report.Section{Status: report.StatusProvided, Data: data}
	return material
}

func reportTestMaterialFacts(material report.Material) map[string]string {
	data, err := json.Marshal(material)
	if err != nil {
		panic(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		panic(err)
	}
	facts := make(map[string]string)
	var collect func(string, any)
	collect = func(path string, value any) {
		switch current := value.(type) {
		case map[string]any:
			for key, child := range current {
				if path == "" && key == "findings" {
					continue
				}
				name := key
				if path != "" {
					name = path + "." + key
				}
				collect(name, child)
			}
		case []any:
			for index, child := range current {
				collect(fmt.Sprintf("%s[%d]", path, index), child)
			}
		case string:
			facts[path] = current
		case json.Number:
			facts[path] = current.String()
		case bool:
			facts[path] = fmt.Sprint(current)
		}
	}
	collect("", root)
	return facts
}

func appendReportMaterialFacts(builder *strings.Builder, facts map[string]string, section string) {
	keys := make([]string, 0, len(facts))
	for key := range facts {
		if reportTestFactAllowedInSection(section, key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := reportTestFactDisplay(key, facts[key])
		fmt.Fprintf(builder, `<div class="fact-row"><strong class="fact-label">%s</strong><span data-fact="%s">%s</span></div>`, reportTestFactLabel(key), html.EscapeString(key), html.EscapeString(value))
	}
}

func reportTestFactLabel(key string) string {
	labels := map[string]string{
		"repository.name": "\u4ed3\u5e93", "repository.identity.status": "\u4ed3\u5e93\u6807\u8bc6", "review.status": "\u5ba1\u67e5\u72b6\u6001", "scope.mode": "\u5ba1\u67e5\u8303\u56f4",
		"review.started_at": "\u5f00\u59cb\u65f6\u95f4", "review.completed_at": "\u7ed3\u675f\u65f6\u95f4", "review.elapsed_ms": "\u8017\u65f6", "review.provider": "\u63d0\u4f9b\u65b9", "review.model": "\u6a21\u578b",
		"scope.requested_from": "\u57fa\u51c6\u63d0\u4ea4", "scope.requested_head": "\u76ee\u6807\u63d0\u4ea4", "scope.exact_range": "\u5b9e\u9645\u8303\u56f4", "scope.source_artifact.status": "\u6765\u6e90",
		"sections.structural_checks.status": "\u9879\u76ee\u7ed3\u6784\u68c0\u67e5", "schema_version": "\u6750\u6599\u7248\u672c", "review.run_id": "\u8fd0\u884c\u6807\u8bc6",
	}
	if label, ok := labels[key]; ok {
		return label
	}
	if strings.HasSuffix(key, ".status") {
		return "\u72b6\u6001"
	}
	if strings.Contains(key, "reason") {
		return "\u539f\u56e0"
	}
	if strings.HasPrefix(key, "coverage.") {
		return "\u5ba1\u67e5\u8986\u76d6"
	}
	if strings.HasPrefix(key, "sections.people.") {
		return "\u4eba\u5458"
	}
	if strings.HasPrefix(key, "sections.achievements.") {
		return "\u6210\u679c"
	}
	if strings.HasPrefix(key, "sections.knowledge_sources.") {
		return "\u77e5\u8bc6\u6765\u6e90"
	}
	if strings.HasPrefix(key, "limitations[") {
		return "\u9650\u5236"
	}
	return "\u6750\u6599\u4e8b\u5b9e"
}

func reportTestFactAllowedInSection(section, key string) bool {
	prefixes := map[string][]string{
		"overview":         {"review.", "repository.", "scope.", "run_failure."},
		"quality-coverage": {"coverage."},
		"changes":          {"sections.git_statistics.", "sections.workspace_snapshot."},
		"achievements":     {"sections.achievements."},
		"people":           {"sections.people."},
		"governance":       {"sections.structural_checks."},
		"limitations":      {"limitations["},
		"sources":          {"schema_version", "review.run_id", "sections.knowledge_sources."},
	}
	for _, prefix := range prefixes[section] {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func reportTestFactDisplay(key, value string) string {
	if !strings.HasSuffix(key, ".status") {
		return value
	}
	labels := map[string]string{
		"provided": "\u5df2\u63d0\u4f9b", "not_collected": "\u672a\u63d0\u4f9b",
		"not_applicable": "\u4e0d\u9002\u7528", "failed": "\u5931\u8d25",
		"complete": "\u5df2\u5b8c\u6210", "partial": "\u90e8\u5206\u5b8c\u6210", "skipped": "\u5df2\u8df3\u8fc7",
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}

func appendReportStatistics(builder *strings.Builder, material report.Material) {
	stats := []struct {
		name  string
		count int
	}{
		{"finding-count", len(material.Findings)},
		{"risk-critical", 0}, {"risk-high", 0}, {"risk-medium", 0}, {"risk-low", 0},
		{"coverage-selected", len(material.Coverage.Selected)}, {"coverage-completed", len(material.Coverage.Completed)},
		{"coverage-failed", len(material.Coverage.Failed)}, {"coverage-skipped", len(material.Coverage.Waived)},
		{"coverage-reused", len(material.Coverage.Reused)},
	}
	for _, finding := range material.Findings {
		for index, severity := range []string{"critical", "high", "medium", "low"} {
			if finding.Severity == severity {
				stats[index+1].count++
			}
		}
	}
	for _, stat := range stats {
		labels := map[string]string{"finding-count": "\u95ee\u9898\u6570\u91cf", "risk-critical": "\u4e25\u91cd", "risk-high": "\u9ad8", "risk-medium": "\u4e2d", "risk-low": "\u4f4e", "coverage-selected": "\u9009\u4e2d", "coverage-completed": "\u5b8c\u6210", "coverage-failed": "\u5931\u8d25", "coverage-skipped": "\u8df3\u8fc7", "coverage-reused": "\u590d\u7528"}
		fmt.Fprintf(builder, `<div class="report-statistic"><strong class="fact-label">%s</strong><output data-stat="%s">%d</output></div>`, labels[stat.name], stat.name, stat.count)
	}
}

func appendReportFinding(builder *strings.Builder, finding report.Finding) {
	fmt.Fprintf(builder, `<article data-finding-id="%s" data-severity="%s" data-category="%s" data-path="%s" data-start-line="%d" data-end-line="%d">`, html.EscapeString(finding.ID), html.EscapeString(finding.Severity), html.EscapeString(finding.Category), html.EscapeString(finding.Path), finding.StartLine, finding.EndLine)
	facts := []struct{ name, value string }{
		{"path", finding.Path}, {"start_line", strconv.Itoa(finding.StartLine)}, {"end_line", strconv.Itoa(finding.EndLine)},
		{"summary_zh", finding.Display.SummaryZH}, {"severity_zh", finding.Display.SeverityZH},
		{"category_zh", finding.Display.CategoryZH}, {"source_content", finding.SourceContent},
		{"evidence_status", reportDisplayFindingFact("evidence_status", string(finding.Evidence.Status))},
		{"recommendation_status", reportDisplayFindingFact("recommendation_status", string(finding.Recommendation.Status))},
	}
	if finding.Evidence.Status == report.StatusProvided {
		facts = append(facts, struct{ name, value string }{"evidence_code", finding.Evidence.Code})
	} else {
		facts = append(facts, struct{ name, value string }{"evidence_reason", finding.Evidence.Reason})
	}
	if finding.Recommendation.Status == report.StatusProvided {
		facts = append(facts, struct{ name, value string }{"recommendation_code", finding.Recommendation.Code})
	} else {
		facts = append(facts, struct{ name, value string }{"recommendation_reason", finding.Recommendation.Reason})
	}
	for _, fact := range facts {
		label := reportFindingFactLabel(fact.name)
		if fact.name == "evidence_code" || fact.name == "recommendation_code" || fact.name == "source_content" {
			fmt.Fprintf(builder, `<div class="fact-row"><strong class="fact-label">%s</strong><pre data-fact="%s">%s</pre></div>`, label, fact.name, html.EscapeString(fact.value))
		} else {
			fmt.Fprintf(builder, `<div class="fact-row"><strong class="fact-label">%s</strong><span data-fact="%s">%s</span></div>`, label, fact.name, html.EscapeString(fact.value))
		}
	}
	builder.WriteString(`</article>`)
}

func reportFindingFactLabel(name string) string {
	return map[string]string{
		"path": "\u6587\u4ef6", "start_line": "\u884c\u53f7", "end_line": "\u884c\u53f7", "summary_zh": "\u6458\u8981",
		"severity_zh": "\u4e25\u91cd\u7b49\u7ea7", "category_zh": "\u7c7b\u522b", "source_content": "\u6e90\u4ee3\u7801",
		"evidence_status": "\u72b6\u6001", "recommendation_status": "\u72b6\u6001", "evidence_code": "\u8bc1\u636e",
		"evidence_reason": "\u539f\u56e0", "recommendation_code": "\u5efa\u8bae", "recommendation_reason": "\u539f\u56e0",
	}[name]
}

func reportDisplayFindingFact(name, value string) string {
	if !strings.HasSuffix(name, "_status") {
		return value
	}
	return map[string]string{"provided": "\u5df2\u63d0\u4f9b", "not_collected": "\u672a\u63d0\u4f9b", "not_applicable": "\u4e0d\u9002\u7528", "failed": "\u5931\u8d25"}[value]
}
