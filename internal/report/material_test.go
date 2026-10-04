// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/session"
)

func validMaterial() Material {
	now := time.Date(2026, 10, 4, 10, 11, 12, 0, time.FixedZone("HKT", 8*60*60))
	return Material{
		SchemaVersion: MaterialSchemaVersion,
		Review: ReviewMetadata{
			RunID:       "run-1",
			StartedAt:   now,
			CompletedAt: now.Add(time.Second),
			Status:      session.StateComplete,
		},
		Repository: Repository{
			Name:     "sample",
			Identity: Fact{Status: StatusProvided, Value: "sha256:" + strings.Repeat("a", 64)},
		},
		Scope: Scope{
			Mode:           session.InputModeCommit,
			RequestedHead:  "abc123",
			ResolvedHead:   Revision{Status: StatusProvided, SHA: strings.Repeat("a", 40)},
			ResolvedBase:   Revision{Status: StatusProvided, SHA: strings.Repeat("b", 40)},
			ExactRange:     strings.Repeat("b", 40) + ".." + strings.Repeat("a", 40),
			SourceArtifact: Fact{Status: StatusNotApplicable, Reason: "commit input has no workspace snapshot"},
		},
		Findings: []Finding{},
		Coverage: session.Coverage{
			Selected:  []session.CoverageItem{},
			Completed: []session.CoverageItem{},
			Reused:    []session.CoverageItem{},
			Failed:    []session.CoverageItem{},
			Waived:    []session.CoverageItem{},
		},
		Sections: MaterialSections{
			GitStatistics:     Section{Status: StatusNotCollected, Reason: "not collected by this ticket"},
			WorkspaceSnapshot: Section{Status: StatusNotApplicable, Reason: "single commit input"},
			Achievements:      Section{Status: StatusNotCollected, Reason: "not collected by this ticket"},
			People:            Section{Status: StatusNotCollected, Reason: "not collected by this ticket"},
			KnowledgeSources:  Section{Status: StatusNotCollected, Reason: "not collected by this ticket"},
			StructuralChecks:  Section{Status: StatusNotCollected, Reason: "not collected by this ticket"},
		},
		Limitations: []Limitation{},
	}
}

func TestValidateMaterialRequiresVersionedFacts(t *testing.T) {
	material := validMaterial()
	if err := ValidateMaterial(material); err != nil {
		t.Fatalf("ValidateMaterial: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Material)
		want   string
	}{
		{"version", func(m *Material) { m.SchemaVersion = "" }, "schema_version"},
		{"run identity", func(m *Material) { m.Review.RunID = "" }, "run_id"},
		{"timezone", func(m *Material) { m.Review.StartedAt = time.Time{} }, "started_at"},
		{"repository", func(m *Material) { m.Repository.Name = "" }, "repository.name"},
		{"commit", func(m *Material) { m.Scope.ResolvedHead = Revision{} }, "resolved_head"},
		{"coverage", func(m *Material) { m.Coverage.Selected = nil }, "coverage.selected"},
		{"limitations", func(m *Material) { m.Limitations = nil }, "limitations"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := validMaterial()
			tc.mutate(&candidate)
			err := ValidateMaterial(candidate)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateMaterial error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestDecodeMaterialChecksJSONCompatibilityAndSize(t *testing.T) {
	encoded, err := json.Marshal(validMaterial())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeMaterial(encoded); err != nil {
		t.Fatalf("DecodeMaterial rejected valid report material: %v", err)
	}
	for name, data := range map[string][]byte{
		"native JSON":    []byte(`{"comments":[]}`),
		"unknown field":  append(append([]byte(nil), encoded[:len(encoded)-1]...), []byte(`,"unknown":true}`)...),
		"trailing value": append(append([]byte(nil), encoded...), []byte(` {}`)...),
		"invalid JSON":   []byte(`{"schema_version":`),
		"oversized":      bytes.Repeat([]byte("x"), MaxMaterialJSONBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeMaterial(data); err == nil {
				t.Fatal("DecodeMaterial accepted incompatible material")
			}
		})
	}
}

func TestValidateMaterialRequiresReasonedNonProvidedLimitations(t *testing.T) {
	material := validMaterial()
	material.Limitations = []Limitation{{Source: "knowledge_sources", Status: StatusNotCollected, Reason: "source provenance not recorded by the run"}}
	if err := ValidateMaterial(material); err != nil {
		t.Fatalf("ValidateMaterial: %v", err)
	}
	material.Limitations[0].Reason = " "
	if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "limitations[0]") {
		t.Fatalf("ValidateMaterial error = %v, want missing limitation reason", err)
	}
}

func TestValidateMaterialRequiresReasonForUnavailableSections(t *testing.T) {
	material := validMaterial()
	material.Sections.KnowledgeSources = Section{Status: StatusNotApplicable}
	if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "knowledge_sources.reason") {
		t.Fatalf("ValidateMaterial error = %v, want knowledge_sources reason error", err)
	}

	material.Sections.KnowledgeSources = Section{Status: StatusFailed, Reason: "knowledge source inspection failed"}
	if err := ValidateMaterial(material); err != nil {
		t.Fatalf("failed section with a reason should be valid: %v", err)
	}
}

func TestValidateMaterialRejectsNullSectionData(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage("null"), json.RawMessage(" \n null \t")} {
		for _, status := range []Status{StatusProvided, StatusFailed} {
			t.Run(string(status)+"/"+string(raw), func(t *testing.T) {
				material := validMaterial()
				section := Section{Status: status, Data: raw}
				if status == StatusFailed {
					section.Reason = "collection failed after returning a null value"
				}
				material.Sections.GitStatistics = section
				if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "git_statistics") {
					t.Fatalf("ValidateMaterial error = %v, want null git_statistics data error", err)
				}
			})
		}
	}
}

