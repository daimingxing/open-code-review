// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRunMainTaskCorrectsMCPArguments(t *testing.T) {
	var serviceCalls atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "knowledge-test", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "read_knowledge", InputSchema: map[string]any{
		"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}, "additionalProperties": false,
	}}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		serviceCalls.Add(1)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "knowledge chapter"}}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return server }, nil))
	defer httpServer.Close()
	client, err := ocrmcp.NewRemoteClient(context.Background(), "knowledge", httpServer.URL, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	toolResponse := func(name, arguments string) *llm.ChatResponse {
		resp := fileReadToolCallResponse(name, arguments)
		resp.Choices[0].Message.ToolCalls[0].Function.Name = name
		return resp
	}
	fake := &fakeClient{responses: []*llm.ChatResponse{
		toolResponse("nonexistent_tool", `{}`),
		toolResponse("read_knowledge", `{"path":42}`),
		toolResponse("read_knowledge", `{"path":"chapter.md","start_line":1}`),
		toolResponse("read_knowledge", `{"path":"chapter.md"}`),
		taskDoneResponse(),
	}}
	deps := newTestDeps(fake)
	ocrmcp.RegisterAll(deps.Tools, client, []string{"read_knowledge"})
	deps.MainToolDefs = ocrmcp.CollectToolDefs([]*ocrmcp.Client{client}, deps.Tools)
	var progress bytes.Buffer
	defer stdout.Swap(&progress)()
	completed, _, err := NewRunner(deps).RunMainTask(context.Background(), []llm.Message{llm.NewTextMessage("system", "Review"), llm.NewTextMessage("user", "Read chapter.md")}, "test")
	if err != nil || !completed {
		t.Fatalf("completed=%v err=%v", completed, err)
	}
	if serviceCalls.Load() != 1 {
		t.Fatalf("service=%d", serviceCalls.Load())
	}
	for i, want := range []string{"Available tools", "Invalid declared fields: [path]", "start_line", "knowledge chapter"} {
		data, _ := json.Marshal(fake.requests[i+1].Messages)
		if !strings.Contains(string(data), want) {
			t.Fatalf("request %d missing feedback %q", i+1, want)
		}
	}
	if !strings.Contains(progress.String(), "task_done: DONE") {
		t.Fatal(progress.String())
	}
}

func TestUsageStatusTracksMissingAndZeroCache(t *testing.T) {
	runner := NewRunner(Deps{})
	runner.RecordUsage(&llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, CacheReadKnown: true})
	if runner.UsageStatus() != "reported" || runner.CacheUsageStatus(false) != "reported" {
		t.Fatal("reported zero cache was lost")
	}
	runner.RecordUsage(&llm.UsageInfo{PromptTokens: 3, Incomplete: true})
	if runner.UsageStatus() != "partial" || runner.CacheUsageStatus(false) != "partial" || runner.CacheUsageStatus(true) != "unavailable" {
		t.Fatal("partial usage was hidden")
	}
	if runner.TotalInputTokens() != 13 || runner.TotalOutputTokens() != 5 {
		t.Fatal("aggregate usage changed")
	}
}

func TestRunMainTaskTextOnlyThenDone(t *testing.T) {
	text := "Report ready."
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &text}}}},
		taskDoneResponse(),
	}}
	var progress bytes.Buffer
	defer stdout.Swap(&progress)()
	completed, _, err := NewRunner(newTestDeps(client)).RunMainTask(context.Background(), []llm.Message{llm.NewTextMessage("system", "Review"), llm.NewTextMessage("user", "Finish")}, "test")
	if err != nil || !completed || len(client.requests) != 2 {
		t.Fatalf("completed=%v err=%v requests=%d", completed, err, len(client.requests))
	}
	if strings.Contains(progress.String(), "parsed") || !strings.Contains(progress.String(), "No tool calls returned") || !strings.Contains(progress.String(), "task_done: DONE") {
		t.Fatal(progress.String())
	}
}
