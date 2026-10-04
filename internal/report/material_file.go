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

type materialOutputFile interface {
	io.Writer
	Stat() (os.FileInfo, error)
	Sync() error
	Close() error
}

func WriteMaterial(target string, automatic bool, material Material) (string, error) {
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
	temp, err := os.CreateTemp(parent, ".ocr-report-*.tmp")
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
		if err := os.Link(tempPath, candidate); err == nil {
			return candidate, nil
		} else if automatic && errors.Is(err, fs.ErrExist) {
			continue
		} else if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("report file %q already exists", candidate)
		}

		source, err := os.Open(tempPath)
		if err != nil {
			return "", fmt.Errorf("open temporary report file for copy: %w", err)
		}
		created, createdInfo, copyErr := copyMaterialExclusively(source, candidate)
		copyErr = errors.Join(copyErr, source.Close())
		if copyErr == nil {
			return candidate, nil
		}
		if !created && errors.Is(copyErr, fs.ErrExist) {
			if automatic {
				continue
			}
			return "", fmt.Errorf("report file %q already exists", candidate)
		}
		if created {
			cleanupErr := removeIncompleteMaterial(candidate, createdInfo)
			return "", errors.Join(fmt.Errorf("create report file %q exclusively: %w", candidate, copyErr), cleanupErr)
		}
		return "", fmt.Errorf("create report file %q exclusively: %w", candidate, copyErr)
	}
	return "", fmt.Errorf("could not allocate a unique report file near %q", absTarget)
}

// 文件系统不支持硬链接时使用独占创建回退；只在路径仍指向本次创建的文件时清理失败目标。 // allow-non-english: preserve the CI marker for this Chinese ownership constraint
func copyMaterialExclusively(source io.Reader, targetPath string) (bool, os.FileInfo, error) {
	return copyMaterialExclusivelyWith(source, targetPath, func(path string) (materialOutputFile, error) {
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	})
}

func copyMaterialExclusivelyWith(source io.Reader, targetPath string, open func(string) (materialOutputFile, error)) (bool, os.FileInfo, error) {
	target, err := open(targetPath)
	if err != nil {
		return false, nil, err
	}

	createdInfo, operationErr := target.Stat()
	if operationErr != nil {
		operationErr = fmt.Errorf("inspect created report material: %w", operationErr)
	} else if _, err := io.Copy(target, source); err != nil {
		operationErr = fmt.Errorf("copy report material: %w", err)
	} else if err := target.Sync(); err != nil {
		operationErr = fmt.Errorf("flush report material: %w", err)
	}
	if err := target.Close(); err != nil {
		operationErr = errors.Join(operationErr, fmt.Errorf("close report material: %w", err))
	}
	if operationErr != nil {
		if err := removeIncompleteMaterial(targetPath, createdInfo); err != nil {
			operationErr = errors.Join(operationErr, fmt.Errorf("remove incomplete report material: %w", err))
		}
		return true, createdInfo, operationErr
	}
	return true, createdInfo, nil
}

func removeIncompleteMaterial(targetPath string, createdInfo os.FileInfo) error {
	if createdInfo == nil {
		return fmt.Errorf("cannot verify ownership of report file %q", targetPath)
	}
	currentInfo, err := os.Stat(targetPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(createdInfo, currentInfo) {
		return nil
	}
	return os.Remove(targetPath)
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
