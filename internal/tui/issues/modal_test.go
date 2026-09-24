package issues

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// opened returns a section 80 cells wide with the modal of issue #999 open,
// with comments. The modal is as large as the section.
func opened(t *testing.T, svc *fakeService, height int) (*host, *detailModal) {
	t.Helper()
	svc.addComments(999, sampleComments(3)...)
	h := started(t, svc, 80, height)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	return h, m
}

func TestCachedModalOpensAtOnce(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(1000, sampleComments(3)...)
	h := started(t, svc, 80, 40)
	// Read ahead, as the section does.
	if _, err := svc.Get(t.Context(), testRepo, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Comments(t.Context(), commentsQuery(testRepo, 1000)); err != nil {
		t.Fatal(err)
	}
	gets, reads := len(svc.getCalls()), len(svc.commentQueries)

	// Open without running the loads: what the modal shows comes from the
	// cache alone.
	seq, ok := sequence(h.Update(keyMsg("enter"))())
	if !ok || len(seq) != 2 {
		t.Fatal("enter should open the modal, then start its loads")
	}
	run(t, h, seq[0])
	view := ansi.Strip(h.modal().View())
	for _, want := range []string{"Look at issue 1000", "I can reproduce this", "Thanks! Fixed on main."} {
		if !strings.Contains(view, want) {
			t.Errorf("modal lacks %q before any load:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Loading") {
		t.Errorf("modal shows a spinner though everything is cached:\n%s", view)
	}
	// Behind it, the issue and the comments are read again.
	run(t, h, seq[1])
	if n := len(svc.getCalls()); n != gets+1 {
		t.Errorf("%d more Gets, want 1 to revalidate", n-gets)
	}
	if n := len(svc.commentQueries); n != reads+1 {
		t.Errorf("%d more comment reads, want 1 to revalidate", n-reads)
	}
}

func TestOpenAndClose(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(1000, core.Comment{ID: "IC_x", Body: "A comment on another issue."})
	h, m := opened(t, svc, 30)
	v := ansi.Strip(m.View())
	if strings.Contains(v, "another issue") {
		t.Errorf("modal of #999 shows a comment on #1000:\n%s", v)
	}
	for _, want := range []string{"Support GitHub Enterprise hosts", "● Open", "hubot opened", "I can reproduce this", "Thanks! Fixed on main."} {
		if !strings.Contains(v, want) {
			t.Errorf("modal lacks %q:\n%s", want, v)
		}
	}
	if got, want := m.Title(), "#999 Support GitHub Enterprise hosts in the repository picker"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	if got := svc.getCalls(); !slices.Equal(got, []int{999}) {
		t.Errorf("Get calls = %v, want [999]", got)
	}
	for _, q := range svc.commentQueries {
		if q.Number != 999 || q.Repo != testRepo || q.PageSize != 0 {
			t.Errorf("comments query %+v, want issue 999 at the default page size", q)
		}
	}
	if !m.thread.Focused() {
		t.Error("the thread should have the focus in the modal")
	}
	if !strings.Contains(ansi.Strip(h.View()), "#1000") {
		t.Errorf("the list behind the modal is gone:\n%s", ansi.Strip(h.View()))
	}

	press(t, h, "esc")
	if h.modal() != nil {
		t.Fatal("esc didn't close the modal")
	}
	if it, _ := h.list.Selected(); it.Number != 999 {
		t.Errorf("selection after closing = #%d, want #999", it.Number)
	}
	if !h.list.Focused() || m.ctx.Err() == nil {
		t.Error("closing should leave the list focused and cancel the modal's fetches")
	}
}

func TestModalKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
		// check inspects the section and the messages that followed.
		check func(t *testing.T, h *host, svc *fakeService, msgs []ui.OpenMsg)
	}{
		{"refresh invalidates the repository, then reloads the thread and the issue", "r", func(t *testing.T, _ *host, svc *fakeService, _ []ui.OpenMsg) {
			t.Helper()
			if got := svc.invalidations(); len(got) != 1 || got[0].repo != testRepo || got[0].gets != 1 {
				t.Errorf("invalidations = %+v, want one of %v after the first Get", got, testRepo)
			}
			if got := svc.getCalls(); len(got) != 2 {
				t.Errorf("Get calls = %v, want two", got)
			}
			if got := len(svc.commentQueries); got != 2 {
				t.Errorf("comment pages read = %d, want 2", got)
			}
		}},
		{"open in browser opens the issue", "o", func(t *testing.T, _ *host, _ *fakeService, msgs []ui.OpenMsg) {
			t.Helper()
			if len(msgs) != 1 || msgs[0].URL != "https://github.com/eggzec/gh-tui/issues/999" {
				t.Errorf("opened %v, want issue 999", msgs)
			}
		}},
		{"the filter key scrolls instead", "f", func(t *testing.T, h *host, _ *fakeService, _ []ui.OpenMsg) {
			t.Helper()
			if h.filter != core.FilterOpen || h.modal() == nil {
				t.Errorf("f changed the filter to %q under the modal", h.filter)
			}
		}},
		{"select does nothing", "enter", func(t *testing.T, h *host, svc *fakeService, _ []ui.OpenMsg) {
			t.Helper()
			if len(h.modals) != 1 || len(svc.getCalls()) != 1 {
				t.Error("enter in the modal opened something")
			}
		}},
		{"moving doesn't move the list", "down", func(t *testing.T, h *host, _ *fakeService, _ []ui.OpenMsg) {
			t.Helper()
			if it, _ := h.list.Selected(); it.Number != 999 {
				t.Errorf("the list moved to #%d under the modal", it.Number)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			h, _ := opened(t, svc, 12)
			var opens []ui.OpenMsg
			for _, m := range press(t, h, tt.key) {
				if o, ok := m.(ui.OpenMsg); ok {
					opens = append(opens, o)
				}
			}
			tt.check(t, h, svc, opens)
		})
	}
}

func TestOpenNothing(t *testing.T) {
	h := started(t, newFakeService(nil), 80, 10)
	if cmd := h.Update(keyMsg("enter")); cmd != nil || h.modal() != nil {
		t.Error("enter on an empty list opened something")
	}
}

func TestOpenUsesCachedIssue(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	cached := sampleIssues(2)[1]
	cached.Body = "The cached body."
	svc.cached[999] = cached
	svc.getErr = errors.New("offline")
	h := started(t, svc, 80, 20)
	msgs := press(t, h, "down", "enter")
	if v := ansi.Strip(h.modal().View()); !strings.Contains(v, "The cached body.") {
		t.Errorf("modal doesn't show the cached issue:\n%s", v)
	}
	n, ok := has[ui.NotifyMsg](msgs)
	if !ok || !strings.Contains(n.Text, "offline") {
		t.Errorf("a failed Get sent %v, want an error toast", msgs)
	}
}

func TestCloseIgnoresLateResults(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)
	press(t, h, "down")
	// Open, holding back the Get, so its result arrives after closing.
	seq, ok := sequence(h.Update(keyMsg("enter"))())
	if !ok || len(seq) != 2 {
		t.Fatal("enter should open the modal, then start its loads")
	}
	run(t, h, seq[0])
	if h.modal() == nil {
		t.Fatal("the modal isn't open before its loads start")
	}
	var late tea.Msg
	for _, c := range seq[1]().(tea.BatchMsg) {
		if c == nil {
			continue
		}
		msg := c()
		if _, ok := msg.(issueMsg); ok {
			late = msg
			continue
		}
		run(t, h, func() tea.Msg { return msg })
	}
	m := h.modal()
	if m == nil || late == nil {
		t.Fatal("enter didn't open a modal that reads its issue")
	}
	press(t, h, "esc")
	if h.modal() != nil || m.ctx.Err() == nil {
		t.Fatal("esc should close the modal and cancel its reads")
	}
	if cmd := m.Update(late); cmd != nil {
		t.Error("a closed modal acted on a late issue")
	}
}

func TestOpenFromSearch(t *testing.T) {
	other := core.RepoRef{Owner: "cli", Name: "cli"}
	t.Run("another repository", func(t *testing.T) {
		svc := newFakeService(sampleIssues(12))
		svc.issues = append(svc.issues, core.Issue{
			Repo: other, Number: 7, Title: "gh issue view hangs", State: core.StateOpen,
			Body: "It never returns.", URL: "https://github.com/cli/cli/issues/7",
		})
		h := started(t, svc, 80, 20)
		msgs := run(t, h, h.Update(ui.OpenIssueMsg{Repo: other, Number: 7}))
		m := h.modal()
		if m == nil || m.repo != other || m.number != 7 {
			t.Fatalf("modal = %+v, want cli/cli#7", m)
		}
		if got := m.Title(); got != "#7 gh issue view hangs" {
			t.Errorf("title = %q", got)
		}
		if v := ansi.Strip(m.View()); !strings.Contains(v, "It never returns.") {
			t.Errorf("modal:\n%s", v)
		}
		if _, ok := has[ui.NotifyMsg](msgs); ok {
			t.Errorf("messages %v, want no error", msgs)
		}
		if h.repo != testRepo {
			t.Errorf("the section moved to %v", h.repo)
		}
		press(t, h, "x")
		if got := svc.changeCalls(); !slices.Equal(got, []string{"close 7"}) {
			t.Errorf("changes = %v, want close 7", got)
		}
	})
	t.Run("before the section starts", func(t *testing.T) {
		svc := newFakeService(sampleIssues(12))
		h := newSection(t, svc, 80, 20)
		run(t, h, h.Update(ui.OpenIssueMsg{Repo: testRepo, Number: 999}))
		m := h.modal()
		if m == nil || !strings.Contains(ansi.Strip(m.View()), "Support GitHub Enterprise hosts") {
			t.Fatal("OpenIssueMsg before Init didn't open the issue")
		}
		if len(svc.listCalls()) != 0 {
			t.Error("opening from the search listed the section's issues")
		}
	})
	t.Run("pull requests are not for this section", func(t *testing.T) {
		h := started(t, newFakeService(sampleIssues(12)), 80, 20)
		run(t, h, h.Update(ui.OpenPullMsg{Repo: testRepo, Number: 999}))
		if h.modal() != nil {
			t.Error("OpenPullMsg opened an issue")
		}
	})
	t.Run("keys wait for the issue", func(t *testing.T) {
		svc := newFakeService(sampleIssues(12))
		svc.getErr = errors.New("offline")
		h := newSection(t, svc, 80, 20)
		run(t, h, h.Update(ui.OpenIssueMsg{Repo: testRepo, Number: 999}))
		m := h.modal()
		if m == nil || m.Title() != "#999" {
			t.Fatalf("modal = %v, want #999 without a title", m)
		}
		press(t, h, "x", "c", "l")
		if len(svc.changeCalls()) != 0 || m.composing != composeNone {
			t.Error("keys acted before the issue arrived")
		}
		assertFits(t, m.View(), 80, 20)
	})
}

func TestRepoMsgKeepsModal(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 20)
	run(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "o", Name: "r"}}))
	if h.modal() != m || m.repo != testRepo {
		t.Error("a new repository should leave the open issue as it is")
	}
}

