// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

func addReportExperience(document string, materials []Material, unitIDs []string) (string, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return "", fmt.Errorf("parse report HTML: %w", err)
	}
	var mainNode, styleNode *html.Node
	var articles []*html.Node
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "main":
				mainNode = node
			case "style":
				styleNode = node
			case "article":
				if attribute(node, "data-finding-id") != "" {
					articles = append(articles, node)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	if mainNode == nil || styleNode == nil {
		return "", fmt.Errorf("report HTML is missing main or stylesheet")
	}
	unitIndexes := make(map[string]int, len(unitIDs))
	for index, unitID := range unitIDs {
		unitIndexes[unitID] = index
	}
	categories := reportFilterCategories(materials)
	categoryIndexes := make(map[string]int, len(categories))
	for _, category := range categories {
		categoryIndexes[category.name] = category.index
	}
	for _, article := range articles {
		unitIndex := 0
		if len(unitIDs) > 0 {
			owner := nearestReviewUnitID(article)
			var ok bool
			unitIndex, ok = unitIndexes[owner]
			if !ok {
				return "", fmt.Errorf("finding has unknown review-unit ownership")
			}
		}
		setAttribute(article, "data-review-unit-index", fmt.Sprint(unitIndex))
		severityIndex := reportSeverityIndex(attribute(article, "data-severity"))
		setAttribute(article, "data-review-severity-index", fmt.Sprint(severityIndex))
		categoryIndex, ok := categoryIndexes[attribute(article, "data-category")]
		if !ok {
			return "", fmt.Errorf("finding has unknown category")
		}
		setAttribute(article, "data-review-category-index", fmt.Sprint(categoryIndex))
		if err := addFindingDisclosure(article); err != nil {
			return "", err
		}
	}
	filters := buildReportFilters(materials, unitIDs, categories)
	mainNode.InsertBefore(filters, mainNode.FirstChild)
	for child := styleNode.FirstChild; child != nil; {
		next := child.NextSibling
		styleNode.RemoveChild(child)
		child = next
	}
	styleNode.AppendChild(&html.Node{Type: html.TextNode, Data: reportHTMLStylesFor(materials)})
	var builder strings.Builder
	if err := html.Render(&builder, root); err != nil {
		return "", fmt.Errorf("render report controls: %w", err)
	}
	return builder.String(), nil
}

func addFindingDisclosure(article *html.Node) error {
	var summaryRow *html.Node
	for child := article.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && hasClass(child, "fact-row") && containsFindingFact(child, "summary_zh") {
			summaryRow = child
			break
		}
	}
	if summaryRow == nil {
		return fmt.Errorf("finding is missing its summary fact row")
	}
	details := &html.Node{Type: html.ElementNode, Data: "details", Attr: []html.Attribute{
		{Key: "class", Val: "finding-details"}, {Key: "open", Val: ""},
	}}
	summary := &html.Node{Type: html.ElementNode, Data: "summary"}
	article.RemoveChild(summaryRow)
	summary.AppendChild(summaryRow)
	content := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "class", Val: "finding-detail-content"}}}
	for child := article.FirstChild; child != nil; {
		next := child.NextSibling
		article.RemoveChild(child)
		content.AppendChild(child)
		child = next
	}
	details.AppendChild(summary)
	details.AppendChild(content)
	article.AppendChild(details)
	return nil
}

func containsFindingFact(node *html.Node, name string) bool {
	if node.Type == html.ElementNode && attribute(node, "data-fact") == name {
		return true
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if containsFindingFact(child, name) {
			return true
		}
	}
	return false
}

func buildReportFilters(materials []Material, unitIDs []string, categories []reportFilterCategory) *html.Node {
	filters := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{
		{Key: "class", Val: "report-filters"}, {Key: "data-report-ui", Val: "filters"},
	}}
	unitField := reportFilterFieldset("\u5ba1\u67e5\u5355\u5143\u6216\u4ed3\u5e93")
	appendFilterOption(unitField, "report-filter-unit", "report-filter-unit-all", "all", "\u5168\u90e8", true)
	for index, material := range materials {
		label := material.Repository.Name
		if label == "" {
			label = fmt.Sprintf("\u5ba1\u67e5\u5355\u5143 %d", index+1)
		}
		if len(unitIDs) > 1 && material.Review.RunID != "" {
			label += " / " + material.Review.RunID
		}
		appendFilterOption(unitField, "report-filter-unit", fmt.Sprintf("report-filter-unit-option-%d", index), fmt.Sprint(index), label, false)
	}
	filters.AppendChild(unitField)

	severityField := reportFilterFieldset("\u4e25\u91cd\u7b49\u7ea7")
	appendFilterOption(severityField, "report-filter-severity", "report-filter-severity-all", "all", "\u5168\u90e8", true)
	for index, item := range []struct{ value, label string }{
		{"critical", "\u4e25\u91cd"}, {"high", "\u9ad8"}, {"medium", "\u4e2d"}, {"low", "\u4f4e"}, {"not_collected", "\u672a\u63d0\u4f9b"},
	} {
		appendFilterOption(severityField, "report-filter-severity", fmt.Sprintf("report-filter-severity-option-%d", index), fmt.Sprint(index), item.label, false)
	}
	filters.AppendChild(severityField)

	categoryField := reportFilterFieldset("\u7c7b\u522b")
	appendFilterOption(categoryField, "report-filter-category", "report-filter-category-all", "all", "\u5168\u90e8", true)
	for _, category := range categories {
		appendFilterOption(categoryField, "report-filter-category", fmt.Sprintf("report-filter-category-option-%d", category.index), fmt.Sprint(category.index), category.label, false)
	}
	filters.AppendChild(categoryField)
	return filters
}

func reportFilterFieldset(legend string) *html.Node {
	fieldset := &html.Node{Type: html.ElementNode, Data: "fieldset"}
	legendNode := &html.Node{Type: html.ElementNode, Data: "legend"}
	legendNode.AppendChild(&html.Node{Type: html.TextNode, Data: legend})
	fieldset.AppendChild(legendNode)
	return fieldset
}

func appendFilterOption(fieldset *html.Node, name, id, value, label string, checked bool) {
	labelNode := &html.Node{Type: html.ElementNode, Data: "label"}
	input := &html.Node{Type: html.ElementNode, Data: "input", Attr: []html.Attribute{
		{Key: "type", Val: "radio"}, {Key: "name", Val: name}, {Key: "id", Val: id}, {Key: "value", Val: value},
	}}
	if checked {
		input.Attr = append(input.Attr, html.Attribute{Key: "checked", Val: ""})
	}
	labelNode.AppendChild(input)
	labelNode.AppendChild(&html.Node{Type: html.TextNode, Data: label})
	fieldset.AppendChild(labelNode)
}

func reportSeverityIndex(severity string) int {
	switch severity {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}
