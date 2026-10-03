// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package report 定义审查结果的独立、版本化报告材料。 // allow-non-english: 用户要求中文代码注释
package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/session"
)

const MaterialSchemaVersion = "1"

type Status string

const (
	StatusProvided      Status = "provided"
	StatusNotCollected  Status = "not_collected"
	StatusNotApplicable Status = "not_applicable"
	StatusFailed        Status = "failed"
)

// Material 是后续报告命令的事实输入；新模式只扩展本结构中的材料分区，不重算原生审查事实。 // allow-non-english: 用户要求中文代码注释
type Material struct {
	SchemaVersion string              `json:"schema_version"`
	Review        ReviewMetadata      `json:"review"`
	Repository    Repository          `json:"repository"`
	Scope         Scope               `json:"scope"`
	Findings      []Finding           `json:"findings"`
	Coverage      session.Coverage    `json:"coverage"`
	RunFailure    *session.RunFailure `json:"run_failure,omitempty"`
	Sections      MaterialSections    `json:"sections"`
	Limitations   []Limitation        `json:"limitations"`
}

// ReviewMetadata 保留本次运行身份和带时区的开始、结束时间。 // allow-non-english: 用户要求中文代码注释
type ReviewMetadata struct {
	RunID       string                `json:"run_id"`
	StartedAt   time.Time             `json:"started_at"`
	CompletedAt time.Time             `json:"completed_at"`
	ElapsedMS   int64                 `json:"elapsed_ms"`
	Status      session.TerminalState `json:"status"`
	OCRVersion  string                `json:"ocr_version,omitempty"`
	Provider    string                `json:"provider,omitempty"`
	Model       string                `json:"model,omitempty"`
}

// Repository 使用仓库目录名展示，并把跨路径身份的可用性明确记录下来。 // allow-non-english: 用户要求中文代码注释
type Repository struct {
	Name     string `json:"name"`
	Identity Fact   `json:"identity"`
}