func TestValidateMaterialChecksFindingFactFieldsAndUniqueIDs(t *testing.T) {
	material := validMaterial()
	findingID := "sha256:" + strings.Repeat("a", 64)
	material.Findings = []Finding{
		{
			ID: findingID, Path: "src/main.go", StartLine: 3, EndLine: 4,
			Severity: "high", SeverityStatus: StatusProvided, Category: "bug", CategoryStatus: StatusProvided, SourceContent: "race condition",
			Display:        FindingDisplay{SummaryZH: "高风险问题，位于 src/main.go:3-4", SeverityZH: "高", CategoryZH: "缺陷"}, // allow-non-english: fixture exercises required Chinese finding text
			Evidence:       FindingEvidence{Status: StatusProvided, Code: "state = next"},
			Recommendation: FindingRecommendation{Status: StatusProvided, Code: "lock(state)"},
		},
		{ID: findingID, Path: "src/main.go", StartLine: 5, EndLine: 5, Severity: "medium", SeverityStatus: StatusProvided, Category: "bug", CategoryStatus: StatusProvided, SourceContent: "another issue",
			Display:        FindingDisplay{SummaryZH: "中风险问题，位于 src/main.go:5", SeverityZH: "中", CategoryZH: "缺陷"}, // allow-non-english: fixture exercises required Chinese finding text
			Evidence:       FindingEvidence{Status: StatusNotCollected, Reason: "native result had no code excerpt"},
			Recommendation: FindingRecommendation{Status: StatusNotCollected, Reason: "native result had no suggestion code"}},
	}
	if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "duplicate finding id") {
		t.Fatalf("ValidateMaterial error = %v, want duplicate finding id", err)
	}

	material.Findings[1].ID = "sha256:" + strings.Repeat("b", 64)
	material.Findings[1].Severity = "urgent"
	if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("ValidateMaterial error = %v, want severity error", err)
	}
}

