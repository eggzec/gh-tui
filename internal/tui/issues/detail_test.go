package issues

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// opened returns a section 80 cells wide showing issue #999 with comments.
func opened(t *testing.T, svc *fakeService, height int) *Section {
	t.Helper()
	svc.addComments(999, sampleComments(3)...)
	s := started(t, svc, 80, height)
	press(t, s, "down", "enter")
	if !s.inDetail {
		t.Fatal("enter didn't open the issue")
	}
	return s
}

func TestOpenAndBack(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(1000, core.Comment{ID: "IC_x", Body: "A comment on another issue."})
	s := opened(t, svc, 30)
	v := ansi.Strip(s.View())
	if strings.Contains(v, "another issue") {
		t.Errorf("detail of #999 shows a comment on #1000:\n%s", v)
	}
	for _, want := range []string{"Support GitHub Enterprise hosts", "● Open", "hubot opened", "I can reproduce this", "Thanks! Fixed on main."} {
		if !strings.Contains(v, want) {
			t.Errorf("detail lacks %q:\n%s", want, v)
		}
	}
	if got := svc.getCalls(); !slices.Equal(got, []int{999}) {
		t.Errorf("Get calls = %v, want [999]", got)
	}
	for _, q := range svc.commentQueries {
		if q.Number != 999 || q.Repo != testRepo || q.PageSize != 0 {
			t.Errorf("comments query %+v, want issue 999 at the default page size", q)
		}
	}
	if s.list.Focused() || !s.detail.Focused() {
		t.Error("the thread should have the focus in the detail")
	}

	press(t, s, "esc")
	if s.inDetail {
		t.Fatal("esc didn't go back")
	}
	if it, _ := s.list.Selected(); it.Number != 999 {
		t.Errorf("selection after back = #%d, want #999", it.Number)
	}
	if !s.list.Focused() || s.detailCtx.Err() == nil {
		t.Error("back should focus the list and cancel the thread's fetches")
	}
	if strings.Contains(ansi.Strip(s.View()), "Thanks! Fixed") {
		t.Error("the list still shows the thread")
	}
}

func TestDetailKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
		// check inspects the section and the messages that followed.
		check func(t *testing.T, s *Section, svc *fakeService, msgs []ui.OpenMsg)
	}{
		{"refresh invalidates the repository, then reloads the thread and the issue", "r", func(t *testing.T, _ *Section, svc *fakeService, _ []ui.OpenMsg) {
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
		{"open in browser opens the issue", "o", func(t *testing.T, _ *Section, _ *fakeService, msgs []ui.OpenMsg) {
			t.Helper()
			if len(msgs) != 1 || msgs[0].URL != "https://github.com/eggzec/gh-tui/issues/999" {
				t.Errorf("opened %v, want issue 999", msgs)
			}
		}},
		{"the filter key scrolls instead", "f", func(t *testing.T, s *Section, _ *fakeService, _ []ui.OpenMsg) {
			t.Helper()
			if s.filter != core.FilterOpen || !s.inDetail {
				t.Errorf("f changed the filter to %q in the detail", s.filter)
			}
		}},
		{"select does nothing", "enter", func(t *testing.T, s *Section, svc *fakeService, _ []ui.OpenMsg) {
			t.Helper()
			if !s.inDetail || len(svc.getCalls()) != 1 {
				t.Error("enter in the detail opened something")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			s := opened(t, svc, 12)
			var opens []ui.OpenMsg
			for _, m := range press(t, s, tt.key) {
				if o, ok := m.(ui.OpenMsg); ok {
					opens = append(opens, o)
				}
			}
			tt.check(t, s, svc, opens)
		})
	}
}

func TestOpenNothing(t *testing.T) {
	s := started(t, newFakeService(nil), 80, 10)
	if cmd := s.Update(keyMsg("enter")); cmd != nil || s.inDetail {
		t.Error("enter on an empty list opened something")
	}
}

func TestOpenUsesCachedIssue(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	cached := sampleIssues(2)[1]
	cached.Body = "The cached body."
	svc.cached[999] = cached
	svc.getErr = errors.New("offline")
	s := started(t, svc, 80, 20)
	msgs := press(t, s, "down", "enter")
	if !strings.Contains(ansi.Strip(s.View()), "The cached body.") {
		t.Errorf("detail doesn't show the cached issue:\n%s", ansi.Strip(s.View()))
	}
	n, ok := has[ui.NotifyMsg](msgs)
	if !ok || !strings.Contains(n.Text, "offline") {
		t.Errorf("a failed Get sent %v, want an error toast", msgs)
	}
}

func TestRepoMsgInDetail(t *testing.T) {
	s := opened(t, newFakeService(sampleIssues(12)), 20)
	run(t, s, s.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "o", Name: "r"}}))
	if s.inDetail {
		t.Error("a new repository kept the detail of another")
	}
}

func TestPendingComment(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, core.Comment{ID: "pending:1", Author: core.User{Login: "me"}, Body: "On it."})
	s := opened(t, svc, 30)
	v := ansi.Strip(s.View())
	if !strings.Contains(v, "me · sending…") || !strings.Contains(v, "On it.") {
		t.Errorf("pending comment not shown as sending:\n%s", v)
	}
}

func TestHelp(t *testing.T) {
	s := opened(t, newFakeService(sampleIssues(12)), 20)
	descs := func() []string {
		var out []string
		for _, b := range s.Help().ShortHelp() {
			if b.Enabled() {
				out = append(out, b.Help().Desc)
			}
		}
		return out
	}
	if got := descs(); !slices.Contains(got, "back") || slices.Contains(got, "filter") {
		t.Errorf("detail help = %v, want back and no filter", got)
	}
	press(t, s, "esc")
	if got := descs(); slices.Contains(got, "back") || !slices.Contains(got, "filter") {
		t.Errorf("list help = %v, want filter and no back", got)
	}
	// No binding in help shares a key with another.
	seen := map[string]string{}
	for _, group := range s.Help().FullHelp() {
		for _, b := range group {
			for _, k := range b.Keys() {
				if prev, ok := seen[k]; ok && prev != b.Help().Desc {
					t.Errorf("key %q is both %q and %q", k, prev, b.Help().Desc)
				}
				seen[k] = b.Help().Desc
			}
		}
	}
	if !key.Matches(keyMsg("f"), s.keys.Filter) || key.Matches(keyMsg("f"), s.keys.feed.PageDown) {
		t.Error("f should filter the list, not page it")
	}
}

func TestViewDetail(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := opened(t, svc, 24)
	t.Run("header", func(t *testing.T) {
		golden.RequireEqual(t, s.header(s.issue))
	})
	t.Run("thread", func(t *testing.T) {
		v := s.View()
		assertFits(t, v, 80, 24)
		golden.RequireEqual(t, v)
	})
	t.Run("closed header", func(t *testing.T) {
		it := s.issue
		it.State = core.StateClosed
		it.Labels, it.Assignees = nil, nil
		golden.RequireEqual(t, s.header(it))
	})
}
