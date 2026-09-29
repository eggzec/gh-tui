package pulls

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

func TestMutations(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		// keys lead to the pull request, and the last one changes it.
		keys []string
		// question is what the change asks before it is made, "" for a
		// change made at once.
		question string
		// want is the change asked of the service, "" for none, and what
		// the DoneMsg names it.
		want, what string
		// after is the state of the pull request once it is done.
		after func(pr core.PullRequest) bool
	}{
		{
			name: "merge squashes by default",
			keys: []string{"m"}, question: "Squash-merge #142 into main?",
			want: "merge squash 142", what: "merge #142",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateMerged },
		},
		{
			name: "merge uses the configured method",
			opts: []Option{WithMergeMethod(core.MergeRebase)},
			keys: []string{"down", "m"}, question: "Rebase-merge #135 into main?",
			want: "merge rebase 135", what: "merge #135",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateMerged },
		},
		{
			name: "merge in the modal merges the open pull request",
			keys: []string{"down", "enter", "m"}, question: "Squash-merge #135 into main?",
			want: "merge squash 135", what: "merge #135",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateMerged },
		},
		{
			name: "close closes an open pull request",
			keys: []string{"x"}, question: "Close PR #142?", want: "close 142", what: "close #142",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateClosed },
		},
		{
			name: "close in the modal",
			keys: []string{"enter", "x"}, question: "Close PR #142?", want: "close 142", what: "close #142",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateClosed },
		},
		{
			name: "reopen reopens a closed pull request",
			keys: []string{"]", "X"}, question: "Reopen PR #93?", want: "reopen 93", what: "reopen #93",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateOpen },
		},
		{
			name: "reopen in the modal",
			keys: []string{"]", "enter", "X"}, question: "Reopen PR #93?", want: "reopen 93", what: "reopen #93",
			after: func(pr core.PullRequest) bool { return pr.State == core.StateOpen },
		},
		{
			name: "toggle draft converts a ready pull request",
			keys: []string{"D"}, question: "Convert PR #142 to a draft?",
			want: "draft 142", what: "convert #142 to draft",
			after: func(pr core.PullRequest) bool { return pr.Draft },
		},
		{
			name: "toggle draft marks a draft ready",
			keys: []string{"down", "down", "D"}, question: "Mark PR #128 ready for review?",
			want: "ready 128", what: "mark #128 ready",
			after: func(pr core.PullRequest) bool { return !pr.Draft },
		},
		{
			name: "toggle draft in the modal",
			keys: []string{"enter", "D"}, question: "Convert PR #142 to a draft?",
			want: "draft 142", what: "convert #142 to draft",
			after: func(pr core.PullRequest) bool { return pr.Draft },
		},
		{
			name: "mark ready in the modal",
			keys: []string{"down", "down", "enter", "D"}, question: "Mark PR #128 ready for review?",
			want: "ready 128", what: "mark #128 ready",
			after: func(pr core.PullRequest) bool { return !pr.Draft },
		},
		{name: "close doesn't apply to a closed pull request", keys: []string{"]", "x"}},
		{name: "reopen doesn't apply to an open pull request", keys: []string{"X"}},
		{name: "merge doesn't apply to a merged pull request", keys: []string{"]", "]", "m"}},
		{name: "toggle draft doesn't apply to a merged pull request", keys: []string{"]", "]", "D"}},
	}
	for _, tt := range tests {
		answers := []string{"y", "n", "esc"}
		if tt.question == "" {
			// Nothing asks, so there is nothing to answer.
			answers = []string{""}
		}
		for _, answer := range answers {
			t.Run(strings.TrimSuffix(tt.name+"/"+answer, "/"), func(t *testing.T) {
				svc := newFakeService()
				s := started(t, svc, 80, 20, tt.opts...)
				last := len(tt.keys) - 1
				for _, k := range tt.keys[:last] {
					press(t, s, k)
				}
				before, _ := s.target()
				detail := s.modal()
				if detail != nil {
					before = detail.detail.PullRequest
				}
				msgs := press(t, s, tt.keys[last])
				if got := question(s); got != tt.question {
					t.Fatalf("asks %q, want %q", got, tt.question)
				}
				if tt.question != "" {
					if got := svc.changes(); len(got) != 0 {
						t.Fatalf("changes = %v before the answer", got)
					}
					// Other keys, even enter and the change keys, do
					// nothing while the question is open.
					for _, k := range []string{"enter", "j", "k", "q", "m", "x", "X", "D", "]"} {
						msgs = append(msgs, press(t, s, k)...)
					}
					if got := question(s); got != tt.question || len(svc.changes()) != 0 {
						t.Fatalf("after other keys asks %q with changes %v", got, svc.changes())
					}
					if pr, _ := s.target(); pr.Number != before.Number && detail == nil {
						t.Fatalf("the cursor moved to #%d behind the question", pr.Number)
					}
					msgs = append(msgs, press(t, s, answer)...)
					if got := question(s); got != "" {
						t.Fatalf("%s left the question %q open", answer, got)
					}
					if s.modal() != detail {
						t.Fatalf("%s changed the modal from %v to %v", answer, detail, s.modal())
					}
				}

				var want []string
				if tt.want != "" && answer != "n" && answer != "esc" {
					want = []string{tt.want}
				}
				if got := svc.changes(); !slices.Equal(got, want) {
					t.Fatalf("changes = %v, want %v", got, want)
				}
				if want == nil {
					if slices.ContainsFunc(msgs, func(m tea.Msg) bool { _, ok := m.(ui.DoneMsg); return ok }) {
						t.Errorf("messages %v, want no change sent", msgs)
					}
					return
				}
				if !slices.Contains(msgs, tea.Msg(ui.DoneMsg{From: ui.PullsTitle, What: tt.what})) {
					t.Errorf("messages %v, want a DoneMsg for %q", msgs, tt.what)
				}
				if !tt.after(svc.state(before.Number)) {
					t.Errorf("#%d after = %+v", before.Number, svc.state(before.Number))
				}
			})
		}
	}
}

