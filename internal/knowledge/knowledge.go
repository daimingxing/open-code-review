// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package knowledge 管理显式授权的磁盘知识目录及其访问依据。 // allow-non-english: user requires Chinese code comments
package knowledge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/pathutil"
)

// Access 区分实际返回的行范围与完整源文件的内容标识，避免将局部返回误认为全文。 // allow-non-english: user requires Chinese code comments
type Access struct {
	Operation    string    `json:"operation"`
	Path         string    `json:"path"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
	StartLine    int       `json:"start_line,omitempty"`
	EndLine      int       `json:"end_line,omitempty"`
	TotalLines   int       `json:"total_lines,omitempty"`
	Partial      bool      `json:"partial"`
	Truncated    bool      `json:"truncated,omitempty"`
	SHA256       string    `json:"sha256,omitempty"`
	Size         int64     `json:"size,omitempty"`
	Modified     int64     `json:"modified,omitempty"`
	Changed      bool      `json:"changed,omitempty"`
	Inherited    bool      `json:"inherited,omitempty"`
	ObservedAt   time.Time `json:"observed_at"`
	Query        string    `json:"query,omitempty"`
	Paths        []string  `json:"paths,omitempty"`
	Matches      []Match   `json:"matches,omitempty"`
	ResultSHA256 string    `json:"result_sha256,omitempty"`
}

type Match struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

// Base 在并发审查分组间共享同一只读目录授权和访问依据。 // allow-non-english: user requires Chinese code comments
type Base struct {
	Root     string
	mu       sync.Mutex
	accesses []Access
}

func Open(dir string) (*Base, error) {
	if dir == "" {
		return nil, nil
	}
	root, err := pathutil.CanonicalPath(dir)
	if err != nil {
		return nil, fmt.Errorf("knowledge directory: %w", err)
	}
	f, err := os.Open(root)
	if err != nil {
		return nil, fmt.Errorf("knowledge directory: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("knowledge directory %q is not a directory", dir)
	}
	if _, err := f.Readdirnames(1); err != nil && err != io.EOF {
		return nil, fmt.Errorf("knowledge directory: %w", err)
	}
	return &Base{Root: root}, nil
}

func (b *Base) Contains(path string) bool {
	if b == nil || !filepath.IsAbs(path) {
		return false
	}
	// 根目录已规范化；路由只比较路径，访问时由 Resolve 校验链接，避免逐个源码路径查验磁盘。 // allow-non-english: user requires Chinese code comments
	rel, err := filepath.Rel(b.Root, filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// Resolve 同时校验请求路径和物理目标，目录内链接不能扩大授权范围。 // allow-non-english: user requires Chinese code comments
func (b *Base) Resolve(path string) (string, error) {
	if !b.Contains(path) {
		return "", fmt.Errorf("path %q is outside knowledge directory", path)
	}
	resolved, err := pathutil.CanonicalPath(path)
	if err != nil {
		return "", err
	}
	if !b.Contains(resolved) {
		return "", fmt.Errorf("path %q escapes knowledge directory", path)
	}
	return resolved, nil
}

func (b *Base) Record(a Access) {
	if b == nil {
		return
	}
	a.ObservedAt = time.Now().UTC()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.accesses = append(b.accesses, a)
}

func (b *Base) Snapshot() []Access {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := append([]Access{}, b.accesses...)
	for i := range out {
		a := &out[i]
		if a.Operation != "file_read" || a.Status != "read" {
			continue
		}
		path, err := b.Resolve(a.Path)
		if err != nil {
			a.Changed = true
			continue
		}
		info, err := os.Stat(path)
		a.Changed = a.Changed || err != nil || info.Size() != a.Size || info.ModTime().UnixNano() != a.Modified
	}
	return out
}

// Restore 只核对历史实际读取的文件，不为恢复审查索引或加载整个知识目录。 // allow-non-english: user requires Chinese code comments
func (b *Base) Restore(root string, accesses []Access) error {
	current := ""
	if b != nil {
		current = b.Root
	}
	if current != root {
		return fmt.Errorf("resume rejected: knowledge directory changed; start a new review")
	}
	if b == nil {
		return nil
	}
	for _, a := range accesses {
		if a.Operation == "file_read" && a.Status == "read" {
			path, err := b.Resolve(a.Path)
			if err != nil {
				return fmt.Errorf("resume rejected: knowledge source unavailable: %w", err)
			}
			digest, err := Digest(path)
			if err != nil || digest != a.SHA256 || a.Changed {
				return fmt.Errorf("resume rejected: knowledge source %q changed; start a new review", a.Path)
			}
		}
		a.Inherited = true
		b.accesses = append(b.accesses, a)
	}
	return nil
}

func Digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

func (b *Base) Files(ctx context.Context, directory string) ([]string, error) {
	root, err := b.Resolve(directory)
	if err != nil {
		return nil, err
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if _, err := b.Resolve(path); err != nil {
			return err
		}
		if d.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
