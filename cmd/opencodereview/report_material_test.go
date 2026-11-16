// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestPrepareReportTarget(t *testing.T) {
	for _, name := range []string{"directory", "new_directory", "wrong_suffix", "no_suffix", "json", "invalid_parent", "invalid_name", "reserved_name"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			fallback := filepath.Join(dir, "default.report.json")
			requested := filepath.Join(dir, "reports", "custom.vue")
			want := filepath.Join(dir, "reports", "custom.json")
			switch name {
			case "directory":
				requested = filepath.Join(dir, "reports.vue")
				if err := os.Mkdir(requested, 0o700); err != nil {
					t.Fatal(err)
				}
				want = filepath.Join(requested, filepath.Base(fallback))
			case "new_directory":
				requested = filepath.Join(dir, "new", "reports") + string(os.PathSeparator)
				want = filepath.Join(requested, filepath.Base(fallback))
			case "no_suffix":
				requested = filepath.Join(dir, "custom")
				want = requested + ".json"
			case "json":
				requested = filepath.Join(dir, "custom.JSON")
				want = requested
			case "invalid_parent":
				if err := os.WriteFile(filepath.Join(dir, "reports"), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
				want = fallback
			case "invalid_name":
				requested = filepath.Join(dir, "bad\x00.json")
				want = fallback
			case "reserved_name":
				if runtime.GOOS != "windows" {
					t.Skip("Windows device name")
				}
				requested = filepath.Join(dir, "CON.json")
				want = fallback
			}
			var got string
			var err error
			stderr := captureStderr(t, func() { got, err = prepareReportTarget(requested, fallback) })
			if err != nil || got != want {
				t.Fatalf("target=%q error=%v, want %q", got, err, want)
			}
			if want == fallback && !strings.Contains(stderr, "falling back") {
				t.Fatalf("missing fallback warning: %s", stderr)
			}
			if _, err := os.Stat(filepath.Dir(want)); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Dir(want))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), "ocr-probe") {
					t.Fatalf("probe leaked: %s", entry.Name())
				}
			}
		})
	}
}

func TestPrepareReportTargetRejectsUnusableDefault(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareReportTarget("", filepath.Join(blocker, "report.json")); err == nil {
		t.Fatal("an unusable default must fail before review")
	}
}

func TestSaveReportMaterialFallsBackAfterDestinationChanges(t *testing.T) {
	dir := t.TempDir()
	fallback := filepath.Join(t.TempDir(), "default.report.json")
	target := filepath.Join(dir, "report.json")
	if err := checkReportTarget(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	var err error
	stderr := captureStderr(t, func() { err = saveReportMaterial(target, fallback, "", reportTestMaterial(false)) })
	if err != nil || !strings.Contains(stderr, "falling back") {
		t.Fatalf("save error=%v\n%s", err, stderr)
	}
	if _, err := os.Stat(fallback); err != nil {
		t.Fatal(err)
	}
}

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
	_, err := buildReportMaterial(nil, t.TempDir(), nil, started, started, "fake", "fake-model", nil, nil)
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