// question returns what the open question asks, in the detail's last line
// or a modal of its own, or "" when none is open.
func question(h *host) string {
	if len(h.modals) == 0 {
		return ""
	}
	switch m := h.modals[len(h.modals)-1].(type) {
	case *ui.ConfirmModal:
		return m.Question()
	case *detailModal:
		if m.ask != nil {
			return m.ask.Question
		}
	}
	return ""
}

func TestASecondYesChangesOnce(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"y y from the list", []string{"m", "y", "y"}, "merge squash 142"},
		{"y enter from the list", []string{"m", "y", "enter"}, "merge squash 142"},
		{"y y in the modal", []string{"enter", "m", "y", "y"}, "merge squash 142"},
		{"y enter in the modal", []string{"enter", "m", "y", "enter"}, "merge squash 142"},
		{"y y on a draft toggle from the list", []string{"D", "y", "y"}, "draft 142"},
		{"y y on a draft toggle in the modal", []string{"enter", "D", "y", "y"}, "draft 142"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			s := started(t, svc, 80, 20)
			// The keys arrive before any of what they start runs, as a
			// repeated key or a paste does.
			var cmds []tea.Cmd
			for _, k := range tt.keys {
				if k == "m" || k == "D" || k == "enter" && len(cmds) == 0 {
					press(t, s, k)
					continue
				}
				cmds = append(cmds, s.Update(keyMsg(k)))
			}
			for _, c := range cmds {
				drain(t, s, c)
			}
			if got := svc.changes(); !slices.Equal(got, []string{tt.want}) {
				t.Errorf("changes = %v, want one %s", got, tt.want)
			}
		})
	}
}

