package pulls

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// modalScreen is the view of the open modal with styles removed.
func modalScreen(tb testing.TB, h *host) string {
	tb.Helper()
	m := h.modal()
	if m == nil {
		tb.Fatal("no modal is open")
	}
	return ansi.Strip(m.View())
}

func TestModal(t *testing.T) {
	tests := []struct {
		name  string
		setup func(svc *fakeService)
		keys  []string
		check func(t *testing.T, h *host, svc *fakeService, msgs []any)
	}{
		{
			name: "select opens the selected pull request with its comments",
			keys: []string{"down", "enter"},
			check: func(t *testing.T, h *host, svc *fakeService, _ []any) {
				t.Helper()
				m := h.modal()
				if m == nil || m.number != 135 {
					t.Fatalf("modal = %v, want #135 open", m)
				}
				if got, want := m.Title(), "#135 Retry GraphQL requests after secondary rate limits"; got != want {
					t.Errorf("title = %q, want %q", got, want)
				}
				if got := svc.got(); !slices.Equal(got, []int{135}) {
					t.Errorf("got details %v, want [135]", got)
				}
				view := modalScreen(t, h)
				for _, want := range []string{"Retry GraphQL requests", "Cold starts read every page", "Does this survive a crash"} {
					if !strings.Contains(view, want) {
						t.Errorf("detail doesn't show %q:\n%s", want, view)
					}
				}
				if !strings.Contains(screen(h), "#142") {
					t.Errorf("the list behind the modal is gone:\n%s", screen(h))
				}
			},
		},
		{
			name: "esc closes the modal and cancels its reads, keeping the selection",
			keys: []string{"down", "down", "enter", "esc"},
			check: func(t *testing.T, h *host, _ *fakeService, _ []any) {
				t.Helper()
				if h.modal() != nil {
					t.Fatal("modal still open")
				}
				if pr, _ := h.feed.Selected(); pr.Number != 128 || !h.feed.Focused() {
					t.Errorf("selected #%d, focused %v; want #128 focused", pr.Number, h.feed.Focused())
				}
			},
		},
		{
			name: "a cached detail shows at once",
			setup: func(svc *fakeService) {
				svc.cached[142] = true
				svc.getErr = errors.New("offline")
			},
			keys: []string{"enter"},
			check: func(t *testing.T, h *host, _ *fakeService, msgs []any) {
				t.Helper()
				if !strings.Contains(modalScreen(t, h), "Cold starts read every page") {
					t.Errorf("cached body not shown:\n%s", modalScreen(t, h))
				}
				if len(msgs) == 0 {
					t.Error("a failed revalidation should notify")
				}
			},
		},
		{
			name:  "a failed detail keeps the list item and notifies",
			setup: func(svc *fakeService) { svc.getErr = errors.New("502 Bad Gateway") },
			keys:  []string{"enter"},
			check: func(t *testing.T, h *host, _ *fakeService, msgs []any) {
				t.Helper()
				if !strings.Contains(modalScreen(t, h), "Add a disk layer to the cache") {
					t.Errorf("header missing:\n%s", modalScreen(t, h))
				}
				f, ok := msgs[0].(ui.FailMsg)
				if len(msgs) != 1 || !ok || f.What != "load #142" || core.Explain(f.What, f.Err).Subject != "eggzec/gh-tui#142" {
					t.Errorf("messages %v, want the failed load of eggzec/gh-tui#142", msgs)
				}
			},
		},
		{
			name: "refresh invalidates the repository, then fetches the detail and the comments again",
			keys: []string{"enter", "r"},
			check: func(t *testing.T, _ *host, svc *fakeService, _ []any) {
				t.Helper()
				if got, want := svc.invalidations(), []invalidation{{repo: repo, lists: 1, gets: 1}}; !slices.Equal(got, want) {
					t.Errorf("invalidations = %+v, want %+v", got, want)
				}
				if got := svc.got(); len(got) != 2 {
					t.Errorf("got details %v, want two", got)
				}
				if n := len(svc.comments); n < 3 {
					t.Errorf("fetched %d comment pages, want the two again", n)
				}
			},
		},
		{
			name: "open in browser opens the open pull request",
			keys: []string{"down", "down", "enter", "o"},
			check: func(t *testing.T, _ *host, _ *fakeService, msgs []any) {
				t.Helper()
				want := ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/pull/128"}
				if !slices.Contains(msgs, any(want)) {
					t.Errorf("messages %v, want %v", msgs, want)
				}
			},
		},
		{
			name: "list keys don't reach the list under the modal",
			keys: []string{"enter", "f", "down"},
			check: func(t *testing.T, h *host, _ *fakeService, _ []any) {
				t.Helper()
				if h.tab != core.StateOpen || h.modal() == nil {
					t.Errorf("tab = %s with modal open %v, want open with the modal", h.tab, h.modal() != nil)
				}
				if pr, _ := h.feed.Selected(); pr.Number != 142 {
					t.Errorf("the list moved to #%d under the modal", pr.Number)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			if tt.setup != nil {
				tt.setup(svc)
			}
			h := started(t, svc, 80, 40)
			var msgs []any
			for _, k := range tt.keys {
				for _, m := range press(t, h, k) {
					switch m.(type) {
					case ui.OpenMsg, ui.NotifyMsg, ui.FailMsg:
						msgs = append(msgs, m)
					}
				}
			}
			tt.check(t, h, svc, msgs)
		})
	}
}

func TestCachedModalOpensAtOnce(t *testing.T) {
	svc := newFakeService()
	svc.cached[142] = true
	svc.commented[commentsQuery(repo, 142)] = true
	h := started(t, svc, 80, 40)
	// Open without running the loads: what the modal shows comes from the
	// cache alone.
	seq, ok := sequence(h.Update(keyMsg("enter"))())
	if !ok || len(seq) != 2 {
		t.Fatal("enter should open the modal, then start its loads")
	}
	drain(t, h, seq[0])
	view := modalScreen(t, h)
	for _, want := range []string{"Cold starts read every page", "Does this survive a crash", "It writes to a temporary file"} {
		if !strings.Contains(view, want) {
			t.Errorf("modal lacks %q before any load:\n%s", want, view)
		}
	}
	// Only the second page of comments, which isn't cached, may load.
	spinner := strings.Index(view, "Loading comments")
	if strings.Contains(view, "Loading…") || spinner >= 0 && spinner < strings.Index(view, "It writes to a temporary file") {
		t.Errorf("modal waits for what is cached:\n%s", view)
	}
	// Behind it, the detail and the comments are read again.
	drain(t, h, seq[1])
	if got := svc.got(); !slices.Equal(got, []int{142}) {
		t.Errorf("got details %v, want #142 revalidated", got)
	}
	if !slices.Contains(svc.comments, commentsQuery(repo, 142)) {
		t.Errorf("comment reads %v, want the first page revalidated", svc.comments)
	}
}

func TestCloseCancelsAndIgnoresLateResults(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 30)
	// Open without running the loads, so their results arrive late.
	seq, ok := sequence(h.Update(keyMsg("enter"))())
	if !ok || len(seq) != 2 {
		t.Fatal("enter should open the modal, then start its loads")
	}
	drain(t, h, seq[0])
	if h.modal() == nil {
		t.Fatal("the modal isn't open before its loads start")
	}
	var get tea.Cmd
	for _, c := range seq[1]().(tea.BatchMsg) {
		if c == nil {
			continue
		}
		if msg := c(); msg != nil {
			if _, ok := msg.(detailMsg); ok {
				get = func() tea.Msg { return msg }
				continue
			}
			drain(t, h, func() tea.Msg { return msg })
		}
	}
	m := h.modal()
	if m == nil || get == nil {
		t.Fatal("enter didn't open a modal that loads its detail")
	}
	press(t, h, "esc")
	if h.modal() != nil || m.ctx.Err() == nil {
		t.Fatal("esc should close the modal and cancel its reads")
	}
	svc.mu.Lock()
	svc.pulls[0].Title = "Changed later"
	svc.mu.Unlock()
	if cmd := m.Update(get()); cmd != nil {
		t.Error("a closed modal acted on a late detail")
	}
	if strings.Contains(ansi.Strip(m.View()), "Changed later") {
		t.Error("a closed modal showed a late detail")
	}
}

