// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

type workspaceSnapshot struct {
	HeadSHA              string                 `json:"head_sha,omitempty"`
	CapturedAt           time.Time              `json:"captured_at"`
	SnapshotSHA256       string                 `json:"snapshot_sha256"`
	StatusSHA256         string                 `json:"status_sha256"`
	Entries              []workspaceStatusEntry `json:"entries"`
	SelectedCount        int                    `json:"selected_count"`
	SourceArtifactSHA256 string                 `json:"source_artifact_sha256,omitempty"`
}

type workspaceStatusEntry struct {
	Path           string `json:"path"`
	IndexStatus    string `json:"index_status"`
	WorktreeStatus string `json:"worktree_status"`
}

type workspaceSnapshotCapture struct {
	Snapshot workspaceSnapshot
	Reason   string
}

func collectWorkspaceSnapshot(ctx context.Context, runner *gitcmd.Runner, repoDir string, capturedAt time.Time) (workspaceSnapshot, error) {
	if runner == nil {
		return workspaceSnapshot{}, fmt.Errorf("git snapshot collector is unavailable")
	}
	status, err := runner.Output(ctx, repoDir, "status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=all")
	if err != nil {
		return workspaceSnapshot{}, fmt.Errorf("git status failed")
	}
	head, _ := runner.Output(ctx, repoDir, "rev-parse", "--verify", "--end-of-options", "HEAD")
	headSHA := strings.TrimSpace(string(head))
	statusSum := sha256.Sum256(status)
	statusSHA := hex.EncodeToString(statusSum[:])
	snapshotSum := sha256.Sum256(append(append([]byte{}, []byte(headSHA)...), append([]byte{0}, status...)...))
	entries, err := parseWorkspaceStatus(status)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	return workspaceSnapshot{
		HeadSHA:        headSHA,
		CapturedAt:     capturedAt,
		SnapshotSHA256: hex.EncodeToString(snapshotSum[:]),
		StatusSHA256:   statusSHA,
		Entries:        entries,
	}, nil
}

func parseWorkspaceStatus(raw []byte) ([]workspaceStatusEntry, error) {
	parts := strings.Split(string(raw), "\x00")
	entries := make([]workspaceStatusEntry, 0, len(parts))
	for _, record := range parts {
		if record == "" {
			continue
		}
		if len(record) < 4 || record[2] != ' ' {
			return nil, fmt.Errorf("git status record is invalid")
		}
		entries = append(entries, workspaceStatusEntry{
			Path:           record[3:],
			IndexStatus:    record[0:1],
			WorktreeStatus: record[1:2],
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func workspaceSnapshotSection(capture workspaceSnapshotCapture, manifest *session.RunManifest) report.Section {
	if capture.Reason != "" {
		return report.Section{Status: report.StatusFailed, Reason: capture.Reason}
	}
	snapshot := capture.Snapshot
	snapshot.SelectedCount = len(manifest.Coverage.Selected)
	snapshot.SourceArtifactSHA256 = manifest.Input.SourceArtifactSHA256
	snapshot.SnapshotSHA256 = workspaceSnapshotDigest(snapshot.HeadSHA, snapshot.StatusSHA256, snapshot.SourceArtifactSHA256)
	data, err := json.Marshal(snapshot)
	if err != nil {
		return report.Section{Status: report.StatusFailed, Reason: "工作区快照无法编码为报告材料"} // allow-non-english: report JSON requires Chinese failure reasons
	}
	return report.Section{Status: report.StatusProvided, Data: data}
}

func workspaceSnapshotDigest(headSHA, statusSHA, sourceArtifactSHA string) string {
	sum := sha256.Sum256([]byte(headSHA + "\x00" + statusSHA + "\x00" + sourceArtifactSHA))
	return hex.EncodeToString(sum[:])
}
