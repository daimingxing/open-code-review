// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
)

// ReviewUnit 以稳定运行标识保留一份材料及其原始问题 ID。 // allow-non-english: 用户要求中文代码注释
type ReviewUnit struct {
	ID       string   `json:"id"`
	Material Material `json:"material"`
}

// MultiReportSummary 只汇总可解释的问题记录数，不把重叠范围伪装成去重总量。 // allow-non-english: 用户要求中文代码注释
type MultiReportSummary struct {
	ReviewUnitCount     int            `json:"review_unit_count"`
	FindingRecordCount  int            `json:"finding_record_count"`
	FindingsBySeverity  map[string]int `json:"findings_by_severity"`
	FindingCountingRule string         `json:"finding_counting_rule"`
}

// MultiReportPerson 只把具有相同有效邮箱的记录认作跨仓库同一身份。 // allow-non-english: 用户要求中文代码注释
type MultiReportPerson struct {
	ID             string                    `json:"id"`
	IdentityStatus string                    `json:"identity_status"`
	DisplayName    string                    `json:"display_name"`
	NameVariants   []string                  `json:"name_variants"`
	Contributions  []MultiReportContribution `json:"contributions"`
}

// MultiReportContribution 将作者或提交者记录绑定到来源审查单元。 // allow-non-english: 用户要求中文代码注释
type MultiReportContribution struct {
	ReviewUnitID string `json:"review_unit_id"`
	CommitSHA    string `json:"commit_sha"`
	Subject      string `json:"subject"`
	Role         string `json:"role"`
	Basis        string `json:"basis"`
}

// MultiReportInput 是仅供本次 HTML 生成使用的内存输入，不作为新的报告材料格式保存。 // allow-non-english: 用户要求中文代码注释
type MultiReportInput struct {
	ReviewUnits []ReviewUnit        `json:"review_units"`
	Summary     MultiReportSummary  `json:"summary"`
	People      []MultiReportPerson `json:"people"`
}

// NewMultiReportInput 按调用者顺序包装经验证的材料，并拒绝同一运行的重复材料。 // allow-non-english: 用户要求中文代码注释
func NewMultiReportInput(materials []Material) (MultiReportInput, error) {
	if len(materials) < 2 {
		return MultiReportInput{}, fmt.Errorf("multi-report input requires at least two report materials")
	}
	input := MultiReportInput{
		ReviewUnits: make([]ReviewUnit, 0, len(materials)),
		Summary: MultiReportSummary{
			ReviewUnitCount:     len(materials),
			FindingsBySeverity:  map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "not_collected": 0},
			FindingCountingRule: "\u6309\u5ba1\u67e5\u5355\u5143\u8ba1\u6570\uff1b\u672a\u5bf9\u8de8\u5355\u5143\u95ee\u9898\u53bb\u91cd\uff0c\u4e0d\u4ee3\u8868\u552f\u4e00\u95ee\u9898\u6570\u3002",
		},
		People: make([]MultiReportPerson, 0),
	}
	seenRunIDs := make(map[string]struct{}, len(materials))
	for _, material := range materials {
		if err := ValidateMaterial(material); err != nil {
			return MultiReportInput{}, fmt.Errorf("validate report material for multi-report: %w", err)
		}
		if _, exists := seenRunIDs[material.Review.RunID]; exists {
			return MultiReportInput{}, fmt.Errorf("duplicate report material run_id %q", material.Review.RunID)
		}
		seenRunIDs[material.Review.RunID] = struct{}{}
		sum := sha256.Sum256([]byte(material.Review.RunID))
		input.ReviewUnits = append(input.ReviewUnits, ReviewUnit{
			ID:       "unit-" + hex.EncodeToString(sum[:]),
			Material: material,
		})
		people, err := peopleForReviewUnit(input.ReviewUnits[len(input.ReviewUnits)-1])
		if err != nil {
			return MultiReportInput{}, err
		}
		input.People = mergeReportPeople(input.People, people)
		input.Summary.FindingRecordCount += len(material.Findings)
		for _, finding := range material.Findings {
			if finding.SeverityStatus == StatusNotCollected {
				input.Summary.FindingsBySeverity["not_collected"]++
			} else {
				input.Summary.FindingsBySeverity[finding.Severity]++
			}
		}
	}
	return input, nil
}

type multiPeopleRecord struct {
	CommitSHA string         `json:"commit_sha"`
	Subject   string         `json:"subject"`
	Author    multiGitPerson `json:"author"`
	Committer multiGitPerson `json:"committer"`
	Basis     string         `json:"basis"`
}

type multiGitPerson struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func peopleForReviewUnit(unit ReviewUnit) ([]MultiReportPerson, error) {
	data := unit.Material.Sections.People.Data
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var record multiPeopleRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("decode people facts for review unit %s: %w", unit.ID, err)
	}
	people := make([]MultiReportPerson, 0, 2)
	for _, identity := range []struct {
		role   string
		person multiGitPerson
	}{{"author", record.Author}, {"committer", record.Committer}} {
		status := "email_verified"
		identityKey := canonicalPersonEmail(identity.person.Email)
		if identityKey == "" {
			identityKey = strings.Join([]string{unit.ID, record.CommitSHA, identity.role, identity.person.Name}, "\x00")
			status = "unit_scoped"
		}
		sum := sha256.Sum256([]byte(identityKey))
		name := strings.TrimSpace(identity.person.Name)
		nameVariants := []string(nil)
		if name != "" {
			nameVariants = append(nameVariants, name)
		}
		people = append(people, MultiReportPerson{
			ID:             "person-" + hex.EncodeToString(sum[:]),
			IdentityStatus: status,
			DisplayName:    name,
			NameVariants:   nameVariants,
			Contributions: []MultiReportContribution{{
				ReviewUnitID: unit.ID,
				CommitSHA:    record.CommitSHA,
				Subject:      record.Subject,
				Role:         identity.role,
				Basis:        record.Basis,
			}},
		})
	}
	return people, nil
}

func canonicalPersonEmail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") || strings.Count(value, "@") != 1 {
		return ""
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value {
		return ""
	}
	at := strings.LastIndexByte(value, '@')
	return value[:at+1] + strings.ToLower(value[at+1:])
}

func mergeReportPeople(existing, incoming []MultiReportPerson) []MultiReportPerson {
	indices := make(map[string]int, len(existing)+len(incoming))
	for index, person := range existing {
		if person.IdentityStatus == "email_verified" {
			indices[person.ID] = index
		}
	}
	for _, person := range incoming {
		if index, ok := indices[person.ID]; ok && person.IdentityStatus == "email_verified" {
			current := &existing[index]
			for _, name := range person.NameVariants {
				if !containsReportName(current.NameVariants, name) {
					current.NameVariants = append(current.NameVariants, name)
				}
			}
			current.Contributions = append(current.Contributions, person.Contributions...)
			continue
		}
		if person.IdentityStatus == "email_verified" {
			indices[person.ID] = len(existing)
		}
		existing = append(existing, person)
	}
	return existing
}

func containsReportName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}
