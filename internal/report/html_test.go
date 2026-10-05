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

	flexible := content
	for _, section := range requiredHTMLSections {
		fixed := "<section data-section=\"" + section + "\"><h2>\u7ae0\u8282\u6807\u9898</h2>"
		flexible = strings.Replace(flexible, fixed, "<section data-section=\""+section+"\"><h2>\u672c\u8282\u6458\u8981</h2>", 1)
	}
	flexible = strings.Replace(flexible, "<strong class=\"fact-label\">\u4e25\u91cd\u7b49\u7ea7</strong>", "<strong class=\"fact-label\">\u98ce\u9669\u5c42\u7ea7</strong>", 1)
	if err := ValidateHTMLDocument(flexible, material); err != nil {
		t.Fatalf("HTML with alternate Chinese headings and labels was rejected: %v", err)
	}

	withoutStatMarkers := markerlessHTMLStatistics(content, material, map[string]string{
		"finding-count":      "Total findings",
		"risk-critical":      "Critical findings",
		"risk-high":          "High risk",
		"risk-medium":        "Medium risk",
		"risk-low":           "Low risk",
		"coverage-selected":  "Selected checks",
		"coverage-completed": "Completed checks",
		"coverage-failed":    "Failed checks",
		"coverage-skipped":   "Skipped checks",
		"coverage-reused":    "Reused checks",
	})
	if err := ValidateHTMLDocument(withoutStatMarkers, material); err != nil {
		t.Fatalf("visible statistics without optional data-stat markers were rejected: %v", err)
	}
	withoutVisibleStat := strings.Replace(content, `<output data-stat="risk-critical">1</output>`, "", 1)
	if withoutVisibleStat == content {
		t.Fatal("fixture did not contain the critical-risk statistic")
	}
	if err := ValidateHTMLDocument(withoutVisibleStat, material); err == nil {
		t.Fatal("ValidateHTMLDocument accepted a missing visible risk statistic")
	}

	for _, summary := range []struct {
		tag  string
		body string
	}{
		{tag: "p", body: "class=\"finding-summary\""},
		{tag: "div"},
		{tag: "li"},
		{tag: "span"},
	} {
		t.Run(summary.tag, func(t *testing.T) {
			freeSummary := strings.Replace(content, `<span data-fact="summary_zh">`, "<"+summary.tag+" "+summary.body+">", 1)
			freeSummary = strings.Replace(freeSummary, "</span></div><div class=\"fact-row\"><strong class=\"fact-label\">\u4e25\u91cd\u7b49\u7ea7", "</"+summary.tag+"></div><div class=\"fact-row\"><strong class=\"fact-label\">\u4e25\u91cd\u7b49\u7ea7", 1)
			if err := ValidateHTMLDocument(freeSummary, material); err != nil {
				t.Fatalf("visible Chinese finding summary without a data-fact marker was rejected: %v", err)
			}
		})
	}
}