func TestOpenFromSearch(t *testing.T) {
	other := core.RepoRef{Owner: "cli", Name: "cli"}
	t.Run("another repository", func(t *testing.T) {
		svc := newFakeService()
		svc.pulls = append(svc.pulls, core.PullRequest{
			Repo: other, Number: 7, Title: "Speed up gh pr list", State: core.StateOpen,
			URL: "https://github.com/cli/cli/pull/7",
		})
		h := started(t, svc, 80, 30)
		msgs := drain(t, h, h.Update(ui.OpenPullMsg{Repo: other, Number: 7}))
		m := h.modal()
		if m == nil || m.repo != other || m.number != 7 {
			t.Fatalf("modal = %+v, want cli/cli#7", m)
		}
		// The title names the repository, which isn't the one selected.
		if got := m.Title(); got != "cli/cli#7 Speed up gh pr list" {
			t.Errorf("title = %q", got)
		}
		if !strings.Contains(modalScreen(t, h), "Speed up gh pr list") {
			t.Errorf("modal:\n%s", modalScreen(t, h))
		}
		if slices.ContainsFunc(msgs, func(m tea.Msg) bool { _, ok := m.(ui.NotifyMsg); return ok }) {
			t.Errorf("messages %v, want no error", msgs)
		}
		if h.repo != repo {
			t.Errorf("the section moved to %v", h.repo)
		}
		// The modal's changes go to its repository.
		press(t, h, "x")
		press(t, h, "y")
		if got := svc.changes(); !slices.Equal(got, []string{"close 7"}) {
			t.Errorf("changes = %v, want close 7", got)
		}
	})
	t.Run("the selected repository away from its screen", func(t *testing.T) {
		h := started(t, newFakeService(), 80, 30)
		drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 135, ShowRepo: true}))
		if m := h.modal(); m == nil || !strings.HasPrefix(m.Title(), "eggzec/gh-tui#135 ") {
			t.Errorf("modal = %v, want its title to name eggzec/gh-tui", m)
		}
	})
	t.Run("before the section starts", func(t *testing.T) {
		svc := newFakeService()
		h := newTest(t, svc, 80, 30)
		drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 135}))
		if m := h.modal(); m == nil || !strings.Contains(modalScreen(t, h), "Retry GraphQL requests") {
			t.Fatal("OpenPullMsg before Init didn't open the pull request")
		}
		if len(svc.listed()) != 0 {
			t.Error("opening from the search listed the section's pull requests")
		}
	})
	t.Run("issues are not for this section", func(t *testing.T) {
		h := started(t, newFakeService(), 80, 30)
		if cmd := h.Update(ui.OpenIssueMsg{Repo: repo, Number: 135}); cmd != nil {
			drain(t, h, cmd)
		}
		if h.modal() != nil {
			t.Error("OpenIssueMsg opened a pull request")
		}
	})
}

