// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type materialTempFile interface {
	io.Writer
	Name() string
	Sync() error
	Close() error
}

func WriteMaterial(target string, automatic bool, material Material) (string, error) {
	return writeMaterialWith(target, automatic, material, func(dir, pattern string) (materialTempFile, error) {
		return os.CreateTemp(dir, pattern)
	}, os.Link)
}

func writeMaterialWith(target string, automatic bool, material Material, createTemp func(string, string) (materialTempFile, error), link func(string, string) error) (string, error) {
	if err := ValidateMaterial(material); err != nil {
		return "", fmt.Errorf("validate report material: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve report path: %w", err)
	}
	parent := filepath.Dir(absTarget)
	info, err := os.Stat(parent)
	if err != nil {
		return "", fmt.Errorf("report parent directory %q: %w", parent, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("report parent path %q is not a directory", parent)
	}
	temp, err := createTemp(parent, ".ocr-report-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary report file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(material); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("encode report material: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("flush report material: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close temporary report file: %w", err)
	}

	for number := 0; number < 10000; number++ {
		candidate := absTarget
		if number > 0 {
			candidate = numberedMaterialPath(absTarget, number)
		}
		if err := link(tempPath, candidate); err == nil {
			return candidate, nil
		} else if automatic && errors.Is(err, fs.ErrExist) {
			continue
		} else if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("report file %q already exists", candidate)
		}
		return "", fmt.Errorf("publish report material %q using a same-directory hard link (the filesystem may not support hard links): %w", candidate, err)
	}
	return "", fmt.Errorf("could not allocate a unique report file near %q", absTarget)
}

func PathsConflict(nativePath, materialPath string) (bool, error) {
	if nativePath == "" || nativePath == "-" || materialPath == "" {
		return false, nil
	}
	nativeAbs, err := filepath.Abs(nativePath)
	if err != nil {
		return false, fmt.Errorf("resolve native output path: %w", err)
	}
	materialAbs, err := filepath.Abs(materialPath)
	if err != nil {
		return false, fmt.Errorf("resolve report path: %w", err)
	}
	nativeInfo, nativeErr := os.Stat(nativeAbs)
	materialInfo, materialErr := os.Stat(materialAbs)
	if nativeErr == nil && materialErr == nil && os.SameFile(nativeInfo, materialInfo) {
		return true, nil
	}
	return samePath(resolveExistingParent(nativeAbs), resolveExistingParent(materialAbs)), nil
}

func resolveExistingParent(filePath string) string {
	parent, err := filepath.EvalSymlinks(filepath.Dir(filePath))
	if err != nil {
		return filepath.Clean(filePath)
	}
	return filepath.Clean(filepath.Join(parent, filepath.Base(filePath)))
}

func samePath(first, second string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(first, second)
	}
	return first == second
}

func numberedMaterialPath(target string, number int) string {
	const suffix = ".report.json"
	if strings.HasSuffix(target, suffix) {
		return strings.TrimSuffix(target, suffix) + fmt.Sprintf("(%d)%s", number, suffix)
	}
	ext := filepath.Ext(target)
	stem := strings.TrimSuffix(target, ext)
	return fmt.Sprintf("%s(%d)%s", stem, number, ext)
}
