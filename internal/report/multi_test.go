// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"encoding/json"
	"fmt"
	"html"
	"slices"
	"sort"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

const (
	multiTestReportTitle              = "\u5ba1\u67e5\u62a5\u544a"
	multiTestRunIDLabel               = "\u8fd0\u884c\u6807\u8bc6"
	multiTestUncollectedSeverityLabel = "\u672a\u63d0\u4f9b\u7b49\u7ea7\u6570\u91cf"
)

func TestNewMultiReportInputPreservesUnitsAndFindingRecords(t *testing.T) {
	first := validHTMLMaterial()
	first.Review.RunID = "run-frontend"
	first.Repository.Name = "frontend"
	second := first
	second.Review.RunID = "run-backend"
	second.Repository.Name = "backend"
	third := second
	third.Review.RunID = "run-worker"
	third.Repository.Name = "worker"

	input, err := NewMultiReportInput([]Material{first, second, third})
	if err != nil {
		t.Fatalf("NewMultiReportInput() error = %v", err)
	}
	if len(input.ReviewUnits) != 3 {
		t.Fatalf("unit count = %d, want 3", len(input.ReviewUnits))
	}
	for index, want := range []string{"frontend", "backend", "worker"} {
		if got := input.ReviewUnits[index].Material.Repository.Name; got != want {
			t.Errorf("unit %d repository = %q, want %q", index, got, want)
		}
	}
	if input.ReviewUnits[0].Material.Findings[0].ID != input.ReviewUnits[1].Material.Findings[0].ID {
		t.Fatal("the fixture must exercise colliding source finding IDs")
	}
	if input.Summary.ReviewUnitCount != 3 || input.Summary.FindingRecordCount != 12 {
		t.Fatalf("summary = %+v, want 3 units and 12 unit-scoped finding records", input.Summary)
	}
	if input.ReviewUnits[0].ID == "" || input.ReviewUnits[0].ID == input.ReviewUnits[1].ID || input.ReviewUnits[0].ID == input.ReviewUnits[2].ID {
		t.Fatalf("unit IDs are empty or collide: %q, %q, %q", input.ReviewUnits[0].ID, input.ReviewUnits[1].ID, input.ReviewUnits[2].ID)
	}
	if input.ReviewUnits[0].ID != mustMultiReportID(t, []Material{third, first})[1] {
		t.Fatal("review-unit identity changed when input order changed")
	}
}

func TestNewMultiReportInputRejectsRepeatedRunIDAcrossInputPaths(t *testing.T) {
	material := validHTMLMaterial()
	material.Review.RunID = "same-review-run"

	_, err := NewMultiReportInput([]Material{material, material})
	if err == nil || !strings.Contains(err.Error(), "run_id") {
		t.Fatalf("NewMultiReportInput() error = %v, want duplicate run_id error", err)
	}
}

func TestNewMultiReportInputMergesOnlyExactEmailIdentity(t *testing.T) {
	first := validHTMLMaterial()
	first.Review.RunID = "person-run-one"
	first = withPeople(first, "Alice Chen", " Alice@Example.com ", "Sam Lee", "")
	second := validHTMLMaterial()
	second.Review.RunID = "person-run-two"
	second = withPeople(second, "A. Chen", "Alice@example.com", "Rin Park", "rin@example.com")
	third := validHTMLMaterial()
	third.Review.RunID = "person-run-three"
	third = withPeople(third, "Sam Lee", "", "Rin Park", "other@example.com")

	input, err := NewMultiReportInput([]Material{first, second, third})
	if err != nil {
		t.Fatalf("NewMultiReportInput() error = %v", err)
	}
	var exactEmail *MultiReportPerson
	var sameNameWithoutIdentity int
	var sameNameDifferentEmail int
	for index := range input.People {
		person := &input.People[index]
		switch {
		case person.IdentityStatus == "email_verified" && slices.Contains(person.NameVariants, "Alice Chen"):
			exactEmail = person
		case person.DisplayName == "Sam Lee" && person.IdentityStatus == "unit_scoped":
			sameNameWithoutIdentity++
		case person.DisplayName == "Rin Park" && person.IdentityStatus == "email_verified":
			sameNameDifferentEmail++
		}
	}
	if exactEmail == nil || len(exactEmail.NameVariants) != 2 || len(exactEmail.Contributions) != 2 {
		t.Fatalf("same-email identities/aliases were not consolidated: %+v", exactEmail)
	}
	if sameNameWithoutIdentity != 2 {
		t.Fatalf("same-name entries without email were merged: got %d groups, want 2", sameNameWithoutIdentity)
	}
	if sameNameDifferentEmail != 2 {
		t.Fatalf("same-name entries with different emails were merged: got %d groups, want 2", sameNameDifferentEmail)
	}
}

