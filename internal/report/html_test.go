// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"testing"
)

func TestValidateHTMLDocumentChecksFindingsRiskCountsAndSections(t *testing.T) {
	material := validHTMLMaterial()
	content := validHTMLDocument(material)
	if err := ValidateHTMLDocument(content, material); err != nil {
		t.Fatalf("valid report HTML rejected: %v", err)
	}

	first := findingHTML(material.Findings[0])
	cases := []struct {
		name string
		edit func(string) string
	}{
		{"missing finding", func(value string) string { return strings.Replace(value, first, "", 1) }},
		{"changed severity", func(value string) string {
			return strings.Replace(value, `data-severity="critical"`, `data-severity="high"`, 1)
		}},
		{"changed risk count", func(value string) string {
			return strings.Replace(value, `data-stat="risk-critical">1`, `data-stat="risk-critical">0`, 1)
		}},
		{"missing section", func(value string) string {
			return strings.Replace(value, `data-section="governance"`, `data-section="unknown"`, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateHTMLDocument(test.edit(content), material); err == nil {
				t.Fatal("ValidateHTMLDocument accepted an incomplete or altered report")
			}
		})
	}
}

func TestValidateHTMLDocumentRejectsActiveContentAndLocalPaths(t *testing.T) {
	material := validHTMLMaterial()
	content := validHTMLDocument(material)
	unsafe := []string{
		`<script>alert(1)</script>`,
		`<div onclick="alert(1)">x</div>`,
		`<img src="https://example.test/a.png">`,
		`<link rel="stylesheet" href="https://example.test/a.css">`,
		`<a href="javascript:alert(1)">x</a>`,
		`<a href="#overview" ping="//example.test/ping">x</a>`,
		`<meta http-equiv="refresh" content="0;url=https://example.test">`,
		`<style>article{height:0;overflow:hidden}</style>`,
		`<style>article{color:transparent}</style>`,
		`<div style="height:0;overflow:hidden">hidden</div>`,
		`C:\Users\alice\private\repo`,
		`C&#58;&#92;Users&#92;alice&#92;private&#92;repo`,
		`/var/lib/open-code-review/config.yml`,
		`/opt/service/private/token.json`,
		`/srv/app/config.yml`,
		`/root/.config/open-code-review`,
		`/usr/local/share/data.json`,
		`&#47;var&#47;lib&#47;open-code-review`,
	}
	for _, value := range unsafe {
		t.Run(value, func(t *testing.T) {
			if err := ValidateHTMLDocument(strings.Replace(content, `</body>`, value+`</body>`, 1), material); err == nil {
				t.Fatalf("ValidateHTMLDocument accepted unsafe content %q", value)
			}
		})
	}
}

func TestValidateHTMLDocumentRejectsEscapedCSSExternalResources(t *testing.T) {
	material := validHTMLMaterial()
	content := strings.Replace(validHTMLDocument(material), `</head>`, `<style>@\69mport "\68ttps://example.test/remote.css";</style></head>`, 1)
	if err := ValidateHTMLDocument(content, material); err == nil {
		t.Fatal("ValidateHTMLDocument accepted CSS-escaped remote import")
	}
}

func TestValidateHTMLDocumentKeepsEvidenceAsEscapedText(t *testing.T) {
	material := validHTMLMaterial()
	content := validHTMLDocument(material)
	if !strings.Contains(content, `return secret &lt;&amp; token`) {
		t.Fatal("fixture did not HTML-escape evidence text")
	}
	if err := ValidateHTMLDocument(content, material); err != nil {
		t.Fatalf("escaped evidence was rejected: %v", err)
	}
	unsafe := strings.Replace(content, `return secret &lt;&amp; token`, `<img src=x onerror=alert(1)>`, 1)
	if err := ValidateHTMLDocument(unsafe, material); err == nil {
		t.Fatal("ValidateHTMLDocument accepted evidence parsed as markup")
	}
}

func TestValidateHTMLDocumentRejectsInventedNonFindingFacts(t *testing.T) {
	material := validHTMLMaterial()
	material.Sections.People = Section{Status: StatusProvided, Data: json.RawMessage(`{"contributors":[{"name":"Alice","commit_count":3}]}`)}
	material.Sections.Achievements = Section{Status: StatusProvided, Data: json.RawMessage(`{"items":[{"title":"Reduced latency","commit":"abc123"}]}`)}
	material.Sections.KnowledgeSources = Section{Status: StatusProvided, Data: json.RawMessage(`{"documents":[{"id":"framework-index","title":"Framework API"}]}`)}
	content := validHTMLDocument(material)
	if err := ValidateHTMLDocument(content, material); err != nil {
		t.Fatalf("report containing material-backed facts was rejected: %v", err)
	}
	for _, test := range []struct {
		name string
		from string
		to   string
	}{
		{"invented participant", `data-fact="sections.people.data.contributors[0].name">Alice`, `data-fact="sections.people.data.contributors[0].name">Mallory`},
		{"invented commit count", `data-fact="sections.people.data.contributors[0].commit_count">3`, `data-fact="sections.people.data.contributors[0].commit_count">42`},
		{"invented achievement", `data-fact="sections.achievements.data.items[0].title">Reduced latency`, `data-fact="sections.achievements.data.items[0].title">Improved security`},
		{"changed section status", `data-fact="sections.people.status">` + "\u5df2\u63d0\u4f9b", `data-fact="sections.people.status">` + "\u672a\u63d0\u4f9b"},
		{"invented knowledge source", `data-fact="sections.knowledge_sources.data.documents[0].id">framework-index`, `data-fact="sections.knowledge_sources.data.documents[0].id">unknown-index`},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := strings.Replace(content, test.from, test.to, 1)
			if mutated == content {
				t.Fatalf("fixture did not contain %q", test.from)
			}
			if err := ValidateHTMLDocument(mutated, material); err == nil {
				t.Fatal("ValidateHTMLDocument accepted an unsupported material claim")
			}
		})
	}

	duplicateFact := `<span data-fact="repository.name">` + html.EscapeString(material.Repository.Name) + `</span>`
	duplicated := strings.Replace(content, duplicateFact, duplicateFact+duplicateFact, 1)
	if duplicated == content {
		t.Fatal("fixture did not contain repository.name fact")
	}
	if err := ValidateHTMLDocument(duplicated, material); err == nil {
		t.Fatal("ValidateHTMLDocument accepted a duplicated material fact")
	}

	inventedProse := strings.Replace(content, `</section><section data-section="governance"`, `<p>Avery made 42 commits.</p></section><section data-section="governance"`, 1)
	if err := ValidateHTMLDocument(inventedProse, material); err == nil {
		t.Fatal("ValidateHTMLDocument accepted free-form unverified report prose")
	}
	footerProse := strings.Replace(content, `</body>`, `<footer>Avery made 42 commits.</footer></body>`, 1)
	if err := ValidateHTMLDocument(footerProse, material); err == nil {
		t.Fatal("ValidateHTMLDocument accepted unverified body content outside main")
	}
}