func TestYesAsksAgain(t *testing.T) {
	t.Run("access lost while asking", func(t *testing.T) {
		svc := newFakeService()
		s := started(t, svc, 80, 20)
		press(t, s, "m")
		drain(t, s, s.Update(ui.CapsMsg{Repo: repo, Caps: core.RepoCaps{Known: true, Permission: core.PermissionRead, Squash: true}}))
		msgs := press(t, s, "y")
		want := ui.NotifyMsg{Level: toast.Info, Text: "You can't merge in eggzec/gh-tui (read access)."}
		if len(svc.changes()) != 0 || !slices.Contains(msgs, tea.Msg(want)) {
			t.Errorf("sent %v and showed %v, want only %v", svc.changes(), msgs, want)
		}
	})

	// edit changes pull request #142 as GitHub has it.
	edit := func(svc *fakeService, e func(pr *core.PullRequest)) {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		e(&svc.pulls[slices.IndexFunc(svc.pulls, func(pr core.PullRequest) bool { return pr.Number == 142 })])
	}
	// reread reads #142 again where the question is asked: the modal's
	// detail, or the list, which shows all states so #142 stays in it.
	reread := func(t *testing.T, s *host) {
		t.Helper()
		if m := s.modal(); m != nil {
			drain(t, s, m.get())
			return
		}
		drain(t, s, s.reload())
	}
	tests := []struct {
		name string
		// shared puts close and reopen on one key, as a config may.
		shared bool
		// lead asks the question about #142, and meddle changes what the
		// question is about before the answer.
		lead   []string
		meddle func(t *testing.T, s *host, svc *fakeService)
	}{
		{
			name: "merged elsewhere, in the modal",
			lead: []string{"enter", "x"},
			meddle: func(t *testing.T, s *host, svc *fakeService) {
				t.Helper()
				edit(svc, func(pr *core.PullRequest) { pr.State = core.StateMerged })
				drain(t, s, s.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
			},
		},
		{
			name: "the merge method changed, from the list",
			lead: []string{"m"},
			meddle: func(t *testing.T, s *host, _ *fakeService) {
				t.Helper()
				drain(t, s, s.Update(ui.CapsMsg{Repo: repo, Caps: core.RepoCaps{Known: true, Permission: core.PermissionWrite, Rebase: true}}))
			},
		},
		{
			name: "the merge method changed, in the modal",
			lead: []string{"enter", "m"},
			meddle: func(t *testing.T, s *host, _ *fakeService) {
				t.Helper()
				drain(t, s, s.Update(ui.CapsMsg{Repo: repo, Caps: core.RepoCaps{Known: true, Permission: core.PermissionWrite, Rebase: true}}))
			},
		},
		{
			name: "retargeted, in the modal",
			lead: []string{"enter", "m"},
			meddle: func(t *testing.T, s *host, svc *fakeService) {
				t.Helper()
				edit(svc, func(pr *core.PullRequest) { pr.BaseRef = "release" })
				reread(t, s)
			},
		},
		{
			name: "close turned reopen on a shared key, from the list", shared: true,
			lead: []string{"]", "]", "]", "x"},
			meddle: func(t *testing.T, s *host, svc *fakeService) {
				t.Helper()
				edit(svc, func(pr *core.PullRequest) { pr.State = core.StateClosed })
				reread(t, s)
			},
		},
		{
			name: "close turned reopen on a shared key, in the modal", shared: true,
			lead: []string{"enter", "x"},
			meddle: func(t *testing.T, s *host, svc *fakeService) {
				t.Helper()
				edit(svc, func(pr *core.PullRequest) { pr.State = core.StateClosed })
				reread(t, s)
			},
		},
		{
			name: "made a draft elsewhere, from the list",
			lead: []string{"D"},
			meddle: func(t *testing.T, s *host, svc *fakeService) {
				t.Helper()
				edit(svc, func(pr *core.PullRequest) { pr.Draft = true })
				reread(t, s)
			},
		},
		{
			name: "made a draft elsewhere, in the modal",
			lead: []string{"enter", "D"},
			meddle: func(t *testing.T, s *host, svc *fakeService) {
				t.Helper()
				edit(svc, func(pr *core.PullRequest) { pr.Draft = true })
				reread(t, s)
			},
		},
		{
			// The question reads the same for #142 of any repository.
			name: "the list shows another repository",
			lead: []string{"x"},
			meddle: func(t *testing.T, s *host, _ *fakeService) {
				t.Helper()
				s.repo = core.RepoRef{Owner: "eggzec", Name: "other"}
			},
		},
		{
			name: "the cursor moved, in the list",
			lead: []string{"x"},
			meddle: func(t *testing.T, s *host, _ *fakeService) {
				t.Helper()
				// As a reload that reorders the list would, behind the
				// question.
				drain(t, s, s.Section.Update(keyMsg("down")))
				if pr, _ := s.target(); pr.Number == 142 {
					t.Fatal("the cursor didn't move")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := config.Default().Keys
			if tt.shared {
				keys = maps.Clone(keys)
				keys[config.ActionReopen] = keys[config.ActionClose]
			}
			svc := newFakeService()
			sec := New(t.Context(), svc, keys, WithClock(func() time.Time { return clock }))
			sec.SetSize(80, 20)
			sec.Focus()
			s := &host{Section: sec}
			drain(t, s, s.Update(ui.RepoMsg{Repo: repo}))
			drain(t, s, s.Init())
			for _, k := range tt.lead {
				press(t, s, k)
			}
			if !strings.Contains(question(s), "#142") {
				t.Fatalf("asks %q, want a question about #142", question(s))
			}
			tt.meddle(t, s, svc)
			msgs := press(t, s, "y")
			want := ui.NotifyMsg{Level: toast.Info, Text: "#142 changed meanwhile, so nothing was sent."}
			if len(svc.changes()) != 0 || !slices.Contains(msgs, tea.Msg(want)) {
				t.Errorf("sent %v and showed %v, want only %v", svc.changes(), msgs, want)
			}
		})
	}
}

func TestMutationShowsAtOnce(t *testing.T) {
	svc := newFakeService()
	s := started(t, svc, 80, 20)
	press(t, s, "enter")
	m := s.modal()
	press(t, s, "m")
	cmd := s.Update(keyMsg("y"))
	if cmd == nil {
		t.Fatal("confirming the merge returned no command")
	}
	// The modal reads the change from the cache before it is sent.
	if m.detail.State != core.StateMerged || !strings.Contains(modalScreen(t, s), " Merged ") {
		t.Errorf("modal before sending:\n%s", modalScreen(t, s))
	}
	// So does the list behind it, once the modal told the section, still
	// before GitHub answers.
	var changed tea.Msg
	for _, c := range cmd().(tea.BatchMsg) {
		if c == nil {
			continue
		}
		if msg, ok := c().(changedMsg); ok {
			changed = msg
		}
	}
	if changed == nil {
		t.Fatal("the modal didn't tell the section about the change")
	}
	drain(t, s, s.Update(changed))
	if svc.state(142).State != core.StateMerged {
		t.Fatal("the fake lost the merge")
	}
	if strings.Contains(screen(s), "#142") {
		t.Errorf("the open list still shows #142 as open:\n%s", screen(s))
	}
}

func TestMutationFromModalReloadsBoth(t *testing.T) {
	svc := newFakeService()
	s := started(t, svc, 80, 20)
	press(t, s, "enter")
	lists := len(svc.listed())
	press(t, s, "D")
	msgs := press(t, s, "y")
	if !slices.Contains(msgs, tea.Msg(ui.DoneMsg{From: ui.PullsTitle, What: "convert #142 to draft"})) {
		t.Fatalf("messages %v, want a DoneMsg", msgs)
	}
	// Once after the change and once after the DoneMsg.
	if got := len(svc.listed()); got < lists+2 {
		t.Errorf("listed %d times, want at least %d", got, lists+2)
	}
	if !strings.Contains(modalScreen(t, s), " Draft ") {
		t.Errorf("modal after the change:\n%s", modalScreen(t, s))
	}
	if pr, _ := s.feed.Selected(); pr.Number != 142 || !pr.Draft {
		t.Errorf("list shows #%d draft %v, want #142 as a draft", pr.Number, pr.Draft)
	}
}

func TestFailedMutationRollsBack(t *testing.T) {
	svc := newFakeService()
	svc.sendErr = errors.New("merge conflict")
	s := started(t, svc, 80, 20)
	press(t, s, "enter")
	press(t, s, "m")
	msgs := press(t, s, "y")
	done := slices.IndexFunc(msgs, func(m tea.Msg) bool { d, ok := m.(ui.DoneMsg); return ok && d.Err != nil })
	if done < 0 {
		t.Fatalf("messages %v, want a failed DoneMsg", msgs)
	}
	if m := s.modal(); m.detail.State != core.StateOpen || !strings.Contains(modalScreen(t, s), " Open ") {
		t.Errorf("after the rollback:\n%s", modalScreen(t, s))
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
	drain(t, s, s.Update(ui.DoneMsg{From: ui.PullsTitle, What: "merge #1"}))
	if len(svc.listed()) != n+1 {
		t.Errorf("listed %d times after DoneMsg, want %d", len(svc.listed()), n+1)
	}
}

func TestHelpOffersWhatApplies(t *testing.T) {
	enabled := func(s *host) []string { return uitest.Enabled(s.KeyLayers()) }
	svc := newFakeService()
	s := started(t, svc, 80, 20)
	if got := enabled(s); !slices.Contains(got, "merge") || !slices.Contains(got, "close") || slices.Contains(got, "reopen") {
		t.Errorf("open help = %v, want merge and close", got)
	}
	press(t, s, "]")
	if got := enabled(s); slices.Contains(got, "merge") || !slices.Contains(got, "reopen") {
		t.Errorf("closed help = %v, want reopen only", got)
	}
}