// Fact 区分已有事实、未采集、不适用和采集失败，缺失值必须带原因。 // allow-non-english: 用户要求中文代码注释
type Fact struct {
	Status Status `json:"status"`
	Value  string `json:"value,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Scope 同时保存用户请求和 manifest 锁定后的提交引用。 // allow-non-english: 用户要求中文代码注释
type Scope struct {
	Mode           string   `json:"mode"`
	RequestedFrom  string   `json:"requested_from,omitempty"`
	RequestedHead  string   `json:"requested_head,omitempty"`
	ResolvedBase   Revision `json:"resolved_base"`
	ResolvedHead   Revision `json:"resolved_head"`
	ExactRange     string   `json:"exact_range,omitempty"`
	SourceArtifact Fact     `json:"source_artifact"`
}

// Revision 表示已解析提交或明确说明该模式没有相应提交端点。 // allow-non-english: 用户要求中文代码注释
type Revision struct {
	Status Status `json:"status"`
	SHA    string `json:"sha,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Finding 保留 OCR 原生问题说明，并以独立字段附加展示、证据和建议材料。 // allow-non-english: 用户要求中文代码注释
type Finding struct {
	ID             string                `json:"id"`
	Path           string                `json:"path"`
	StartLine      int                   `json:"start_line"`
	EndLine        int                   `json:"end_line"`
	Severity       string                `json:"severity"`
	Category       string                `json:"category"`
	SourceContent  string                `json:"source_content"`
	Display        FindingDisplay        `json:"display"`
	Evidence       FindingEvidence       `json:"evidence"`
	Recommendation FindingRecommendation `json:"recommendation"`
}

type FindingDisplay struct {
	SummaryZH  string `json:"summary_zh"`
	SeverityZH string `json:"severity_zh"`
	CategoryZH string `json:"category_zh"`
}

type FindingEvidence struct {
	Status Status `json:"status"`
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type FindingRecommendation struct {
	Status Status `json:"status"`
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Section 用于逐项标注统计、成果和外部依据的采集状态；failed 可同时保留部分 data。 // allow-non-english: 用户要求中文代码注释
type Section struct {
	Status Status          `json:"status"`
	Reason string          `json:"reason,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// Limitation 保留报告消费方无法从当前材料确认的范围和失败原因。 // allow-non-english: 用户要求中文代码注释
type Limitation struct {
	Source string `json:"source"`
	Status Status `json:"status"`
	Reason string `json:"reason"`
}

// MaterialSections 固定报告主题名称，03–05 在相应任务填充 data 并改写状态。 // allow-non-english: 用户要求中文代码注释
type MaterialSections struct {
	GitStatistics     Section `json:"git_statistics"`
	WorkspaceSnapshot Section `json:"workspace_snapshot"`
	Achievements      Section `json:"achievements"`
	People            Section `json:"people"`
	KnowledgeSources  Section `json:"knowledge_sources"`
	StructuralChecks  Section `json:"structural_checks"`
}

type FindingIdentity struct {
	RepositoryIdentity string
	RepositoryName     string
	Mode               string
	ResolvedHead       string
	Path               string
	StartLine          int
	EndLine            int
	Severity           string
	Category           string
	SourceContent      string
	DuplicateOrdinal   int
}

func NewFindingID(identity FindingIdentity) string {
	data, _ := json.Marshal(identity)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidateMaterial(material Material) error {
	if material.SchemaVersion != MaterialSchemaVersion {
		return fmt.Errorf("schema_version: unsupported report material version %q", material.SchemaVersion)
	}
	if material.Review.RunID == "" {
		return fmt.Errorf("review.run_id is required")
	}
	if material.Review.StartedAt.IsZero() || !material.Review.StartedAt.After(time.Time{}) {
		return fmt.Errorf("review.started_at must be a non-zero timestamp with a timezone")
	}
	if material.Review.CompletedAt.IsZero() || !material.Review.CompletedAt.After(time.Time{}) {
		return fmt.Errorf("review.completed_at must be a non-zero timestamp with a timezone")
	}
	if material.Review.CompletedAt.Before(material.Review.StartedAt) {
		return fmt.Errorf("review.completed_at must not precede started_at")
	}
	if material.Review.ElapsedMS < 0 {
		return fmt.Errorf("review.elapsed_ms cannot be negative")
	}
	switch material.Review.Status {
	case session.StateComplete, session.StatePartial, session.StateSkipped, session.StateFailed:
	default:
		return fmt.Errorf("review.status %q is not a native terminal state", material.Review.Status)
	}
	if material.Repository.Name == "" {
		return fmt.Errorf("repository.name is required")
	}
	if err := validateFact("repository.identity", material.Repository.Identity); err != nil {
		return err
	}
	if material.Repository.Identity.Status == StatusProvided && !validPrefixedSHA256(material.Repository.Identity.Value) {
		return fmt.Errorf("repository.identity.value must be a sha256-prefixed digest")
	}
	if err := validateScope(material.Scope, material.Review.Status); err != nil {
		return err
	}
	if material.Scope.SourceArtifact.Status == StatusProvided && !validSHA256(material.Scope.SourceArtifact.Value) {
		return fmt.Errorf("scope.source_artifact.value must be a SHA-256 digest")
	}
	if material.RunFailure != nil {
		if material.Review.Status != session.StateFailed {
			return fmt.Errorf("run_failure requires a failed review status")
		}
		switch material.RunFailure.Classification {
		case session.RunFailureInput, session.RunFailureConfiguration, session.RunFailureTimeout, session.RunFailureCancelled,
			session.RunFailureBudget, session.RunFailureInternal, session.RunFailureUnknown:
		default:
			return fmt.Errorf("run_failure.classification %q is invalid", material.RunFailure.Classification)
		}
	}
	if material.Findings == nil {
		return fmt.Errorf("findings must be present, including when empty")
	}
	if err := validateCoverage(material.Coverage); err != nil {
		return err
	}
	if err := validateSections(material.Sections); err != nil {
		return err
	}
	if material.Limitations == nil {
		return fmt.Errorf("limitations must be present, including when empty")
	}
	seenLimitations := make(map[string]struct{}, len(material.Limitations))
	for i, limitation := range material.Limitations {
		if limitation.Source == "" || strings.TrimSpace(limitation.Reason) == "" {
			return fmt.Errorf("limitations[%d] requires source and reason", i)
		}
		if limitation.Status == StatusProvided {
			return fmt.Errorf("limitations[%d].status cannot be provided", i)
		}
		switch limitation.Status {
		case StatusNotCollected, StatusNotApplicable, StatusFailed:
		default:
			return fmt.Errorf("limitations[%d].status %q is invalid", i, limitation.Status)
		}
		if _, exists := seenLimitations[limitation.Source]; exists {
			return fmt.Errorf("limitations[%d].source %q is duplicated", i, limitation.Source)
		}
		seenLimitations[limitation.Source] = struct{}{}
	}
	seen := make(map[string]struct{}, len(material.Findings))
	for i, finding := range material.Findings {
		if err := validateFinding(i, finding); err != nil {
			return err
		}
		if _, exists := seen[finding.ID]; exists {
			return fmt.Errorf("findings[%d].id: duplicate finding id %q", i, finding.ID)
		}
		seen[finding.ID] = struct{}{}
	}
	return nil
}

func validateScope(scope Scope, state session.TerminalState) error {
	switch scope.Mode {
	case session.InputModeCommit, session.InputModeRange, session.InputModeWorkspace:
	default:
		return fmt.Errorf("scope.mode %q is invalid", scope.Mode)
	}
	if scope.Mode == session.InputModeCommit && scope.RequestedHead == "" {
		return fmt.Errorf("scope.requested_head is required for commit mode")
	}
	if scope.Mode == session.InputModeCommit && scope.RequestedFrom != "" {
		return fmt.Errorf("scope.requested_from is not valid for commit mode")
	}
	if scope.Mode == session.InputModeRange && (scope.RequestedFrom == "" || scope.RequestedHead == "") {
		return fmt.Errorf("scope requested refs are required for range mode")
	}
	if scope.Mode == session.InputModeWorkspace && (scope.RequestedFrom != "" || scope.RequestedHead != "" || scope.ExactRange != "") {
		return fmt.Errorf("scope requested refs and exact_range are not valid for workspace mode")
	}
	if err := validateRevision("scope.resolved_head", scope.ResolvedHead); err != nil {
		return err
	}
	if err := validateRevision("scope.resolved_base", scope.ResolvedBase); err != nil {
		return err
	}
	if scope.ExactRange != "" {
		if scope.ResolvedBase.Status != StatusProvided || scope.ResolvedHead.Status != StatusProvided || scope.ExactRange != scope.ResolvedBase.SHA+".."+scope.ResolvedHead.SHA {
			return fmt.Errorf("scope.exact_range must match the resolved base and head")
		}
	}
	if scope.Mode == session.InputModeRange && state != session.StateFailed && scope.ExactRange == "" {
		return fmt.Errorf("scope.exact_range is required for a successful range mode")
	}
	if scope.Mode == session.InputModeRange && scope.ResolvedBase.Status == StatusProvided && scope.ResolvedHead.Status == StatusProvided && scope.ExactRange == "" {
		return fmt.Errorf("scope.exact_range is required when range endpoints are resolved")
	}
	if scope.Mode == session.InputModeCommit && scope.ResolvedBase.Status == StatusProvided && scope.ResolvedHead.Status == StatusProvided && scope.ExactRange == "" {
		return fmt.Errorf("scope.exact_range is required when commit mode has a comparison base")
	}
	if scope.Mode == session.InputModeCommit && scope.ResolvedBase.Status == StatusNotApplicable && scope.ExactRange != "" {
		return fmt.Errorf("scope.exact_range must be absent when commit mode has no comparison base")
	}
	if state != session.StateFailed {
		switch scope.Mode {
		case session.InputModeRange:
			if scope.ResolvedBase.Status != StatusProvided || scope.ResolvedHead.Status != StatusProvided {
				return fmt.Errorf("scope resolved endpoints are required for range mode")
			}
		case session.InputModeCommit:
			if scope.ResolvedHead.Status != StatusProvided || (scope.ResolvedBase.Status != StatusProvided && scope.ResolvedBase.Status != StatusNotApplicable) {
				return fmt.Errorf("scope resolved commit and comparison base are required for commit mode")
			}
		case session.InputModeWorkspace:
			if (scope.ResolvedBase.Status != StatusProvided && scope.ResolvedBase.Status != StatusNotApplicable) || scope.ResolvedHead.Status != StatusNotApplicable {
				return fmt.Errorf("scope workspace endpoints must match the available HEAD and have no resolved head")
			}
		}
	} else if scope.Mode == session.InputModeWorkspace && scope.ResolvedHead.Status != StatusNotApplicable {
		return fmt.Errorf("scope workspace resolved head must be not_applicable")
	}
	if err := validateFact("scope.source_artifact", scope.SourceArtifact); err != nil {
		return err
	}
	return nil
}

func validateRevision(name string, revision Revision) error {
	switch revision.Status {
	case StatusProvided:
		if (len(revision.SHA) != 40 && len(revision.SHA) != 64) || !isHex(revision.SHA) || revision.Reason != "" {
			return fmt.Errorf("%s must include a full Git SHA and no reason when provided", name)
		}
	case StatusNotCollected, StatusNotApplicable, StatusFailed:
		if revision.SHA != "" || strings.TrimSpace(revision.Reason) == "" {
			return fmt.Errorf("%s requires a reason and no SHA when status is %q", name, revision.Status)
		}
	default:
		return fmt.Errorf("%s.status %q is invalid", name, revision.Status)
	}
	return nil
}

func validateFact(name string, fact Fact) error {
	switch fact.Status {
	case StatusProvided:
		if fact.Value == "" || fact.Reason != "" {
			return fmt.Errorf("%s must include a value and no reason when provided", name)
		}
	case StatusNotCollected, StatusNotApplicable, StatusFailed:
		if fact.Value != "" || strings.TrimSpace(fact.Reason) == "" {
			return fmt.Errorf("%s requires a reason and no value when status is %q", name, fact.Status)
		}
	default:
		return fmt.Errorf("%s.status %q is invalid", name, fact.Status)
	}
	return nil
}

func validateFinding(index int, finding Finding) error {
	prefix := fmt.Sprintf("findings[%d]", index)
	if !validFindingID(finding.ID) {
		return fmt.Errorf("%s.id must be a sha256-prefixed stable identifier", prefix)
	}
	normalizedPath := strings.ReplaceAll(finding.Path, "\\", "/")
	cleanedPath := path.Clean(normalizedPath)
	windowsAbsolutePath := len(normalizedPath) > 2 &&
		((normalizedPath[0] >= 'a' && normalizedPath[0] <= 'z') || (normalizedPath[0] >= 'A' && normalizedPath[0] <= 'Z')) &&
		normalizedPath[1] == ':' && normalizedPath[2] == '/'
	if finding.Path == "" || cleanedPath == "." || cleanedPath == ".." || path.IsAbs(normalizedPath) || strings.HasPrefix(cleanedPath, "../") || windowsAbsolutePath {
		return fmt.Errorf("%s.path must be a repository-relative path", prefix)
	}
	if finding.StartLine < 0 || finding.EndLine < finding.StartLine || ((finding.StartLine == 0) != (finding.EndLine == 0)) {
		return fmt.Errorf("%s line position is invalid", prefix)
	}
	if !validSeverity(finding.Severity) {
		return fmt.Errorf("%s.severity %q is invalid", prefix, finding.Severity)
	}
	if !validCategory(finding.Category) {
		return fmt.Errorf("%s.category %q is invalid", prefix, finding.Category)
	}
	if finding.SourceContent == "" {
		return fmt.Errorf("%s.source_content is required", prefix)
	}
	if finding.Display.SummaryZH == "" || finding.Display.SeverityZH == "" || finding.Display.CategoryZH == "" {
		return fmt.Errorf("%s.display requires Chinese summary, severity and category labels", prefix)
	}
	if err := validateEvidence(prefix+".evidence", finding.Evidence.Status, finding.Evidence.Code, finding.Evidence.Reason); err != nil {
		return err
	}
	if err := validateEvidence(prefix+".recommendation", finding.Recommendation.Status, finding.Recommendation.Code, finding.Recommendation.Reason); err != nil {
		return err
	}
	return nil
}

func validateEvidence(name string, status Status, value, reason string) error {
	switch status {
	case StatusProvided:
		if strings.TrimSpace(value) == "" || reason != "" {
			return fmt.Errorf("%s requires evidence and no reason when provided", name)
		}
	case StatusNotCollected, StatusNotApplicable, StatusFailed:
		if value != "" || strings.TrimSpace(reason) == "" {
			return fmt.Errorf("%s requires a reason and no value when status is %q", name, status)
		}
	default:
		return fmt.Errorf("%s.status %q is invalid", name, status)
	}
	return nil
}

func validateCoverage(coverage session.Coverage) error {
	sets := []struct {
		name  string
		items []session.CoverageItem
	}{
		{"selected", coverage.Selected},
		{"completed", coverage.Completed},
		{"reused", coverage.Reused},
		{"failed", coverage.Failed},
		{"waived", coverage.Waived},
	}
	for _, set := range sets {
		if set.items == nil {
			return fmt.Errorf("coverage.%s must be present, including when empty", set.name)
		}
	}
	selected := make(map[string]struct{}, len(coverage.Selected))
	for _, item := range coverage.Selected {
		if !validSHA256(item.ItemID) {
			return fmt.Errorf("coverage.selected contains an invalid item_id %q", item.ItemID)
		}
		if _, exists := selected[item.ItemID]; exists {
			return fmt.Errorf("coverage.selected contains duplicate item_id %q", item.ItemID)
		}
		selected[item.ItemID] = struct{}{}
	}
	terminal := make(map[string]struct{}, len(selected))
	for _, set := range sets[1:] {
		for _, item := range set.items {
			if !validSHA256(item.ItemID) {
				return fmt.Errorf("coverage contains an invalid item_id %q", item.ItemID)
			}
			if _, exists := selected[item.ItemID]; !exists {
				return fmt.Errorf("coverage.%s contains unselected item_id %q", set.name, item.ItemID)
			}
			if _, exists := terminal[item.ItemID]; exists {
				return fmt.Errorf("coverage item_id %q appears in multiple terminal sets", item.ItemID)
			}
			terminal[item.ItemID] = struct{}{}
		}
	}
	if len(terminal) != len(selected) {
		return fmt.Errorf("coverage terminal item sets do not match selected items")
	}
	return nil
}

func validateSections(sections MaterialSections) error {
	all := []struct {
		name    string
		section Section
	}{
		{"git_statistics", sections.GitStatistics},
		{"workspace_snapshot", sections.WorkspaceSnapshot},
		{"achievements", sections.Achievements},
		{"people", sections.People},
		{"knowledge_sources", sections.KnowledgeSources},
		{"structural_checks", sections.StructuralChecks},
	}
	for _, item := range all {
		section := item.section
		nullData := strings.TrimSpace(string(section.Data)) == "null"
		switch section.Status {
		case StatusProvided:
			if len(section.Data) == 0 || !json.Valid(section.Data) || nullData || section.Reason != "" {
				return fmt.Errorf("sections.%s requires valid data and no reason when provided", item.name)
			}
		case StatusNotCollected, StatusNotApplicable:
			if len(section.Data) != 0 || strings.TrimSpace(section.Reason) == "" {
				return fmt.Errorf("sections.%s.reason is required and data must be absent when status is %q", item.name, section.Status)
			}
		case StatusFailed:
			if strings.TrimSpace(section.Reason) == "" || (len(section.Data) != 0 && (!json.Valid(section.Data) || nullData)) {
				return fmt.Errorf("sections.%s requires a reason and valid optional data when failed", item.name)
			}
		default:
			return fmt.Errorf("sections.%s.status %q is invalid", item.name, section.Status)
		}
	}
	return nil
}

func validFindingID(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validSHA256(strings.TrimPrefix(value, "sha256:"))
}

func validPrefixedSHA256(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validSHA256(strings.TrimPrefix(value, "sha256:"))
}

func validSHA256(value string) bool {
	return len(value) == 64 && isHex(value)
}

func isHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return value != ""
}

func validSeverity(severity string) bool {
	switch severity {
	case "critical", "high", "medium", "low":
		return true
	default:
		return false
	}
}

func validCategory(category string) bool {
	switch category {
	case "bug", "security", "performance", "maintainability", "test", "style", "documentation", "other":
		return true
	default:
		return false
	}
}