func TestValidateHTMLDocumentRequiresSemanticLabelsForMarkerlessStatistics(t *testing.T) {
	material := validHTMLMaterial()
	material.Findings = material.Findings[:3]
	content := markerlessHTMLStatistics(validHTMLDocument(material), material, map[string]string{
		"finding-count":      "Total findings",
		"risk-critical":      "Critical findings",
		"risk-high":          "High risk",
		"risk-medium":        "\u4e2d\u98ce\u9669",
		"risk-low":           "Low",
		"coverage-selected":  "Selected checks",
		"coverage-completed": "\u5df2\u5b8c\u6210",
		"coverage-failed":    "Failed checks",
		"coverage-skipped":   "\u8df3\u8fc7",
		"coverage-reused":    "Reused checks",
	})

	t.Run("correct labels", func(t *testing.T) {
		if err := ValidateHTMLDocument(content, material); err != nil {
			t.Fatalf("ValidateHTMLDocument rejected markerless statistics with semantic labels: %v", err)
		}
	})
	t.Run("label and value share a text node", func(t *testing.T) {
		singleTextNode := strings.Replace(content, `<div><span>Failed checks</span><strong>0</strong></div>`, `<p>Failed checks: 0</p>`, 1)
		if singleTextNode == content {
			t.Fatal("fixture did not contain the failed-coverage statistic")
		}
		if err := ValidateHTMLDocument(singleTextNode, material); err != nil {
			t.Fatalf("ValidateHTMLDocument rejected a statistic label and value in one text node: %v", err)
		}
	})

	t.Run("swapped severity values", func(t *testing.T) {
		swapped := strings.Replace(content, "<span>\u4e2d\u98ce\u9669</span><strong>1</strong>", "<span>\u4e2d\u98ce\u9669</span><strong>0</strong>", 1)
		swapped = strings.Replace(swapped, `<span>Low</span><strong>0</strong>`, `<span>Low</span><strong>1</strong>`, 1)
		if swapped == content {
			t.Fatal("fixture did not contain the expected medium and low statistics")
		}
		if err := ValidateHTMLDocument(swapped, material); err == nil {
			t.Fatal("ValidateHTMLDocument accepted severity counts associated with the wrong labels")
		}
	})

	t.Run("unlabelled value", func(t *testing.T) {
		unlabelled := strings.Replace(content, `<span>Critical findings</span><strong>1</strong>`, `<span>Unlabelled</span><strong>1</strong>`, 1)
		if unlabelled == content {
			t.Fatal("fixture did not contain the critical-risk statistic")
		}
		if err := ValidateHTMLDocument(unlabelled, material); err == nil {
			t.Fatal("ValidateHTMLDocument accepted a markerless statistic without a semantic label")
		}
	})
}

func TestValidateHTMLDocumentDoesNotExemptStatisticCountsFromUnrelatedClaims(t *testing.T) {
	material := validHTMLMaterial()
	for index := range material.Findings {
		material.Findings[index].StartLine += 100
		material.Findings[index].EndLine += 100
	}
	content := validHTMLDocument(material)
	content = strings.Replace(content, `</section><section data-section="governance"`, `<p>Avery made 4 commits.</p></section><section data-section="governance"`, 1)
	if err := ValidateHTMLDocument(content, material); err == nil || !strings.Contains(err.Error(), "unsupported numeric claim") {
		t.Fatalf("ValidateHTMLDocument() = %v, want unsupported numeric claim rejection", err)
	}
}

func TestValidateHTMLDocumentRequiresVisibleRecommendations(t *testing.T) {
	material := validHTMLMaterial()
	content := validHTMLDocument(material)
	providedCode := `<pre data-fact="recommendation_code">` + html.EscapeString(material.Findings[0].Recommendation.Code) + `</pre>`
	withoutCode := strings.Replace(content, providedCode, "", 1)
	if withoutCode == content {
		t.Fatal("fixture did not contain the provided recommendation")
	}
	if err := ValidateHTMLDocument(withoutCode, material); err == nil || !strings.Contains(err.Error(), "omits its recommendation") {
		t.Fatalf("ValidateHTMLDocument() = %v, want missing recommendation diagnostic", err)
	}

	unavailableReason := `<span data-fact="recommendation_reason">` + html.EscapeString(material.Findings[1].Recommendation.Reason) + `</span>`
	withoutReason := strings.Replace(content, unavailableReason, "", 1)
	if withoutReason == content {
		t.Fatal("fixture did not contain the unavailable recommendation reason")
	}
	if err := ValidateHTMLDocument(withoutReason, material); err == nil || !strings.Contains(err.Error(), "omits its recommendation") {
		t.Fatalf("ValidateHTMLDocument() = %v, want missing recommendation reason diagnostic", err)
	}
}

