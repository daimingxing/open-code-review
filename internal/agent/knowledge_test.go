// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

type knowledgeClient func(llm.ChatRequest) *llm.ChatResponse

func TestKnowledgePromptAbsentDoesNotRequestReading(t *testing.T) {
	a := &Agent{args: Args{Template: template.Template{MainTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "system", Content: "Review."}, {Role: "user", Content: "Changes."}}}}}}
	messages := a.buildMainTaskMessages("", "", "", "", "")
	if strings.Contains(messages[0].ExtractText(), "Use the native") || !strings.Contains(messages[0].ExtractText(), "no extra knowledge directory") || strings.Contains(messages[1].ExtractText(), "<knowledge_bases>") {
		t.Fatal("unconfigured review received a knowledge reading instruction or empty block")
	}
}

func (f knowledgeClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return f(req), nil
}

func TestKnowledgeSurvivesSharedCompression(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "context.md")
	if err := os.WriteFile(path, []byte(strings.Repeat("framework contract context evidence\n", 450)), 0600); err != nil {
		t.Fatal(err)
	}
	base, err := knowledge.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := template.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	tpl.MaxTokens, tpl.MaxToolRequestTimes, tpl.MaxReviewRounds = 4000, 10, 1
	tpl.PlanTask = nil
	tpl.MainTask.Messages = []template.ChatMessage{{Role: "system", Content: "Review code with tools."}, {Role: "user", Content: "Review {{diffs}}"}}
	reg := tool.NewRegistry()
	reg.Register(tool.NewFileRead(&tool.FileReader{RepoDir: t.TempDir(), Knowledge: base}))
	var rounds, compressions atomic.Int32
	var continued atomic.Bool
	client := knowledgeClient(func(req llm.ChatRequest) *llm.ChatResponse {
		if len(req.Tools) == 0 {
			compressions.Add(1)
			text := "### Identified Code Issues\nNone.\n### Tool Call Conclusions\nRead partial knowledge.\n### Pending Tasks\nContinue reading.\n### Key Decisions\nUse reference directory.\n### Current Progress\nReview ongoing."
			return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &text}}}}
		}
		var text strings.Builder
		for _, msg := range req.Messages {
			text.WriteString(msg.ExtractText())
		}
		encodedRoot, _ := json.Marshal([]string{root})
		if !strings.Contains(text.String(), "<knowledge_bases>") || !strings.Contains(text.String(), string(encodedRoot)) || !strings.Contains(text.String(), "Reference knowledge") {
			t.Error("knowledge task information lost")
		}
		if strings.Contains(text.String(), "<previous_review_summary>") {
			continued.Store(true)
		}
		n := rounds.Add(1)
		if n > 3 {
			return agentTaskDoneResponse()
		}
		end := 1
		if n == 2 {
			end = 450
		}
		args, _ := json.Marshal(map[string]any{"file_path": path, "start_line": 1, "end_line": end})
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{{ID: "read", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: string(args)}}}}}}}
	})
	a := New(Args{RepoDir: t.TempDir(), Knowledge: base, Template: *tpl, LLMClient: client, Tools: reg, MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "file_read"}}, {Type: "function", Function: llm.FunctionDef{Name: "task_done"}}}})
	g := FileGroup{Diffs: []model.Diff{{NewPath: "main.go", Diff: "+code"}}}
	completed, stop, err := a.executeGroupSubtask(context.Background(), g)
	if err != nil || !completed || stop != nil || compressions.Load() == 0 || !continued.Load() {
		t.Fatalf("compression continuation: completed=%v stop=%v err=%v compression=%d continued=%v", completed, stop, err, compressions.Load(), continued.Load())
	}
	if len(base.Snapshot()) != 3 {
		t.Fatalf("tool calls did not continue after compression: %+v", base.Snapshot())
	}
	if err := a.Session().Finalize(); err != nil {
		t.Fatal(err)
	}
}
