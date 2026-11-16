// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestCollectFindingCommittersUsesPinnedLinesAndMultipleCommitters(t *testing.T) {
	repo := t.TempDir()
	retryTestGit(t, repo, "init", "-q", "-b", "main")
	commit := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		retryTestGit(t, repo, "add", "main.go")
		retryTestGit(t, repo, "-c", "user.name="+name, "-c", "user.email=committer@example.test", "commit", "-q", "--author=Original Author <author@example.test>", "-m", "change")
	}
	commit("Base", "package p\nvar A = 0\nvar B = 0\nvar C = 0\n")
	commit("Alice", "package p\nvar A = 1\nvar B = 0\nvar C = 1\n")
	commit("Bob", "package p\nvar A = 1\nvar B = 2\nvar C = 1\n")
	runner := gitcmd.New(1)
	sha, err := runner.Output(t.Context(), repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	commit("Later", "package p\nvar A = 3\nvar B = 3\nvar C = 3\n")
	for _, mode := range []string{session.InputModeCommit, session.InputModeRange} {
		findings := []report.Finding{{ID: "test", Path: "main.go", StartLine: 2, EndLine: 4}}
		scope := report.Scope{Mode: mode, ResolvedHead: report.Revision{Status: report.StatusProvided, SHA: strings.TrimSpace(string(sha))}}
		limitations := collectFindingCommitters(runner, repo, scope, findings)
		if len(limitations) != 0 || !reflect.DeepEqual(findings[0].CommitterNames, []string{"Alice", "Bob"}) {
			t.Fatalf("mode=%s names=%v limitations=%v", mode, findings[0].CommitterNames, limitations)
		}
	}
	for _, tc := range []struct {
		name   string
		mode   string
		path   string
		start  int
		end    int
		status report.Status
	}{
		{"workspace", session.InputModeWorkspace, "main.go", 2, 4, report.StatusNotCollected},
		{"no position", session.InputModeRange, "main.go", 0, 0, report.StatusNotCollected},
		{"missing file", session.InputModeRange, "missing.go", 2, 4, report.StatusFailed},
		{"missing lines", session.InputModeRange, "main.go", 200, 400, report.StatusFailed},
		{"outside", session.InputModeRange, "../main.go", 2, 4, report.StatusNotCollected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			findings := []report.Finding{{ID: "test", Path: tc.path, StartLine: tc.start, EndLine: tc.end}}
			scope := report.Scope{Mode: tc.mode, ResolvedHead: report.Revision{Status: report.StatusProvided, SHA: strings.TrimSpace(string(sha))}}
			limitations := collectFindingCommitters(runner, repo, scope, findings)
			if findings[0].CommitterNames == nil || len(findings[0].CommitterNames) != 0 || len(limitations) != 1 || limitations[0].Status != tc.status {
				t.Fatalf("names=%v limitations=%v", findings[0].CommitterNames, limitations)
			}
		})
	}
}
