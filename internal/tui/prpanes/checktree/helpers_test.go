package checktree

import (
	"strings"
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

func theme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

func styles(set string) Styles {
	return NewStyles(theme(), ui.NewIcons(set), set)
}

func opts() Options { return Options{Now: now, Prefix: "c1/"} }

// job is a job of a workflow run of GitHub Actions that ended in c, after a
// minute and five seconds.
func job(runID, id int64, name string, c core.Conclusion) core.CommitCheck {
	return core.CommitCheck{
		ID: id, JobID: id, RunID: runID, Name: name, Workflow: "CI",
		Status: core.RunCompleted, Conclusion: c,
		DetailsURL: "https://github.com/o/r/actions/runs/1/job/" + name,
		StartedAt:  now.Add(-65 * time.Second), CompletedAt: now,
		RunNumber: 12,
		RunURL:    "https://github.com/o/r/actions/runs/1",
	}
}

// running is a job that started 12 seconds ago.
func running(id int64, name string) core.CommitCheck {
	j := job(1, id, name, "")
	j.Status, j.CompletedAt, j.StartedAt = core.RunInProgress, time.Time{}, now.Add(-12*time.Second)
	return j
}

var codecov = &core.CheckApp{Name: "Codecov", Slug: "codecov"}

// appRun is a check run of the App app, with n annotations.
func appRun(app *core.CheckApp, id int64, name string, c core.Conclusion, n int) core.CommitCheck {
	return core.CommitCheck{
		ID: id, Name: name, Status: core.RunCompleted, Conclusion: c, Annotations: n,
		DetailsURL: "https://github.com/o/r/runs/" + name,
		StartedAt:  now.Add(-30 * time.Second), CompletedAt: now,
		SuiteID: 77, App: app,
	}
}

func status(context, state string) core.StatusContext {
	return core.StatusContext{Context: context, State: state, Description: context + " " + state, TargetURL: "https://ci.example.com/" + context}
}

func commit(runs []core.CommitCheck, statuses ...core.StatusContext) core.CommitChecks {
	return core.CommitChecks{SHA: sha, Runs: runs, Statuses: statuses, Total: len(runs) + len(statuses)}
}

// text draws rows in a tree of width and height, without colors or links.
func text(tb testing.TB, rows []treeview.Node, width, height int) string {
	tb.Helper()
	m := treeview.New(treeview.Source{}, treeview.WithSize(width, height), treeview.WithRows(rows))
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// drawn draws rows in a tree of width and height, with its colors and links.
func drawn(rows []treeview.Node, width, height int) string {
	m := treeview.New(treeview.Source{}, treeview.WithSize(width, height), treeview.WithRows(rows))
	return m.View()
}

func text1(s string) string { return ansi.Strip(s) }

func ansiStrip(s string) string { return ansi.Strip(s) }

// ids returns the IDs of rows.
func ids(rows []treeview.Node) []string {
	out := make([]string, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].ID)
	}
	return out
}

// first returns the plain first line of each of rows, with its depth as
// dots.
func first(rows []treeview.Node) []string {
	out := make([]string, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		out = append(out, strings.Repeat(".", r.Depth)+ansi.Strip(r.Lines[0])+" | "+ansi.Strip(r.Right))
	}
	return out
}