func TestValidateHTMLDocumentAcceptsFindingOnOtherAllowedElement(t *testing.T) {
	for _, wrapper := range []struct {
		name   string
		before string
		tag    string
		after  string
	}{
		{name: "div", tag: "div"},
		{name: "section", tag: "section"},
		{name: "blockquote", tag: "blockquote"},
		{name: "li", before: "<ul>", tag: "li", after: "</ul>"},
		{name: "dd", before: "<dl><dt>\u95ee\u9898</dt>", tag: "dd", after: "</dl>"},
		{name: "fieldset", tag: "fieldset"},
		{name: "td", before: "<table><tbody><tr>", tag: "td", after: "</tr></tbody></table>"},
		{name: "th", before: "<table><tbody><tr>", tag: "th", after: "</tr></tbody></table>"},
	} {
		t.Run(wrapper.name, func(t *testing.T) {
			material := validHTMLMaterial()
			document := validHTMLDocument(material)
			document = strings.Replace(document, `<article data-finding-id=`, wrapper.before+"<"+wrapper.tag+` data-finding-id=`, 1)
			document = strings.Replace(document, `</article>`, "</"+wrapper.tag+">"+wrapper.after, 1)
			if err := ValidateHTMLDocument(document, material); err != nil {
				t.Fatalf("ValidateHTMLDocument() rejected a finding on %s: %v", wrapper.tag, err)
			}
		})
	}
}

func TestValidateHTMLDocumentRequiresFlowContentFindingContainer(t *testing.T) {
	material := validHTMLMaterial()
	document := validHTMLDocument(material)
	document = strings.Replace(document, `<article data-finding-id=`, `<p data-finding-id=`, 1)
	document = strings.Replace(document, `</article>`, `</p>`, 1)
	if err := ValidateHTMLDocument(document, material); err == nil || !strings.Contains(err.Error(), "flow-content container") {
		t.Fatalf("ValidateHTMLDocument() = %v, want a finding container diagnostic", err)
	}
}

func TestValidateHTMLDocumentRequiresValidFindingContainerParent(t *testing.T) {
	material := validHTMLMaterial()
	document := validHTMLDocument(material)
	document = strings.Replace(document, `<article data-finding-id=`, `<li data-finding-id=`, 1)
	document = strings.Replace(document, `</article>`, `</li>`, 1)
	if err := ValidateHTMLDocument(document, material); err == nil || !strings.Contains(err.Error(), "valid parent") {
		t.Fatalf("ValidateHTMLDocument() = %v, want a finding parent diagnostic", err)
	}
}

