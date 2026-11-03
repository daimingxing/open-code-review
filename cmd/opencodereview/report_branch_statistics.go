// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/report"
	"github.com/alibaba/open-code-review/internal/session"
)

type branchGitStatistics struct {
	BaseSHA               string                   `json:"base_sha"`
	HeadSHA               string                   `json:"head_sha"`
	CommitCount           int                      `json:"commit_count"`
	FinalDiff             branchChangeStatistics   `json:"final_diff"`
	PerCommitCumulative   branchChangeStatistics   `json:"per_commit_cumulative"`
	PerCommit             []branchCommitStatistics `json:"per_commit"`
	StatisticsExplanation string                   `json:"statistics_explanation"`
}

type branchChangeStatistics struct {
	PathChanges       int   `json:"path_changes"`
	TextInsertions    int64 `json:"text_insertions"`
	TextDeletions     int64 `json:"text_deletions"`
	BinaryPathChanges int   `json:"binary_path_changes"`
}

type branchCommitStatistics struct {
	SHA       string                 `json:"sha"`
	Parents   []string               `json:"parents"`
	Subject   string                 `json:"subject"`
	Author    branchGitIdentity      `json:"author"`
	Committer branchGitIdentity      `json:"committer"`
	Diff      branchChangeStatistics `json:"diff_from_first_parent"`
}

type branchGitIdentity struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
}

