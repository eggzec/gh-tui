package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

func TestMutations(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		// keys lead to the pull request, and the last one changes it.
		keys []string
		// want is the change asked of the service, "" for none, and what
		// the DoneMsg names it.
		want, what string
		// after is the state of the pull request once it is done.
		after func(pr core.PullRequest) bool
	}{
		{
			name: "merge squashes by default",
			keys: []string{"m"}, want: "merge squash 142", what: "merge #142",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateMerged },
		},
		{
			name: "merge uses the configured method",
			opts: []Option{WithMergeMethod(core.MergeRebase)},
			keys: []string{"down", "m"}, want: "merge rebase 135", what: "merge #135",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateMerged },
		},
		{
			name: "merge in the detail merges the open pull request",
			keys: []string{"down", "enter", "m"}, want: "merge squash 135", what: "merge #135",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateMerged },
		},
		{
			name: "close closes an open pull request",
			keys: []string{"x"}, want: "close 142", what: "close #142",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateClosed },
		},
		{
			name: "reopen reopens a closed pull request",
			keys: []string{"f", "X"}, want: "reopen 93", what: "reopen #93",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateOpen },
		},
		{
			name: "toggle draft converts a ready pull request",
			keys: []string{"D"}, want: "draft 142", what: "convert #142 to draft",
			after: func(pr core.PullRequest) bool { return pr.Draft },
		},
		{
			name: "toggle draft marks a draft ready",
			keys: []string{"down", "down", "D"}, want: "ready 128", what: "mark #128 ready",
			after: func(pr core.PullRequest) bool { return !pr.Draft },
		},
		{name: "close doesn't apply to a closed pull request", keys: []string{"f", "x"}},
		{name: "reopen doesn't apply to an open pull request", keys: []string{"X"}},
		{name: "merge doesn't apply to a merged pull request", keys: []string{"f", "f", "m"}},
		{name: "toggle draft doesn't apply to a merged pull request", keys: []string{"f", "f", "D"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			s := started(t, svc, 80, 20, tt.opts...)
			last := len(tt.keys) - 1
			for _, k := range tt.keys[:last] {
				press(t, s, k)
			}
			before, _ := s.target()
			msgs := press(t, s, tt.keys[last])

			var want []string
			if tt.want != "" {
				want = []string{tt.want}
			}
			if got := svc.changes(); !slices.Equal(got, want) {
				t.Fatalf("changes = %v, want %v", got, want)
			}
			if tt.want == "" {
				return
			}
			if !slices.Contains(msgs, tea.Msg(ui.DoneMsg{What: tt.what})) {
				t.Errorf("messages %v, want a DoneMsg for %q", msgs, tt.what)
			}
			if !tt.after(svc.state(before.Number)) {
				t.Errorf("#%d after = %+v", before.Number, svc.state(before.Number))
			}
		})
	}
}

func TestMutationShowsAtOnce(t *testing.T) {
	svc := newFakeService()
	s := started(t, svc, 80, 20)
	press(t, s, "enter")
	cmd := s.Update(keyMsg("m"))
	if cmd == nil {
		t.Fatal("merge returned no command")
	}
	// The detail reads the change from the cache before it is sent.
	if s.detail.State != core.StateMerged || !strings.Contains(screen(s), " Merged ") {
		t.Errorf("detail before sending:\n%s", screen(s))
	}
}

func TestFailedMutationRollsBack(t *testing.T) {
	svc := newFakeService()
	svc.sendErr = errors.New("merge conflict")
	s := started(t, svc, 80, 20)
	press(t, s, "enter")
	msgs := press(t, s, "m")
	done := slices.IndexFunc(msgs, func(m tea.Msg) bool { d, ok := m.(ui.DoneMsg); return ok && d.Err != nil })
	if done < 0 {
		t.Fatalf("messages %v, want a failed DoneMsg", msgs)
	}
	if s.detail.State != core.StateOpen || !strings.Contains(screen(s), " Open ") {
		t.Errorf("after the rollback:\n%s", screen(s))
	}
	press(t, s, "esc")
	if pr, _ := s.feed.Selected(); pr.Number != 142 || pr.State != core.StateOpen {
		t.Errorf("list shows #%d %s, want #142 open", pr.Number, pr.State)
	}
}

func TestMutationGuards(t *testing.T) {
	t.Run("nothing selected", func(t *testing.T) {
		svc := newFakeService()
		svc.pulls = nil
		s := started(t, svc, 80, 20)
		for _, k := range []string{"m", "x", "X", "D"} {
			if cmd := s.Update(keyMsg(k)); cmd != nil {
				t.Errorf("%s with nothing selected returned a command", k)
			}
		}
		if got := svc.changes(); len(got) != 0 {
			t.Errorf("changes = %v, want none", got)
		}
	})
	t.Run("merging a draft asks to mark it ready", func(t *testing.T) {
		svc := newFakeService()
		s := started(t, svc, 80, 20)
		press(t, s, "down")
		msgs := press(t, s, "down")
		msgs = append(msgs, press(t, s, "m")...)
		want := ui.NotifyMsg{Level: toast.Warning, Text: "Mark #128 ready for review before merging it."}
		if !slices.Contains(msgs, tea.Msg(want)) || len(svc.changes()) != 0 {
			t.Errorf("messages %v with changes %v, want only %v", msgs, svc.changes(), want)
		}
	})
	t.Run("before a repository", func(t *testing.T) {
		svc := newFakeService()
		s := newTest(t, svc, 80, 20)
		drain(t, s, s.Init())
		if cmd := s.Update(keyMsg("m")); cmd != nil || len(svc.changes()) != 0 {
			t.Error("merge without a repository did something")
		}
	})
}

func TestDoneMsgReloads(t *testing.T) {
	svc := newFakeService()
	s := started(t, svc, 80, 20)
	n := len(svc.listed())
	drain(t, s, s.Update(ui.DoneMsg{What: "merge #1"}))
	if len(svc.listed()) != n+1 {
		t.Errorf("listed %d times after DoneMsg, want %d", len(svc.listed()), n+1)
	}
}

func TestHelpOffersWhatApplies(t *testing.T) {
	enabled := func(s *Section) []string {
		var d []string
		for _, b := range s.Help().ShortHelp() {
			if b.Enabled() {
				d = append(d, b.Help().Desc)
			}
		}
		return d
	}
	svc := newFakeService()
	s := started(t, svc, 80, 20)
	if got := enabled(s); !slices.Contains(got, "merge") || !slices.Contains(got, "close") || slices.Contains(got, "reopen") {
		t.Errorf("open help = %v, want merge and close", got)
	}
	press(t, s, "f")
	if got := enabled(s); slices.Contains(got, "merge") || !slices.Contains(got, "reopen") {
		t.Errorf("closed help = %v, want reopen only", got)
	}
}
