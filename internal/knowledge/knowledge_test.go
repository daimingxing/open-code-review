// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryValidationAndResume(t *testing.T) {
	if base, err := Open(""); err != nil || base != nil {
		t.Fatalf("unconfigured: %+v %v", base, err)
	}
	t.Chdir(t.TempDir())
	dir := "knowledge space \u77e5\u8bc6"
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	base, err := Open(dir)
	if err != nil || !filepath.IsAbs(base.Root) {
		t.Fatalf("relative directory: %+v %v", base, err)
	}
	files, err := base.Files(context.Background(), base.Root)
	if err != nil || len(files) != 0 {
		t.Fatalf("empty directory: %v %v", files, err)
	}
	path := filepath.Join(base.Root, "contract.md")
	if err := os.WriteFile(path, []byte("contract\n"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := Digest(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	base.Record(Access{Operation: "file_read", Path: path, Status: "read", SHA256: digest, Size: info.Size(), Modified: info.ModTime().UnixNano()})
	snapshot := base.Snapshot()
	if len(snapshot) != 1 || snapshot[0].Changed {
		t.Fatalf("initial evidence: %+v", snapshot)
	}
	child, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Restore(base.Root, snapshot); err != nil || !child.Snapshot()[0].Inherited {
		t.Fatalf("resume evidence: %v", err)
	}
	if err := child.Restore(t.TempDir(), snapshot); err == nil {
		t.Fatal("changed knowledge config accepted")
	}
	var disabled *Base
	if err := disabled.Restore(base.Root, snapshot); err == nil {
		t.Fatal("removed knowledge config accepted")
	}
	if err := os.WriteFile(path, []byte("modified contract\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !base.Snapshot()[0].Changed {
		t.Fatal("changed source was hidden")
	}
	if err := child.Restore(base.Root, snapshot); err == nil {
		t.Fatal("changed knowledge source accepted")
	}
}
