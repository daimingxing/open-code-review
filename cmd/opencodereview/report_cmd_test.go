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

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestReportHTMLRequestTimeoutUsesRemainingStageBudget(t *testing.T) {
	if maxReportDuration != 10*time.Minute {
		t.Fatalf("HTML generation budget = %s, want 10m", maxReportDuration)
	}
	started := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name            string
		elapsed         time.Duration
		endpointTimeout time.Duration
		want            time.Duration
	}{
		{name: "remaining total budget caps request", elapsed: 9*time.Minute + 50*time.Second, endpointTimeout: 30 * time.Second, want: 10 * time.Second},
		{name: "endpoint timeout caps request", elapsed: 20 * time.Second, endpointTimeout: 5 * time.Second, want: 5 * time.Second},
		{name: "unconfigured endpoint uses remaining budget", elapsed: time.Minute, want: 9 * time.Minute},
		{name: "expired total budget rejects request", elapsed: 10 * time.Minute, endpointTimeout: 30 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := reportHTMLRequestTimeout(started, started.Add(test.elapsed), test.endpointTimeout)
			if got != test.want {
				t.Fatalf("request timeout = %s, want %s", got, test.want)
			}
		})
	}
}

func TestReportHTMLRequestMaxTokensReservesReasoningAndStageBudget(t *testing.T) {
	tests := []struct {
		name      string
		used      int
		wantLimit int
	}{
		{name: "first attempt leaves reasoning headroom", wantLimit: maxReportAttemptCompletionTokens},
		{name: "remaining stage budget caps attempt", used: 150000, wantLimit: maxReportTotalOutputTokens - 150000},
		{name: "small remaining stage budget caps attempt", used: maxReportTotalOutputTokens - 4096, wantLimit: 4096},
		{name: "exhausted stage budget rejects attempt", used: maxReportTotalOutputTokens},
		{name: "over budget rejects attempt", used: maxReportTotalOutputTokens + 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := reportHTMLRequestMaxTokens(test.used); got != test.wantLimit {
				t.Fatalf("request token limit = %d, want %d", got, test.wantLimit)
			}
		})
	}
}

