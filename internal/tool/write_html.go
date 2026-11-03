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

// HTMLDraft 保存当前运行的草稿；显式编辑只替换原文，不进行 HTML 内容审查。 // allow-non-english: 用户要求中文代码注释
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

func (d *HTMLDraft) edit(oldText, newText string, maxBytes int) error {
	if d == nil {
		return fmt.Errorf("HTML draft store is unavailable")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.written {
		return fmt.Errorf("no HTML draft exists; call write_html first")
	}
	if oldText == "" {
		return fmt.Errorf("old_text must not be empty")
	}
	count := strings.Count(d.content, oldText)
	if count == 1 && strings.Index(d.content, oldText) != strings.LastIndex(d.content, oldText) {
		count = 2
	}
	if count != 1 {
		return fmt.Errorf("old_text must match exactly once (matched %d); read the draft and include more context", count)
	}
	updated := strings.Replace(d.content, oldText, newText, 1)
	if strings.TrimSpace(updated) == "" {
		return fmt.Errorf("edited HTML draft must not be empty")
	}
	if maxBytes > 0 && len(updated) > maxBytes {
		return fmt.Errorf("html exceeds %d bytes", maxBytes)
	}
	d.content = updated
	return nil
}

type EditHTMLProvider struct {
	Draft    *HTMLDraft
	MaxBytes int
}

func NewEditHTML(draft *HTMLDraft, maxBytes int) *EditHTMLProvider {
	return &EditHTMLProvider{Draft: draft, MaxBytes: maxBytes}
}

func (p *EditHTMLProvider) Tool() Tool { return Dynamic("edit_html") }

func (p *EditHTMLProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	oldText, oldOK := args["old_text"].(string)
	newText, newOK := args["new_text"].(string)
	if !oldOK || !newOK {
		return "Error: old_text and new_text must be strings; new_text may be empty", nil
	}
	if err := p.Draft.edit(oldText, newText, p.MaxBytes); err != nil {
		return "Error: " + err.Error(), nil
	}
	return "HTML draft updated; not published. Use read_html to inspect it, or task_done when complete.", nil
}

type ReadHTMLProvider struct{ Draft *HTMLDraft }

func NewReadHTML(draft *HTMLDraft) *ReadHTMLProvider { return &ReadHTMLProvider{Draft: draft} }
func (p *ReadHTMLProvider) Tool() Tool               { return Dynamic("read_html") }

func (p *ReadHTMLProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	content, written := p.Draft.Content()
	if !written {
		return "Error: no HTML draft exists; call write_html first", nil
	}
	start, end := 1, 0
	for key, target := range map[string]*int{"start_line": &start, "end_line": &end} {
		if value, exists := args[key]; exists {
			n, ok := value.(float64)
			if !ok || n < 1 || n > float64(len(content)+1) || float64(int(n)) != n {
				return "Error: " + key + " must be a positive integer within the draft", nil
			}
			*target = int(n)
		}
	}
	lines := strings.Split(content, "\n")
	if start > len(lines) || (end > 0 && end < start) {
		return "Error: requested line range is outside the HTML draft", nil
	}
	if raw, exists := args["start_offset"]; exists || (start <= len(lines) && len(lines[start-1]) > 16000) {
		offset := 0
		if exists {
			n, ok := raw.(float64)
			if !ok || n < 0 || n > float64(len(content)) || float64(int(n)) != n {
				return "Error: start_offset must be a nonnegative character offset", nil
			}
			offset = int(n)
		} else {
			for _, line := range lines[:start-1] {
				offset += len([]rune(line)) + 1
			}
		}
		chars := []rune(content)
		if offset >= len(chars) {
			return "Error: start_offset is outside the HTML draft", nil
		}
		stop := offset + 4000
		if stop > len(chars) {
			stop = len(chars)
		}
		return fmt.Sprintf("HTML draft\nCHARACTER_RANGE: %d-%d (zero-based, end-exclusive)\nNEXT_OFFSET: %d\nIS_TRUNCATED: %t\n%s", offset, stop, stop, stop < len(chars), string(chars[offset:stop])), nil
	}
	if end == 0 || end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return "Error: requested line range is outside the HTML draft", nil
	}
	requestedEnd := end
	if end-start+1 > fileReadMaxLines {
		end = start + fileReadMaxLines - 1
	}
	var b strings.Builder
	chars := 0
	displayEnd := start - 1
	for i := start - 1; i < end; i++ {
		if chars+len(lines[i]) > 16000 {
			break
		}
		fmt.Fprintf(&b, "%d|%s\n", i+1, lines[i])
		chars += len(lines[i])
		displayEnd = i + 1
	}
	return fmt.Sprintf("HTML draft (Total lines: %d)\nLINE_RANGE: %d-%d\nIS_TRUNCATED: %t\n%s", len(lines), start, displayEnd, displayEnd < requestedEnd, b.String()), nil
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
	return fmt.Sprintf("HTML draft saved (%d bytes); not published. Use read_html/edit_html for local changes, or task_done when complete.", len(content)), nil
}
