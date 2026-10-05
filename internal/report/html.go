// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

const MaxHTMLDocumentBytes = 4 << 20
const reportHTMLStyles = `
* { box-sizing: border-box; }
body { max-width: 76rem; margin: 0 auto; padding: 1.5rem; background: #fff; color: #202a33; font-family: system-ui, sans-serif; line-height: 1.55; }
section { padding: 1.25rem 0; border-bottom: 1px solid #c7cdd3; }
h1, h2, h3 { line-height: 1.25; }
h2 { margin: 0 0 .75rem; font-size: 1.35rem; }
.fact-row { display: grid; grid-template-columns: minmax(8rem, 12rem) minmax(0, 1fr); gap: .5rem 1rem; padding: .35rem 0; }
.fact-label { color: #4b5563; font-weight: 600; }
[data-fact] { min-width: 0; overflow-wrap: anywhere; word-break: break-word; }
.report-statistics { display: grid; grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr)); gap: .75rem; }
.report-statistic { padding: .75rem; border: 1px solid #c7cdd3; border-radius: 4px; }
output { display: block; font-size: 1.25rem; font-weight: 700; font-variant-numeric: tabular-nums; }
[data-finding-id]:not(td):not(th) { display: block; }
[data-finding-id] { max-width: 100%; min-width: 0; margin: 1rem 0; padding: 1rem; border: 1px solid #aeb7c0; border-radius: 4px; }
section:not([data-section="finding-details"]) > div[data-review-unit-id] { margin: .75rem 0; padding: .5rem 0 .75rem; border-bottom: 1px solid #c7cdd3; }
[data-finding-id] h3 { margin-top: 0; }
pre { max-width: 100%; white-space: pre-wrap; overflow-wrap: anywhere; }
.fact-row pre { margin: 0; }
.report-filters { display: grid; gap: .75rem; margin: 1rem 0 1.5rem; }
.report-filters fieldset { min-width: 0; margin: 0; padding: .65rem .8rem; border: 1px solid #aeb7c0; border-radius: 4px; }
.report-filters legend { padding: 0 .25rem; font-weight: 700; }
.report-filters label { display: inline-flex; align-items: center; gap: .35rem; margin: .25rem .9rem .25rem 0; cursor: pointer; }
.report-filters input { accent-color: #176b74; }
.report-filters input:focus-visible, summary:focus-visible { outline: 3px solid #176b74; outline-offset: 3px; }
details.finding-details { margin: .5rem 0; }
details.finding-details > summary { cursor: pointer; border-radius: 2px; }
details.finding-details > summary .fact-row { margin: 0; }
details.finding-details[open] > .finding-detail-content { padding-top: .25rem; }
@media (max-width: 600px) { body { padding: 1rem; } .fact-row { grid-template-columns: minmax(0, 1fr); } }
@media print { body { max-width: none; padding: 0; color: #000; } .report-filters { display: none !important; } section, [data-finding-id], .fact-row { break-inside: avoid; page-break-inside: avoid; } details.finding-details:not([open]) > .finding-detail-content { display: block !important; } details.finding-details > summary { list-style: none; } }
`

var (
	localAbsolutePath = regexp.MustCompile(`(?i)([a-z]:[\\/]|\\\\[^\\]+\\|(?:^|[\s"'=(:,>])/(?:[a-z0-9._-]+/)+[a-z0-9._-]+(?:/[a-z0-9._-]+)*|(?:^|[\s"'=(:,>])/(?:root|home|users|private|tmp|var|opt|srv|etc|usr|mnt|media|workplace|workspace|volumes|system|applications|library)(?:[/\s"'>]|$))`)
	numericClaim      = regexp.MustCompile(`\b\d+\b`)
)

var requiredHTMLSections = []string{
	"overview", "quality-coverage", "finding-details", "changes", "achievements", "people", "governance", "limitations", "sources",
}

// IsRequiredHTMLSection 报告章节是否属于必需结构。 // allow-non-english: 用户要求中文代码注释
func IsRequiredHTMLSection(section string) bool {
	for _, required := range requiredHTMLSections {
		if section == required {
			return true
		}
	}
	return false
}

var allowedHTMLTags = map[string]struct{}{
	"html": {}, "head": {}, "body": {}, "title": {}, "meta": {}, "style": {}, "main": {}, "section": {},
	"h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {}, "div": {}, "span": {}, "strong": {},
	"em": {}, "b": {}, "i": {}, "p": {}, "pre": {}, "code": {}, "ul": {}, "ol": {}, "li": {}, "dl": {},
	"dt": {}, "dd": {}, "blockquote": {}, "br": {}, "hr": {}, "table": {}, "thead": {}, "tbody": {},
	"tr": {}, "th": {}, "td": {}, "output": {}, "time": {}, "mark": {}, "a": {}, "article": {},
	"fieldset": {}, "legend": {}, "label": {}, "input": {}, "details": {}, "summary": {},
}

var allowedHTMLFindingTags = map[string]struct{}{
	"article": {}, "section": {}, "div": {}, "blockquote": {}, "li": {},
	"dd": {}, "fieldset": {}, "td": {}, "th": {},
}

