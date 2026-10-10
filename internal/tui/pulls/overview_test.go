package pulls

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// blocked makes the detail of a pull request one that an author has most
// to do on: two failing checks, one of them required, a reviewer who asked
// for changes, three unresolved threads, a merge the rules block, and
// reviewers of each kind.
func blocked(d *core.PullRequestDetail) {
	d.Caps.Authored = true
	d.Milestone = "v0.4"
	d.Assignees = []core.User{{Login: "octocat"}}
	d.ReviewDecision = core.ReviewChangesRequested
	d.CheckCounts = core.CheckCounts{Passed: 10, Failed: 2}
	d.FailingChecks = []core.FailingCheck{
		{Name: "test (ubuntu-latest)", Run: true, ID: 11, Required: true, Reason: "tea_test.go:54 want a frame, got none"},
		{Name: "codecov/patch", Reason: "62.50% of diff hit (target 80%)"},
	}
	d.Merge = core.MergeInfo{
		Mergeable: core.MergeableYes, Status: core.MergeBlocked,
		Methods:        []core.MergeMethod{core.MergeCommit, core.MergeSquash},
		RequiredChecks: core.CheckCounts{Passed: 2, Failed: 1},
		Rules:          core.MergeRules{Known: true, Approvals: 1, Checks: []string{"test (ubuntu-latest)", "lint", "build"}},
	}
	d.Reviewers = core.Reviewers{
		Verdicts: []core.Verdict{
			{Author: core.User{Login: "hubot"}, State: core.ReviewStateChangesRequested, SubmittedAt: clock.Add(-24 * time.Hour)},
			{Author: core.User{Login: "monalisa"}, State: core.ReviewStateApproved, SubmittedAt: clock.Add(-3 * time.Hour)},
		},
		VerdictsTotal:  2,
		Requested:      []core.ReviewRequest{{Team: "eggzec/core"}},
		RequestedTotal: 1,
	}
	d.Threads = core.ThreadSummary{
		Total: 3, Unresolved: 3,
		Threads: []core.ReviewThread{
			{ID: "t1", Path: "internal/cache/disk.go", Line: 45, Author: core.User{Login: "hubot"}, Excerpt: "Swallowing err hides a full disk."},
			{ID: "t2", Path: "internal/cache/disk.go", Line: 88, Author: core.User{Login: "hubot"}, Excerpt: "Why a mutex here?"},
			{ID: "t3", Path: "cmd/gh-tui/main.go", Line: 12, Author: core.User{Login: "monalisa"}, Excerpt: "Unused."},
		},
	}
}

// overviewOf returns the modal of #142, as tweak makes its detail, open on
// its Overview at width by height.
func overviewOf(tb testing.TB, width, height int, icons string, tweak func(*core.PullRequestDetail), opts ...Option) (*host, *detailModal) {
	tb.Helper()
	svc := newFakeService()
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	svc.tweaks = map[int]func(*core.PullRequestDetail){142: tweak}
	opts = append([]Option{WithChecks(&fakeChecks{job: true}), WithIcons(ui.NewIcons(icons))}, opts...)
	h := started(tb, svc, width, height, opts...)
	h.SetTheme(theme(true))
	press(tb, h, "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter opened no pull request")
	}
	return h, m
}

func TestOverviewViews(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
		icons         string
	}{
		{"120x40", 116, 37, config.IconsUnicode},
		{"80x24", 76, 21, config.IconsUnicode},
		{"ascii", 76, 21, config.IconsASCII},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, m := overviewOf(t, tt.width, tt.height, tt.icons, blocked)
			v := m.View()
			lines := strings.Split(v, "\n")
			if len(lines) != tt.height {
				t.Errorf("the view has %d lines, want %d", len(lines), tt.height)
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != tt.width {
					t.Errorf("line %d is %d cells wide, want %d", i+1, w, tt.width)
				}
			}
			golden.RequireEqual(t, v)
		})
	}
}