func TestReportCommandRendersExistingMaterialAndRejectsAlteredModelFacts(t *testing.T) {
	setTestHome(t, t.TempDir())
	responses := make(chan string, 4)
	requests := make(chan map[string]any, 16)
	var responseCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- request
		content := <-responses
		completionTokens := 456
		if responseCount.Add(1) == 1 {
			completionTokens = 70000
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "report-test", "model": "fake-report-model",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": content},
			}},
			"usage": map[string]any{"prompt_tokens": 123, "completion_tokens": completionTokens, "total_tokens": 123 + completionTokens},
		})
	}))
	defer server.Close()
	t.Setenv("OCR_LLM_URL", server.URL+"/v1")
	t.Setenv("OCR_LLM_TOKEN", "report-test-token")
	t.Setenv("OCR_LLM_MODEL", "fake-report-model")
	t.Setenv("OCR_LLM_PROTOCOL", "openai")

	material := reportTestLongMaterial()
	visibleTokens := llm.CountTokensForModel(reportHTMLFixture(material), "fake-report-model")
	if visibleTokens > maxReportVisibleOutputTokens {
		t.Fatalf("60-finding fixture requires %d visible output tokens, above the %d-token report budget", visibleTokens, maxReportVisibleOutputTokens)
	}
	input := writeReportInput(t, material)
	out := filepath.Join(t.TempDir(), "four-risks.html")
	responses <- reportHTMLFixture(material)
	stdout, stderr, err := executeReportCommand([]string{"--input", input, "--output", out})
	if err != nil {
		t.Fatalf("report command failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, out) || !strings.Contains(stderr, fmt.Sprintf("input_tokens=123 output_tokens=70000 visible_output_tokens=%d usage=reported", visibleTokens)) {
		t.Fatalf("command diagnostics are incomplete: stdout=%q stderr=%q", stdout, stderr)
	}
	request := <-requests
	if request["max_tokens"] != float64(maxReportAttemptCompletionTokens) && request["max_completion_tokens"] != float64(maxReportAttemptCompletionTokens) {
		t.Fatalf("long report request did not reserve provider reasoning headroom: %#v", request)
	}
	if _, ok := request["tools"]; ok {
		t.Fatal("report request must not expose tools")
	}
	messages, ok := request["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("report request messages = %#v, want only template and report material", request["messages"])
	}
	templateMessage, ok := messages[0].(map[string]any)
	if !ok || templateMessage["role"] != "system" || !strings.Contains(fmt.Sprint(templateMessage["content"]), "data-fact") {
		t.Fatalf("report request is missing the template's fact-binding instructions: %#v", messages[0])
	}
	materialJSON, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	userMessage, ok := messages[1].(map[string]any)
	if !ok || userMessage["role"] != "user" || userMessage["content"] != string(materialJSON) {
		t.Fatalf("model did not receive the existing report material as its only user input: %#v", messages[1])
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if evidencePath := os.Getenv("OCR_REPORT_HTML_EVIDENCE_FILE"); evidencePath != "" {
		if err := os.WriteFile(evidencePath, data, 0o600); err != nil {
			t.Fatalf("write browser evidence HTML: %v", err)
		}
	}
	if err := report.ValidateHTMLDocument(string(data), material); err != nil {
		t.Fatalf("saved HTML failed fact and safety validation: %v", err)
	}
	for _, severity := range []string{"critical", "high", "medium", "low"} {
		if !strings.Contains(string(data), `data-severity="`+severity+`"`) {
			t.Errorf("saved HTML omitted %s findings", severity)
		}
	}
	if !strings.Contains(string(data), "not collected") || !strings.Contains(string(data), `return secret &lt;&amp; token`) {
		t.Fatal("partial state, knowledge gap, or safely escaped evidence was omitted")
	}

	zeroMaterial := reportTestMaterial(false)
	zeroInput := writeReportInput(t, zeroMaterial)
	zeroOutput := filepath.Join(t.TempDir(), "zero.html")
	responses <- reportHTMLFixture(zeroMaterial)
	if _, stderr, err := executeReportCommand([]string{"--input", zeroInput, "--output", zeroOutput}); err != nil {
		t.Fatalf("zero-finding report failed: %v\nstderr: %s", err, stderr)
	}
	<-requests
	zeroHTML, err := os.ReadFile(zeroOutput)
	if err != nil {
		t.Fatal(err)
	}
	writeReportBrowserEvidence(t, "OCR_REPORT_HTML_SHORT_EVIDENCE_FILE", zeroHTML)
	if !strings.Contains(string(zeroHTML), `data-stat="finding-count">0`) {
		t.Fatal("zero-finding report did not show a computed zero")
	}

	badOutput := filepath.Join(t.TempDir(), "altered.html")
	criticalCount := 0
	for _, finding := range material.Findings {
		if finding.Severity == "critical" {
			criticalCount++
		}
	}
	alteredStatistics := strings.Replace(reportHTMLFixture(material), fmt.Sprintf(`data-stat="risk-critical">%d`, criticalCount), fmt.Sprintf(`data-stat="risk-critical">%d`, criticalCount+1), 1)
	for attempt := 0; attempt < maxReportHTMLAttempts; attempt++ {
		responses <- alteredStatistics
	}
	_, badStatisticsStderr, err := executeReportCommand([]string{"--input", input, "--output", badOutput})
	if err == nil || !strings.Contains(badStatisticsStderr+err.Error(), "statistic") {
		t.Fatalf("altered model statistics were accepted: err=%v stderr=%s", err, badStatisticsStderr)
	}
	if got := strings.Count(badStatisticsStderr, "phase=html_generation attempt="); got != maxReportHTMLAttempts {
		t.Fatalf("invalid statistics made %d repair attempts, want %d: %s", got, maxReportHTMLAttempts, badStatisticsStderr)
	}
	for attempt := 0; attempt < maxReportHTMLAttempts; attempt++ {
		request = <-requests
		messages = request["messages"].([]any)
		userMessage = messages[1].(map[string]any)
		if userMessage["content"] != string(materialJSON) {
			t.Fatalf("repair attempt %d changed the report material input", attempt+1)
		}
	}
	if _, err := os.Stat(badOutput); !os.IsNotExist(err) {
		t.Fatalf("invalid model HTML left a final file: %v", err)
	}
	badPeopleOutput := filepath.Join(t.TempDir(), "invented-people.html")
	peopleSection := `<section data-section="people">`
	forgedPeople := strings.Replace(reportHTMLFixture(material), `</section><section data-section="governance"`, `<p>Avery made 42 commits.</p></section><section data-section="governance"`, 1)
	if !strings.Contains(forgedPeople, peopleSection) {
		t.Fatal("fixture is missing the people section")
	}
	for attempt := 0; attempt < maxReportHTMLAttempts; attempt++ {
		responses <- forgedPeople
	}
	_, badPeopleStderr, err := executeReportCommand([]string{"--input", input, "--output", badPeopleOutput})
	if err == nil || !strings.Contains(badPeopleStderr+err.Error(), "unverified narrative") {
		t.Fatalf("invented people details were accepted: err=%v stderr=%s", err, badPeopleStderr)
	}
	if calls := strings.Count(badPeopleStderr, "phase=html_generation attempt="); calls != maxReportHTMLAttempts {
		t.Fatalf("invalid people facts made %d repair attempts, want %d: %s", calls, maxReportHTMLAttempts, badPeopleStderr)
	}
	for attempt := 0; attempt < maxReportHTMLAttempts; attempt++ {
		request = <-requests
		if attempt > 0 {
			requestJSON, _ := json.Marshal(request)
			if strings.Contains(string(requestJSON), "42 commits") {
				t.Fatal("retry prompt included unverified report text as instructions")
			}
		}
	}
	if strings.Contains(badPeopleStderr+err.Error(), "42 commits") {
		t.Fatal("unverified report text leaked into CLI diagnostics")
	}
	if _, err := os.Stat(badPeopleOutput); !os.IsNotExist(err) {
		t.Fatalf("invented facts left a final HTML file: %v", err)
	}
}

func TestReportCommandKeepsProviderErrorBodyOutOfRetryAndDiagnostics(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(true)
	input := writeReportInput(t, material)
	secret := `C:\Users\private\request-body bearer=do-not-print`
	var calls atomic.Int32
	requests := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- request
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"error":{"message":%q}}`, secret)
			return
		}
		writeReportCompletion(w, reportHTMLFixture(material), "stop")
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	output := filepath.Join(t.TempDir(), "provider-error-retry.html")
	stdout, stderr, err := executeReportCommand([]string{"--input", input, "--output", output})
	if err != nil {
		t.Fatalf("safe provider retry failed: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(stdout+stderr, secret) || strings.Contains(stdout+stderr, "request-body") {
		t.Fatalf("provider error body leaked to command output: stdout=%q stderr=%q", stdout, stderr)
	}
	firstRequest, secondRequest := <-requests, <-requests
	secondMessages, ok := secondRequest["messages"].([]any)
	if !ok || len(secondMessages) < 3 {
		t.Fatalf("retry request did not include a separate diagnostic message: %#v", secondRequest["messages"])
	}
	secondRequestJSON, _ := json.Marshal(secondRequest)
	if strings.Contains(string(secondRequestJSON), secret) || strings.Contains(string(secondRequestJSON), "request-body") {
		t.Fatalf("provider error body leaked into the retry request: %s", secondRequestJSON)
	}
	firstMessages := firstRequest["messages"].([]any)
	firstInput := firstMessages[1].(map[string]any)["content"]
	secondInput := secondMessages[1].(map[string]any)["content"]
	if firstInput != secondInput {
		t.Fatal("retry changed the existing report material JSON")
	}
	diagnostic := secondMessages[2].(map[string]any)["content"]
	if !strings.Contains(fmt.Sprint(diagnostic), "400") {
		t.Fatalf("retry did not retain a safe provider status diagnostic: %#v", diagnostic)
	}
}

func TestReportCommandStopsRetryingAfterCallerCancellation(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		cancel()
		writeReportCompletion(w, reportHTMLFixture(material), "stop")
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
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("canceled report command error = %v, want cancellation", err)
	}
	if calls.Load() != 1 || strings.Count(stderr.String(), "phase=html_generation attempt=") != 1 {
		t.Fatalf("caller cancellation retried the model: calls=%d stderr=%s", calls.Load(), stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("caller cancellation published a partial report: %v", err)
	}
}

func TestReportCommandRepairsInvalidHTMLFromExistingMaterial(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(true)
	input := writeReportInput(t, material)
	inputJSON, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	requests := make(chan map[string]any, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- request
		if calls.Add(1) == 1 {
			wrongHeading := strings.Replace(reportHTMLFixture(material), "<h2>\u8d28\u91cf\u4e0e\u8986\u76d6</h2>", "<h2>\u9519\u8bef\u6807\u9898</h2>", 1)
			writeReportCompletion(w, wrongHeading, "stop")
			return
		}
		writeReportCompletion(w, reportHTMLFixture(material), "stop")
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)

	output := filepath.Join(t.TempDir(), "repaired.html")
	stdout, stderr, err := executeReportCommand([]string{"--input", input, "--output", output})
	if err != nil {
		t.Fatalf("report command did not recover: %v\nstderr: %s", err, stderr)
	}
	if calls.Load() != 2 {
		t.Fatalf("model received %d requests, want one failed and one repair request", calls.Load())
	}
	first, second := <-requests, <-requests
	firstMessages := first["messages"].([]any)
	secondMessages := second["messages"].([]any)
	firstUser := firstMessages[1].(map[string]any)
	secondUser := secondMessages[1].(map[string]any)
	if firstUser["content"] != string(inputJSON) || secondUser["content"] != firstUser["content"] {
		t.Fatal("repair request did not reuse the exact existing report JSON")
	}
	if _, ok := second["tools"]; ok {
		t.Fatal("repair request unexpectedly exposed tools")
	}
	var repairPrompt strings.Builder
	for _, message := range secondMessages {
		repairPrompt.WriteString(fmt.Sprint(message.(map[string]any)["content"]))
		repairPrompt.WriteByte('\n')
	}
	for _, required := range []string{"required report section or Chinese heading", "quality-coverage", "<!doctype html>", "data-fact", "do not summarize, omit, merge, rewrite, or truncate"} {
		if !strings.Contains(repairPrompt.String(), required) {
			t.Fatalf("repair request omitted required structure or fact instruction %q", required)
		}
	}
	if !strings.Contains(stderr, "phase=html_generation") || !strings.Contains(stderr, "attempts=2") || !strings.Contains(stderr, "recovered=true") {
		t.Fatalf("HTML repair metrics are incomplete: %s", stderr)
	}
	if !strings.Contains(stdout, output) {
		t.Fatalf("successful repair did not report its output path: %s", stdout)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.ValidateHTMLDocument(string(data), material); err != nil {
		t.Fatalf("repaired output failed validation: %v", err)
	}
}

func TestReportCommandRepairsUnsafeHTMLFromExistingMaterial(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(true)
	input := writeReportInput(t, material)
	inputJSON, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	requests := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- request
		if calls.Add(1) == 1 {
			invalid := strings.Replace(reportHTMLFixture(material), "</body>", "<script>active</script></body>", 1)
			writeReportCompletion(w, invalid, "stop")
			return
		}
		writeReportCompletion(w, reportHTMLFixture(material), "stop")
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)

	output := filepath.Join(t.TempDir(), "repaired-unsafe.html")
	_, stderr, err := executeReportCommand([]string{"--input", input, "--output", output})
	if err != nil {
		t.Fatalf("report command did not repair unsafe HTML: %v\nstderr: %s", err, stderr)
	}
	first, second := <-requests, <-requests
	firstMessages := first["messages"].([]any)
	secondMessages := second["messages"].([]any)
	firstUser := firstMessages[1].(map[string]any)
	secondUser := secondMessages[1].(map[string]any)
	if firstUser["content"] != string(inputJSON) || secondUser["content"] != firstUser["content"] {
		t.Fatal("unsafe HTML repair did not reuse the unchanged report JSON")
	}
	var repairPrompt strings.Builder
	for _, message := range secondMessages {
		repairPrompt.WriteString(fmt.Sprint(message.(map[string]any)["content"]))
		repairPrompt.WriteByte('\n')
	}
	for _, required := range []string{"unsupported, hidden, or unsafe content", "<!doctype html>", "data-fact", "fixed Chinese fact label"} {
		if !strings.Contains(repairPrompt.String(), required) {
			t.Fatalf("unsafe HTML repair prompt omitted required instruction %q", required)
		}
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("repaired safe HTML was not published: %v", err)
	}
}

func TestReportCommandStopsWhenCumulativeCompletionUsageExceedsBudget(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)
	var calls atomic.Int32
	requests := make(chan map[string]any, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- request
		call := calls.Add(1)
		if call == 1 {
			writeReportCompletionWithUsage(w, "<html>truncated response", "stop", 300, maxReportTotalOutputTokens/2)
			return
		}
		writeReportCompletionWithUsage(w, reportHTMLFixture(material), "stop", 300, maxReportTotalOutputTokens/2+1)
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	output := filepath.Join(t.TempDir(), "over-budget.html")
	_, stderr, err := executeReportCommand([]string{"--input", input, "--output", output})
	if err == nil || !strings.Contains(err.Error(), "total model completion usage") || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("over-budget model output was accepted: err=%v stderr=%s", err, stderr)
	}
	if calls.Load() != 2 {
		t.Fatalf("budget-exhausting response made %d attempts, want 2", calls.Load())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("over-budget response left a final HTML file: %v", err)
	}
	if !strings.Contains(stderr, fmt.Sprintf("output_tokens=%d", maxReportTotalOutputTokens+1)) || !strings.Contains(stderr, "recovered=false") {
		t.Fatalf("over-budget attempt metrics were not accumulated: %s", stderr)
	}
	for attempt := 0; attempt < 2; attempt++ {
		request := <-requests
		maxTokens := request["max_tokens"]
		if maxTokens == nil {
			maxTokens = request["max_completion_tokens"]
		}
		if maxTokens != float64(maxReportAttemptCompletionTokens) {
			t.Fatalf("attempt %d requested max tokens=%v, want %d", attempt+1, maxTokens, maxReportAttemptCompletionTokens)
		}
	}
}

func TestReportOutputBudgetDiagnostics(t *testing.T) {
	tests := []struct {
		name                    string
		visible, total          int
		wantDiagnosticSubstring string
	}{
		{name: "visible document limit", visible: maxReportVisibleOutputTokens + 1, total: 1, wantDiagnosticSubstring: "visible model output"},
		{name: "cumulative completion limit", visible: 100, total: maxReportTotalOutputTokens + 1, wantDiagnosticSubstring: "total model completion usage"},
		{name: "within both limits", visible: maxReportVisibleOutputTokens, total: maxReportTotalOutputTokens},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := reportHTMLOutputBudgetDiagnostic(test.visible, test.total)
			if test.wantDiagnosticSubstring == "" {
				if got != "" {
					t.Fatalf("budget diagnostic = %q, want none", got)
				}
				return
			}
			if !strings.Contains(got, test.wantDiagnosticSubstring) {
				t.Fatalf("budget diagnostic = %q, want %q", got, test.wantDiagnosticSubstring)
			}
		})
	}
}

func TestReportCommandMultiInputPreservesOrderAndRejectsDuplicateMaterialBeforeRequest(t *testing.T) {
	setTestHome(t, t.TempDir())
	var calls atomic.Int32
	requests := make(chan report.MultiReportInput, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		messages, ok := request["messages"].([]any)
		if !ok || len(messages) != 2 {
			http.Error(w, "unexpected messages", http.StatusBadRequest)
			return
		}
		user, ok := messages[1].(map[string]any)
		if !ok || user["role"] != "user" {
			http.Error(w, "missing report envelope", http.StatusBadRequest)
			return
		}
		var input report.MultiReportInput
		if err := json.Unmarshal([]byte(fmt.Sprint(user["content"])), &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- input
		writeReportCompletion(w, reportMultiHTMLFixture(input), "stop")
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)

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

	outDir := t.TempDir()
	firstOutput := filepath.Join(outDir, "multi-first.html")
	if _, stderr, err := executeReportCommand([]string{"--input", paths[0], "--input", paths[1], "--input", paths[2], "--output", firstOutput}); err != nil {
		t.Fatalf("multi-input report failed: %v\nstderr: %s", err, stderr)
	}
	input := <-requests
	if got := []string{input.ReviewUnits[0].Material.Review.RunID, input.ReviewUnits[1].Material.Review.RunID, input.ReviewUnits[2].Material.Review.RunID}; !slices.Equal(got, []string{first.Review.RunID, second.Review.RunID, third.Review.RunID}) {
		t.Fatalf("review-unit order = %v", got)
	}
	if input.Summary.ReviewUnitCount != 3 || input.Summary.FindingRecordCount != 8 || input.Summary.FindingsBySeverity["not_collected"] != 1 {
		t.Fatalf("summary = %+v, want 3 units, 8 unit-scoped finding records, and 1 finding without collected severity", input.Summary)
	}
	if input.ReviewUnits[0].Material.Findings[0].ID != input.ReviewUnits[2].Material.Findings[0].ID {
		t.Fatal("fixture must contain colliding finding IDs in different repositories")
	}
	var mergedPerson *report.MultiReportPerson
	for index := range input.People {
		person := &input.People[index]
		if person.IdentityStatus == "email_verified" && slices.Contains(person.NameVariants, "Alice Chen") {
			mergedPerson = person
		}
	}
	if mergedPerson == nil || len(mergedPerson.Contributions) != 2 || len(mergedPerson.NameVariants) != 2 {
		t.Fatalf("reliable cross-repository identity was not represented: %+v", mergedPerson)
	}
	firstHTML, err := os.ReadFile(firstOutput)
	if err != nil {
		t.Fatal(err)
	}
	writeMultiBrowserEvidence(t, "OCR_MULTI_REPORT_HTML_EVIDENCE_FILE", firstHTML)
	if strings.Count(string(firstHTML), `data-finding-id="`+first.Findings[0].ID+`"`) != 2 || !strings.Contains(string(firstHTML), `data-severity=""`) || !strings.Contains(string(firstHTML), "\u672a\u5bf9\u8de8\u5355\u5143\u95ee\u9898\u53bb\u91cd") {
		t.Fatal("saved report lost an unknown-severity finding, colliding finding, or the unit-scoped counting rule")
	}
	if err := report.ValidateMultiHTMLDocument(string(firstHTML), input); err != nil {
		t.Fatalf("saved report did not pass multi-unit fact/safety validation: %v", err)
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
	reordered := <-requests
	if got := []string{reordered.ReviewUnits[0].Material.Review.RunID, reordered.ReviewUnits[1].Material.Review.RunID, reordered.ReviewUnits[2].Material.Review.RunID}; !slices.Equal(got, []string{third.Review.RunID, second.Review.RunID, first.Review.RunID}) {
		t.Fatalf("reordered review-unit order = %v", got)
	}

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
		t.Fatalf("duplicate material with a different path was accepted: %v", err)
	}
	if calls.Load() != beforeDuplicate {
		t.Fatalf("duplicate input reached the model: calls before=%d after=%d", beforeDuplicate, calls.Load())
	}
	if _, err := os.Stat(duplicateOutput); !os.IsNotExist(err) {
		t.Fatalf("duplicate input left an output file: %v", err)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			t.Fatalf("multi-input command produced an unexpected merged JSON file %q", entry.Name())
		}
	}
}

func TestReportCommandMultiInputFailureDoesNotPublishAndCanRetry(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	material.Review.RunID = "retry-unit-one"
	second := reportTestMaterial(true)
	second.Review.RunID = "retry-unit-two"
	paths := []string{writeReportInput(t, material), writeReportInput(t, second)}
	type requestEvidence struct {
		input  string
		prompt string
	}
	var calls atomic.Int32
	receivedInputs := make(chan requestEvidence, maxReportHTMLAttempts+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		messages := request["messages"].([]any)
		user := messages[1].(map[string]any)
		var input report.MultiReportInput
		if err := json.Unmarshal([]byte(fmt.Sprint(user["content"])), &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var prompt strings.Builder
		for _, message := range messages {
			if item, ok := message.(map[string]any); ok {
				prompt.WriteString(fmt.Sprint(item["content"]))
				prompt.WriteByte('\n')
			}
		}
		receivedInputs <- requestEvidence{input: fmt.Sprint(user["content"]), prompt: prompt.String()}
		switch calls.Add(1) {
		case 1:
			invalid := strings.Replace(reportMultiHTMLFixture(input), "<h2>\u8d28\u91cf\u4e0e\u8986\u76d6</h2>", "<h2>\u9519\u8bef\u6807\u9898</h2>", 1)
			writeReportCompletion(w, invalid, "stop")
		case 2:
			invalid := strings.Replace(reportMultiHTMLFixture(input), "</body>", "<script>active</script></body>", 1)
			writeReportCompletion(w, invalid, "stop")
		case 3:
			writeReportCompletion(w, "<html>not a report</html>", "stop")
		default:
			writeReportCompletion(w, reportMultiHTMLFixture(input), "stop")
		}
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)
	out := filepath.Join(t.TempDir(), "retry.html")
	args := []string{"--input", paths[0], "--input", paths[1], "--output", out}
	_, failureStderr, err := executeReportCommand(args)
	if err == nil {
		t.Fatal("invalid model HTML was accepted")
	}
	if got := strings.Count(failureStderr, "phase=html_generation attempt="); got != maxReportHTMLAttempts {
		t.Fatalf("unrecoverable output made %d requests, want %d: %s", got, maxReportHTMLAttempts, failureStderr)
	}
	if !strings.Contains(failureStderr, fmt.Sprintf("output_tokens=%d", 456*maxReportHTMLAttempts)) || !strings.Contains(failureStderr, "recovered=false") {
		t.Fatalf("exhausted repair metrics were not accumulated: %s", failureStderr)
	}
	var originalInput string
	for attempt := 0; attempt < maxReportHTMLAttempts; attempt++ {
		request := <-receivedInputs
		if attempt == 0 {
			originalInput = request.input
		} else if request.input != originalInput {
			t.Fatalf("repair attempt %d changed the report JSON", attempt+1)
		}
		if attempt == 1 && !strings.Contains(request.prompt, "required report section or Chinese heading") {
			t.Fatal("second attempt omitted the first heading diagnostic")
		}
		if attempt == 2 && !strings.Contains(request.prompt, "unsupported, hidden, or unsafe content") {
			t.Fatal("third attempt omitted the unsafe-element diagnostic")
		}
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("invalid model HTML left a final output file: %v", err)
	}
	if _, stderr, err := executeReportCommand(args); err != nil {
		t.Fatalf("retry with valid multi-unit HTML failed: %v\nstderr: %s", err, stderr)
	}
	if calls.Load() != maxReportHTMLAttempts+1 {
		t.Fatalf("model was called %d times, want %d total requests", calls.Load(), maxReportHTMLAttempts+1)
	}
	if secondInput := <-receivedInputs; secondInput.input != originalInput {
		t.Fatal("fresh CLI invocation changed the original report JSON")
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("successful retry did not publish HTML: %v", err)
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		messages := request["messages"].([]any)
		user := messages[1].(map[string]any)
		var input report.MultiReportInput
		if err := json.Unmarshal([]byte(fmt.Sprint(user["content"])), &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeReportCompletion(w, reportMultiHTMLFixture(input), "stop")
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
	input, err := report.NewMultiReportInput([]report.Material{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if err := report.ValidateMultiHTMLDocument(string(document), input); err != nil {
		t.Fatalf("long multi-input report failed validation: %v", err)
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
		writeReportCompletion(w, reportHTMLFixture(material), "stop")
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
	cmd.SetArgs(nil)
	cmd.SetContext(context.Background())
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
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

func TestReportCommandBoundsInputBeforeModelRequest(t *testing.T) {
	setTestHome(t, t.TempDir())
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeReportCompletion(w, "", "stop")
	}))
	defer server.Close()
	configureReportLLM(t, server.URL)

	material := reportTestMaterial(false)
	material.Repository.Name = strings.Repeat("repository-name ", 35_000)
	input := writeReportInput(t, material)
	_, stderr, err := executeReportCommand([]string{"--input", input, "--output", filepath.Join(t.TempDir(), "must-not-exist.html")})
	if err == nil || !strings.Contains(err.Error(), "tokens") {
		t.Fatalf("oversized model input was accepted: err=%v stderr=%s", err, stderr)
	}
	if requests.Load() != 0 {
		t.Fatalf("input over the token budget reached the model %d times", requests.Load())
	}
}

func TestReportCommandRejectsTimeoutAndTruncatedModelOutput(t *testing.T) {
	setTestHome(t, t.TempDir())
	material := reportTestMaterial(false)
	input := writeReportInput(t, material)

	t.Run("timeout", func(t *testing.T) {
		started := make(chan struct{}, maxReportHTMLAttempts)
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			started <- struct{}{}
			time.Sleep(1500 * time.Millisecond)
			writeReportCompletion(w, reportHTMLFixture(material), "stop")
		}))
		defer server.Close()
		configureReportLLM(t, server.URL)
		t.Setenv("OCR_LLM_TIMEOUT", "1")
		out := filepath.Join(t.TempDir(), "timeout.html")
		_, stderr, err := executeReportCommand([]string{"--input", input, "--output", out})
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timed out") && !strings.Contains(strings.ToLower(err.Error()), "deadline") {
			t.Fatalf("timed-out request was accepted: err=%v stderr=%s", err, stderr)
		}
		select {
		case <-started:
		default:
			t.Fatal("timeout test never reached the model endpoint")
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("timeout left a final HTML file: %v", err)
		}
		if !strings.Contains(stderr, "elapsed=") || !strings.Contains(stderr, "usage=estimated") {
			t.Fatalf("timeout diagnostics missing: %s", stderr)
		}
		if calls.Load() != maxReportHTMLAttempts {
			t.Fatalf("timeout recovery made %d requests, want %d", calls.Load(), maxReportHTMLAttempts)
		}
	})

	t.Run("truncated output", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			writeReportCompletion(w, reportHTMLFixture(material), "length")
		}))
		defer server.Close()
		configureReportLLM(t, server.URL)
		out := filepath.Join(t.TempDir(), "truncated.html")
		_, stderr, err := executeReportCommand([]string{"--input", input, "--output", out})
		if err == nil || !strings.Contains(err.Error(), "token limit") {
			t.Fatalf("truncated model output was accepted: err=%v stderr=%s", err, stderr)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("truncated output left a final HTML file: %v", err)
		}
		if calls.Load() != maxReportHTMLAttempts {
			t.Fatalf("truncated response made %d requests, want %d", calls.Load(), maxReportHTMLAttempts)
		}
	})
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

func writeReportCompletion(w http.ResponseWriter, content, finishReason string) {
	writeReportCompletionWithUsage(w, content, finishReason, 123, 456)
}

func writeReportCompletionWithUsage(w http.ResponseWriter, content, finishReason string, inputTokens, outputTokens int) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "report-test", "model": "fake-report-model",
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": finishReason,
			"message": map[string]any{"role": "assistant", "content": content},
		}},
		"usage": map[string]any{"prompt_tokens": inputTokens, "completion_tokens": outputTokens, "total_tokens": inputTokens + outputTokens},
	})
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