func TestValidateHTMLDocumentRequiresTermForDescriptionFinding(t *testing.T) {
	material := validHTMLMaterial()
	document := validHTMLDocument(material)
	document = strings.Replace(document, `<article data-finding-id=`, `<dl><dd data-finding-id=`, 1)
	document = strings.Replace(document, `</article>`, `</dd></dl>`, 1)
	if err := ValidateHTMLDocument(document, material); err == nil || !strings.Contains(err.Error(), "valid parent") {
		t.Fatalf("ValidateHTMLDocument() = %v, want a finding parent diagnostic", err)
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

func TestValidateHTMLDocumentRequiresVisibleEvidenceAndSectionContent(t *testing.T) {
	material := validHTMLMaterial()
	content := validHTMLDocument(material)
	evidence := html.EscapeString(material.Findings[0].Evidence.Code)
	missingEvidence := strings.Replace(content, `data-fact="evidence_code">`+evidence, `data-fact="evidence_code"></span><span>`, 1)
	if missingEvidence == content {
		t.Fatal("fixture did not contain the finding evidence")
	}
	if err := ValidateHTMLDocument(missingEvidence, material); err == nil || !strings.Contains(err.Error(), "omits its evidence") {
		t.Fatalf("ValidateHTMLDocument() = %v, want missing evidence rejection", err)
	}

	sectionStart := strings.Index(content, `<section data-section="sources">`)
	if sectionStart < 0 {
		t.Fatal("fixture is missing the sources section")
	}
	headingEnd := strings.Index(content[sectionStart:], `</h2>`)
	if headingEnd < 0 {
		t.Fatal("fixture is missing the sources section heading")
	}
	contentStart := sectionStart + headingEnd + len(`</h2>`)
	sectionEnd := strings.Index(content[contentStart:], `</section>`)
	if sectionEnd < 0 {
		t.Fatal("fixture is missing the sources section end")
	}
	emptySection := content[:contentStart] + `</section>` + content[contentStart+sectionEnd+len(`</section>`):]
	if err := ValidateHTMLDocument(emptySection, material); err == nil || !strings.Contains(err.Error(), `missing required section "sources"`) {
		t.Fatalf("ValidateHTMLDocument() = %v, want empty required section rejection", err)
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
	if err := ValidateHTMLDocument(duplicated, material); err != nil {
		t.Fatalf("duplicate presentation of an unchanged fact was rejected: %v", err)
	}

	neutralProse := strings.Replace(content, `</section><section data-section="governance"`, "<p>\u672c\u6b21\u5ba1\u67e5\u60c5\u51b5\u5982\u4e0b</p></section><section data-section=\"governance\"", 1)
	if err := ValidateHTMLDocument(neutralProse, material); err != nil {
		t.Fatalf("alternate narrative was rejected despite unchanged structured findings and statistics: %v", err)
	}
	inventedProse := strings.Replace(content, `</section><section data-section="governance"`, `<p>Avery made 42 commits.</p></section><section data-section="governance"`, 1)
	if err := ValidateHTMLDocument(inventedProse, material); err == nil || !strings.Contains(err.Error(), "unsupported numeric claim") {
		t.Fatalf("ValidateHTMLDocument() = %v, want ungrounded numeric claim rejection", err)
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

func TestValidateHTMLDocumentRejectsDefaultHiddenContainers(t *testing.T) {
	material := validHTMLMaterial()
	content := validHTMLDocument(material)
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
			hidden := strings.Replace(content, firstFinding, "<"+tag+">"+firstFinding+"</"+tag+">", 1)
			if hidden == content {
				t.Fatal("fixture did not contain first finding")
			}
			if err := ValidateHTMLDocument(hidden, material); err == nil {
				t.Fatalf("ValidateHTMLDocument accepted findings inside closed <%s>", tag)
			}
		})
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
		fmt.Fprintf(&builder, "<section data-section=\"%s\"><h2>\u7ae0\u8282\u6807\u9898</h2>", section)
		appendMaterialFacts(&builder, facts, section)
		builder.WriteString("<p>\u672c\u8282\u5185\u5bb9\u89c1\u8f93\u5165\u6750\u6599</p>")
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

func markerlessHTMLStatistics(document string, material Material, labels map[string]string) string {
	for name, count := range htmlStatisticCounts(material) {
		marked := fmt.Sprintf(`<output data-stat="%s">%d</output>`, name, count)
		unmarked := fmt.Sprintf(`<div><span>%s</span><strong>%d</strong></div>`, labels[name], count)
		document = strings.Replace(document, marked, unmarked, 1)
	}
	return document
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
		{"path", finding.Path},
		{"start_line", fmt.Sprint(finding.StartLine)},
		{"end_line", fmt.Sprint(finding.EndLine)},
		{"summary_zh", finding.Display.SummaryZH},
		{"severity_zh", finding.Display.SeverityZH},
		{"category_zh", finding.Display.CategoryZH},
		{"source_content", finding.SourceContent},
	}
	for _, fact := range facts {
		writeFindingFact(&builder, fact.name, fact.value)
	}
	writeFindingFact(&builder, "evidence_status", displayFindingFact("evidence_status", string(finding.Evidence.Status)))
	if finding.Evidence.Status == StatusProvided {
		writeFindingFact(&builder, "evidence_code", finding.Evidence.Code)
	} else {
		writeFindingFact(&builder, "evidence_reason", finding.Evidence.Reason)
	}
	writeFindingFact(&builder, "recommendation_status", displayFindingFact("recommendation_status", string(finding.Recommendation.Status)))
	if finding.Recommendation.Status == StatusProvided {
		writeFindingFact(&builder, "recommendation_code", finding.Recommendation.Code)
	} else {
		writeFindingFact(&builder, "recommendation_reason", finding.Recommendation.Reason)
	}
	builder.WriteString(`</article>`)
	return builder.String()
}

func writeFindingFact(builder *strings.Builder, name, value string) {
	label := findingHTMLLabels[name]
	if name == "evidence_code" || name == "recommendation_code" || name == "source_content" {
		fmt.Fprintf(builder, `<div class="fact-row"><strong class="fact-label">%s</strong><pre data-fact="%s">%s</pre></div>`, label, name, html.EscapeString(value))
		return
	}
	fmt.Fprintf(builder, `<div class="fact-row"><strong class="fact-label">%s</strong><span data-fact="%s">%s</span></div>`, label, name, html.EscapeString(value))
}