// below the sidebar's width the reviewers stand in a block of their own.
func TestOverviewReviewersBlock(t *testing.T) {
	_, m := overviewOf(t, 100, 37, config.IconsUnicode, blocked)
	v := ansi.Strip(m.View())
	for _, want := range []string{"Reviewers", "hubot changes", "monalisa approved", "@eggzec/core requested", "Assignees", "Labels", "Milestone"} {
		if !strings.Contains(v, want) {
			t.Errorf("the overview lacks %q:\n%s", want, v)
		}
	}
	for l := range strings.SplitSeq(v, "\n") {
		if strings.Contains(l, "│") {
			t.Errorf("a sidebar shows below its width: %q", l)
		}
	}
}

// attentionOf returns the subjects of the rows of the Attention list.
func attentionOf(m *detailModal) []string {
	rows := m.attentionRows()
	out := make([]string, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].subject)
	}
	return out
}

func TestAttentionRowsByRole(t *testing.T) {
	reviewing := func(d *core.PullRequestDetail) {
		blocked(d)
		d.Caps.Authored = false
		d.Merge.Mergeable, d.Merge.Status = core.MergeableConflicting, core.MergeDirty
		d.Reviewers.Requested = []core.ReviewRequest{{User: core.User{Login: "reviewer"}}}
	}
	behind := func(d *core.PullRequestDetail) {
		d.Merge = core.MergeInfo{Mergeable: core.MergeableYes, Status: core.MergeBehind}
		d.CheckCounts = core.CheckCounts{Passed: 3}
		d.Threads = core.ThreadSummary{}
	}
	tests := []struct {
		name   string
		tweak  func(*core.PullRequestDetail)
		viewer string
		want   []string
	}{
		{
			"author", blocked, "",
			[]string{
				"test (ubuntu-latest)", "codecov/patch", "hubot requested changes", "3 unresolved threads",
			},
		},
		{
			"author with conflicts last", func(d *core.PullRequestDetail) {
				blocked(d)
				d.Merge.Status = core.MergeDirty
			}, "",
			[]string{"test (ubuntu-latest)", "codecov/patch", "hubot requested changes", "3 unresolved threads", "Conflicts with main"},
		},
		{
			"requested reviewer", reviewing, "reviewer",
			[]string{
				"Review requested from you", "test (ubuntu-latest)", "codecov/patch", "hubot requested changes", "3 unresolved threads", "Conflicts with main",
			},
		},
		{
			"somebody else", reviewing, "",
			[]string{
				"Conflicts with main", "test (ubuntu-latest)", "codecov/patch", "hubot requested changes", "3 unresolved threads",
			},
		},
		{"behind", behind, "", []string{"Behind main"}},
		{"nothing", func(d *core.PullRequestDetail) { d.CheckCounts = core.CheckCounts{Passed: 2} }, "", nil},
		{
			"more failing checks than named", func(d *core.PullRequestDetail) {
				d.CheckCounts = core.CheckCounts{Failed: 9}
				for i := range 6 {
					d.FailingChecks = append(d.FailingChecks, core.FailingCheck{Name: "job " + strconv.Itoa(i)})
				}
			}, "",
			[]string{"job 0", "job 1", "job 2", "job 3", "job 4", "4 more failing checks"},
		},
		{
			"merged", func(d *core.PullRequestDetail) {
				blocked(d)
				d.State = core.StateMerged
			}, "", nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := func(context.Context) (string, error) { return tt.viewer, nil }
			h, m := overviewOf(t, 100, 30, config.IconsUnicode, tt.tweak, WithViewer(viewer))
			drain(t, h, m.readViewer())
			got := attentionOf(m)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("attention = %q, want %q", got, tt.want)
			}
		})
	}
}

// The role comes from who opened the pull request when GitHub doesn't say
// that the viewer did.
func TestAttentionRoleFromTheViewerLogin(t *testing.T) {
	tweak := func(d *core.PullRequestDetail) {
		blocked(d)
		d.Caps.Authored = false
		d.Merge.Status = core.MergeDirty
	}
	viewer := func(context.Context) (string, error) { return "octocat", nil }
	h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak, WithViewer(viewer))
	drain(t, h, m.readViewer())
	if got := attentionOf(m); got[0] != "test (ubuntu-latest)" || got[len(got)-1] != "Conflicts with main" {
		t.Errorf("attention = %q, want an author's order", got)
	}
}

