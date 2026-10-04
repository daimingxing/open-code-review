// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func WriteHTML(target string, automatic bool, document string, material Material) (string, error) {
	return writeHTMLWith(target, automatic, document, material, func(dir, pattern string) (materialTempFile, error) {
		return os.CreateTemp(dir, pattern)
	}, os.Link)
}

func writeHTMLWith(target string, automatic bool, document string, material Material, createTemp func(string, string) (materialTempFile, error), link func(string, string) error) (string, error) {
	if err := ValidateHTMLDocument(document, material); err != nil {
		return "", fmt.Errorf("validate generated HTML: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve HTML path: %w", err)
	}
	parent := filepath.Dir(absTarget)
	info, err := os.Stat(parent)
	if err != nil {
		return "", fmt.Errorf("HTML parent directory %q: %w", parent, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("HTML parent path %q is not a directory", parent)
	}
	temp, err := createTemp(parent, ".ocr-html-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary HTML file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write([]byte(document)); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("write temporary HTML file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("flush temporary HTML file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close temporary HTML file: %w", err)
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
			return "", fmt.Errorf("HTML file %q already exists", candidate)
		} else {
			return "", fmt.Errorf("publish HTML %q using a same-directory hard link (the filesystem may not support hard links): %w", candidate, err)
		}
	}
	return "", fmt.Errorf("could not allocate a unique HTML file near %q", absTarget)
}