func branchGitStatisticsSection(runner *gitcmd.Runner, repoDir string, scope report.Scope) report.Section {
	if scope.Mode != session.InputModeRange && scope.Mode != session.InputModeCommit {
		return notCollectedSection("本次输入不是提交或分支比较，未计算 Git 统计") // allow-non-english: report JSON requires Chinese user-facing facts
	}
	if scope.Mode == session.InputModeCommit && scope.ResolvedBase.Status == report.StatusNotApplicable {
		return notCollectedSection("根提交没有父提交，未计算基于父提交的 Git 统计") // allow-non-english: report JSON requires Chinese user-facing facts
	}
	if scope.ResolvedBase.Status != report.StatusProvided || scope.ResolvedHead.Status != report.StatusProvided {
		return report.Section{Status: report.StatusFailed, Reason: "原生审查未解析出完整的分支比较提交范围，无法计算 Git 统计"} // allow-non-english: report JSON requires Chinese failure reasons
	}
	if runner == nil {
		return report.Section{Status: report.StatusFailed, Reason: "Git 统计采集器不可用"} // allow-non-english: report JSON requires Chinese failure reasons
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	statistics, err := collectBranchGitStatistics(ctx, runner, repoDir, scope.ResolvedBase.SHA, scope.ResolvedHead.SHA)
	data, marshalErr := json.Marshal(statistics)
	if marshalErr != nil {
		return report.Section{Status: report.StatusFailed, Reason: "Git 统计结果无法编码为报告材料"} // allow-non-english: report JSON requires Chinese failure reasons
	}
	if err != nil {
		return report.Section{Status: report.StatusFailed, Reason: "Git 统计采集未能完整完成；材料保留已取得的部分数据", Data: data} // allow-non-english: report JSON requires Chinese failure reasons
	}
	return report.Section{Status: report.StatusProvided, Data: data}
}

func collectBranchGitStatistics(
	ctx context.Context,
	runner *gitcmd.Runner,
	repoDir, baseSHA, headSHA string,
) (branchGitStatistics, error) {
	statistics := branchGitStatistics{
		BaseSHA:               baseSHA,
		HeadSHA:               headSHA,
		PerCommit:             []branchCommitStatistics{},
		StatisticsExplanation: "final_diff 是 base 到 head 的唯一净差异；per_commit_cumulative 是每个新提交相对首父提交的活动累计，可能重复计算同一文件或行；path_changes 按 --no-renames 的路径变更行计数，二进制文件不计入文本增删行。", // allow-non-english: report JSON requires Chinese explanation of statistic semantics
	}

	finalDiff, err := runner.Output(ctx, repoDir, "diff", "--numstat", "--no-renames", baseSHA, headSHA)
	if err != nil {
		return statistics, err
	}
	statistics.FinalDiff, err = parseBranchNumstat(string(finalDiff))
	if err != nil {
		return statistics, err
	}

	format := "--format=%x00OCR-REPORT-COMMIT%x00%H%x00%P%x00%an%x00%ae%x00%cn%x00%ce%x00%aI%x00%cI%x00%s"
	logOutput, err := runner.Output(ctx, repoDir, "log", "--reverse", "--numstat", "--no-renames", format, baseSHA+".."+headSHA)
	if err != nil {
		return statistics, err
	}
	commits, err := parseBranchGitLog(string(logOutput))
	if err != nil {
		return statistics, err
	}
	for index := range commits {
		commit := &commits[index]
		if len(commit.Parents) > 1 {
			mergeDiff, diffErr := runner.Output(ctx, repoDir, "diff", "--numstat", "--no-renames", commit.Parents[0], commit.SHA)
			if diffErr != nil {
				statistics.PerCommit = append(statistics.PerCommit, commits[:index+1]...)
				statistics.CommitCount = len(statistics.PerCommit)
				return statistics, diffErr
			}
			commit.Diff, diffErr = parseBranchNumstat(string(mergeDiff))
			if diffErr != nil {
				statistics.PerCommit = append(statistics.PerCommit, commits[:index+1]...)
				statistics.CommitCount = len(statistics.PerCommit)
				return statistics, diffErr
			}
		}
		if err := addBranchChangeStatistics(&statistics.PerCommitCumulative, commit.Diff); err != nil {
			statistics.PerCommit = append(statistics.PerCommit, commits[:index+1]...)
			statistics.CommitCount = len(statistics.PerCommit)
			return statistics, err
		}
	}
	statistics.PerCommit = commits
	statistics.CommitCount = len(commits)
	return statistics, nil
}

func parseBranchGitLog(output string) ([]branchCommitStatistics, error) {
	const marker = "\x00OCR-REPORT-COMMIT\x00"
	parts := strings.Split(output, marker)
	commits := make([]branchCommitStatistics, 0, len(parts)-1)
	for _, part := range parts[1:] {
		part = strings.TrimLeft(part, "\r\n")
		header, numstat, ok := strings.Cut(part, "\n")
		if !ok {
			return nil, fmt.Errorf("Git commit statistics have an incomplete record")
		}
		fields := strings.Split(strings.TrimSuffix(header, "\r"), "\x00")
		if len(fields) != 9 {
			return nil, fmt.Errorf("Git commit statistics have an invalid metadata record")
		}
		commit := branchCommitStatistics{
			SHA:     fields[0],
			Parents: strings.Fields(fields[1]),
			Subject: fields[8],
		}
		var err error
		if commit.Author.Timestamp, err = time.Parse(time.RFC3339, fields[6]); err != nil {
			return nil, fmt.Errorf("Git author timestamp is invalid")
		}
		if commit.Committer.Timestamp, err = time.Parse(time.RFC3339, fields[7]); err != nil {
			return nil, fmt.Errorf("Git committer timestamp is invalid")
		}
		commit.Author.Name = fields[2]
		commit.Author.Email = fields[3]
		commit.Committer.Name = fields[4]
		commit.Committer.Email = fields[5]
		commit.Diff, err = parseBranchNumstat(numstat)
		if err != nil {
			return nil, err
		}
		commits = append(commits, commit)
	}
	return commits, nil
}

func parseBranchNumstat(output string) (branchChangeStatistics, error) {
	statistics := branchChangeStatistics{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			return branchChangeStatistics{}, fmt.Errorf("Git numstat record is invalid")
		}
		statistics.PathChanges++
		if fields[0] == "-" || fields[1] == "-" {
			statistics.BinaryPathChanges++
			continue
		}
		insertions, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return branchChangeStatistics{}, fmt.Errorf("Git insertion count is invalid")
		}
		deletions, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return branchChangeStatistics{}, fmt.Errorf("Git deletion count is invalid")
		}
		if insertions > math.MaxInt64-statistics.TextInsertions || deletions > math.MaxInt64-statistics.TextDeletions {
			return branchChangeStatistics{}, fmt.Errorf("Git line statistics overflow")
		}
		statistics.TextInsertions += insertions
		statistics.TextDeletions += deletions
	}
	if err := scanner.Err(); err != nil {
		return branchChangeStatistics{}, err
	}
	return statistics, nil
}

func addBranchChangeStatistics(total *branchChangeStatistics, add branchChangeStatistics) error {
	if math.MaxInt-total.PathChanges < add.PathChanges || math.MaxInt-total.BinaryPathChanges < add.BinaryPathChanges ||
		add.TextInsertions > math.MaxInt64-total.TextInsertions || add.TextDeletions > math.MaxInt64-total.TextDeletions {
		return fmt.Errorf("Git cumulative statistics overflow")
	}
	total.PathChanges += add.PathChanges
	total.BinaryPathChanges += add.BinaryPathChanges
	total.TextInsertions += add.TextInsertions
	total.TextDeletions += add.TextDeletions
	return nil
}