func TestOverviewEnterGoesToTheTarget(t *testing.T) {
	t.Run("failing check opens its log", func(t *testing.T) {
		tweak := func(d *core.PullRequestDetail) {
			blocked(d)
			// The fake checks have a failing job with the ID 1.
			d.FailingChecks[0].ID = 1
			d.FailingChecks[0].Name = "codecov"
		}
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak)
		press(t, h, "enter")
		if m.tab != checksTab || m.checks == nil {
			t.Fatalf("enter showed tab %d with checks %v, want the Checks tab", m.tab, m.checks)
		}
		if !m.checks.StepOut() {
			t.Error("the Checks tab is on its list, want it in the check")
		}
	})
	t.Run("a check of an app opens what it reported", func(t *testing.T) {
		tweak := func(d *core.PullRequestDetail) {
			blocked(d)
			d.FailingChecks[0] = core.FailingCheck{Name: "codecov", Run: true, ID: 1}
		}
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak, WithChecks(&fakeChecks{}))
		press(t, h, "enter")
		if v := ansi.Strip(m.checks.View()); m.tab != checksTab || !strings.Contains(v, "Coverage fell") {
			t.Errorf("enter showed tab %d with:\n%s\nwant the detail of the check", m.tab, v)
		}
	})
	t.Run("more failing checks open the list", func(t *testing.T) {
		tweak := func(d *core.PullRequestDetail) {
			blocked(d)
			d.CheckCounts.Failed = 7
			d.FailingChecks = d.FailingChecks[:1]
		}
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak)
		press(t, h, "j")
		press(t, h, "enter")
		if m.tab != checksTab || m.checks.StepOut() {
			t.Errorf("enter showed tab %d, want the list of checks", m.tab)
		}
	})
	t.Run("changes requested go to the line of the thread", func(t *testing.T) {
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, blocked)
		press(t, h, "j")
		press(t, h, "j")
		press(t, h, "enter")
		if m.tab != filesTab || m.files == nil {
			t.Fatalf("enter showed tab %d with files %v, want the Files tab", m.tab, m.files)
		}
		want := diff.Pos{Path: "internal/cache/disk.go", Side: diff.NewSide, Line: 45}
		if got, ok := m.files.diff.Position(); !ok || got != want {
			t.Errorf("the diff's cursor is on %v (a line %v), want %v, the first thread of hubot", got, ok, want)
		}
		if m.files.focus != diffPane {
			t.Error("the diff doesn't have the focus")
		}
	})
	t.Run("threads go to the first", func(t *testing.T) {
		tweak := func(d *core.PullRequestDetail) {
			blocked(d)
			d.Reviewers.Verdicts = nil
			d.ReviewDecision = core.ReviewNone
			d.FailingChecks, d.CheckCounts = nil, core.CheckCounts{Passed: 3}
			d.Threads.Threads = d.Threads.Threads[2:]
			d.Threads.Unresolved = 1
		}
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak)
		press(t, h, "enter")
		want := diff.Pos{Path: "cmd/gh-tui/main.go", Side: diff.NewSide, Line: 12}
		if got, ok := m.files.diff.Position(); m.tab != filesTab || !ok || got != want {
			t.Errorf("enter showed tab %d with the cursor on %v (a line %v), want %v", m.tab, got, ok, want)
		}
	})
	t.Run("a line the diff doesn't show goes to its file", func(t *testing.T) {
		tweak := func(d *core.PullRequestDetail) {
			blocked(d)
			d.FailingChecks, d.CheckCounts = nil, core.CheckCounts{Passed: 3}
			d.Reviewers.Verdicts, d.ReviewDecision = nil, core.ReviewNone
			d.Threads.Threads = d.Threads.Threads[1:2]
			d.Threads.Unresolved = 1
		}
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak)
		press(t, h, "enter")
		if _, ok := m.files.diff.Position(); ok {
			t.Error("the cursor is on a line, want it on the header of the file")
		}
		if cur, ok := m.files.diff.CurrentFile(); m.tab != filesTab || !ok || cur.Path != "internal/cache/disk.go" {
			t.Errorf("enter showed tab %d on %q, want the file of the thread", m.tab, cur.Path)
		}
	})
	t.Run("an outdated thread goes to the conversation", func(t *testing.T) {
		tweak := func(d *core.PullRequestDetail) {
			blocked(d)
			d.FailingChecks, d.CheckCounts = nil, core.CheckCounts{Passed: 3}
			d.Reviewers.Verdicts, d.ReviewDecision = nil, core.ReviewNone
			d.Threads.Threads = []core.ReviewThread{{ID: "t9", Path: "gone.go", Line: 4, Outdated: true}}
			d.Threads.Unresolved = 1
		}
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak)
		press(t, h, "enter")
		if m.tab != conversationTab {
			t.Errorf("enter showed tab %d, want the conversation, which the diff can't stand in for", m.tab)
		}
	})
	t.Run("changes without a thread go to the conversation", func(t *testing.T) {
		tweak := func(d *core.PullRequestDetail) {
			blocked(d)
			d.FailingChecks, d.CheckCounts = nil, core.CheckCounts{Passed: 3}
			d.Threads = core.ThreadSummary{}
		}
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak)
		press(t, h, "enter")
		if m.tab != conversationTab {
			t.Errorf("enter showed tab %d, want the conversation", m.tab)
		}
	})
	t.Run("conflicts open the page", func(t *testing.T) {
		tweak := func(d *core.PullRequestDetail) {
			d.Merge.Status = core.MergeDirty
			d.CheckCounts = core.CheckCounts{Passed: 3}
		}
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak)
		var opened []string
		for _, msg := range press(t, h, "enter") {
			if o, ok := msg.(ui.OpenMsg); ok {
				opened = append(opened, o.URL)
			}
		}
		if want := m.detail.URL; len(opened) != 1 || opened[0] != want || m.tab != overviewTab {
			t.Errorf("enter opened %v on tab %d, want %s", opened, m.tab, want)
		}
	})
	t.Run("a review asked goes to the files", func(t *testing.T) {
		tweak := func(d *core.PullRequestDetail) {
			d.Reviewers.Requested = []core.ReviewRequest{{User: core.User{Login: "reviewer"}}}
			d.CheckCounts = core.CheckCounts{Passed: 3}
		}
		viewer := func(context.Context) (string, error) { return "reviewer", nil }
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak, WithViewer(viewer))
		drain(t, h, m.readViewer())
		press(t, h, "enter")
		if m.tab != filesTab {
			t.Errorf("enter showed tab %d, want the Files tab", m.tab)
		}
	})
	t.Run("a failing check without a Checks tab opens the page", func(t *testing.T) {
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, blocked)
		m.newChecks = nil
		var opened []string
		for _, msg := range press(t, h, "enter") {
			if o, ok := msg.(ui.OpenMsg); ok {
				opened = append(opened, o.URL)
			}
		}
		if want := m.detail.URL + "/checks"; len(opened) != 1 || opened[0] != want || m.tab != overviewTab {
			t.Errorf("enter opened %v on tab %d, want %s", opened, m.tab, want)
		}
	})
	t.Run("no rows, no target", func(t *testing.T) {
		h, m := overviewOf(t, 100, 30, config.IconsUnicode, func(d *core.PullRequestDetail) { d.CheckCounts = core.CheckCounts{Passed: 3} })
		press(t, h, "enter")
		if m.tab != overviewTab {
			t.Errorf("enter on an empty list showed tab %d", m.tab)
		}
	})
}

