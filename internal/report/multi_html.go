// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

func ValidateMultiHTMLDocument(document string, input MultiReportInput) error {
	if err := validateMultiReportInput(input); err != nil {
		return err
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
	var mainNode *html.Node
	sections := make(map[string]*html.Node)
	unitGroupsBySection := make(map[string][]string, len(requiredHTMLSections))
	owners := make(map[string]struct{}, len(input.ReviewUnits))
	unitIndexes := make(map[string]int, len(input.ReviewUnits))
	for _, unit := range input.ReviewUnits {
		owners[unit.ID] = struct{}{}
	}
	for index, unit := range input.ReviewUnits {
		unitIndexes[unit.ID] = index
	}
	seenFacts := make(map[string]bool)
	seenFindings := make(map[struct{ unitID, findingID string }]bool)
	seenStats := make(map[struct{ unitID, name string }]bool)
	lastUnitBySection := make(map[string]int)
	for _, section := range requiredHTMLSections {
		lastUnitBySection[section] = -1
	}
	recordUnitOrder := func(section string, index int) error {
		if index < lastUnitBySection[section] {
			return fmt.Errorf("HTML section %q does not preserve report input order", section)
		}
		lastUnitBySection[section] = index
		return nil
	}
	expectedFacts, err := multiReportHTMLFacts(input)
	if err != nil {
		return err
	}
	styleCount := 0
	var visit func(*html.Node) error
	visit = func(node *html.Node) error {
		if node.Type == html.CommentNode {
			return fmt.Errorf("HTML report cannot contain hidden comments")
		}
		if node.Type == html.TextNode && localAbsolutePath.MatchString(node.Data) {
			return fmt.Errorf("HTML document contains a local absolute path")
		}
		if node.Type != html.ElementNode {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if err := visit(child); err != nil {
					return err
				}
			}
			return nil
		}
		tag := strings.ToLower(node.Data)
		if _, allowed := allowedHTMLTags[tag]; !allowed {
			return fmt.Errorf("HTML document contains unsupported <%s> content", tag)
		}
		if err := validateAttributes(node); err != nil {
			return err
		}
		switch tag {
		case "html":
			if !strings.EqualFold(attribute(node, "lang"), "zh-CN") {
				return fmt.Errorf("HTML document must declare lang=zh-CN")
			}
		case "meta":
			if strings.EqualFold(attribute(node, "http-equiv"), "refresh") {
				return fmt.Errorf("HTML document contains a refresh directive")
			}
		case "style":
			styleCount++
			materials := make([]Material, len(input.ReviewUnits))
			for index, unit := range input.ReviewUnits {
				materials[index] = unit.Material
			}
			if nodeText(node) != reportHTMLStyles && nodeText(node) != reportHTMLStylesFor(materials) {
				return fmt.Errorf("HTML document contains a stylesheet outside the report template")
			}
		case "main":
			if mainNode != nil {
				return fmt.Errorf("HTML document must contain one main element")
			}
			mainNode = node
			if attribute(node, "data-report-kind") != "multi-unit" {
				return fmt.Errorf("main element must identify a multi-unit report")
			}
			if attribute(node, "data-review-unit-count") != strconv.Itoa(len(input.ReviewUnits)) {
				return fmt.Errorf("main review-unit count does not match the report input")
			}
		case "title":
			if strings.TrimSpace(nodeText(node)) == "" {
				return fmt.Errorf("HTML document requires a title")
			}
		}
		if name := attribute(node, "data-review-unit-id"); name != "" {
			if _, ok := owners[name]; !ok {
				return fmt.Errorf("HTML document refers to unknown review unit %q", name)
			}
		}
		if name := attribute(node, "data-section"); name != "" {
			if sections[name] != nil {
				return fmt.Errorf("HTML document duplicates required section %q", name)
			}
			sections[name] = node
		}
		if unitID := attribute(node, "data-review-unit-id"); unitID != "" && node.Parent != nil {
			section := attribute(node.Parent, "data-section")
			if section != "" {
				if tag != "div" {
					return fmt.Errorf("review-unit group in section %q must use a div element", section)
				}
				unitGroupsBySection[section] = append(unitGroupsBySection[section], unitID)
				if err := recordUnitOrder(section, unitIndexes[unitID]); err != nil {
					return err
				}
				if section == "overview" {
					if err := validateReviewUnitHeading(node, unitIndexes[unitID]); err != nil {
						return err
					}
				}
			}
		}
		if findingID := attribute(node, "data-finding-id"); findingID != "" {
			if tag != "article" {
				return fmt.Errorf("HTML finding %q must use an article element", findingID)
			}
			unitID := attribute(node, "data-review-unit-id")
			if unitID == "" {
				return fmt.Errorf("HTML finding %q has no review-unit ownership", findingID)
			}
			if enclosingSection(node) != "finding-details" {
				return fmt.Errorf("HTML finding %q must appear in the finding-details section", findingID)
			}
			if err := recordUnitOrder(enclosingSection(node), unitIndexes[unitID]); err != nil {
				return err
			}
			key := struct{ unitID, findingID string }{unitID, findingID}
			if seenFindings[key] {
				return fmt.Errorf("HTML document duplicates finding %q in review unit %q", findingID, unitID)
			}
			seenFindings[key] = true
		}
		if name := attribute(node, "data-stat"); name != "" {
			if enclosingSection(node) != "quality-coverage" {
				return fmt.Errorf("HTML statistic %q must appear in quality-coverage", name)
			}
			unitID := nearestReviewUnitID(node)
			if unitID == "" {
				return fmt.Errorf("HTML statistic %q has no review-unit ownership", name)
			}
			key := struct{ unitID, name string }{unitID, name}
			if seenStats[key] {
				return fmt.Errorf("HTML document duplicates statistic %q in review unit %q", name, unitID)
			}
			seenStats[key] = true
			if err := recordUnitOrder(enclosingSection(node), unitIndexes[unitID]); err != nil {
				return err
			}
		}
		if name := attribute(node, "data-fact"); name != "" && !insideFinding(node) {
			if hasNestedFact(node) {
				return fmt.Errorf("HTML fact %q cannot contain another data-fact", name)
			}
			expected, exists := expectedFacts[name]
			if !exists {
				return fmt.Errorf("HTML contains an unsupported fact %q", name)
			}
			section := enclosingSection(node)
			if !multiHTMLFactAllowedInSection(section, name) {
				return fmt.Errorf("HTML contains unsupported fact %q in section %q", name, section)
			}
			if name == "summary.findings_by_severity.not_collected" {
				if err := validateFixedHTMLFactLabel(node, "\u672a\u63d0\u4f9b\u7b49\u7ea7\u6570\u91cf"); err != nil {
					return fmt.Errorf("HTML fact %q: %w", name, err)
				}
			}
			if unitIndex, materialFact := multiMaterialFactIndex(name); materialFact {
				wantUnit := input.ReviewUnits[unitIndex].ID
				if nearestReviewUnitID(node) != wantUnit {
					return fmt.Errorf("HTML material fact %q has incorrect review-unit ownership", name)
				}
				if err := recordUnitOrder(section, unitIndex); err != nil {
					return err
				}
			} else if nearestReviewUnitID(node) != "" {
				return fmt.Errorf("HTML aggregate fact %q cannot be assigned to one review unit", name)
			}
			actual := normalizedHTMLText(nodeText(node))
			if actual != expected && actual != displayHTMLFact(name, expected) {
				return fmt.Errorf("HTML fact %q does not match the report input", name)
			}
			factKey := section + "\x00" + name
			if seenFacts[factKey] {
				return fmt.Errorf("HTML document duplicates report fact %q", name)
			}
			seenFacts[factKey] = true
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
	if mainNode == nil || styleCount != 1 || !containsHan(nodeText(mainNode)) {
		return fmt.Errorf("HTML document requires one multi-unit main, one fixed stylesheet, and Chinese report text")
	}
	for _, name := range requiredHTMLSections {
		section := sections[name]
		if section == nil || !hasAncestor(section, mainNode) || strings.TrimSpace(nodeText(section)) == "" {
			return fmt.Errorf("HTML document is missing required section %q", name)
		}
		if err := validateSectionHeading(section, name); err != nil {
			return err
		}
		groups := unitGroupsBySection[name]
		if len(groups) != len(input.ReviewUnits) {
			return fmt.Errorf("HTML section %q must contain one direct group per review unit", name)
		}
		for index, unit := range input.ReviewUnits {
			if groups[index] != unit.ID {
				return fmt.Errorf("HTML section %q does not preserve report input order", name)
			}
		}
	}
	for name := range expectedFacts {
		if _, materialFact := multiMaterialFactIndex(name); materialFact {
			continue
		}
		if !seenFacts[multiHTMLFactSection(name)+"\x00"+name] {
			return fmt.Errorf("HTML document omits report fact %q", name)
		}
	}
	for index, unit := range input.ReviewUnits {
		projection, err := projectMultiHTMLUnit(document, unit, index)
		if err != nil {
			return err
		}
		if err := ValidateHTMLDocument(projection, unit.Material); err != nil {
			return fmt.Errorf("validate HTML for review unit %q: %w", unit.ID, err)
		}
	}
	return nil
}

func validateMultiReportInput(input MultiReportInput) error {
	materials := make([]Material, len(input.ReviewUnits))
	for index, unit := range input.ReviewUnits {
		materials[index] = unit.Material
	}
	expected, err := NewMultiReportInput(materials)
	if err != nil {
		return err
	}
	actualJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode multi-report input: %w", err)
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return fmt.Errorf("encode expected multi-report input: %w", err)
	}
	if !bytes.Equal(actualJSON, expectedJSON) {
		return fmt.Errorf("multi-report summary or people facts do not match the source materials")
	}
	return nil
}

func multiReportHTMLFacts(input MultiReportInput) (map[string]string, error) {
	facts := make(map[string]string)
	for index, unit := range input.ReviewUnits {
		materialFacts, err := materialHTMLFacts(unit.Material)
		if err != nil {
			return nil, err
		}
		for path, value := range materialFacts {
			facts[fmt.Sprintf("review_units[%d].material.%s", index, path)] = value
		}
	}
	for prefix, value := range map[string]any{"summary": input.Summary, "people": input.People} {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode multi-report %s facts: %w", prefix, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var root any
		if err := decoder.Decode(&root); err != nil {
			return nil, fmt.Errorf("decode multi-report %s facts: %w", prefix, err)
		}
		var collect func(string, any)
		collect = func(path string, current any) {
			switch item := current.(type) {
			case map[string]any:
				for key, child := range item {
					collect(path+"."+key, child)
				}
			case []any:
				for itemIndex, child := range item {
					collect(fmt.Sprintf("%s[%d]", path, itemIndex), child)
				}
			case string:
				facts[path] = item
			case json.Number:
				facts[path] = item.String()
			case bool:
				facts[path] = strconv.FormatBool(item)
			}
		}
		collect(prefix, root)
	}
	return facts, nil
}

func multiMaterialFactIndex(name string) (int, bool) {
	if !strings.HasPrefix(name, "review_units[") {
		return 0, false
	}
	close := strings.Index(name, "].material.")
	if close < 0 {
		return 0, false
	}
	index, err := strconv.Atoi(name[len("review_units["):close])
	return index, err == nil
}

func multiHTMLFactAllowedInSection(section, name string) bool {
	if strings.HasPrefix(name, "review_units[") {
		parts := strings.SplitN(name, ".material.", 2)
		if len(parts) != 2 {
			return false
		}
		return htmlFactAllowedInSection(section, parts[1])
	}
	if strings.HasPrefix(name, "summary.findings_by_severity.") {
		return section == "quality-coverage"
	}
	if strings.HasPrefix(name, "summary.") {
		return section == "overview"
	}
	return strings.HasPrefix(name, "people[") && section == "people"
}

func multiHTMLFactSection(name string) string {
	if strings.HasPrefix(name, "summary.findings_by_severity.") {
		return "quality-coverage"
	}
	if strings.HasPrefix(name, "summary.") {
		return "overview"
	}
	if strings.HasPrefix(name, "people[") {
		return "people"
	}
	return ""
}

func nearestReviewUnitID(node *html.Node) string {
	for current := node; current != nil; current = current.Parent {
		if value := attribute(current, "data-review-unit-id"); value != "" {
			return value
		}
	}
	return ""
}

func validateReviewUnitHeading(group *html.Node, unitIndex int) error {
	var firstFact *html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		if firstFact != nil {
			return
		}
		if node.Type == html.ElementNode && attribute(node, "data-fact") != "" && !insideFinding(node) {
			firstFact = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(group)
	want := fmt.Sprintf("review_units[%d].material.review.run_id", unitIndex)
	if firstFact == nil || attribute(firstFact, "data-fact") != want {
		return fmt.Errorf("overview review-unit groups must begin with their run_id fact")
	}
	for row := firstFact.Parent; row != nil && row != group; row = row.Parent {
		if row.Type != html.ElementNode || !hasClass(row, "fact-row") {
			continue
		}
		labelCount := 0
		for child := row.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && hasClass(child, "fact-label") {
				labelCount++
				if strings.TrimSpace(nodeText(child)) != "\u8fd0\u884c\u6807\u8bc6" {
					return fmt.Errorf("overview run_id fact must use the fixed run-id label")
				}
			}
		}
		if labelCount != 1 {
			return fmt.Errorf("overview run_id fact must have one fixed label")
		}
		return nil
	}
	return fmt.Errorf("overview run_id fact must appear in a labeled fact row")
}

func validateFixedHTMLFactLabel(fact *html.Node, label string) error {
	for parent := fact.Parent; parent != nil; parent = parent.Parent {
		if parent.Type != html.ElementNode || !hasClass(parent, "fact-row") {
			continue
		}
		labelCount := 0
		for child := parent.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && hasClass(child, "fact-label") {
				labelCount++
				if strings.TrimSpace(nodeText(child)) != label {
					return fmt.Errorf("fact must use its fixed label")
				}
			}
		}
		if labelCount == 1 {
			return nil
		}
		return fmt.Errorf("fact must have exactly one fixed label")
	}
	return fmt.Errorf("fact must appear in a labeled fact row")
}

