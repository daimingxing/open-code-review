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

// PreparedHTML contains a fully validated, safe-to-publish report document.
type PreparedHTML struct {
	document string
}

func WriteHTML(target string, automatic bool, document string, material Material) (string, error) {
	prepared, err := PrepareHTML(document, material)
	if err != nil {
		return "", err
	}
	return WritePreparedHTML(target, automatic, prepared)
}

func PrepareHTML(document string, material Material) (PreparedHTML, error) {
	prepared, err := addReportHTMLStyles(document)
	if err != nil {
		return PreparedHTML{}, fmt.Errorf("prepare generated HTML: %w", err)
	}
	if err := ValidateHTMLDocument(prepared, material); err != nil {
		return PreparedHTML{}, fmt.Errorf("validate generated HTML: %w", err)
	}
	prepared, err = addReportExperience(prepared, []Material{material}, nil)
	if err != nil {
		return PreparedHTML{}, fmt.Errorf("add report controls: %w", err)
	}
	if err := ValidateHTMLDocument(prepared, material); err != nil {
		return PreparedHTML{}, fmt.Errorf("validate interactive HTML: %w", err)
	}
	return PreparedHTML{document: prepared}, nil
}

func WritePreparedHTML(target string, automatic bool, prepared PreparedHTML) (string, error) {
	if prepared.document == "" || len(prepared.document) > MaxHTMLDocumentBytes {
		return "", fmt.Errorf("prepared HTML document size must be between 1 byte and %d bytes", MaxHTMLDocumentBytes)
	}
	return publishHTML(target, automatic, prepared.document, func(dir, pattern string) (materialTempFile, error) {
		return os.CreateTemp(dir, pattern)
	}, os.Link)
}

func WriteMultiHTML(target string, automatic bool, document string, input MultiReportInput) (string, error) {
	prepared, err := PrepareMultiHTML(document, input)
	if err != nil {
		return "", err
	}
	return WritePreparedMultiHTML(target, automatic, prepared)
}

func PrepareMultiHTML(document string, input MultiReportInput) (PreparedHTML, error) {
	prepared, err := addReportHTMLStyles(document)
	if err != nil {
		return PreparedHTML{}, fmt.Errorf("prepare generated HTML: %w", err)
	}
	if err := ValidateMultiHTMLDocument(prepared, input); err != nil {
		return PreparedHTML{}, fmt.Errorf("validate generated HTML: %w", err)
	}
	materials := make([]Material, len(input.ReviewUnits))
	unitIDs := make([]string, len(input.ReviewUnits))
	for index, unit := range input.ReviewUnits {
		materials[index] = unit.Material
		unitIDs[index] = unit.ID
	}
	prepared, err = addReportExperience(prepared, materials, unitIDs)
	if err != nil {
		return PreparedHTML{}, fmt.Errorf("add report controls: %w", err)
	}
	if err := ValidateMultiHTMLDocument(prepared, input); err != nil {
		return PreparedHTML{}, fmt.Errorf("validate interactive HTML: %w", err)
	}
	return PreparedHTML{document: prepared}, nil
}

func WritePreparedMultiHTML(target string, automatic bool, prepared PreparedHTML) (string, error) {
	if prepared.document == "" || len(prepared.document) > MaxHTMLDocumentBytes {
		return "", fmt.Errorf("prepared HTML document size must be between 1 byte and %d bytes", MaxHTMLDocumentBytes)
	}
	return publishHTML(target, automatic, prepared.document, func(dir, pattern string) (materialTempFile, error) {
		return os.CreateTemp(dir, pattern)
	}, os.Link)
}

func writeHTMLWith(target string, automatic bool, document string, material Material, createTemp func(string, string) (materialTempFile, error), link func(string, string) error) (string, error) {
	return writeHTMLWithValidation(target, automatic, document, func(prepared string) error {
		return ValidateHTMLDocument(prepared, material)
	}, createTemp, link)
}

func writeMultiHTMLWith(target string, automatic bool, document string, input MultiReportInput, createTemp func(string, string) (materialTempFile, error), link func(string, string) error) (string, error) {
	return writeHTMLWithValidation(target, automatic, document, func(prepared string) error {
		return ValidateMultiHTMLDocument(prepared, input)
	}, createTemp, link)
}

func writeHTMLWithValidation(target string, automatic bool, document string, validate func(string) error, createTemp func(string, string) (materialTempFile, error), link func(string, string) error) (string, error) {
	document, err := addReportHTMLStyles(document)
	if err != nil {
		return "", fmt.Errorf("prepare generated HTML: %w", err)
	}
	if err := validate(document); err != nil {
		return "", fmt.Errorf("validate generated HTML: %w", err)
	}
	return publishHTML(target, automatic, document, createTemp, link)
}

func publishHTML(target string, automatic bool, document string, createTemp func(string, string) (materialTempFile, error), link func(string, string) error) (string, error) {
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