func TestOverviewCursorMoves(t *testing.T) {
	h, m := overviewOf(t, 100, 30, config.IconsUnicode, blocked)
	rows := len(m.attentionRows())
	if rows < 4 {
		t.Fatalf("%d rows, want several", rows)
	}
	for _, step := range []struct {
		key  string
		want int
	}{{"j", 1}, {"down", 2}, {"k", 1}, {"G", rows - 1}, {"j", rows - 1}, {"g", 0}, {"k", 0}, {"end", rows - 1}, {"home", 0}, {"ctrl+d", rows - 1}} {
		press(t, h, step.key)
		if m.ov.cursor != step.want {
			t.Fatalf("%s left the cursor on %d, want %d", step.key, m.ov.cursor, step.want)
		}
	}
	v := ansi.Strip(m.View())
	var marked []string
	for l := range strings.SplitSeq(v, "\n") {
		if strings.HasPrefix(l, "▌") {
			marked = append(marked, l)
		}
	}
	if len(marked) != 1 || !strings.Contains(marked[0], "3 unresolved threads") {
		t.Errorf("the cursor rows = %q, want the last row, the threads", marked)
	}
}

func TestOverviewKeyLayer(t *testing.T) {
	_, m := overviewOf(t, 100, 30, config.IconsUnicode, blocked)
	layers := m.KeyLayers()
	if got := layers[len(layers)-1].Context; got != "pull_overview" {
		t.Fatalf("the last layer is %q, want the Overview's", got)
	}
	enabled := func(b key.Binding) bool { return b.Enabled() }
	if !enabled(m.overviewLayer()[0]) {
		t.Error("enter isn't offered with rows")
	}
	m.detail.State = core.StateMerged
	if enabled(m.overviewLayer()[0]) {
		t.Error("enter is offered with no rows")
	}
}

