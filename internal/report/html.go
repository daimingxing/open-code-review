// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"encoding/json"
	"fmt"
	"regexp"
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
article { max-width: 100%; min-width: 0; margin: 1rem 0; padding: 1rem; border: 1px solid #aeb7c0; border-radius: 4px; }
article h3 { margin-top: 0; }
pre { max-width: 100%; white-space: pre-wrap; overflow-wrap: anywhere; }
.fact-row pre { margin: 0; }
@media (max-width: 600px) { body { padding: 1rem; } .fact-row { grid-template-columns: minmax(0, 1fr); } }
`

var (
	localAbsolutePath = regexp.MustCompile(`(?i)([a-z]:[\\/]|\\\\[^\\]+\\|(?:^|[\s"'=(:,>])/(?:[a-z0-9._-]+/)+[a-z0-9._-]+(?:/[a-z0-9._-]+)*|(?:^|[\s"'=(:,>])/(?:root|home|users|private|tmp|var|opt|srv|etc|usr|mnt|media|workplace|workspace|volumes|system|applications|library)(?:[/\s"'>]|$))`)
)

var requiredHTMLSections = []string{
	"overview", "quality-coverage", "finding-details", "changes", "achievements", "people", "governance", "limitations", "sources",
}

var requiredHTMLHeadings = map[string]string{
	"overview":         "\u62a5\u544a\u6982\u89c8",
	"quality-coverage": "\u8d28\u91cf\u4e0e\u8986\u76d6",
	"finding-details":  "\u95ee\u9898\u660e\u7ec6",
	"changes":          "\u4ed3\u5e93\u53d8\u66f4",
	"achievements":     "\u5de5\u4f5c\u6210\u679c",
	"people":           "\u4eba\u5458\u660e\u7ec6",
	"governance":       "\u9879\u76ee\u7ed3\u6784\u68c0\u67e5",
	"limitations":      "\u9650\u5236\u4e0e\u672a\u786e\u8ba4\u4e8b\u9879",
	"sources":          "\u6750\u6599\u6765\u6e90",
}

var allowedHTMLLabels = map[string]struct{}{
	"\u4ed3\u5e93": {}, "\u4ed3\u5e93\u6807\u8bc6": {}, "\u5ba1\u67e5\u72b6\u6001": {}, "\u5ba1\u67e5\u8303\u56f4": {},
	"\u5f00\u59cb\u65f6\u95f4": {}, "\u7ed3\u675f\u65f6\u95f4": {}, "\u8017\u65f6": {}, "\u63d0\u4f9b\u65b9": {},
	"\u6a21\u578b": {}, "\u57fa\u51c6\u63d0\u4ea4": {}, "\u76ee\u6807\u63d0\u4ea4": {}, "\u5b9e\u9645\u8303\u56f4": {},
	"\u5ba1\u67e5\u8986\u76d6": {}, "\u8bc1\u636e": {}, "\u5efa\u8bae": {}, "\u6458\u8981": {}, "\u4e25\u91cd\u7b49\u7ea7": {},
	"\u7c7b\u522b": {}, "\u6587\u4ef6": {}, "\u884c\u53f7": {}, "\u72b6\u6001": {}, "\u539f\u56e0": {},
	"\u77e5\u8bc6\u6765\u6e90": {}, "\u6210\u679c": {}, "\u4eba\u5458": {}, "\u6765\u6e90": {}, "\u9650\u5236": {},
	"\u6750\u6599\u7248\u672c": {}, "\u8fd0\u884c\u6807\u8bc6": {}, "\u9009\u4e2d": {}, "\u6e90\u4ee3\u7801": {},
	"\u6750\u6599\u4e8b\u5b9e":             {},
	"\u9879\u76ee\u7ed3\u6784\u68c0\u67e5": {},
	"\u95ee\u9898\u6570\u91cf":             {}, "\u4e25\u91cd": {}, "\u9ad8": {}, "\u4e2d": {}, "\u4f4e": {},
	"\u5b8c\u6210": {}, "\u5931\u8d25": {}, "\u8df3\u8fc7": {}, "\u590d\u7528": {},
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
			case "script", "iframe", "frame", "frameset", "object", "embed", "form", "input", "button", "link", "img", "picture", "source", "video", "audio", "track", "svg", "math", "base", "template", "details", "dialog":
				return fmt.Errorf("HTML document contains disallowed <%s> content", tag)
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
				if nodeText(node) != reportHTMLStyles {
					return fmt.Errorf("HTML document contains a stylesheet outside the report template")
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
		if section == nil || !hasAncestor(section, mainNode) || strings.TrimSpace(nodeText(section)) == "" {
			return fmt.Errorf("HTML document is missing required section %q", name)
		}
		if err := validateSectionHeading(section, name); err != nil {
			return err
		}
	}
	if material.Sections.StructuralChecks.Status != StatusProvided {
		status := displayHTMLFact("sections.structural_checks.status", string(material.Sections.StructuralChecks.Status))
		if !strings.Contains(nodeText(sections["governance"]), status) {
			return fmt.Errorf("governance section must show the structural check status")
		}
	}
	for _, limitation := range material.Limitations {
		if !strings.Contains(nodeText(sections["limitations"]), limitation.Reason) {
			return fmt.Errorf("limitations section omits a recorded limitation")
		}
	}
	if !strings.Contains(nodeText(sections["sources"]), material.Review.RunID) || !strings.Contains(nodeText(sections["sources"]), material.SchemaVersion) {
		return fmt.Errorf("sources section must include the report schema version and run id")
	}
	if err := validateHTMLFindings(findings, material); err != nil {
		return err
	}
	if err := validateHTMLStats(stats, material); err != nil {
		return err
	}
	return validateHTMLClaims(bodyNode, material)
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

func validateHTMLFindings(nodes map[string]*html.Node, material Material) error {
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
		actualFacts := findAllDataFacts(node)
		if len(actualFacts) != len(expectedFacts) {
			return fmt.Errorf("HTML finding %q has an incomplete or extra fact set", finding.ID)
		}
		for name, matches := range actualFacts {
			expected, exists := expectedFacts[name]
			if !exists || len(matches) != 1 || normalizedHTMLText(nodeText(matches[0])) != normalizedHTMLText(expected) {
				return fmt.Errorf("HTML finding %q omits, adds, or changes fact %q", finding.ID, name)
			}
			if err := validateFindingFactPresentation(matches[0], name); err != nil {
				return fmt.Errorf("HTML finding %q: %w", finding.ID, err)
			}
		}
	}
	return nil
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

func validateFindingFactPresentation(fact *html.Node, name string) error {
	label, ok := findingHTMLLabels[name]
	if !ok {
		return fmt.Errorf("finding fact %q has no fixed label", name)
	}
	for parent := fact.Parent; parent != nil; parent = parent.Parent {
		if parent.Type != html.ElementNode || !hasClass(parent, "fact-row") {
			continue
		}
		labelCount := 0
		for child := parent.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && hasClass(child, "fact-label") {
				labelCount++
				if strings.TrimSpace(nodeText(child)) != label {
					return fmt.Errorf("finding fact %q has an incorrect fixed label", name)
				}
			}
		}
		if labelCount == 1 {
			return nil
		}
		return fmt.Errorf("finding fact %q must have exactly one fixed label", name)
	}
	return fmt.Errorf("finding fact %q must be presented in a labeled fact row", name)
}

var findingHTMLLabels = map[string]string{
	"path": "\u6587\u4ef6", "start_line": "\u884c\u53f7", "end_line": "\u884c\u53f7",
	"summary_zh": "\u6458\u8981", "severity_zh": "\u4e25\u91cd\u7b49\u7ea7", "category_zh": "\u7c7b\u522b",
	"source_content": "\u6e90\u4ee3\u7801", "evidence_status": "\u72b6\u6001", "recommendation_status": "\u72b6\u6001",
	"evidence_code": "\u8bc1\u636e", "evidence_reason": "\u539f\u56e0",
	"recommendation_code": "\u5efa\u8bae", "recommendation_reason": "\u539f\u56e0",
}

func hasClass(node *html.Node, class string) bool {
	for _, value := range strings.Fields(attribute(node, "class")) {
		if value == class {
			return true
		}
	}
	return false
}

func validateHTMLStats(stats map[string]*html.Node, material Material) error {
	expected := map[string]int{
		"finding-count":     len(material.Findings),
		"coverage-selected": len(material.Coverage.Selected), "coverage-completed": len(material.Coverage.Completed),
		"coverage-failed": len(material.Coverage.Failed), "coverage-skipped": len(material.Coverage.Waived),
		"coverage-reused": len(material.Coverage.Reused),
	}
	for _, severity := range []string{"critical", "high", "medium", "low"} {
		expected["risk-"+severity] = 0
	}
	for _, finding := range material.Findings {
		expected["risk-"+finding.Severity]++
	}
	if len(stats) != len(expected) {
		return fmt.Errorf("HTML report contains %d statistics; expected %d", len(stats), len(expected))
	}
	for name, count := range expected {
		node, ok := stats[name]
		if !ok || strings.TrimSpace(nodeText(node)) != strconv.Itoa(count) || !insideMain(node) {
			return fmt.Errorf("HTML statistic %q does not match the report material", name)
		}
	}
	return nil
}

func validateSectionHeading(section *html.Node, name string) error {
	wanted := requiredHTMLHeadings[name]
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
	if heading == nil || strings.TrimSpace(nodeText(heading)) != wanted {
		return fmt.Errorf("HTML section %q must use its required Chinese heading", name)
	}
	return nil
}

func validateHTMLClaims(bodyNode *html.Node, material Material) error {
	facts, err := materialHTMLFacts(material)
	if err != nil {
		return err
	}
	seen := make(map[string]map[string]int)
	var visit func(*html.Node) error
	visit = func(node *html.Node) error {
		if node.Type == html.ElementNode {
			if !insideMain(node) && node != bodyNode && node.Data != "main" {
				if strings.TrimSpace(nodeText(node)) != "" || attribute(node, "data-fact") != "" {
					return fmt.Errorf("HTML body contains report content outside main")
				}
			}
			if factName := attribute(node, "data-fact"); factName != "" {
				if insideFinding(node) {
					return nil
				}
				section := enclosingSection(node)
				expected, exists := facts[factName]
				if !exists || !htmlFactAllowedInSection(section, factName) {
					return fmt.Errorf("HTML contains an unsupported fact %q in section %q", factName, section)
				}
				if hasNestedFact(node) {
					return fmt.Errorf("HTML fact %q cannot contain another data-fact", factName)
				}
				actual := normalizedHTMLText(nodeText(node))
				if actual != expected && actual != displayHTMLFact(factName, expected) {
					return fmt.Errorf("HTML fact %q does not match the report material", factName)
				}
				if seen[section] == nil {
					seen[section] = make(map[string]int)
				}
				seen[section][factName]++
				if seen[section][factName] > 1 {
					return fmt.Errorf("HTML section %q duplicates report fact %q", section, factName)
				}
				return nil
			}
			if attribute(node, "data-stat") != "" {
				if hasNestedFact(node) {
					return fmt.Errorf("HTML statistic cannot contain a data-fact")
				}
				return nil
			}
			if isHeading(node.Data) {
				if !isRequiredHTMLHeading(strings.TrimSpace(nodeText(node))) {
					return fmt.Errorf("HTML contains an unsupported heading")
				}
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
				if _, ok := allowedHTMLLabels[value]; !ok {
					return fmt.Errorf("HTML contains unverified narrative text %q", value)
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
	for section, prefixes := range requiredHTMLFactPrefixes {
		for name := range facts {
			if hasAnyPrefix(name, prefixes) && seen[section][name] == 0 {
				return fmt.Errorf("HTML section %q omits report fact %q", section, name)
			}
		}
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

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
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

func isRequiredHTMLHeading(value string) bool {
	for _, heading := range requiredHTMLHeadings {
		if value == heading {
			return true
		}
	}
	return false
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
		if strings.Contains(value, "javascript:") || strings.Contains(value, "data:") || strings.Contains(value, "http:") || strings.Contains(value, "https:") {
			return fmt.Errorf("HTML document contains an external or unsafe URL")
		}
	}
	return nil
}

func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
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