func TestModalBeforeItsDetail(t *testing.T) {
	svc := newFakeService()
	svc.getErr = errors.New("offline")
	h := newTest(t, svc, 80, 20)
	drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 135}))
	m := h.modal()
	// No repository is selected, so the title names it.
	if m == nil || m.Title() != "eggzec/gh-tui#135" {
		t.Fatalf("modal = %v, want eggzec/gh-tui#135 without a title", m)
	}
	for _, k := range []string{"m", "x", "X", "D", "o"} {
		if msgs := press(t, h, k); len(msgs) != 0 {
			t.Errorf("%s before the detail sent %v", k, msgs)
		}
	}
	if len(svc.changes()) != 0 {
		t.Errorf("changes = %v, want none", svc.changes())
	}
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 20 {
		t.Errorf("view has %d lines, want 20", len(lines))
	}
}

func TestRepoMsgKeepsModal(t *testing.T) {
	h := started(t, newFakeService(), 80, 20)
	press(t, h, "enter")
	drain(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
	if m := h.modal(); m == nil || m.repo != repo {
		t.Error("a new repository should leave the open pull request as it is")
	}
}

func TestHelpFollowsTheView(t *testing.T) {
	h := started(t, newFakeService(), 80, 20)
	if got := uitest.Enabled(h.KeyLayers()); !slices.Contains(got, "filter") || slices.Contains(got, "back") {
		t.Errorf("list help = %v, want filter and no back", got)
	}
	press(t, h, "enter")
	if got := uitest.Enabled(h.modal().KeyLayers()); slices.Contains(got, "filter") || !slices.Contains(got, "back") {
		t.Errorf("modal help = %v, want back and no filter", got)
	}
	// The bubbles lose the keys the section takes.
	for _, l := range h.modal().KeyLayers() {
		for _, b := range l.Bindings {
			if b.Enabled() && b.Help().Desc != "refresh" && slices.Contains(b.Keys(), "r") {
				t.Errorf("%q also claims r", b.Help().Desc)
			}
		}
	}
	if key.Matches(keyMsg("f"), h.keys.feed.PageDown) {
		t.Error("the feed's page down still takes the filter key")
	}
}

func TestCommentsWrapWithinWidth(t *testing.T) {
	h := started(t, newFakeService(), 40, 20)
	press(t, h, "enter")
	c := core.Comment{Author: core.User{Login: "octocat"}, Body: strings.Repeat("word ", 40), CreatedAt: clock}
	for l := range strings.SplitSeq(h.modal().renderComment(c, 40), "\n") {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("line is %d wide: %q", w, ansi.Strip(l))
		}
	}
}