// What the Overview doesn't have yet it says is on its way, and the
// failure of the detail reads as it does on the other tabs.
func TestOverviewBeforeTheDetail(t *testing.T) {
	_, m := overviewOf(t, 100, 30, config.IconsUnicode, blocked)
	m.loaded, m.detail = false, core.PullRequestDetail{}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Loading") {
		t.Errorf("no loading:\n%s", v)
	}
}

// quiet makes the detail one with no checks and no reviews, then changes
// it with tweak.
func quiet(tweak func(*core.PullRequestDetail)) func(*core.PullRequestDetail) {
	return func(d *core.PullRequestDetail) {
		d.Checks, d.CheckCounts, d.ReviewDecision = core.ChecksNone, core.CheckCounts{}, core.ReviewNone
		tweak(d)
	}
}

func TestOverviewStripWords(t *testing.T) {
	tests := []struct {
		name  string
		tweak func(*core.PullRequestDetail)
		short bool
		want  string
	}{
		{"blocked in full", blocked, false, "Blocked · ✗ 2 failing · ± changes requested · ● 3 unresolved"},
		{"blocked short", blocked, true, "Blocked · ✗ 2 · ± changes · ● 3"},
		{"ready", func(d *core.PullRequestDetail) {
			d.ReviewDecision, d.CheckCounts = core.ReviewApproved, core.CheckCounts{Passed: 12}
		}, false, "Ready · ✓ 12 checks · ✓ approved"},
		{"draft", quiet(func(d *core.PullRequestDetail) { d.Draft = true }), false, "Draft"},
		{"conflicts", quiet(func(d *core.PullRequestDetail) { d.Merge.Status = core.MergeDirty }), false, "Conflicts"},
		{"behind", quiet(func(d *core.PullRequestDetail) { d.Merge.Status = core.MergeBehind }), false, "Behind"},
		{"checking", quiet(func(d *core.PullRequestDetail) { d.Merge = core.MergeInfo{} }), false, "Checking…"},
		{"pending", quiet(func(d *core.PullRequestDetail) { d.CheckCounts = core.CheckCounts{Pending: 2, Passed: 1} }), false, "Ready · ◐ 2 pending"},
		{"merged says nothing", func(d *core.PullRequestDetail) { d.State = core.StateMerged }, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, m := overviewOf(t, 100, 30, config.IconsUnicode, tt.tweak)
			if got := ansi.Strip(m.strip(tt.short)); got != tt.want {
				t.Errorf("strip = %q, want %q", got, tt.want)
			}
		})
	}
}

// The description is cut to the room that is left, with a line that says
// where the rest is.
func TestOverviewDescriptionIsCut(t *testing.T) {
	svc := newFakeService()
	svc.pulls[0].Body = longBody()
	h := started(t, svc, 80, 24, WithIcons(ui.NewIcons(config.IconsUnicode)))
	press(t, h, "enter")
	m := h.modal()
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "the rest is in Conversation") {
		t.Errorf("a long description isn't cut with a hint:\n%s", v)
	}
	if got := len(strings.Split(v, "\n")); got != 24 {
		t.Errorf("the view has %d lines, want 24", got)
	}
}