func TestNewMultiReportInputKeepsEmailLocalPartCaseDistinct(t *testing.T) {
	first := validHTMLMaterial()
	first.Review.RunID = "person-case-one"
	first = withPeople(first, "Owner", "Owner@corp.example", "Committer", "")
	second := validHTMLMaterial()
	second.Review.RunID = "person-case-two"
	second = withPeople(second, "Owner", "owner@corp.example", "Committer", "")

	input, err := NewMultiReportInput([]Material{first, second})
	if err != nil {
		t.Fatalf("NewMultiReportInput() error = %v", err)
	}
	var owners []*MultiReportPerson
	for index := range input.People {
		person := &input.People[index]
		if person.IdentityStatus == "email_verified" && person.DisplayName == "Owner" {
			owners = append(owners, person)
		}
	}
	if len(owners) != 2 || owners[0].ID == owners[1].ID {
		t.Fatalf("distinct local-part case was merged: %+v", owners)
	}
}

func TestNewMultiReportInputCountsMissingSeverityExplicitly(t *testing.T) {
	first := validHTMLMaterial()
	first.Review.RunID = "severity-unknown"
	first.Findings[0].Severity = ""
	first.Findings[0].SeverityStatus = StatusNotCollected
	first.Findings[0].SeverityReason = "native result omitted severity"
	first.Findings[0].Display.SeverityZH = "\u672a\u63d0\u4f9b"
	second := validHTMLMaterial()
	second.Review.RunID = "severity-known"

	input, err := NewMultiReportInput([]Material{first, second})
	if err != nil {
		t.Fatalf("NewMultiReportInput() error = %v", err)
	}
	if got := input.Summary.FindingsBySeverity["not_collected"]; got != 1 {
		t.Fatalf("not-collected severity count = %d, want 1", got)
	}
	if _, exists := input.Summary.FindingsBySeverity[""]; exists {
		t.Fatal("summary must not contain an empty severity key")
	}
	document := validMultiHTMLDocument(input)
	if err := ValidateMultiHTMLDocument(document, input); err != nil {
		t.Fatalf("multi-unit HTML with an explicitly uncollected severity was rejected: %v", err)
	}
	badLabel := strings.Replace(document, `class="fact-label">`+multiTestUncollectedSeverityLabel, `class="fact-label">incorrect`, 1)
	if badLabel == document || ValidateMultiHTMLDocument(badLabel, input) == nil {
		t.Fatal("multi-unit HTML accepted an incorrect uncollected-severity label")
	}
}

func TestValidateMultiHTMLDocumentRequiresReviewUnitOwnershipForFindingIDs(t *testing.T) {
	first := validHTMLMaterial()
	first.Review.RunID = "first-unit"
	second := first
	second.Review.RunID = "second-unit"
	second.Repository.Name = "another-repository"
	input, err := NewMultiReportInput([]Material{first, second})
	if err != nil {
		t.Fatalf("NewMultiReportInput() error = %v", err)
	}

	missingOwner := `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>` + multiTestReportTitle + `</title></head><body><main data-report-kind="multi-unit" data-review-unit-count="2"><article data-finding-id="` + first.Findings[0].ID + `"></article></main></body></html>`
	if err := ValidateMultiHTMLDocument(missingOwner, input); err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("ValidateMultiHTMLDocument() error = %v, want missing review-unit ownership", err)
	}

	document := validMultiHTMLDocument(input)
	if got := strings.Count(document, `data-finding-id="`+first.Findings[0].ID+`"`); got != 2 {
		t.Fatalf("HTML retained %d copies of the colliding finding, want one per review unit", got)
	}
	if err := ValidateMultiHTMLDocument(document, input); err != nil {
		t.Fatalf("ValidateMultiHTMLDocument() rejected colliding IDs owned by separate units: %v", err)
	}
	article := `<article data-review-unit-id="` + input.ReviewUnits[1].ID + `" data-finding-id="` + first.Findings[0].ID + `"`
	withoutOwner := strings.Replace(document, article, `<article data-finding-id="`+first.Findings[0].ID+`"`, 1)
	if withoutOwner == document {
		t.Fatal("fixture did not contain the second unit's colliding finding")
	}
	if err := ValidateMultiHTMLDocument(withoutOwner, input); err == nil {
		t.Fatal("ValidateMultiHTMLDocument accepted a colliding finding after its unit ownership was removed")
	}

	reordered := swapFirstUnitWrappers(t, document, "overview")
	if err := ValidateMultiHTMLDocument(reordered, input); err == nil || !strings.Contains(err.Error(), "input order") {
		t.Fatalf("ValidateMultiHTMLDocument() error = %v, want input-order rejection", err)
	}
}