func TestValidateHTMLDocumentRejectsUnverifiedBodyContentOutsideMain(t *testing.T) {
	material := validHTMLMaterial()
	content := strings.Replace(validHTMLDocument(material), `</body>`, `<footer>Avery made 42 commits.</footer></body>`, 1)
	if err := ValidateHTMLDocument(content, material); err == nil {
		t.Fatal("ValidateHTMLDocument accepted unverified body content outside main")
	}
}

func validHTMLMaterial() Material {
	material := validMaterial()
	material.Findings = []Finding{
		{ID: "sha256:" + strings.Repeat("1", 64), Path: "src/a.go", StartLine: 1, EndLine: 1, Severity: "critical", SeverityStatus: StatusProvided, Category: "security", CategoryStatus: StatusProvided, SourceContent: "return secret <& token", Display: FindingDisplay{SummaryZH: "\u786c\u7f16\u7801\u51ed\u636e", SeverityZH: "\u4e25\u91cd", CategoryZH: "\u5b89\u5168"}, Evidence: FindingEvidence{Status: StatusProvided, Code: "if token != \"\" {"}, Recommendation: FindingRecommendation{Status: StatusProvided, Code: "read the token from a secure store"}},
		{ID: "sha256:" + strings.Repeat("2", 64), Path: "src/b.go", StartLine: 2, EndLine: 3, Severity: "high", SeverityStatus: StatusProvided, Category: "bug", CategoryStatus: StatusProvided, SourceContent: "unchecked result", Display: FindingDisplay{SummaryZH: "\u8fd4\u56de\u7ed3\u679c\u672a\u68c0\u67e5", SeverityZH: "\u9ad8", CategoryZH: "\u7f3a\u9677"}, Evidence: FindingEvidence{Status: StatusNotCollected, Reason: "\u672a\u6536\u96c6\u72ec\u7acb\u8bc1\u636e"}, Recommendation: FindingRecommendation{Status: StatusNotCollected, Reason: "\u672a\u63d0\u4f9b\u4fee\u590d\u5efa\u8bae"}},
		{ID: "sha256:" + strings.Repeat("3", 64), Path: "src/c.go", StartLine: 4, EndLine: 4, Severity: "medium", SeverityStatus: StatusProvided, Category: "performance", CategoryStatus: StatusProvided, SourceContent: "repeated allocation", Display: FindingDisplay{SummaryZH: "\u5faa\u73af\u4e2d\u91cd\u590d\u5206\u914d", SeverityZH: "\u4e2d", CategoryZH: "\u6027\u80fd"}, Evidence: FindingEvidence{Status: StatusNotCollected, Reason: "\u672a\u6536\u96c6\u72ec\u7acb\u8bc1\u636e"}, Recommendation: FindingRecommendation{Status: StatusNotCollected, Reason: "\u672a\u63d0\u4f9b\u4fee\u590d\u5efa\u8bae"}},
		{ID: "sha256:" + strings.Repeat("4", 64), Path: "src/d.go", StartLine: 5, EndLine: 5, Severity: "low", SeverityStatus: StatusProvided, Category: "style", CategoryStatus: StatusProvided, SourceContent: "unused value", Display: FindingDisplay{SummaryZH: "\u672a\u4f7f\u7528\u7684\u503c", SeverityZH: "\u4f4e", CategoryZH: "\u98ce\u683c"}, Evidence: FindingEvidence{Status: StatusNotCollected, Reason: "\u672a\u6536\u96c6\u72ec\u7acb\u8bc1\u636e"}, Recommendation: FindingRecommendation{Status: StatusNotCollected, Reason: "\u672a\u63d0\u4f9b\u4fee\u590d\u5efa\u8bae"}},
	}
	return material
}