// The finder of the changed files and the search of the diff work from the
// Overview: the finder opens over it and a file chosen lands in the Files tab.
func TestOverviewFindsAFileAndSearchesItsDiff(t *testing.T) {
	t.Parallel()
	h, m := overviewOf(t, 96, 30, config.IconsUnicode, blocked)
	if !m.onOverview() {
		t.Fatalf("the modal opened on tab %d, want the Overview", m.tab)
	}
	press(t, h, "ctrl+p")
	if m.find == nil {
		t.Fatal("ctrl+p on the Overview opened no finder")
	}
	typeQuery(t, h, "yaml")
	press(t, h, "enter")
	if m.find != nil || m.tab != filesTab || m.files == nil {
		t.Fatalf("choosing a file left tab %d with files %v and finder %v, want the Files tab", m.tab, m.files, m.find)
	}
	if cur, _ := m.files.diff.CurrentFile(); cur.Path != "internal/config/default.yaml" {
		t.Errorf("the diff is in %q, want the file chosen", cur.Path)
	}
	press(t, h, "/")
	if !m.searchingDiff() {
		t.Error("/ on the Files tab, reached from the Overview, started no search")
	}
}

// onRow returns the subject of the row that has the cursor.
func onRow(m *detailModal) string {
	rows := m.attentionRows()
	return rows[m.cursorRow(rows)].subject
}

// The cursor stays on its row when the list changes around it: when the
// viewer is found, and a row is added above and another moves.
func TestOverviewCursorFollowsItsRowWhenTheViewerArrives(t *testing.T) {
	t.Parallel()
	tweak := func(d *core.PullRequestDetail) {
		blocked(d)
		d.Caps.Authored = false
		d.Merge.Mergeable, d.Merge.Status = core.MergeableConflicting, core.MergeDirty
		d.Reviewers.Requested = []core.ReviewRequest{{User: core.User{Login: "reviewer"}}}
	}
	viewer := func(context.Context) (string, error) { return "reviewer", nil }
	h, m := overviewOf(t, 100, 30, config.IconsUnicode, tweak, WithViewer(viewer))
	if got := onRow(m); got != "Conflicts with main" {
		t.Fatalf("the cursor is on %q, want the first row, the conflicts", got)
	}
	drain(t, h, m.readViewer())
	if got := attentionOf(m)[0]; got != "Review requested from you" {
		t.Fatalf("the first row is %q, want the review asked, now the viewer is known", got)
	}
	if got := onRow(m); got != "Conflicts with main" {
		t.Errorf("the cursor moved to %q, want it on the row it was on", got)
	}
}

// A detail read again changes the rows: the cursor follows its row when it
// is still there, and keeps its place when it is gone.
func TestOverviewRowsAndCursorFollowAReread(t *testing.T) {
	t.Parallel()
	h, m := overviewOf(t, 100, 30, config.IconsUnicode, blocked)
	press(t, h, "j")
	press(t, h, "j")
	press(t, h, "j")
	if got := onRow(m); got != "3 unresolved threads" {
		t.Fatalf("the cursor is on %q, want the threads", got)
	}
	reread := func(drop func(*core.PullRequestDetail)) {
		d := m.detail
		d.FailingChecks = slices.Clone(d.FailingChecks)
		drop(&d)
		h.Update(detailMsg{thread: m.thread.ID(), detail: d})
	}
	want := onRow(m)
	// A check above the cursor stops failing.
	reread(func(d *core.PullRequestDetail) { d.FailingChecks = d.FailingChecks[1:]; d.CheckCounts.Failed-- })
	if got := attentionOf(m); slices.Contains(got, "test (ubuntu-latest)") {
		t.Errorf("attention = %q, still has the check that stopped failing", got)
	}
	if got := onRow(m); got != want {
		t.Errorf("the cursor moved to %q, want it on %q", got, want)
	}
	// The row under the cursor goes: the cursor stays in its place.
	i := m.cursorRow(m.attentionRows())
	reread(func(d *core.PullRequestDetail) {
		d.Threads = core.ThreadSummary{}
		d.Reviewers.Verdicts, d.ReviewDecision = nil, core.ReviewNone
	})
	rows := m.attentionRows()
	if got := m.cursorRow(rows); got != min(i, len(rows)-1) {
		t.Errorf("the cursor is on row %d of %d, want %d or the last", got, len(rows), i)
	}
}