func validFindingContainerParent(node *html.Node) bool {
	if node.Parent == nil || node.Parent.Type != html.ElementNode {
		return false
	}
	parent := strings.ToLower(node.Parent.Data)
	switch strings.ToLower(node.Data) {
	case "li":
		return parent == "ul" || parent == "ol"
	case "dd":
		if parent != "dl" {
			return false
		}
		for sibling := node.PrevSibling; sibling != nil; sibling = sibling.PrevSibling {
			if sibling.Type != html.ElementNode {
				continue
			}
			switch strings.ToLower(sibling.Data) {
			case "dt":
				return true
			case "dd":
				continue
			default:
				return false
			}
		}
		return false
	case "td", "th":
		return parent == "tr"
	default:
		return true
	}
}

func ValidateHTMLDocument(document string, material Material) error {
	if err := ValidateMaterial(material); err != nil {
		return fmt.Errorf("validate report material: %w", err)
	}
	if len(document) == 0 || len(document) > MaxHTMLDocumentBytes {
		return fmt.Errorf("HTML document size must be between 1 byte and %d bytes", MaxHTMLDocumentBytes)
	}
	trimmed := strings.TrimSpace(document)
	if !strings.HasPrefix(strings.ToLower(trimmed), "<!doctype html>") {
		return fmt.Errorf("HTML document must begin with an HTML doctype")
	}
	if localAbsolutePath.MatchString(document) {
		return fmt.Errorf("HTML document contains a local absolute path")
	}

	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return fmt.Errorf("parse HTML document: %w", err)
	}
	var htmlNode, headNode, bodyNode, mainNode *html.Node
	sections := make(map[string]*html.Node)
	stats := make(map[string]*html.Node)
	findings := make(map[string]*html.Node)
	title := ""
	charset := false
	styleCount := 0
	var visit func(*html.Node) error
	visit = func(node *html.Node) error {
		if node.Type == html.TextNode && localAbsolutePath.MatchString(node.Data) {
			return fmt.Errorf("HTML document contains a local absolute path")
		}
		if node.Type == html.ElementNode {
			tag := strings.ToLower(node.Data)
			if _, allowed := allowedHTMLTags[tag]; !allowed {
				return fmt.Errorf("HTML document contains unsupported <%s> content", tag)
			}
			switch tag {
			case "html":
				if htmlNode != nil {
					return fmt.Errorf("HTML document must contain one html element")
				}
				htmlNode = node
				if value := attribute(node, "lang"); !strings.EqualFold(value, "zh-CN") {
					return fmt.Errorf("HTML document must declare lang=zh-CN")
				}
			case "head":
				headNode = node
			case "body":
				bodyNode = node
			case "main":
				if mainNode != nil {
					return fmt.Errorf("HTML document must contain one main element")
				}
				mainNode = node
				if value := attribute(node, "data-review-status"); value != string(material.Review.Status) {
					return fmt.Errorf("main review status does not match the report material")
				}
				if value := attribute(node, "data-run-id"); value != material.Review.RunID {
					return fmt.Errorf("main run id does not match the report material")
				}
			case "title":
				title = nodeText(node)
			case "meta":
				if strings.EqualFold(attribute(node, "http-equiv"), "refresh") {
					return fmt.Errorf("HTML document contains a refresh directive")
				}
				if strings.EqualFold(attribute(node, "charset"), "utf-8") {
					charset = true
				}
			case "style":
				styleCount++
				if nodeText(node) != reportHTMLStyles && nodeText(node) != reportHTMLStylesFor([]Material{material}) {
					return fmt.Errorf("HTML document contains a stylesheet outside the report template")
				}
			case "fieldset":
				if !isHTMLFindingContent(node) && !insideReportFilters(node) {
					return fmt.Errorf("HTML fieldsets must belong to a finding or report filters")
				}
			case "legend":
				parent := node.Parent
				if !insideFinding(node) && (parent == nil || parent.Type != html.ElementNode || !strings.EqualFold(parent.Data, "fieldset") || !insideReportFilters(parent)) {
					return fmt.Errorf("HTML legends must belong to a finding or report-filter fieldset")
				}
			case "details":
				if !hasAttribute(node, "open") || !hasClass(node, "finding-details") || !insideFinding(node) {
					return fmt.Errorf("HTML details must be an open finding disclosure")
				}
			case "summary":
				if node.Parent == nil || node.Parent.Type != html.ElementNode || !strings.EqualFold(node.Parent.Data, "details") {
					return fmt.Errorf("HTML summary must belong to a finding disclosure")
				}
			}
			if err := validateAttributes(node); err != nil {
				return err
			}
			if value := attribute(node, "data-section"); value != "" {
				if sections[value] != nil {
					return fmt.Errorf("HTML document duplicates required section %q", value)
				}
				sections[value] = node
			}
			if value := attribute(node, "data-stat"); value != "" {
				if _, exists := stats[value]; exists {
					return fmt.Errorf("HTML document duplicates statistic %q", value)
				}
				stats[value] = node
			}
			if value := attribute(node, "data-finding-id"); value != "" {
				if _, ok := allowedHTMLFindingTags[tag]; !ok {
					return fmt.Errorf("HTML finding %q must use a flow-content container", value)
				}
				if !validFindingContainerParent(node) {
					return fmt.Errorf("HTML finding %q must use a valid parent for its container", value)
				}
				if _, exists := findings[value]; exists {
					return fmt.Errorf("HTML document duplicates finding %q", value)
				}
				findings[value] = node
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return err
	}
	if htmlNode == nil || headNode == nil || bodyNode == nil || mainNode == nil || strings.TrimSpace(title) == "" || !charset {
		return fmt.Errorf("HTML document requires html, head, utf-8 charset, title, body, and main elements")
	}
	if styleCount != 1 {
		return fmt.Errorf("HTML document must contain exactly one report stylesheet")
	}
	if !hasAncestor(mainNode, bodyNode) {
		return fmt.Errorf("main element must be inside body")
	}
	if !containsHan(nodeText(mainNode)) {
		return fmt.Errorf("HTML document must contain Chinese report text")
	}
	for _, name := range requiredHTMLSections {
		section := sections[name]
		if section == nil || !hasAncestor(section, mainNode) || !hasVisibleSectionContent(section) {
			return fmt.Errorf("HTML document is missing required section %q", name)
		}
		if err := validateSectionHeading(section, name); err != nil {
			return err
		}
	}
	if err := validateHTMLFindings(findings, material, sections["finding-details"]); err != nil {
		return err
	}
	statisticValues, err := validateHTMLStats(stats, material, sections["quality-coverage"])
	if err != nil {
		return err
	}
	return validateHTMLClaims(bodyNode, material, statisticValues)
}

func addReportHTMLStyles(document string) (string, error) {
	trimmed := strings.TrimSpace(document)
	if !strings.HasPrefix(strings.ToLower(trimmed), "<!doctype html>") {
		return "", fmt.Errorf("HTML document must begin with an HTML doctype")
	}
	if len(document) == 0 || len(document) > MaxHTMLDocumentBytes {
		return "", fmt.Errorf("HTML document size must be between 1 byte and %d bytes", MaxHTMLDocumentBytes)
	}
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return "", fmt.Errorf("parse model HTML: %w", err)
	}
	var headNode *html.Node
	var visit func(*html.Node) error
	visit = func(node *html.Node) error {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "fieldset":
				if !isHTMLFindingContent(node) {
					return fmt.Errorf("model HTML fieldsets must belong to a finding")
				}
			case "legend":
				parent := node.Parent
				if parent == nil || parent.Type != html.ElementNode || !strings.EqualFold(parent.Data, "fieldset") || !isHTMLFindingContent(parent) {
					return fmt.Errorf("model HTML legends must belong to a finding fieldset")
				}
			case "label", "input", "details", "summary":
				return fmt.Errorf("model HTML cannot provide report controls")
			}
			if attribute(node, "data-report-ui") != "" || attribute(node, "data-review-unit-index") != "" || attribute(node, "data-review-severity-index") != "" || attribute(node, "data-review-category-index") != "" {
				return fmt.Errorf("model HTML cannot provide report control markers")
			}
			if strings.EqualFold(node.Data, "head") && headNode == nil {
				headNode = node
			}
			if strings.EqualFold(node.Data, "style") {
				return fmt.Errorf("model HTML must not provide a stylesheet")
			}
			for _, attr := range node.Attr {
				if strings.EqualFold(attr.Key, "style") {
					return fmt.Errorf("model HTML must not provide style attributes")
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return "", err
	}
	if headNode == nil {
		return "", fmt.Errorf("model HTML must contain a head element")
	}
	styleNode := &html.Node{Type: html.ElementNode, Data: "style"}
	styleNode.AppendChild(&html.Node{Type: html.TextNode, Data: reportHTMLStyles})
	headNode.AppendChild(styleNode)
	var builder strings.Builder
	if err := html.Render(&builder, root); err != nil {
		return "", fmt.Errorf("render report stylesheet: %w", err)
	}
	return builder.String(), nil
}

func validateHTMLFindings(nodes map[string]*html.Node, material Material, findingDetails *html.Node) error {
	if len(nodes) != len(material.Findings) {
		return fmt.Errorf("HTML finding set contains %d items; report material contains %d", len(nodes), len(material.Findings))
	}
	for _, finding := range material.Findings {
		node := nodes[finding.ID]
		if node == nil {
			return fmt.Errorf("HTML document omits finding %q", finding.ID)
		}
		var mainNode *html.Node
		for current := node.Parent; current != nil; current = current.Parent {
			if current.Type == html.ElementNode && strings.EqualFold(current.Data, "main") {
				mainNode = current
				break
			}
		}
		if mainNode == nil {
			return fmt.Errorf("HTML finding %q must appear inside main", finding.ID)
		}
		if !hasAncestor(node, findingDetails) {
			return fmt.Errorf("HTML finding %q must appear in the finding-details section", finding.ID)
		}
		for name, expected := range map[string]string{
			"severity": finding.Severity, "category": finding.Category, "path": finding.Path,
			"start-line": strconv.Itoa(finding.StartLine), "end-line": strconv.Itoa(finding.EndLine),
		} {
			if attribute(node, "data-"+name) != expected {
				return fmt.Errorf("HTML finding %q has an incorrect %s", finding.ID, name)
			}
		}
		expectedFacts := map[string]string{
			"path": finding.Path, "start_line": strconv.Itoa(finding.StartLine), "end_line": strconv.Itoa(finding.EndLine),
			"summary_zh": finding.Display.SummaryZH, "severity_zh": finding.Display.SeverityZH,
			"category_zh": finding.Display.CategoryZH, "source_content": finding.SourceContent,
			"evidence_status":       displayFindingFact("evidence_status", string(finding.Evidence.Status)),
			"recommendation_status": displayFindingFact("recommendation_status", string(finding.Recommendation.Status)),
		}
		if finding.Evidence.Status == StatusProvided {
			expectedFacts["evidence_code"] = finding.Evidence.Code
		} else {
			expectedFacts["evidence_reason"] = finding.Evidence.Reason
		}
		if finding.Recommendation.Status == StatusProvided {
			expectedFacts["recommendation_code"] = finding.Recommendation.Code
		} else {
			expectedFacts["recommendation_reason"] = finding.Recommendation.Reason
		}
		visibleText := normalizedHTMLText(nodeText(node))
		if !findingHasVisibleDescription(node) {
			return fmt.Errorf("HTML finding %q must contain a visible Chinese description", finding.ID)
		}
		for _, fact := range []struct{ name, value string }{
			{"path", finding.Path}, {"start_line", strconv.Itoa(finding.StartLine)},
			{"end_line", strconv.Itoa(finding.EndLine)}, {"severity_zh", finding.Display.SeverityZH},
			{"source_content", finding.SourceContent},
		} {
			if value := normalizedHTMLText(fact.value); value != "" && !strings.Contains(visibleText, value) {
				return fmt.Errorf("HTML finding %q omits a required displayed fact", finding.ID)
			}
		}
		evidenceValue := finding.Evidence.Code
		if finding.Evidence.Status != StatusProvided {
			evidenceValue = finding.Evidence.Reason
		}
		if value := normalizedHTMLText(evidenceValue); value != "" && !strings.Contains(visibleText, value) {
			return fmt.Errorf("HTML finding %q omits its evidence", finding.ID)
		}
		recommendationValue := finding.Recommendation.Code
		if finding.Recommendation.Status != StatusProvided {
			recommendationValue = finding.Recommendation.Reason
		}
		if value := normalizedHTMLText(recommendationValue); value != "" && !strings.Contains(visibleText, value) {
			return fmt.Errorf("HTML finding %q omits its recommendation", finding.ID)
		}
		for name, matches := range findAllDataFacts(node) {
			expected, exists := expectedFacts[name]
			if !exists {
				return fmt.Errorf("HTML finding contains an unsupported fact name")
			}
			if len(matches) != 1 || normalizedHTMLText(nodeText(matches[0])) != normalizedHTMLText(expected) {
				return fmt.Errorf("HTML finding %q contains an incorrect fact %q", finding.ID, name)
			}
		}
	}
	return nil
}

func findingHasVisibleDescription(finding *html.Node) bool {
	return findingSummaryNode(finding) != nil
}

func findingSummaryNode(finding *html.Node) *html.Node {
	if facts := findAllDataFacts(finding); len(facts["summary_zh"]) == 1 {
		if strings.TrimSpace(nodeText(facts["summary_zh"][0])) != "" {
			return facts["summary_zh"][0]
		}
	}
	var found *html.Node
	foundPriority, foundLength := 0, 0
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type != html.ElementNode {
			return
		}
		if strings.EqualFold(node.Data, "pre") || strings.EqualFold(node.Data, "code") ||
			attribute(node, "data-fact") != "" || hasClass(node, "fact-label") {
			return
		}
		if !hasClass(node, "fact-row") {
			text := strings.TrimSpace(nodeText(node))
			hanCount := countHan(text)
			if hanCount >= 2 && len([]rune(text)) <= 300 {
				priority := 0
				if isHeading(node.Data) || strings.EqualFold(node.Data, "p") {
					priority = 1
				}
				if hasClass(node, "finding-summary") {
					priority = 2
				}
				if strings.EqualFold(node.Data, "div") || strings.EqualFold(node.Data, "li") || strings.EqualFold(node.Data, "span") {
					priority = 1
				}
				if priority > foundPriority || priority == foundPriority && len([]rune(text)) > foundLength {
					found, foundPriority, foundLength = node, priority, len([]rune(text))
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	for child := finding.FirstChild; child != nil; child = child.NextSibling {
		visit(child)
	}
	return found
}

func countHan(value string) int {
	count := 0
	for _, char := range value {
		if unicode.Is(unicode.Han, char) {
			count++
		}
	}
	return count
}

func displayFindingFact(name, value string) string {
	if !strings.HasSuffix(name, "_status") {
		return value
	}
	switch value {
	case "provided":
		return "\u5df2\u63d0\u4f9b"
	case "not_collected":
		return "\u672a\u63d0\u4f9b"
	case "not_applicable":
		return "\u4e0d\u9002\u7528"
	case "failed":
		return "\u5931\u8d25"
	default:
		return value
	}
}

var findingHTMLLabels = map[string]string{
	"path": "\u6587\u4ef6", "start_line": "\u884c\u53f7", "end_line": "\u884c\u53f7",
	"summary_zh": "\u6458\u8981", "severity_zh": "\u4e25\u91cd\u7b49\u7ea7", "category_zh": "\u7c7b\u522b",
	"source_content": "\u6e90\u4ee3\u7801", "evidence_status": "\u72b6\u6001", "recommendation_status": "\u72b6\u6001",
	"evidence_code": "\u8bc1\u636e", "evidence_reason": "\u539f\u56e0",
	"recommendation_code": "\u5efa\u8bae", "recommendation_reason": "\u539f\u56e0",
}

func HTMLFindingFactLabel(name string) (string, bool) {
	label, ok := findingHTMLLabels[name]
	return label, ok
}

func hasClass(node *html.Node, class string) bool {
	for _, value := range strings.Fields(attribute(node, "class")) {
		if value == class {
			return true
		}
	}
	return false
}

func validateHTMLStats(stats map[string]*html.Node, material Material, qualityCoverage *html.Node) (map[*html.Node]string, error) {
	expected := htmlStatisticCounts(material)
	for name, node := range stats {
		count, ok := expected[name]
		if !ok || strings.TrimSpace(nodeText(node)) != strconv.Itoa(count) || !insideMain(node) || !hasAncestor(node, qualityCoverage) {
			return nil, fmt.Errorf("HTML statistic %q does not match the report material", name)
		}
	}
	markerlessValues, markerlessNames, err := markerlessHTMLStatisticValues(qualityCoverage, stats, expected)
	if err != nil {
		return nil, err
	}
	for name := range expected {
		if stats[name] != nil {
			continue
		}
		if _, ok := markerlessNames[name]; !ok {
			return nil, fmt.Errorf("HTML quality statistics omit a labeled value for %q", name)
		}
	}
	return markerlessValues, nil
}

type htmlStatisticValue struct {
	node  *html.Node
	value string
}

func markerlessHTMLStatisticValues(qualityCoverage *html.Node, stats map[string]*html.Node, expected map[string]int) (map[*html.Node]string, map[string]struct{}, error) {
	values := make(map[*html.Node]string)
	names := make(map[string]struct{})
	var occurrences []htmlStatisticValue
	collectHTMLStatisticContext(qualityCoverage, &occurrences, nil)
	for _, occurrence := range occurrences {
		for group := occurrence.node.Parent; group != nil && group != qualityCoverage; group = group.Parent {
			if group.Type != html.ElementNode {
				continue
			}
			var groupValues []htmlStatisticValue
			var text strings.Builder
			collectHTMLStatisticContext(group, &groupValues, &text)
			if len(groupValues) != 1 || groupValues[0].node != occurrence.node || groupValues[0].value != occurrence.value {
				continue
			}
			labels := htmlStatisticLabelsInText(text.String())
			if len(labels) != 1 {
				continue
			}
			for name := range labels {
				if occurrence.value != strconv.Itoa(expected[name]) {
					return nil, nil, fmt.Errorf("HTML statistic %q does not match the report material", name)
				}
				values[occurrence.node] = occurrence.value
				names[name] = struct{}{}
			}
			break
		}
	}
	for name := range stats {
		delete(names, name)
	}
	return values, names, nil
}

func collectHTMLStatisticContext(node *html.Node, values *[]htmlStatisticValue, text *strings.Builder) {
	if node.Type == html.ElementNode && (attribute(node, "data-fact") != "" || attribute(node, "data-stat") != "") {
		return
	}
	if node.Type == html.TextNode {
		if text != nil {
			text.WriteByte(' ')
			text.WriteString(node.Data)
		}
		for _, value := range numericClaim.FindAllString(node.Data, -1) {
			*values = append(*values, htmlStatisticValue{node: node, value: value})
		}
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectHTMLStatisticContext(child, values, text)
	}
}

var htmlStatisticLabelAliases = map[string][]string{
	"finding-count":      {"findings", "finding", "issues", "issue", "defects", "defect", "problems", "problem", "\u95ee\u9898\u603b\u6570", "\u7f3a\u9677\u603b\u6570", "\u95ee\u9898\u6570", "\u7f3a\u9677\u6570", "\u53d1\u73b0\u6570"},
	"risk-critical":      {"critical", "blocker", "\u4e25\u91cd", "\u81f4\u547d", "\u6781\u9ad8"},
	"risk-high":          {"high", "\u9ad8\u98ce\u9669", "\u9ad8\u5371", "\u9ad8"},
	"risk-medium":        {"medium", "moderate", "\u4e2d\u7b49", "\u4e2d\u98ce\u9669", "\u4e2d\u5371", "\u4e2d"},
	"risk-low":           {"low", "minor", "\u4f4e\u98ce\u9669", "\u4f4e\u5371", "\u4f4e"},
	"coverage-selected":  {"selected", "planned", "in scope", "\u9009\u4e2d", "\u9009\u62e9", "\u8ba1\u5212", "\u7eb3\u5165", "\u5e94\u68c0\u67e5", "\u68c0\u67e5\u8303\u56f4"},
	"coverage-completed": {"completed", "complete", "checked", "executed", "\u5df2\u5b8c\u6210", "\u5b8c\u6210", "\u5df2\u6267\u884c", "\u5df2\u68c0\u67e5"},
	"coverage-failed":    {"failed", "failure", "errors", "error", "\u5931\u8d25", "\u672a\u901a\u8fc7", "\u9519\u8bef"},
	"coverage-skipped":   {"skipped", "waived", "excluded", "not run", "\u8df3\u8fc7", "\u8c41\u514d", "\u6392\u9664", "\u672a\u6267\u884c", "\u672a\u68c0\u67e5"},
	"coverage-reused":    {"reused", "re-use", "cached", "cache hit", "\u590d\u7528", "\u91cd\u7528", "\u7f13\u5b58\u547d\u4e2d", "\u5df2\u7f13\u5b58"},
}

type htmlStatisticLabelMatch struct {
	name  string
	start int
	end   int
}

func htmlStatisticLabelsInText(value string) map[string]struct{} {
	value = strings.ToLower(value)
	var matches []htmlStatisticLabelMatch
	for name, aliases := range htmlStatisticLabelAliases {
		for _, alias := range aliases {
			for start := 0; start < len(value); {
				relative := strings.Index(value[start:], alias)
				if relative < 0 {
					break
				}
				start += relative
				end := start + len(alias)
				if statisticAliasBoundary(value, start, end, alias) {
					matches = append(matches, htmlStatisticLabelMatch{name: name, start: start, end: end})
				}
				start = end
			}
		}
	}
	labels := make(map[string]struct{})
	for _, match := range matches {
		shadowed := false
		for _, other := range matches {
			if other.name != match.name && other.end-other.start > match.end-match.start && other.start < match.end && match.start < other.end {
				shadowed = true
				break
			}
		}
		if !shadowed {
			labels[match.name] = struct{}{}
		}
	}
	if _, hasRisk := labels["risk-critical"]; hasRisk {
		delete(labels, "finding-count")
	}
	for _, severity := range []string{"risk-high", "risk-medium", "risk-low"} {
		if _, hasRisk := labels[severity]; hasRisk {
			delete(labels, "finding-count")
			break
		}
	}
	return labels
}

func statisticAliasBoundary(value string, start, end int, alias string) bool {
	if strings.IndexFunc(alias, func(r rune) bool {
		return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
	}) < 0 {
		return true
	}
	return (start == 0 || !isASCIIWordByte(value[start-1])) && (end == len(value) || !isASCIIWordByte(value[end]))
}

func isASCIIWordByte(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' || value >= 'a' && value <= 'z'
}

func htmlStatisticCounts(material Material) map[string]int {
	counts := map[string]int{
		"finding-count":     len(material.Findings),
		"coverage-selected": len(material.Coverage.Selected), "coverage-completed": len(material.Coverage.Completed),
		"coverage-failed": len(material.Coverage.Failed), "coverage-skipped": len(material.Coverage.Waived),
		"coverage-reused": len(material.Coverage.Reused),
	}
	for _, severity := range []string{"critical", "high", "medium", "low"} {
		counts["risk-"+severity] = 0
	}
	for _, finding := range material.Findings {
		if finding.SeverityStatus == StatusProvided {
			counts["risk-"+finding.Severity]++
		}
	}
	return counts
}

func validateSectionHeading(section *html.Node, name string) error {
	var heading *html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		if heading != nil || node.Type != html.ElementNode {
			return
		}
		if isHeading(node.Data) {
			heading = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(section)
	if heading == nil || strings.TrimSpace(nodeText(heading)) == "" {
		return fmt.Errorf("HTML section %q must contain a non-empty heading", name)
	}
	return nil
}

func hasVisibleSectionContent(section *html.Node) bool {
	var found bool
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if found {
			return
		}
		if node.Type == html.ElementNode && (isHeading(node.Data) || hasClass(node, "fact-label")) {
			return
		}
		if node.Type == html.TextNode && strings.TrimSpace(node.Data) != "" {
			found = true
			return
		}
		for child := node.FirstChild; child != nil && !found; child = child.NextSibling {
			visit(child)
		}
	}
	visit(section)
	return found
}

func validateHTMLClaims(bodyNode *html.Node, material Material, statisticValues map[*html.Node]string) error {
	facts, err := materialHTMLFacts(material)
	if err != nil {
		return err
	}
	knownNumbers := make(map[string]struct{})
	for _, value := range facts {
		for _, number := range numericClaim.FindAllString(value, -1) {
			knownNumbers[number] = struct{}{}
		}
	}
	var visit func(*html.Node) error
	visit = func(node *html.Node) error {
		if node.Type == html.ElementNode {
			if attribute(node, "data-report-ui") == "filters" {
				return nil
			}
			if !insideMain(node) && node != bodyNode && node.Data != "main" {
				if strings.TrimSpace(nodeText(node)) != "" || attribute(node, "data-fact") != "" {
					return fmt.Errorf("HTML body contains report content outside main")
				}
			}
			if factName := attribute(node, "data-fact"); factName != "" {
				if insideFinding(node) {
					return nil
				}
				expected, exists := facts[factName]
				if !exists {
					return fmt.Errorf("HTML contains an unsupported fact")
				}
				if hasNestedFact(node) {
					return fmt.Errorf("HTML fact %q cannot contain another data-fact", factName)
				}
				actual := normalizedHTMLText(nodeText(node))
				if actual != expected && actual != displayHTMLFact(factName, expected) {
					return fmt.Errorf("HTML fact %q does not match the report material", factName)
				}
				return nil
			}
			if attribute(node, "data-stat") != "" {
				if hasNestedFact(node) {
					return fmt.Errorf("HTML statistic cannot contain a data-fact")
				}
				return nil
			}
			if isHeading(node.Data) || hasClass(node, "fact-label") {
				return nil
			}
		}
		if node.Type == html.CommentNode {
			return fmt.Errorf("HTML report cannot contain hidden comments")
		}
		if node.Type == html.TextNode {
			value := strings.TrimSpace(node.Data)
			if value != "" {
				if !insideMain(node) {
					return fmt.Errorf("HTML body contains report text outside main")
				}
				if !insideFinding(node) {
					for _, number := range numericClaim.FindAllString(value, -1) {
						if statisticValue, statistic := statisticValues[node]; statistic && number == statisticValue {
							continue
						}
						if _, exists := knownNumbers[number]; !exists {
							return fmt.Errorf("HTML contains an unsupported numeric claim")
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(bodyNode); err != nil {
		return err
	}
	return nil
}

var requiredHTMLFactPrefixes = map[string][]string{
	"overview":     {"review.", "repository.", "scope.", "run_failure."},
	"changes":      {"sections.git_statistics.", "sections.workspace_snapshot."},
	"achievements": {"sections.achievements."},
	"people":       {"sections.people."},
	"governance":   {"sections.structural_checks."},
	"limitations":  {"limitations["},
	"sources":      {"schema_version", "review.run_id", "sections.knowledge_sources."},
}

func materialHTMLFacts(material Material) (map[string]string, error) {
	data, err := json.Marshal(material)
	if err != nil {
		return nil, fmt.Errorf("encode report material facts: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode report material facts: %w", err)
	}
	facts := make(map[string]string)
	var collect func(string, any)
	collect = func(path string, value any) {
		switch current := value.(type) {
		case map[string]any:
			for key, child := range current {
				if path == "" && key == "findings" {
					continue
				}
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				collect(childPath, child)
			}
		case []any:
			for index, child := range current {
				collect(fmt.Sprintf("%s[%d]", path, index), child)
			}
		case string:
			facts[path] = current
		case json.Number:
			facts[path] = current.String()
		case bool:
			facts[path] = strconv.FormatBool(current)
		}
	}
	collect("", root)
	return facts, nil
}

func htmlFactAllowedInSection(section, fact string) bool {
	if section == "quality-coverage" {
		return strings.HasPrefix(fact, "coverage.")
	}
	for _, prefix := range requiredHTMLFactPrefixes[section] {
		if strings.HasPrefix(fact, prefix) {
			return true
		}
	}
	return false
}

func displayHTMLFact(name, value string) string {
	if !strings.HasSuffix(name, ".status") {
		return value
	}
	switch value {
	case "provided":
		return "\u5df2\u63d0\u4f9b"
	case "not_collected":
		return "\u672a\u63d0\u4f9b"
	case "not_applicable":
		return "\u4e0d\u9002\u7528"
	case "failed":
		return "\u5931\u8d25"
	case "complete":
		return "\u5df2\u5b8c\u6210"
	case "partial":
		return "\u90e8\u5206\u5b8c\u6210"
	case "skipped":
		return "\u5df2\u8df3\u8fc7"
	default:
		return value
	}
}

func enclosingSection(node *html.Node) string {
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Type == html.ElementNode {
			if name := attribute(current, "data-section"); name != "" {
				return name
			}
		}
	}
	return ""
}

func insideFinding(node *html.Node) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Type == html.ElementNode && attribute(current, "data-finding-id") != "" {
			return true
		}
	}
	return false
}

func isHTMLFindingContent(node *html.Node) bool {
	return attribute(node, "data-finding-id") != "" || insideFinding(node)
}

func insideReportFilters(node *html.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current.Type == html.ElementNode && attribute(current, "data-report-ui") == "filters" {
			return true
		}
	}
	return false
}

func hasNestedFact(node *html.Node) bool {
	var found bool
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current != node && current.Type == html.ElementNode && attribute(current, "data-fact") != "" {
			found = true
			return
		}
		for child := current.FirstChild; child != nil && !found; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return found
}

func isHeading(tag string) bool {
	return len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6'
}

func insideMain(node *html.Node) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Type == html.ElementNode && strings.EqualFold(current.Data, "main") {
			return true
		}
	}
	return false
}

func containsHan(value string) bool {
	for _, char := range value {
		if unicode.Is(unicode.Han, char) {
			return true
		}
	}
	return false
}

func validateAttributes(node *html.Node) error {
	allowed := map[string]struct{}{
		"class": {}, "id": {}, "lang": {}, "charset": {}, "name": {}, "content": {}, "role": {},
		"scope": {}, "colspan": {}, "rowspan": {}, "headers": {}, "dir": {}, "href": {},
		"type": {}, "value": {}, "checked": {}, "open": {},
	}
	blocked := map[string]struct{}{
		"style": {}, "src": {}, "srcset": {}, "action": {}, "formaction": {}, "poster": {}, "ping": {},
		"background": {}, "manifest": {}, "xlink:href": {}, "srcdoc": {}, "codebase": {}, "archive": {},
		"data": {}, "lowsrc": {}, "dynsrc": {}, "formtarget": {}, "title": {}, "aria-label": {},
		"aria-description": {}, "aria-valuetext": {}, "alt": {},
	}
	allowedData := map[string]struct{}{
		"data-review-status": {}, "data-run-id": {}, "data-section": {}, "data-stat": {},
		"data-finding-id": {}, "data-severity": {}, "data-category": {}, "data-path": {},
		"data-start-line": {}, "data-end-line": {}, "data-fact": {},
		"data-report-kind": {}, "data-review-unit-count": {}, "data-review-unit-id": {},
		"data-report-ui": {}, "data-review-unit-index": {}, "data-review-severity-index": {}, "data-review-category-index": {},
	}
	for _, attr := range node.Attr {
		name := strings.ToLower(attr.Key)
		value := strings.TrimSpace(strings.ToLower(attr.Val))
		if localAbsolutePath.MatchString(attr.Val) {
			return fmt.Errorf("HTML document contains a local absolute path")
		}
		if strings.HasPrefix(name, "on") {
			return fmt.Errorf("HTML document contains disallowed %q attribute", name)
		}
		if _, disallowed := blocked[name]; disallowed {
			return fmt.Errorf("HTML document contains disallowed %q attribute", name)
		}
		if strings.HasPrefix(name, "data-") {
			if _, ok := allowedData[name]; !ok {
				return fmt.Errorf("HTML document contains unsupported %q attribute", name)
			}
		} else if _, ok := allowed[name]; !ok {
			return fmt.Errorf("HTML document contains unsupported %q attribute", name)
		}
		if name == "hidden" || (name == "aria-hidden" && value == "true") {
			return fmt.Errorf("HTML document contains hidden content")
		}
		if name == "href" && !strings.HasPrefix(value, "#") {
			return fmt.Errorf("HTML document contains an external or unsafe link")
		}
		if name == "type" && (strings.ToLower(node.Data) != "input" || value != "radio") {
			return fmt.Errorf("HTML document contains an unsupported input type")
		}
		if (name == "checked" && (node.Data != "input" || value != "")) || (name == "open" && (node.Data != "details" || value != "")) {
			return fmt.Errorf("HTML document contains an unsupported boolean attribute")
		}
		if strings.Contains(value, "javascript:") || strings.Contains(value, "data:") || strings.Contains(value, "http:") || strings.Contains(value, "https:") {
			return fmt.Errorf("HTML document contains an external or unsafe URL")
		}
	}
	return nil
}

func reportHTMLStylesFor(materials []Material) string {
	var styles strings.Builder
	styles.WriteString(reportHTMLStyles)
	for index := range materials {
		fmt.Fprintf(&styles, `main:has(#report-filter-unit-option-%d:checked) [data-finding-id]:not([data-review-unit-index="%d"]) { display: none; }`, index, index)
	}
	for index := range []string{"critical", "high", "medium", "low", "not_collected"} {
		fmt.Fprintf(&styles, `main:has(#report-filter-severity-option-%d:checked) [data-finding-id]:not([data-review-severity-index="%d"]) { display: none; }`, index, index)
	}
	for _, category := range reportFilterCategories(materials) {
		fmt.Fprintf(&styles, `main:has(#report-filter-category-option-%d:checked) [data-finding-id]:not([data-review-category-index="%d"]) { display: none; }`, category.index, category.index)
	}
	return styles.String()
}

type reportFilterCategory struct {
	index int
	name  string
	label string
}

func reportFilterCategories(materials []Material) []reportFilterCategory {
	labels := make(map[string]string)
	for _, material := range materials {
		for _, finding := range material.Findings {
			if _, exists := labels[finding.Category]; !exists {
				labels[finding.Category] = finding.Display.CategoryZH
			}
		}
	}
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)
	categories := make([]reportFilterCategory, len(names))
	for index, name := range names {
		categories[index] = reportFilterCategory{index: index, name: name, label: labels[name]}
	}
	return categories
}

func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func hasAttribute(node *html.Node, name string) bool {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return true
		}
	}
	return false
}

func findAllDataFacts(root *html.Node) map[string][]*html.Node {
	found := make(map[string][]*html.Node)
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if name := attribute(node, "data-fact"); name != "" {
				found[name] = append(found[name], node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return found
}

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return builder.String()
}

func hasAncestor(node, ancestor *html.Node) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if current == ancestor {
			return true
		}
	}
	return false
}

func normalizedHTMLText(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}