func validHTMLDocument(material Material) string {
	document, err := addReportHTMLStyles(validHTMLModelDocument(material))
	if err != nil {
		panic(err)
	}
	return document
}

func validHTMLModelDocument(material Material) string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>`)
	builder.WriteString("\u5ba1\u67e5\u62a5\u544a")
	builder.WriteString(`</title></head><body>`)
	fmt.Fprintf(&builder, `<main data-review-status="%s" data-run-id="%s">`, html.EscapeString(string(material.Review.Status)), html.EscapeString(material.Review.RunID))
	facts, err := materialHTMLFacts(material)
	if err != nil {
		panic(err)
	}
	for _, section := range requiredHTMLSections {
		fmt.Fprintf(&builder, `<section data-section="%s"><h2>%s</h2>`, section, requiredHTMLHeadings[section])
		appendMaterialFacts(&builder, facts, section)
		switch section {
		case "quality-coverage":
			appendStatistics(&builder, material)
		case "finding-details":
			for _, finding := range material.Findings {
				builder.WriteString(findingHTML(finding))
			}
		}
		builder.WriteString(`</section>`)
	}
	builder.WriteString(`</main></body></html>`)
	return builder.String()
}

func appendMaterialFacts(builder *strings.Builder, facts map[string]string, section string) {
	keys := make([]string, 0, len(facts))
	for key := range facts {
		if htmlFactAllowedInSection(section, key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := displayHTMLFact(key, facts[key])
		fmt.Fprintf(builder, `<span data-fact="%s">%s</span>`, html.EscapeString(key), html.EscapeString(value))
	}
}

func appendStatistics(builder *strings.Builder, material Material) {
	stats := []struct {
		name  string
		count int
	}{
		{"finding-count", len(material.Findings)},
		{"risk-critical", 0}, {"risk-high", 0}, {"risk-medium", 0}, {"risk-low", 0},
		{"coverage-selected", len(material.Coverage.Selected)}, {"coverage-completed", len(material.Coverage.Completed)},
		{"coverage-failed", len(material.Coverage.Failed)}, {"coverage-skipped", len(material.Coverage.Waived)},
		{"coverage-reused", len(material.Coverage.Reused)},
	}
	for _, finding := range material.Findings {
		for index, severity := range []string{"critical", "high", "medium", "low"} {
			if finding.Severity == severity {
				stats[index+1].count++
			}
		}
	}
	for _, stat := range stats {
		fmt.Fprintf(builder, `<output data-stat="%s">%d</output>`, stat.name, stat.count)
	}
}

func findingHTML(finding Finding) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, `<article data-finding-id="%s" data-severity="%s" data-category="%s" data-path="%s" data-start-line="%d" data-end-line="%d">`, html.EscapeString(finding.ID), html.EscapeString(finding.Severity), html.EscapeString(finding.Category), html.EscapeString(finding.Path), finding.StartLine, finding.EndLine)
	facts := []struct{ name, value string }{
		{"summary_zh", finding.Display.SummaryZH},
		{"severity_zh", finding.Display.SeverityZH},
		{"category_zh", finding.Display.CategoryZH},
		{"source_content", finding.SourceContent},
	}
	for _, fact := range facts {
		fmt.Fprintf(&builder, `<span data-fact="%s">%s</span>`, fact.name, html.EscapeString(fact.value))
	}
	fmt.Fprintf(&builder, `<span data-fact="evidence_status">%s</span>`, finding.Evidence.Status)
	if finding.Evidence.Status == StatusProvided {
		fmt.Fprintf(&builder, `<pre data-fact="evidence_code">%s</pre>`, html.EscapeString(finding.Evidence.Code))
	} else {
		fmt.Fprintf(&builder, `<span data-fact="evidence_reason">%s</span>`, html.EscapeString(finding.Evidence.Reason))
	}
	fmt.Fprintf(&builder, `<span data-fact="recommendation_status">%s</span>`, finding.Recommendation.Status)
	if finding.Recommendation.Status == StatusProvided {
		fmt.Fprintf(&builder, `<pre data-fact="recommendation_code">%s</pre>`, html.EscapeString(finding.Recommendation.Code))
	} else {
		fmt.Fprintf(&builder, `<span data-fact="recommendation_reason">%s</span>`, html.EscapeString(finding.Recommendation.Reason))
	}
	builder.WriteString(`</article>`)
	return builder.String()
}
