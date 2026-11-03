// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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

type injectedMaterialTempFile struct {
	file    *os.File
	failAt  string
	failure error
}

func (f *injectedMaterialTempFile) Name() string { return f.file.Name() }
func (f *injectedMaterialTempFile) Write(data []byte) (int, error) {
	if f.failAt == "write" {
		return 0, f.failure
	}
	return f.file.Write(data)
}
func (f *injectedMaterialTempFile) Sync() error {
	if f.failAt == "sync" {
		return f.failure
	}
	return f.file.Sync()
}
func (f *injectedMaterialTempFile) Close() error {
	err := f.file.Close()
	if f.failAt == "close" {
		return errors.Join(err, f.failure)
	}
	return err
}

func createOSMaterialTemp(dir, pattern string) (materialTempFile, error) {
	return os.CreateTemp(dir, pattern)
}

func TestWriteMaterialTempFileFailuresDoNotPublish(t *testing.T) {
	for _, phase := range []string{"write", "sync", "close"} {
		t.Run(phase, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "report.json")
			injected := errors.New("injected temporary " + phase + " failure")
			var tempPath string
			createTemp := func(dir, pattern string) (materialTempFile, error) {
				file, err := os.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}
				tempPath = file.Name()
				return &injectedMaterialTempFile{file: file, failAt: phase, failure: injected}, nil
			}

			if _, err := writeMaterialWith(target, false, validMaterial(), createTemp, os.Link); !errors.Is(err, injected) {
				t.Fatalf("%s failure = %v, want injected error", phase, err)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("%s failure published final material: %v", phase, err)
			}
			if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
				t.Fatalf("%s failure left temporary material: %v", phase, err)
			}
		})
	}
}

func TestWriteMaterialLinkFailureDoesNotPublish(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit", true: "automatic"}[automatic], func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "review.report.json")
			if automatic {
				if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			linkCalls := 0
			link := func(_, newPath string) error {
				linkCalls++
				if automatic && newPath == target {
					return fs.ErrExist
				}
				return errors.New("operation not supported")
			}

			_, err := writeMaterialWith(target, automatic, validMaterial(), createOSMaterialTemp, link)
			if err == nil || !strings.Contains(err.Error(), "hard link") {
				t.Fatalf("link failure = %v, want an explicit hard-link publication error", err)
			}
			if automatic {
				if linkCalls != 2 {
					t.Fatalf("automatic publication tried link %d times, want collision then unsupported numbered path", linkCalls)
				}
				data, readErr := os.ReadFile(target)
				if readErr != nil || string(data) != "keep" {
					t.Fatalf("existing automatic target = %q, %v; want original content", data, readErr)
				}
				if _, statErr := os.Stat(numberedMaterialPath(target, 1)); !os.IsNotExist(statErr) {
					t.Fatalf("unsupported automatic publication left numbered material: %v", statErr)
				}
			} else if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Fatalf("unsupported explicit publication left final material: %v", statErr)
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
