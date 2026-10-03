// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestParseReviewFlagsOptionalReportPath(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantEnabled bool
		wantPath    string
		wantErr     bool
	}{
		{name: "omitted", args: []string{"--commit", "HEAD"}},
		{name: "bare", args: []string{"--report", "--commit", "HEAD"}, wantEnabled: true},
		{name: "explicit empty path", args: []string{"--report=", "--commit", "HEAD"}, wantEnabled: true},
		{name: "separated path", args: []string{"--report", "report.json", "--commit", "HEAD"}, wantEnabled: true, wantPath: "report.json"},
		{name: "path with spaces followed by option", args: []string{"--commit", "HEAD", "--report", "reports/final review.json", "--format", "json"}, wantEnabled: true, wantPath: "reports/final review.json"},
		{name: "equals path", args: []string{"--report=reports/final review.json", "--commit", "HEAD"}, wantEnabled: true, wantPath: "reports/final review.json"},
		{name: "path before adjacent option", args: []string{"--report", "report.json", "--provider", "anthropic", "--commit", "HEAD"}, wantEnabled: true, wantPath: "report.json"},
		{name: "bare before adjacent option", args: []string{"--report", "--format", "json", "--commit", "HEAD"}, wantEnabled: true},
		{name: "preview rejects bare report", args: []string{"--preview", "--report", "--commit", "HEAD"}, wantErr: true},
		{name: "preview rejects explicit report path", args: []string{"--preview", "--report", "report.json", "--commit", "HEAD"}, wantErr: true},
		{name: "extra positional after path", args: []string{"--report", "report.json", "unexpected", "--commit", "HEAD"}, wantErr: true},
		{name: "extra positional without report", args: []string{"unexpected", "--commit", "HEAD"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseReviewFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("parseReviewFlags should reject extra positional arguments")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseReviewFlags: %v", err)
			}
			if opts.reportEnabled != tc.wantEnabled || opts.reportPath != tc.wantPath {
				t.Fatalf("reportEnabled=%v reportPath=%q, want %v %q", opts.reportEnabled, opts.reportPath, tc.wantEnabled, tc.wantPath)
			}
		})
	}
}

func TestBuildReportMaterialRejectsMissingManifest(t *testing.T) {
	started := time.Now()
	_, err := buildReportMaterial(nil, t.TempDir(), nil, started, started, "fake", "fake-model", nil)
	if err == nil || !strings.Contains(err.Error(), "native run manifest is unavailable") {
		t.Fatalf("buildReportMaterial error = %v, want missing-manifest error", err)
	}
}

func TestDefaultReportPathUsesSafeCommitLabelAndLocalStartTime(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "sample repository")
	started := time.Date(2026, 10, 4, 10, 11, 12, 0, time.FixedZone("HKT", 8*60*60))
	scope := report.Scope{
		Mode:          session.InputModeCommit,
		RequestedHead: "feature:unsafe/name",
		ResolvedHead:  report.Revision{Status: report.StatusProvided, SHA: "abcdef0123456789"},
	}
	got := defaultReportPath(repoDir, scope, started)
	name := filepath.Base(got)
	if strings.ContainsAny(name, ` <>:"/\\|?*`) {
		t.Fatalf("default report file name contains unsafe characters: %q", name)
	}
	wantPrefix := "sample-repository__commit-feature-unsafe-name__"
	if !strings.HasPrefix(name, wantPrefix) || !strings.HasSuffix(name, ".report.json") {
		t.Fatalf("default report file name = %q, want prefix %q and report suffix", name, wantPrefix)
	}
	if !strings.Contains(name, started.Local().Format("20060102-150405")) {
		t.Fatalf("default report file name %q does not include local start time", name)
	}
	if filepath.Dir(got) != repoDir {
		t.Fatalf("default report path = %q, want repository root %q", got, repoDir)
	}
}

func TestSanitizeReportLabelClipsLongLabels(t *testing.T) {
	got := sanitizeReportLabel(strings.Repeat("a", 80), 48)
	if len([]rune(got)) != 48 {
		t.Fatalf("sanitized label rune count = %d, want 48", len([]rune(got)))
	}
}

func TestPathsConflictUsesResolvedPaths(t *testing.T) {
	dir := t.TempDir()
	native := filepath.Join(dir, "native.json")
	conflict, err := report.PathsConflict(native, filepath.Join(dir, "nested", "..", "native.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !conflict {
		t.Fatal("equivalent output paths should conflict")
	}
	conflict, err = report.PathsConflict("-", filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if conflict {
		t.Fatal("stdout should not conflict with a file path")
	}
}
