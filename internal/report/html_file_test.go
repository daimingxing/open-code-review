// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHTMLRefusesOverwriteAndNumbersAutomaticNames(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "report-2026-10-04.html")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteHTML(target, false, validHTMLDocument(validHTMLMaterial()), validHTMLMaterial()); err == nil {
		t.Fatal("WriteHTML should refuse an existing explicit path")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
		t.Fatalf("existing target = %q, %v; want original content", data, err)
	}
	written, err := WriteHTML(target, true, validHTMLDocument(validHTMLMaterial()), validHTMLMaterial())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(written) != "report-2026-10-04(1).html" {
		t.Fatalf("automatic path = %q", written)
	}
}

func TestWriteHTMLHardLinkFailureDoesNotPublish(t *testing.T) {
	target := filepath.Join(t.TempDir(), "report.html")
	document := validHTMLDocument(validHTMLMaterial())
	_, err := writeHTMLWith(target, false, document, validHTMLMaterial(), createOSMaterialTemp, func(string, string) error {
		return errors.New("operation not supported")
	})
	if err == nil || !strings.Contains(err.Error(), "hard link") {
		t.Fatalf("writeHTMLWith error = %v, want hard-link failure", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("unsupported hard link left final file: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unsupported hard link left temporary files: %v", entries)
	}
}

func TestWriteHTMLRejectsInvalidDocumentBeforePublishing(t *testing.T) {
	target := filepath.Join(t.TempDir(), "report.html")
	material := validHTMLMaterial()
	if _, err := WriteHTML(target, false, strings.Replace(validHTMLDocument(material), `risk-critical">1`, `risk-critical">9`, 1), material); err == nil {
		t.Fatal("WriteHTML should reject altered statistics")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("invalid HTML left final file: %v", err)
	}
}
