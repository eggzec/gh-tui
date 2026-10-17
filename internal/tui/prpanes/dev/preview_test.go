package dev

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

var now = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func previewOpts(set string) PreviewOptions {
	return PreviewOptions{Repo: self, Styles: opts(set).Styles, Now: now, Width: 50}
}

func plain(s string) string {
	lines := strings.Split(ansi.Strip(s), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}

func issuePreview() core.IssuePreview {
	return core.IssuePreview{
		Reference: issue(self, 12, "Crash on empty input when the file is a symlink to a directory", core.StateOpen),
		Author:    "ada", CreatedAt: now.Add(-72 * time.Hour),
		Labels:            []core.Label{{Name: "bug", Color: "d73a4a"}, {Name: "good first issue", Color: "7057ff"}},
		Blockers:          []core.Reference{issue(self, 9, "Parser rewrite", core.StateOpen)},
		BlockersTotal:     1,
		BlockersSupported: true,
		Body:              "Running the tool on an empty file panics.\n\n\nSteps:\n1. make an empty file\n2. run it\n\x1b[31mhostile\x1b[0m",
		Comments: []core.PreviewComment{
			{Author: "bob", Body: "I can reproduce this.", CreatedAt: now.Add(-48 * time.Hour)},
			{Author: "ada", Body: "Fix incoming.", CreatedAt: now.Add(-2 * time.Hour)},
		},
		CommentsTotal: 5,
	}
}

func pullPreview() core.PullPreview {
	return core.PullPreview{
		Reference: pull(30, "Follow-up cleanup", core.StateOpen, false),
		Author:    "ada", Head: "cleanup", Base: "main",
		Additions: 40, Deletions: 12, ChangedFiles: 3,
		ReviewDecision: core.ReviewApproved,
		Reviews:        []core.PreviewReview{{Author: "bob", State: core.ReviewStateApproved}},
		Labels:         []core.Label{{Name: "chore", Color: "cfd3d7"}},
		Checks:         core.CheckCounts{Passed: 3, Pending: 1},
		Body:           strings.Repeat("A line of the description.\n", 9),
	}
}

func TestPreviewGolden(t *testing.T) {
	t.Parallel()
	for _, set := range []string{config.IconsNerd, config.IconsASCII} {
		o := previewOpts(set)
		for name, got := range map[string]string{
			"issue":  IssuePreview(issuePreview(), o),
			"pull":   PullPreview(pullPreview(), o),
			"header": HeaderPreview(GroupCloses, fixture(), o),
			"empty":  HeaderPreview(GroupLinked, core.PullLinks{}, o),
		} {
			t.Run(name+"-"+set, func(t *testing.T) {
				t.Parallel()
				golden.RequireEqual(t, plain(got))
			})
		}
	}
}

func TestPreviewCutsLongPullBody(t *testing.T) {
	t.Parallel()
	got := plain(PullPreview(pullPreview(), previewOpts(config.IconsASCII)))
	if n := strings.Count(got, "A line of the description."); n != pullBodyLines {
		t.Fatalf("%d body lines shown, want %d:\n%s", n, pullBodyLines, got)
	}
}
