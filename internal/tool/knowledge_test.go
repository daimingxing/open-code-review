// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/knowledge"
)

func TestKnowledgeReadBoundsFailuresAndEscape(t *testing.T) {
	repo, root, outside := t.TempDir(), t.TempDir(), t.TempDir()
	base, err := knowledge.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	fr := &FileReader{RepoDir: repo, Mode: ModeRange, Ref: "not-a-ref", Knowledge: base}
	path := filepath.Join(root, "long.md")
	if err := os.WriteFile(path, []byte(strings.Repeat("line\n", 750)), 0600); err != nil {
		t.Fatal(err)
	}
	reader := NewFileRead(fr)
	result, err := reader.Execute(context.Background(), map[string]any{"file_path": path})
	if err != nil || !strings.Contains(result, "IS_TRUNCATED: true") {
		t.Fatalf("truncation: %s %v", result, err)
	}
	result, err = reader.Execute(context.Background(), map[string]any{"file_path": path, "start_line": float64(501)})
	if err != nil || !strings.Contains(result, "501|line") {
		t.Fatalf("continuation: %s %v", result, err)
	}
	result, err = reader.Execute(context.Background(), map[string]any{"file_path": path, "start_line": float64(1), "end_line": float64(500)})
	if err != nil || !strings.Contains(result, "IS_TRUNCATED: false") || base.Snapshot()[2].Truncated || !base.Snapshot()[2].Partial {
		t.Fatalf("explicit partial range is not truncation: %s %v %+v", result, err, base.Snapshot())
	}
	_, err = reader.Execute(context.Background(), map[string]any{"file_path": filepath.Join(root, "missing.md")})
	if err == nil {
		t.Fatal("missing knowledge file succeeded")
	}
	secret := filepath.Join(outside, "secret.md")
	sourceReader := *fr
	sourceReader.Mode, sourceReader.Ref = ModeWorkspace, ""
	before := len(base.Snapshot())
	result, err = NewFileFind(&sourceReader).Execute(context.Background(), map[string]any{"query_name": "long.md"})
	if err != nil || strings.Contains(result, "long.md") || len(base.Snapshot()) != before {
		t.Fatalf("source lookup unexpectedly walked knowledge: %s %v", result, err)
	}
	if err := os.WriteFile(secret, []byte("outside-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := fr.Read(context.Background(), secret); err == nil {
		t.Fatal("outside read accepted")
	}
	if _, err := NewCodeSearch(fr).Execute(context.Background(), map[string]any{"directory": outside, "search_text": "secret"}); err == nil {
		t.Fatal("outside search accepted")
	}
	if _, err := NewFileFind(fr).Execute(context.Background(), map[string]any{"directory": outside, "query_name": "secret"}); err == nil {
		t.Fatal("outside lookup accepted")
	}
	link := filepath.Join(root, "escape.md")
	if err := os.Symlink(secret, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Skipf("platform does not permit symlink: %v", err)
		}
		cmd := exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-Command", "New-Item -ItemType Junction -Path $env:OCR_TEST_LINK -Target $env:OCR_TEST_TARGET | Out-Null")
		cmd.Env = append(os.Environ(), "OCR_TEST_LINK="+link, "OCR_TEST_TARGET="+outside)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("create Windows junction: %v %s", err, output)
		}
		link = filepath.Join(link, "secret.md")
	}
	if _, err := fr.Read(context.Background(), link); err == nil {
		t.Fatal("symlink escape accepted")
	}
	result, err = NewCodeSearch(fr).Execute(context.Background(), map[string]any{"directory": root, "search_text": "outside-secret"})
	if err != nil || strings.Contains(result, "outside-secret") {
		t.Fatalf("search leaked symlink: %s %v", result, err)
	}
}
