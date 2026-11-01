package merge

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

const sha = "0123456789abcdef0123456789abcdef01234567"

var now = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// styles returns the styles of the unicode icons in the dark or light theme.
func styles(tb testing.TB, dark bool) Styles {
	tb.Helper()
	p, err := config.Default().Palette(dark)
	if err != nil {
		tb.Fatal(err)
	}
	return NewStyles(ui.NewTheme(p, dark), ui.NewIcons(config.IconsUnicode), config.IconsUnicode)
}

// opts returns the options at width, with both keys bound.
func opts(tb testing.TB, width int) Options {
	tb.Helper()
	return Options{Width: width, Styles: styles(tb, true), Now: now, Keys: Hints{Merge: "M", Update: "U"}}
}

func user(login, name string) core.User { return core.User{Login: login, Name: name} }

// detail is a pull request from feat/calc into main.
func detail() core.PullRequestDetail {
	d := core.PullRequestDetail{BaseSHA: sha}
	d.HeadRef, d.BaseRef, d.HeadSHA = "feat/calc", "main", sha
	return d
}

func job(id int64, name string, c core.Conclusion) core.CommitCheck {
	return core.CommitCheck{
		ID: id, JobID: id, RunID: 1, Name: name, Workflow: "CI",
		Status: core.RunCompleted, Conclusion: c,
		DetailsURL: "https://github.com/o/r/actions/runs/1/job/" + name,
		StartedAt:  now.Add(-65 * time.Second), CompletedAt: now,
		RunNumber: 12,
		RunURL:    "https://github.com/o/r/actions/runs/1",
	}
}

// running is a job that started 12 seconds ago.
func running(id int64, name string) core.CommitCheck {
	j := job(id, name, "")
	j.Status, j.CompletedAt, j.StartedAt = core.RunInProgress, time.Time{}, now.Add(-12*time.Second)
	return j
}

func appRun(id int64, name string, c core.Conclusion) core.CommitCheck {
	return core.CommitCheck{
		ID: id, Name: name, Status: core.RunCompleted, Conclusion: c,
		DetailsURL: "https://github.com/o/r/runs/" + name,
		StartedAt:  now.Add(-30 * time.Second), CompletedAt: now,
		SuiteID: 77, App: &core.CheckApp{Name: "Codecov", Slug: "codecov"},
	}
}

func status(context, state string) core.StatusContext {
	return core.StatusContext{Context: context, State: state, Description: context + " " + state, TargetURL: "https://ci.example.com/" + context}
}

func commit(runs []core.CommitCheck, statuses ...core.StatusContext) core.CommitChecks {
	return core.CommitChecks{SHA: sha, Runs: runs, Statuses: statuses, Total: len(runs) + len(statuses)}
}

// failing is a commit with a failing and a running job and a pending status.
func failing() core.CommitChecks {
	return commit([]core.CommitCheck{
		job(1, "test", core.ConclusionFailure),
		job(2, "lint", core.ConclusionSuccess),
		running(3, "build"),
		appRun(4, "coverage", core.ConclusionFailure),
	}, status("ci/jenkins", "pending"))
}

// everything is a pull request that every blocker stands in the way of.
func everything() Data {
	d := detail()
	d.Draft = true
	d.ReviewDecision = core.ReviewRequired
	d.Merge.Mergeable = core.MergeableConflicting
	d.Merge.Status = core.MergeBehind
	d.Reviewers = core.Reviewers{
		Requested: []core.ReviewRequest{
			{Team: "acme/core", TeamName: "core team", CodeOwner: true},
			{User: user("dave", "")},
		},
		Verdicts: []core.Verdict{
			{Author: user("carol", "Carol Diaz"), State: core.ReviewStateChangesRequested},
			{Author: user("erin", ""), State: core.ReviewStateApproved},
			{Author: user("frank", "Frank"), State: core.ReviewStateChangesRequested},
		},
	}
	return Data{Detail: d, Checks: failing(), ChecksLoaded: true, Behind: 3, Method: "squash"}
}

func plain(s string) string { return ansi.Strip(s) }

// ids returns the IDs of rows.
func ids(rows []treeview.Node) []string {
	out := make([]string, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].ID)
	}
	return out
}

// first returns the plain first line of each of rows.
func first(rows []treeview.Node) []string {
	out := make([]string, 0, len(rows))
	for i := range rows {
		out = append(out, plain(rows[i].Lines[0]))
	}
	return out
}
