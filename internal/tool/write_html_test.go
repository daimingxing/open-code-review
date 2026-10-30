// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"strings"
	"testing"
)

func TestWriteHTMLProviderStoresDraftWithoutParsing(t *testing.T) {
	draft := &HTMLDraft{}
	provider := NewWriteHTML(draft, 1024)
	if provider.Tool().Name() != WriteHTMLName {
		t.Fatalf("tool name = %q, want %q", provider.Tool().Name(), WriteHTMLName)
	}
	raw := "<!doctype html><html><body><script>alert(1)</script>ok</body></html>"
	result, err := provider.Execute(context.Background(), map[string]any{"html": raw})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "saved") {
		t.Fatalf("unexpected result: %s", result)
	}
	got, ok := draft.Content()
	if !ok || got != raw {
		t.Fatalf("draft = (%q, %v), want exact raw HTML", got, ok)
	}
}

func TestWriteHTMLProviderRejectsEmptyAndOversized(t *testing.T) {
	draft := &HTMLDraft{}
	provider := NewWriteHTML(draft, 8)
	result, err := provider.Execute(context.Background(), map[string]any{"html": "   "})
	if err != nil {
		t.Fatalf("empty Execute: %v", err)
	}
	if !strings.Contains(result, "required") {
		t.Fatalf("empty result = %q", result)
	}
	if _, ok := draft.Content(); ok {
		t.Fatal("empty html must not mark draft written")
	}
	result, err = provider.Execute(context.Background(), map[string]any{"html": "0123456789"})
	if err != nil {
		t.Fatalf("oversized Execute: %v", err)
	}
	if !strings.Contains(result, "exceeds") {
		t.Fatalf("oversized result = %q", result)
	}
}

func TestHTMLDraftLocalEditing(t *testing.T) {
	ctx := context.Background()
	draft := &HTMLDraft{}
	edit := NewEditHTML(draft, 1024)
	read := NewReadHTML(draft)
	if result, _ := edit.Execute(ctx, map[string]any{"old_text": "x", "new_text": "y"}); !strings.Contains(result, "no HTML draft") {
		t.Fatal(result)
	}
	if result, _ := read.Execute(ctx, nil); !strings.Contains(result, "no HTML draft") {
		t.Fatal(result)
	}
	original := "<main>\n<output>14 / 14</output>\n<script>run()</script>\n</main>"
	if err := draft.Set(original); err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"old_text": "", "new_text": "x"},
		{"old_text": "missing", "new_text": "x"},
		{"old_text": "14", "new_text": "13"},
		{"old_text": "<main>"},
		{"old_text": "<main>", "new_text": strings.Repeat("x", 1100)},
	} {
		result, err := edit.Execute(ctx, args)
		if err != nil || !strings.HasPrefix(result, "Error:") {
			t.Fatalf("result=%q err=%v", result, err)
		}
		if got, _ := draft.Content(); got != original {
			t.Fatal("failed edit changed draft")
		}
	}
	result, err := read.Execute(ctx, map[string]any{"start_line": float64(2), "end_line": float64(2)})
	if err != nil || !strings.Contains(result, "LINE_RANGE: 2-2") || !strings.Contains(result, "2|<output>14 / 14</output>") || strings.Contains(result, "run()") {
		t.Fatalf("read=%q err=%v", result, err)
	}
	for _, replacement := range []string{"<output>13 / 13</output>", ""} {
		old := "<output>14 / 14</output>"
		if replacement == "" {
			old = "<output>13 / 13</output>"
		}
		result, err = edit.Execute(ctx, map[string]any{"old_text": old, "new_text": replacement})
		if err != nil || !strings.Contains(result, "not published") {
			t.Fatalf("edit=%q err=%v", result, err)
		}
	}
	if got, _ := draft.Content(); got != strings.Replace(original, "<output>14 / 14</output>", "", 1) {
		t.Fatal("unexpected final draft")
	}
}

func TestReadHTMLBounds(t *testing.T) {
	draft := &HTMLDraft{}
	_ = draft.Set(strings.Repeat("line\n", 600))
	read := NewReadHTML(draft)
	result, _ := read.Execute(context.Background(), nil)
	if !strings.Contains(result, "LINE_RANGE: 1-500") || !strings.Contains(result, "IS_TRUNCATED: true") {
		t.Fatal(result)
	}
	for _, args := range []map[string]any{{"start_line": 1.5}, {"start_line": float64(0)}, {"start_line": "2"}, {"start_line": float64(605)}, {"start_line": float64(3), "end_line": float64(2)}} {
		if result, _ := read.Execute(context.Background(), args); !strings.HasPrefix(result, "Error:") {
			t.Fatal(result)
		}
	}
	_ = draft.Set(strings.Repeat("x", 17000))
	if result, _ := read.Execute(context.Background(), nil); !strings.Contains(result, "NEXT_OFFSET: 4000") || !strings.Contains(result, "IS_TRUNCATED: true") {
		t.Fatal(result)
	}
	if result, _ := read.Execute(context.Background(), map[string]any{"start_offset": float64(16000)}); !strings.Contains(result, "CHARACTER_RANGE: 16000-17000") || !strings.Contains(result, "IS_TRUNCATED: false") {
		t.Fatal(result)
	}
}

func TestHTMLDraftRejectsOverlappingMatches(t *testing.T) {
	draft := &HTMLDraft{}
	_ = draft.Set("aaa")
	if err := draft.edit("aa", "b", 100); err == nil {
		t.Fatal("overlapping matches were accepted")
	}
	if got, _ := draft.Content(); got != "aaa" {
		t.Fatal(got)
	}
}

func TestReadHTMLLongUnicodeLine(t *testing.T) {
	draft := &HTMLDraft{}
	_ = draft.Set(strings.Repeat("\u4e2d", 8000))
	read := NewReadHTML(draft)
	result, _ := read.Execute(context.Background(), nil)
	if !strings.Contains(result, "NEXT_OFFSET: 4000") || strings.Count(result, "\u4e2d") != 4000 {
		t.Fatal("long Unicode line was not bounded correctly")
	}
	result, _ = read.Execute(context.Background(), map[string]any{"start_offset": float64(4000)})
	if !strings.Contains(result, "IS_TRUNCATED: false") || strings.Count(result, "\u4e2d") != 4000 {
		t.Fatal("Unicode continuation was not preserved")
	}
}