func projectMultiHTMLUnit(document string, unit ReviewUnit, unitIndex int) (string, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return "", fmt.Errorf("parse multi-unit HTML projection: %w", err)
	}
	var mainNode *html.Node
	var findMain func(*html.Node)
	findMain = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "main") {
			mainNode = node
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			findMain(child)
		}
	}
	findMain(root)
	if mainNode == nil {
		return "", fmt.Errorf("multi-unit HTML has no main element")
	}
	var filter func(*html.Node, string) (bool, error)
	filter = func(node *html.Node, owner string) (bool, error) {
		if node.Type == html.ElementNode {
			if strings.EqualFold(node.Data, "style") {
				for child := node.FirstChild; child != nil; {
					next := child.NextSibling
					node.RemoveChild(child)
					child = next
				}
				node.AppendChild(&html.Node{Type: html.TextNode, Data: reportHTMLStylesFor([]Material{unit.Material})})
			}
			if nodeOwner := attribute(node, "data-review-unit-id"); nodeOwner != "" {
				owner = nodeOwner
				if owner != unit.ID {
					return false, nil
				}
				removeAttribute(node, "data-review-unit-id")
			}
			if findingID := attribute(node, "data-finding-id"); findingID != "" {
				if owner != unit.ID {
					return false, nil
				}
			}
			if factName := attribute(node, "data-fact"); factName != "" && !insideFinding(node) {
				prefix := fmt.Sprintf("review_units[%d].material.", unitIndex)
				if strings.HasPrefix(factName, prefix) {
					setAttribute(node, "data-fact", strings.TrimPrefix(factName, prefix))
				} else {
					return false, nil
				}
			}
			if attribute(node, "data-stat") != "" && owner != unit.ID {
				return false, nil
			}
			if strings.EqualFold(node.Data, "main") {
				removeAttribute(node, "data-report-kind")
				removeAttribute(node, "data-review-unit-count")
				setAttribute(node, "data-review-status", string(unit.Material.Review.Status))
				setAttribute(node, "data-run-id", unit.Material.Review.RunID)
			}
		}
		for child := node.FirstChild; child != nil; {
			next := child.NextSibling
			keep, err := filter(child, owner)
			if err != nil {
				return false, err
			}
			if !keep {
				node.RemoveChild(child)
			}
			child = next
		}
		return true, nil
	}
	if _, err := filter(root, ""); err != nil {
		return "", err
	}
	var builder strings.Builder
	if err := html.Render(&builder, root); err != nil {
		return "", fmt.Errorf("render review-unit HTML projection: %w", err)
	}
	return builder.String(), nil
}

func setAttribute(node *html.Node, name, value string) {
	for index := range node.Attr {
		if strings.EqualFold(node.Attr[index].Key, name) {
			node.Attr[index].Val = value
			return
		}
	}
	node.Attr = append(node.Attr, html.Attribute{Key: name, Val: value})
}

func removeAttribute(node *html.Node, name string) {
	attributes := node.Attr[:0]
	for _, attr := range node.Attr {
		if !strings.EqualFold(attr.Key, name) {
			attributes = append(attributes, attr)
		}
	}
	node.Attr = attributes
}
