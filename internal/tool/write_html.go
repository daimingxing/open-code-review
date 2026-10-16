// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

const WriteHTMLName = "write_html"

// HTMLDraft holds the latest write_html payload until the command publishes a file.
// Content is stored as-is: nothing here parses, validates, or rewrites HTML.
type HTMLDraft struct {
	mu      sync.Mutex
	content string
	written bool
}

func (d *HTMLDraft) Content() (string, bool) {
	if d == nil {
		return "", false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.content, d.written
}

func (d *HTMLDraft) Set(content string) error {
	if d == nil {
		return fmt.Errorf("HTML draft store is unavailable")
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("html content must not be empty")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.content = content
	d.written = true
	return nil
}

type WriteHTMLProvider struct {
	Draft     *HTMLDraft
	MaxBytes  int
	toolValue Tool
}

func NewWriteHTML(draft *HTMLDraft, maxBytes int) *WriteHTMLProvider {
	return &WriteHTMLProvider{
		Draft:     draft,
		MaxBytes:  maxBytes,
		toolValue: Dynamic(WriteHTMLName),
	}
}

func (p *WriteHTMLProvider) Tool() Tool { return p.toolValue }

func (p *WriteHTMLProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	content, _ := args["html"].(string)
	if strings.TrimSpace(content) == "" {
		return "Error: html is required and must not be empty", nil
	}
	if p.MaxBytes > 0 && len(content) > p.MaxBytes {
		return fmt.Sprintf("Error: html exceeds %d bytes", p.MaxBytes), nil
	}
	if err := p.Draft.Set(content); err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	return fmt.Sprintf("HTML draft saved (%d bytes). Call task_done when the report is complete.", len(content)), nil
}
