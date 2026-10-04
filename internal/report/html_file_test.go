// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"errors"
	"fmt"
	"html"
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

func TestWriteHTMLAddsAccessibleFiltersFoldableFindingsAndPrintStyles(t *testing.T) {
	material := validHTMLMaterial()
	target := filepath.Join(t.TempDir(), "report.html")
	if _, err := WriteHTML(target, false, validHTMLModelDocument(material), material); err != nil {
		t.Fatalf("WriteHTML failed: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	document := string(data)
	for _, expected := range []string{
		`<fieldset`, `<legend>审查单元或仓库</legend>`, `name="report-filter-unit"`, // allow-non-english: 规格要求的中文报告标签
		`<legend>严重等级</legend>`, `name="report-filter-severity"`, // allow-non-english: 规格要求的中文报告标签
		`<legend>类别</legend>`, `name="report-filter-category"`, // allow-non-english: 规格要求的中文报告标签
		`<details class="finding-details" open=""><summary>`,
		`@media print`, `:focus-visible`, `data-review-unit-index="0"`,
		`<label`,
	} {
		if !strings.Contains(document, expected) {
			t.Errorf("saved report omitted accessible experience element %q", expected)
		}
	}
	if got := strings.Count(document, `<article `); got != len(material.Findings) {
		t.Fatalf("saved report has %d findings, want %d", got, len(material.Findings))
	}
	if strings.Contains(document, "<script") || strings.Contains(document, "<form") || strings.Contains(document, "https://") {
		t.Fatal("generated controls added active or external content")
	}
	if err := ValidateHTMLDocument(document, material); err != nil {
		t.Fatalf("saved interactive report failed validation: %v", err)
	}
}

func TestWriteMultiHTMLFiltersByReviewUnitAndPreservesFindingOwnership(t *testing.T) {
	first := validHTMLMaterial()
	first.Review.RunID = "experience-unit-one"
	first.Repository.Name = "frontend-repository"
	second := validHTMLMaterial()
	second.Review.RunID = "experience-unit-two"
	second.Repository.Name = "backend-repository"
	input, err := NewMultiReportInput([]Material{first, second})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "multi.html")
	document := multiHTMLWithoutStyles(t, validMultiHTMLDocument(input))
	if _, err := WriteMultiHTML(target, false, document, input); err != nil {
		t.Fatalf("WriteMultiHTML failed: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Count(content, `name="report-filter-unit"`) != len(input.ReviewUnits)+1 {
		t.Fatalf("review-unit filter has the wrong option count: %d", strings.Count(content, `name="report-filter-unit"`))
	}
	for _, expected := range []string{"frontend-repository", "backend-repository", `data-review-unit-index="0"`, `data-review-unit-index="1"`, `@media print`} {
		if !strings.Contains(content, expected) {
			t.Errorf("multi-report omitted filter fact or interaction %q", expected)
		}
	}
	if strings.Count(content, `<article `) != 2*len(first.Findings) {
		t.Fatalf("multi-report finding count changed: %d", strings.Count(content, `<article `))
	}
	if err := ValidateMultiHTMLDocument(content, input); err != nil {
		t.Fatalf("saved multi-report failed validation: %v", err)
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

func TestWriteHTMLAcceptsNonArticleFinding(t *testing.T) {
	material := validHTMLMaterial()
	document := validHTMLModelDocument(material)
	summaryText := html.EscapeString(material.Findings[0].Display.SummaryZH)
	summaryRow := fmt.Sprintf(`<div class="fact-row"><strong class="fact-label">%s</strong><span data-fact="summary_zh">%s</span></div>`, findingHTMLLabels["summary_zh"], summaryText)
	if !strings.Contains(document, summaryRow) {
		t.Fatal("fixture did not contain the finding summary fact row")
	}
	document = strings.Replace(document, summaryRow, `<div>`+summaryText+`</div>`, 1)
	document = strings.Replace(document, `<article data-finding-id=`, `<div data-finding-id=`, 1)
	document = strings.Replace(document, `</article>`, `</div>`, 1)
	target := filepath.Join(t.TempDir(), "report.html")
	if _, err := WriteHTML(target, false, document, material); err != nil {
		t.Fatalf("WriteHTML() rejected a finding on a div: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read published HTML: %v", err)
	}
	if !strings.Contains(string(content), `<div data-finding-id=`) {
		t.Fatal("published HTML lost the non-article finding element")
	}
	if !strings.Contains(string(content), `<summary><div>`+summaryText+`</div></summary>`) {
		t.Fatal("published HTML did not use the visible div as the finding disclosure summary")
	}
	if !strings.Contains(string(content), `data-review-category-index=`) || !strings.Contains(string(content), `[data-finding-id]:not([data-review-category-index=`) {
		t.Fatal("published HTML did not make the finding element available to category filtering")
	}
}

func TestWriteMultiHTMLRefusesOverwriteAndUsesFixedStyles(t *testing.T) {
	first := validHTMLMaterial()
	first.Review.RunID = "first-unit"
	second := validHTMLMaterial()
	second.Review.RunID = "second-unit"
	input, err := NewMultiReportInput([]Material{first, second})
	if err != nil {
		t.Fatal(err)
	}
	document := multiHTMLWithoutStyles(t, validMultiHTMLDocument(input))
	target := filepath.Join(t.TempDir(), "report.html")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteMultiHTML(target, false, document, input); err == nil {
		t.Fatal("WriteMultiHTML should refuse an existing explicit path")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
		t.Fatalf("existing target = %q, %v; want original content", data, err)
	}
	written, err := WriteMultiHTML(target, true, document, input)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(written) != "report(1).html" {
		t.Fatalf("automatic path = %q", written)
	}
	data, err := os.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "<style>") != 1 || !strings.Contains(string(data), reportHTMLStyles) {
		t.Fatal("WriteMultiHTML did not insert exactly one fixed report stylesheet")
	}
}

func TestWriteMultiHTMLRejectsInvalidDocumentWithoutPublishing(t *testing.T) {
	first := validHTMLMaterial()
	first.Review.RunID = "first-unit"
	second := validHTMLMaterial()
	second.Review.RunID = "second-unit"
	input, err := NewMultiReportInput([]Material{first, second})
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Replace(multiHTMLWithoutStyles(t, validMultiHTMLDocument(input)), `risk-critical">1`, `risk-critical">9`, 1)
	target := filepath.Join(t.TempDir(), "report.html")
	if _, err := WriteMultiHTML(target, false, document, input); err == nil {
		t.Fatal("WriteMultiHTML should reject altered statistics")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("invalid multi-unit HTML left final file: %v", err)
	}
}

func TestWriteMultiHTMLHardLinkFailureDoesNotPublish(t *testing.T) {
	first := validHTMLMaterial()
	first.Review.RunID = "first-unit"
	second := validHTMLMaterial()
	second.Review.RunID = "second-unit"
	input, err := NewMultiReportInput([]Material{first, second})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "report.html")
	_, err = writeMultiHTMLWith(target, false, multiHTMLWithoutStyles(t, validMultiHTMLDocument(input)), input, createOSMaterialTemp, func(string, string) error {
		return errors.New("operation not supported")
	})
	if err == nil || !strings.Contains(err.Error(), "hard link") {
		t.Fatalf("writeMultiHTMLWith error = %v, want hard-link failure", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("unsupported hard link left final file: %v", err)
	}
}

func multiHTMLWithoutStyles(t *testing.T, document string) string {
	t.Helper()
	withoutStyles := strings.Replace(document, "<style>"+reportHTMLStyles+"</style>", "", 1)
	if withoutStyles == document {
		t.Fatal("multi-unit HTML fixture is missing the injected stylesheet")
	}
	return withoutStyles
}
