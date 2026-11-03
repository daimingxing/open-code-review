// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/pathutil"
	"github.com/bmatcuk/doublestar/v4"
)

func (fr *FileReader) contentPath(path string) (string, string, error) {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(fr.RepoDir, path)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", "", err
	}
	if fr.Knowledge.Contains(abs) {
		return abs, "", nil
	}
	root, err := filepath.Abs(fr.RepoDir)
	if err != nil {
		return "", "", err
	}
	if !pathutil.WithinBase(root, abs) {
		return "", "", fmt.Errorf("path %q is outside repository and authorized knowledge directory", path)
	}
	rel, err := filepath.Rel(root, abs)
	return abs, filepath.ToSlash(rel), err
}

func (fr *FileReader) readKnowledgeLines(path string, start, max, end int) (lines []string, total int, retErr error) {
	a := knowledge.Access{Operation: "file_read", Path: path, Status: "failed", Partial: true}
	defer func() {
		if retErr != nil {
			a.Reason = retErr.Error()
		}
		fr.Knowledge.Record(a)
	}()
	resolved, err := fr.Knowledge.Resolve(path)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(resolved)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("knowledge path is not a regular file")
	}
	h := sha256.New()
	lines, total, err = scanLines(io.TeeReader(f, h), start, max)
	if err != nil {
		return nil, total, err
	}
	if total > 0 && start > total {
		return nil, total, fmt.Errorf("file has only %d lines, requested start %d", total, start)
	}
	a.Status, a.StartLine, a.EndLine, a.TotalLines = "read", start, start-1+len(lines), total
	a.Partial = start > 1 || len(lines) < total
	a.Truncated = start-1+len(lines) < total && len(lines) == fileReadMaxLines && (end == 0 || end > a.EndLine)
	a.SHA256, a.Size, a.Modified = fmt.Sprintf("sha256:%x", h.Sum(nil)), info.Size(), info.ModTime().UnixNano()
	return lines, total, nil
}

func (fr *FileReader) knowledgeDirectory(args map[string]any) (string, error) {
	dir, _ := args["directory"].(string)
	if dir == "" {
		return "", nil
	}
	abs, _, err := fr.contentPath(dir)
	if err != nil {
		return "", err
	}
	if !fr.Knowledge.Contains(abs) {
		return "", fmt.Errorf("directory must be inside the configured knowledge directory; omit it for code searches")
	}
	return fr.Knowledge.Resolve(abs)
}

func knowledgePatterns(path, root string, patterns []string) (bool, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false, err
	}
	rel = filepath.ToSlash(rel)
	included, hasInclude := false, false
	for _, pattern := range patterns {
		exclude := strings.HasPrefix(pattern, ":(exclude)") || strings.HasPrefix(pattern, ":!")
		pattern = strings.TrimPrefix(strings.TrimPrefix(pattern, ":(exclude)"), ":!")
		if filepath.IsAbs(pattern) {
			if !pathutil.WithinBase(root, pattern) {
				return false, fmt.Errorf("file pattern is outside knowledge directory")
			}
			pattern, err = filepath.Rel(root, pattern)
			if err != nil {
				return false, err
			}
		}
		pattern = filepath.ToSlash(pattern)
		if hasTraversalPathComponent(pattern) {
			return false, fmt.Errorf("file_patterns must not contain ..")
		}
		matched, err := doublestar.Match(strings.TrimSuffix(pattern, "/"), rel)
		if err != nil {
			return false, err
		}
		matched = matched || strings.HasPrefix(rel, strings.TrimSuffix(pattern, "/")+"/")
		if !strings.Contains(pattern, "/") {
			baseMatch, e := doublestar.Match(pattern, filepath.Base(path))
			if e != nil {
				return false, e
			}
			matched = matched || baseMatch
		}
		if exclude && matched {
			return false, nil
		}
		if !exclude {
			hasInclude = true
			included = included || matched
		}
	}
	return !hasInclude || included, nil
}

func (p *CodeSearchProvider) searchKnowledge(ctx context.Context, root, text string, sensitive, perl bool, patterns []string) (result string, retErr error) {
	ctx, cancel := context.WithTimeout(ctx, gitGrepTimeout)
	defer cancel()
	a := knowledge.Access{Operation: "code_search", Path: root, Status: "searched", Query: text, Partial: true}
	defer func() {
		if retErr != nil {
			a.Status = "failed"
			a.Reason = retErr.Error()
		}
		if result != "" {
			a.ResultSHA256 = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(result)))
		}
		a.Truncated = strings.Contains(result, "Results truncated") || strings.Contains(result, "Note: Showing")
		p.FileReader.Knowledge.Record(a)
	}()
	files, err := p.FileReader.Knowledge.Files(ctx, root)
	if err != nil {
		return "", err
	}
	var selected []string
	for _, file := range files {
		ok, err := knowledgePatterns(file, root, patterns)
		if err != nil {
			return "", err
		}
		if ok {
			rel, err := filepath.Rel(root, file)
			if err != nil {
				return "", err
			}
			selected = append(selected, ":(literal)"+filepath.ToSlash(rel))
		}
	}
	if len(selected) == 0 {
		return "No matches found", nil
	}
	reader := *p.FileReader
	reader.RepoDir, reader.Ref = root, ""
	search := &CodeSearchProvider{FileReader: &reader, diskFiles: true}
	result, err = search.gitGrep(ctx, text, sensitive, perl, selected)
	if err != nil {
		return "", err
	}
	lines := strings.Split(result, "\n")
	matchedPath := ""
	for i, line := range lines {
		if strings.HasPrefix(line, "File: ") {
			lines[i] = "File: " + filepath.ToSlash(filepath.Join(root, strings.TrimPrefix(line, "File: ")))
			matchedPath = strings.TrimPrefix(lines[i], "File: ")
		} else if number, _, ok := strings.Cut(line, "|"); ok && matchedPath != "" {
			if lineNo, err := strconv.Atoi(number); err == nil {
				a.Matches = append(a.Matches, knowledge.Match{Path: matchedPath, Line: lineNo})
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}
