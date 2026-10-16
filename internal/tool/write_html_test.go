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