func TestPendingComment(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999,
		core.Comment{ID: "pending:1", Author: core.User{Login: "me"}, Body: "On it."},
		core.Comment{ID: "pending:2", Body: "Sent without knowing who I am."},
	)
	_, m := opened(t, svc, 30)
	v := ansi.Strip(m.View())
	for _, want := range []string{"me · sending…", "On it.", "you · sending…", "Sent without knowing"} {
		if !strings.Contains(v, want) {
			t.Errorf("pending comments lack %q:\n%s", want, v)
		}
	}
}

// enabled returns the descriptions of the enabled keys in short help.
func enabled(km interface{ ShortHelp() []key.Binding }) []string {
	var out []string
	for _, b := range km.ShortHelp() {
		if b.Enabled() {
			out = append(out, b.Help().Desc)
		}
	}
	return out
}

func TestHelp(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 20)
	if got := enabled(m.Help()); !slices.Contains(got, "back") || slices.Contains(got, "filter") {
		t.Errorf("modal help = %v, want back and no filter", got)
	}
	if got := enabled(h.Help()); slices.Contains(got, "back") || !slices.Contains(got, "filter") {
		t.Errorf("list help = %v, want filter and no back", got)
	}
	// No binding in help shares a key with another.
	for _, km := range []interface{ FullHelp() [][]key.Binding }{h.Help(), m.Help()} {
		seen := map[string]string{}
		for _, group := range km.FullHelp() {
			for _, b := range group {
				for _, k := range b.Keys() {
					if prev, ok := seen[k]; ok && prev != b.Help().Desc {
						t.Errorf("key %q is both %q and %q", k, prev, b.Help().Desc)
					}
					seen[k] = b.Help().Desc
				}
			}
		}
	}
	if !key.Matches(keyMsg("f"), h.keys.Filter) || key.Matches(keyMsg("f"), h.keys.feed.PageDown) {
		t.Error("f should filter the list, not page it")
	}
}

func TestViewModal(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 24)
	t.Run("header", func(t *testing.T) {
		golden.RequireEqual(t, m.header(m.issue))
	})
	t.Run("thread", func(t *testing.T) {
		v := m.View()
		assertFits(t, v, 80, 24)
		golden.RequireEqual(t, v)
	})
	t.Run("comment prompt", func(t *testing.T) {
		press(t, h, "c")
		typeText(t, h, "Thanks, I can reproduce it.")
		defer press(t, h, "esc")
		v := m.View()
		assertFits(t, v, 80, 24)
		golden.RequireEqual(t, v)
	})
	t.Run("labels prompt", func(t *testing.T) {
		press(t, h, "l")
		defer press(t, h, "esc")
		v := m.View()
		assertFits(t, v, 80, 24)
		golden.RequireEqual(t, v)
	})
	t.Run("closed header", func(t *testing.T) {
		it := m.issue
		it.State = core.StateClosed
		it.Labels, it.Assignees = nil, nil
		golden.RequireEqual(t, m.header(it))
	})
}