func swapFirstUnitWrappers(t *testing.T, document, sectionName string) string {
	t.Helper()
	root, err := xhtml.Parse(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	var target *xhtml.Node
	var find func(*xhtml.Node)
	find = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && attribute(node, "data-section") == sectionName {
			target = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(root)
	if target == nil {
		t.Fatalf("fixture is missing section %q", sectionName)
	}
	var units []*xhtml.Node
	for child := target.FirstChild; child != nil; child = child.NextSibling {
		if attribute(child, "data-review-unit-id") != "" {
			units = append(units, child)
		}
	}
	if len(units) < 2 {
		t.Fatal("fixture needs at least two unit wrappers")
	}
	for _, unit := range units[:2] {
		target.RemoveChild(unit)
	}
	target.AppendChild(units[1])
	target.AppendChild(units[0])
	var builder strings.Builder
	if err := xhtml.Render(&builder, root); err != nil {
		t.Fatal(err)
	}
	return builder.String()
}

func validMultiHTMLDocument(input MultiReportInput) string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>` + multiTestReportTitle + `</title></head><body>`)
	fmt.Fprintf(&builder, `<main data-report-kind="multi-unit" data-review-unit-count="%d">`, len(input.ReviewUnits))
	facts, err := multiReportHTMLFacts(input)
	if err != nil {
		panic(err)
	}
	for _, section := range requiredHTMLSections {
		fmt.Fprintf(&builder, `<section data-section="%s"><h2>%s</h2>`, section, requiredHTMLHeadings[section])
		for index, unit := range input.ReviewUnits {
			materialFacts, err := materialHTMLFacts(unit.Material)
			if err != nil {
				panic(err)
			}
			fmt.Fprintf(&builder, `<div data-review-unit-id="%s">`, unit.ID)
			materialKeys := make([]string, 0, len(materialFacts))
			for name := range materialFacts {
				if htmlFactAllowedInSection(section, name) {
					materialKeys = append(materialKeys, name)
				}
			}
			sort.Strings(materialKeys)
			if section == "overview" {
				sort.SliceStable(materialKeys, func(left, right int) bool {
					return materialKeys[left] == "review.run_id" && materialKeys[right] != "review.run_id"
				})
			}
			for _, name := range materialKeys {
				factName := fmt.Sprintf("review_units[%d].material.%s", index, name)
				if section == "overview" && name == "review.run_id" {
					fmt.Fprintf(&builder, `<div class="fact-row"><strong class="fact-label">`+multiTestRunIDLabel+`</strong><span data-fact="%s">%s</span></div>`, html.EscapeString(factName), html.EscapeString(displayHTMLFact(name, materialFacts[name])))
				} else {
					fmt.Fprintf(&builder, `<span data-fact="%s">%s</span>`, html.EscapeString(factName), html.EscapeString(displayHTMLFact(name, materialFacts[name])))
				}
			}
			if section == "quality-coverage" {
				appendStatistics(&builder, unit.Material)
			}
			if section == "finding-details" {
				for _, finding := range unit.Material.Findings {
					article := findingHTML(finding)
					article = strings.Replace(article, `<article data-finding-id=`, `<article data-review-unit-id="`+unit.ID+`" data-finding-id=`, 1)
					builder.WriteString(article)
				}
			}
			builder.WriteString(`</div>`)
		}
		keys := make([]string, 0, len(facts))
		for name := range facts {
			if !strings.HasPrefix(name, "review_units[") && multiHTMLFactAllowedInSection(section, name) {
				keys = append(keys, name)
			}
		}
		sort.Strings(keys)
		for _, name := range keys {
			if name == "summary.findings_by_severity.not_collected" {
				fmt.Fprintf(&builder, `<div class="fact-row"><strong class="fact-label">%s</strong><span data-fact="%s">%s</span></div>`, multiTestUncollectedSeverityLabel, html.EscapeString(name), html.EscapeString(facts[name]))
			} else {
				fmt.Fprintf(&builder, `<span data-fact="%s">%s</span>`, html.EscapeString(name), html.EscapeString(facts[name]))
			}
		}
		builder.WriteString(`</section>`)
	}
	builder.WriteString(`</main></body></html>`)
	document, err := addReportHTMLStyles(builder.String())
	if err != nil {
		panic(err)
	}
	return document
}

func withPeople(material Material, authorName, authorEmail, committerName, committerEmail string) Material {
	data, err := json.Marshal(struct {
		CommitSHA string `json:"commit_sha"`
		Subject   string `json:"subject"`
		Author    struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		Committer struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"committer"`
		Basis string `json:"basis"`
	}{
		CommitSHA: "abc123",
		Subject:   "Change a package",
		Author: struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}{Name: authorName, Email: authorEmail},
		Committer: struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}{Name: committerName, Email: committerEmail},
		Basis: "Git commit metadata",
	})
	if err != nil {
		panic(err)
	}
	material.Sections.People = Section{Status: StatusProvided, Data: data}
	return material
}

func mustMultiReportID(t *testing.T, materials []Material) []string {
	t.Helper()
	input, err := NewMultiReportInput(materials)
	if err != nil {
		t.Fatalf("NewMultiReportInput() error = %v", err)
	}
	ids := make([]string, len(input.ReviewUnits))
	for index, unit := range input.ReviewUnits {
		ids[index] = unit.ID
	}
	return ids
}