func TestValidateMaterialChecksMissingFindingLabelFacts(t *testing.T) {
	material := validMaterial()
	material.Findings = []Finding{{
		ID: "sha256:" + strings.Repeat("f", 64), Path: "src/main.go", StartLine: 1, EndLine: 1,
		SeverityStatus: StatusNotCollected, SeverityReason: "native result omitted severity",
		CategoryStatus: StatusNotCollected, CategoryReason: "native result omitted category",
		SourceContent: "finding", Display: FindingDisplay{SummaryZH: "等级未提供，类别未提供", SeverityZH: "未提供", CategoryZH: "未提供"}, // allow-non-english: fixture exercises missing Chinese finding labels
		Evidence:       FindingEvidence{Status: StatusNotCollected, Reason: "no source excerpt"},
		Recommendation: FindingRecommendation{Status: StatusNotCollected, Reason: "no suggestion"},
	}}
	if err := ValidateMaterial(material); err != nil {
		t.Fatalf("ValidateMaterial rejected explicit missing label facts: %v", err)
	}
	material.Findings[0].Display.SeverityZH = "低" // allow-non-english: fixture proves missing status cannot claim a severity
	if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("ValidateMaterial error = %v, want severity display mismatch error", err)
	}
	material.Findings[0].Display.SeverityZH = "未提供" // allow-non-english: restore explicit missing label

	material.Findings[0].SeverityReason = ""
	if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("ValidateMaterial error = %v, want missing severity reason error", err)
	}
	material.Findings[0].Severity = "urgent"
	material.Findings[0].SeverityStatus = StatusProvided
	material.Findings[0].SeverityReason = ""
	if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("ValidateMaterial error = %v, want unsupported severity error", err)
	}
}

func TestValidateMaterialRejectsAbsoluteAndTraversalPaths(t *testing.T) {
	for _, path := range []string{`.`, `foo/..`, `..\outside.go`, `C:\secret\file.go`, `\\server\share\file.go`, `src\..\..\outside.go`} {
		t.Run(path, func(t *testing.T) {
			material := validMaterial()
			material.Findings = []Finding{{
				ID: "sha256:" + strings.Repeat("c", 64), Path: path, StartLine: 1, EndLine: 1,
				Severity: "high", SeverityStatus: StatusProvided, Category: "bug", CategoryStatus: StatusProvided, SourceContent: "finding",
				Display:        FindingDisplay{SummaryZH: "原始审查意见：finding", SeverityZH: "高", CategoryZH: "缺陷"}, // allow-non-english: fixture exercises required Chinese finding text
				Evidence:       FindingEvidence{Status: StatusNotCollected, Reason: "no source excerpt"},
				Recommendation: FindingRecommendation{Status: StatusNotCollected, Reason: "no suggestion"},
			}}
			if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "repository-relative") {
				t.Fatalf("ValidateMaterial(%q) error = %v, want repository-relative path error", path, err)
			}
		})
	}
}

func TestValidateMaterialAllowsColonInRepositoryRelativePaths(t *testing.T) {
	material := validMaterial()
	material.Findings = []Finding{{
		ID: "sha256:" + strings.Repeat("e", 64), Path: "src/a:b.go", StartLine: 1, EndLine: 1,
		Severity: "high", SeverityStatus: StatusProvided, Category: "bug", CategoryStatus: StatusProvided, SourceContent: "finding",
		Display:        FindingDisplay{SummaryZH: "原始审查意见：finding", SeverityZH: "高", CategoryZH: "缺陷"}, // allow-non-english: fixture exercises required Chinese finding text
		Evidence:       FindingEvidence{Status: StatusNotCollected, Reason: "no source excerpt"},
		Recommendation: FindingRecommendation{Status: StatusNotCollected, Reason: "no suggestion"},
	}}
	if err := ValidateMaterial(material); err != nil {
		t.Fatalf("ValidateMaterial rejected a POSIX repository path containing a colon: %v", err)
	}
}

func TestValidateMaterialRequiresFullGitSHAs(t *testing.T) {
	material := validMaterial()
	material.Scope.ResolvedHead.SHA = "abcdef"
	if err := ValidateMaterial(material); err == nil || !strings.Contains(err.Error(), "full Git SHA") {
		t.Fatalf("ValidateMaterial error = %v, want full Git SHA error", err)
	}
}

func TestFindingIDIsDeterministicAndScopeSensitive(t *testing.T) {
	input := FindingIdentity{
		RepositoryIdentity: "repo-a",
		Mode:               session.InputModeCommit,
		ResolvedHead:       strings.Repeat("a", 40),
		Path:               "src/main.go",
		StartLine:          7,
		EndLine:            8,
		Severity:           "high",
		Category:           "bug",
		SourceContent:      "race condition",
	}
	first := NewFindingID(input)
	if first == "" || first != NewFindingID(input) {
		t.Fatalf("finding ID is not deterministic: %q vs %q", first, NewFindingID(input))
	}
	input.ResolvedHead = strings.Repeat("b", 40)
	if first == NewFindingID(input) {
		t.Fatal("finding ID did not change with its resolved scope")
	}
}