// The comments say what went wrong the way the user should read it,
// naming the pull request, without the error's chain, request or status
// code.
func TestCommentsErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"offline", fmt.Errorf("pull comments: github: POST /graphql: %w", core.ErrOffline), "✗ Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("pull comments: github: 403 Forbidden: %w", core.ErrForbidden), "✗ You don't have access to eggzec/gh-tui · o to open on GitHub"},
		{"internal", errors.New("pull comments: github: decode: unexpected EOF"), "✗ Something went wrong · r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			svc.commentsErr = tt.err
			h := started(t, svc, 100, 30)
			press(t, h, "enter")
			if v := modalScreen(t, h); !strings.Contains(v, tt.want) || strings.Contains(v, "github:") || strings.Contains(v, "403") || strings.Contains(v, "/graphql") {
				t.Errorf("modal = %q, want %q", v, tt.want)
			}
		})
	}
}

// failed returns the action and error of the failure in msgs, a FailMsg
// or a DoneMsg with an error, or "" and nil when there is none.
func failed(msgs []tea.Msg) (what string, err error) {
	for _, m := range msgs {
		switch m := m.(type) {
		case ui.FailMsg:
			return m.What, m.Err
		case ui.DoneMsg:
			if m.Err != nil {
				return m.What, m.Err
			}
		}
	}
	return "", nil
}

// The toast of a failed load or change of the modal names the pull
// request, never the repository, and shows nothing of the error's chain.
func TestModalFailureToasts(t *testing.T) {
	for _, f := range uitest.Failures() {
		t.Run(f.Name+"/load", func(t *testing.T) {
			svc := newFakeService()
			svc.getErr = f.Err
			h := started(t, svc, 80, 20)
			what, err := failed(press(t, h, "enter"))
			uitest.CheckToast(t, f, "load #142", "eggzec/gh-tui#142", uitest.Toast(what, err))
		})
		t.Run(f.Name+"/merge", func(t *testing.T) {
			svc := newFakeService()
			svc.sendErr = f.Err
			h := started(t, svc, 80, 20)
			press(t, h, "enter")
			press(t, h, "m")
			what, err := failed(press(t, h, "y"))
			uitest.CheckToast(t, f, "merge #142", "eggzec/gh-tui#142", uitest.Toast(what, err))
		})
		t.Run(f.Name+"/close from the list", func(t *testing.T) {
			svc := newFakeService()
			svc.sendErr = f.Err
			h := started(t, svc, 80, 20)
			press(t, h, "x")
			what, err := failed(press(t, h, "y"))
			uitest.CheckToast(t, f, "close #142", "eggzec/gh-tui#142", uitest.Toast(what, err))
		})
	}
}
