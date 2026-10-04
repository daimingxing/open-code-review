// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
)

func TestWriteMaterialRejectsExistingExplicitPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteMaterial(target, false, validMaterial()); err == nil {
		t.Fatal("WriteMaterial should refuse an existing explicit path")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("existing file was changed to %q", got)
	}
}

func TestWriteMaterialRejectsMissingParent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "missing", "report.json")
	if _, err := WriteMaterial(target, false, validMaterial()); err == nil {
		t.Fatal("WriteMaterial should reject a missing parent directory")
	}
	if _, err := os.Stat(filepath.Dir(target)); !os.IsNotExist(err) {
		t.Fatalf("missing parent was unexpectedly created: %v", err)
	}
}

func TestWriteMaterialNumbersAutomaticCollisionsExclusively(t *testing.T) {
	target := filepath.Join(t.TempDir(), "review.report.json")
	const count = 12
	paths := make(chan string, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			written, err := WriteMaterial(target, true, validMaterial())
			if err != nil {
				errs <- err
				return
			}
			paths <- written
		}()
	}
	wg.Wait()
	close(paths)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	seen := make(map[string]struct{}, count)
	for written := range paths {
		if _, exists := seen[written]; exists {
			t.Errorf("two writers returned the same path %q", written)
		}
		seen[written] = struct{}{}
		data, err := os.ReadFile(written)
		if err != nil {
			t.Errorf("read %q: %v", written, err)
			continue
		}
		var material Material
		if err := json.Unmarshal(data, &material); err != nil {
			t.Errorf("decode %q: %v", written, err)
		}
	}
	if len(seen) != count {
		t.Fatalf("created %d unique files, want %d", len(seen), count)
	}
}

func TestCopyMaterialExclusivelyDoesNotOverwriteAndRemovesPartialFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "report.json")
	created, _, err := copyMaterialExclusively(strings.NewReader("complete"), target)
	if err != nil || !created {
		t.Fatalf("copyMaterialExclusively = (%t, %v), want created file", created, err)
	}
	if _, _, err := copyMaterialExclusively(strings.NewReader("replacement"), target); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("copyMaterialExclusively on existing target = %v, want ErrExist", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "complete" {
		t.Fatalf("existing target = %q, %v; want original content", data, err)
	}

	partial := filepath.Join(t.TempDir(), "partial.json")
	copyErr := errors.New("injected read failure")
	created, _, err = copyMaterialExclusively(io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(copyErr)), partial)
	if !created || !errors.Is(err, copyErr) {
		t.Fatalf("failed copy = (%t, %v), want created file and source error", created, err)
	}
	if _, statErr := os.Stat(partial); !os.IsNotExist(statErr) {
		t.Fatalf("failed copy left a partial destination: %v", statErr)
	}
}

func TestRemoveIncompleteMaterialKeepsReplacedTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "report.json")
	ownedFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	ownedInfo, err := ownedFile.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := ownedFile.Close(); err != nil {
		t.Fatal(err)
	}

	backup := target + ".owned"
	if err := os.Rename(target, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeIncompleteMaterial(target, ownedInfo); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("replacement target = %q, %v; want preserved replacement", data, err)
	}
}

type injectedMaterialOutputFile struct {
	file    *os.File
	failAt  string
	failure error
}

func (f *injectedMaterialOutputFile) Stat() (os.FileInfo, error) {
	return f.file.Stat()
}

func (f *injectedMaterialOutputFile) Write(data []byte) (int, error) {
	if f.failAt != "write" {
		return f.file.Write(data)
	}
	n := len(data) / 2
	written, err := f.file.Write(data[:n])
	return written, errors.Join(err, f.failure)
}

func (f *injectedMaterialOutputFile) Sync() error {
	if f.failAt == "sync" {
		return f.failure
	}
	return f.file.Sync()
}

func (f *injectedMaterialOutputFile) Close() error {
	err := f.file.Close()
	if f.failAt == "close" {
		return errors.Join(err, f.failure)
	}
	return err
}

func TestCopyMaterialExclusivelyRemovesFailedTargetsOnWriteSyncAndClose(t *testing.T) {
	for _, phase := range []string{"write", "sync", "close"} {
		t.Run(phase, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "report.json")
			injected := errors.New("injected " + phase + " failure")
			open := func(path string) (materialOutputFile, error) {
				file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
				if err != nil {
					return nil, err
				}
				return &injectedMaterialOutputFile{file: file, failAt: phase, failure: injected}, nil
			}

			created, _, err := copyMaterialExclusivelyWith(strings.NewReader("report material"), target, open)
			if !created || !errors.Is(err, injected) {
				t.Fatalf("failed %s = (%t, %v), want created target and injected error", phase, created, err)
			}
			if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Fatalf("failed %s left a target file: %v", phase, statErr)
			}
		})
	}
}

func TestPathsConflictResolvesEquivalentPaths(t *testing.T) {
	dir := t.TempDir()
	native := filepath.Join(dir, "native.json")
	material := filepath.Join(dir, "nested", "..", "native.json")
	conflict, err := PathsConflict(native, material)
	if err != nil {
		t.Fatal(err)
	}
	if !conflict {
		t.Fatal("equivalent paths should conflict")
	}
	conflict, err = PathsConflict(native, filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if conflict {
		t.Fatal("different paths should not conflict")
	}
}
