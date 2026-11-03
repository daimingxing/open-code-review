// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"

	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/report"
)

func nativeKnowledgeSection(base *knowledge.Base) report.Section {
	if base == nil {
		return knowledgeSourcesSection()
	}
	accesses := base.Snapshot()
	if len(accesses) == 0 {
		return notCollectedSection("A knowledge directory was configured, but no knowledge access was observed.")
	}
	reads, failed := 0, false
	for _, access := range accesses {
		if access.Operation == "file_read" && access.Status == "read" {
			reads++
		}
		failed = failed || access.Status == "failed" || access.Changed
	}
	readingStatus := "not_read"
	if reads > 0 {
		readingStatus = "read_observed"
	}
	data, err := json.Marshal(struct {
		SourceType        string             `json:"source_type"`
		Directory         string             `json:"directory"`
		Accesses          []knowledge.Access `json:"accesses"`
		ApplicationStatus string             `json:"application_status"`
		ReadingStatus     string             `json:"reading_status"`
		Limitations       []string           `json:"limitations"`
	}{"native_tools", base.Root, accesses, "not_verified", readingStatus, []string{"Tool access records show what was retrieved, not whether knowledge was correctly applied. Search and partial reads do not establish full-document coverage. Changes are checked against metadata of files actually read; the directory is not snapshotted."}})
	if err != nil {
		return report.Section{Status: report.StatusFailed, Reason: "Could not serialize knowledge access evidence."}
	}
	if failed {
		return report.Section{Status: report.StatusFailed, Reason: "Knowledge access failed or a previously read source changed; available evidence is retained.", Data: data}
	}
	return report.Section{Status: report.StatusProvided, Data: data}
}
