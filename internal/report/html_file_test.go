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
	if _, err := WriteHTML(target, false, validHTMLModelDocument(validHTMLMaterial()), validHTMLMaterial()); err == nil {
		t.Fatal("WriteHTML should refuse an existing explicit path")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
		t.Fatalf("existing target = %q, %v; want original content", data, err)
	}
	written, err := WriteHTML(target, true, validHTMLModelDocument(validHTMLMaterial()), validHTMLMaterial())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(written) != "report-2026-10-04(1).html" {
		t.Fatalf("automatic path = %q", written)
	}
	data, err := os.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "<style>") != 1 || !strings.Contains(string(data), reportHTMLStyles) {
		t.Fatal("WriteHTML did not insert exactly one fixed report stylesheet")
	}
}

func TestWriteHTMLHardLinkFailureDoesNotPublish(t *testing.T) {
	target := filepath.Join(t.TempDir(), "report.html")
	document := validHTMLModelDocument(validHTMLMaterial())
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
	if _, err := WriteHTML(target, false, strings.Replace(validHTMLModelDocument(material), `risk-critical">1`, `risk-critical">9`, 1), material); err == nil {
		t.Fatal("WriteHTML should reject altered statistics")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("invalid HTML left final file: %v", err)
	}
}

func TestWriteHTMLRejectsModelStylesAndDoesNotPublish(t *testing.T) {
	material := validHTMLMaterial()
	base := validHTMLModelDocument(material)
	for name, document := range map[string]string{
		"hidden findings": strings.Replace(base, `</head>`, `<style>article{height:0;overflow:hidden}</style></head>`, 1),
		"style attribute": strings.Replace(base, `<main `, `<main style="display:none" `, 1),
	} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "report.html")
			if _, err := WriteHTML(target, false, document, material); err == nil {
				t.Fatal("WriteHTML accepted model-controlled styles")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("rejected model styles left a final file: %v", err)
			}
		})
	}
}

func TestWriteHTMLRejectsDefaultHiddenContainersAndDoesNotPublish(t *testing.T) {
	material := validHTMLMaterial()
	base := validHTMLModelDocument(material)
	firstFinding := findingHTML(material.Findings[0])
	for name, tag := range map[string]string{
		"closed details":   "details",
		"closed dialog":    "dialog",
		"canvas fallback":  "canvas",
		"noscript content": "noscript",
		"noembed content":  "noembed",
		"noframes content": "noframes",
	} {
		t.Run(name, func(t *testing.T) {
			document := strings.Replace(base, firstFinding, "<"+tag+">"+firstFinding+"</"+tag+">", 1)
			target := filepath.Join(t.TempDir(), "report.html")
			if _, err := WriteHTML(target, false, document, material); err == nil {
				t.Fatalf("WriteHTML accepted findings inside closed <%s>", tag)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("rejected closed <%s> left a final file: %v", tag, err)
			}
		})
	}
}

func TestWriteHTMLRejectsNonArticleFindingAndDoesNotPublish(t *testing.T) {
	material := validHTMLMaterial()
	document := validHTMLModelDocument(material)
	document = strings.Replace(document, `<article data-finding-id=`, `<div data-finding-id=`, 1)
	document = strings.Replace(document, `</article>`, `</div>`, 1)
	target := filepath.Join(t.TempDir(), "report.html")
	if _, err := WriteHTML(target, false, document, material); err == nil {
		t.Fatal("WriteHTML accepted a finding without an article element")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("rejected non-article finding left a final file: %v", err)
	}
}
