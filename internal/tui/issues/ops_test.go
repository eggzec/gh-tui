package issues

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSetState(t *testing.T) {
	tests := []struct {
		name string
		// setup brings the section to the issue to act on.
		setup   func(t *testing.T, s *host)
		key     string
		sendErr error
		// question is what the key asks before the change is made.
		question string
		// wantChange is the change asked of the service, or "" for none.
		wantChange string
		// wantBefore and wantAfter are the states shown before and after
		// GitHub answers, or "" if the issue isn't shown.
		wantBefore, wantAfter core.State
	}{
		{
			name:       "close in the open list drops the issue once confirmed",
			setup:      func(t *testing.T, s *host) { t.Helper(); press(t, s, "down") },
			key:        "x",
			question:   "Close issue #999?",
			wantChange: "close 999",
			wantBefore: core.StateClosed,
			wantAfter:  "",
		},
		{
			// All issues, so the closed one stays in the list.
			name:       "close in the list",
			setup:      func(t *testing.T, s *host) { t.Helper(); press(t, s, "]", "]", "down") },
			key:        "x",
			question:   "Close issue #999?",
			wantChange: "close 999",
			wantBefore: core.StateClosed,
			wantAfter:  core.StateClosed,
		},
		{
			name: "reopen in the list",
			setup: func(t *testing.T, s *host) {
				t.Helper()
				press(t, s, "]", "]", "down", "down", "down", "down")
			},
			key:        "X",
			question:   "Reopen issue #996?",
			wantChange: "reopen 996",
			wantBefore: core.StateOpen,
			wantAfter:  core.StateOpen,
		},
		{
			name:       "close in the modal",
			setup:      func(t *testing.T, s *host) { t.Helper(); press(t, s, "down", "enter") },
			key:        "x",
			question:   "Close issue #999?",
			wantChange: "close 999",
			wantBefore: core.StateClosed,
			wantAfter:  core.StateClosed,
		},
		{
			name:       "a refused close rolls back",
			setup:      func(t *testing.T, s *host) { t.Helper(); press(t, s, "down", "enter") },
			key:        "x",
			sendErr:    errors.New("403 Forbidden"),
			question:   "Close issue #999?",
			wantChange: "close 999",
			wantBefore: core.StateClosed,
			wantAfter:  core.StateOpen,
		},
		{
			name:  "reopen an open issue does nothing",
			setup: func(t *testing.T, s *host) { t.Helper(); press(t, s, "down") },
			key:   "X",
		},
		{
			name: "close a closed issue does nothing",
			setup: func(t *testing.T, s *host) {
				t.Helper()
				press(t, s, "]", "enter")
			},
			key: "x",
		},
	}
	for _, tt := range tests {
		answers := []string{"y", "n", "esc"}
		if tt.wantChange == "" {
			answers = []string{""}
		}
		for _, answer := range answers {
			t.Run(strings.TrimSuffix(tt.name+"/"+answer, "/"), func(t *testing.T) {
				svc := newFakeService(sampleIssues(12))
				svc.sendErr = tt.sendErr
				s := started(t, svc, 80, 20)
				tt.setup(t, s)
				target, _ := s.target()
				detail := s.modal()
				if detail != nil {
					target = detail.issue
				}

				msgs := press(t, s, tt.key)
				if tt.wantChange == "" {
					if question(s) != "" || len(svc.changeCalls()) != 0 || len(msgs) != 0 {
						t.Fatalf("%s asked %q for %v", tt.key, question(s), svc.changeCalls())
					}
					return
				}
				if got := question(s); got != tt.question {
					t.Fatalf("%s asks %q, want %q", tt.key, got, tt.question)
				}
				// Other keys, even enter and the change keys, do nothing
				// while the question is open.
				press(t, s, "enter", "j", "k", "q", "x", "X", "c", "l", "]")
				if got := question(s); got != tt.question || len(svc.changeCalls()) != 0 {
					t.Fatalf("after other keys asks %q with changes %v", got, svc.changeCalls())
				}
				if detail == nil {
					if it, _ := s.target(); it.Number != target.Number {
						t.Fatalf("the cursor moved to #%d behind the question", it.Number)
					}
				}
				cmd := s.Update(keyMsg(answer))
				if answer == "n" || answer == "esc" {
					msgs := run(t, s, cmd)
					if _, sent := has[ui.DoneMsg](msgs); sent || len(svc.changeCalls()) != 0 {
						t.Errorf("%s sent %v", answer, svc.changeCalls())
					}
					if question(s) != "" || s.modal() != detail {
						t.Errorf("%s left %q asked and the modal %v, want %v", answer, question(s), s.modal(), detail)
					}
					return
				}
				done := runHolding(t, s, cmd)
				if question(s) != "" || s.modal() != detail {
					t.Errorf("%s left %q asked and the modal %v, want %v", answer, question(s), s.modal(), detail)
				}
				if got := svc.changeCalls(); !slices.Equal(got, []string{tt.wantChange}) {
					t.Fatalf("changes = %v, want %s", got, tt.wantChange)
				}
				if got := shownState(s, target.Number); got != tt.wantBefore {
					t.Errorf("before the answer #%d shows %q, want %q", target.Number, got, tt.wantBefore)
				}
				verb, _, _ := strings.Cut(tt.wantChange, " ")
				want := ui.DoneMsg{From: ui.IssuesTitle, What: verb + " #" + strings.TrimPrefix(tt.wantChange, verb+" "), Err: tt.sendErr}
				if len(done) != 1 || done[0].What != want.What || !errors.Is(done[0].Err, tt.sendErr) {
					t.Fatalf("done = %v, want %v", done, want)
				}

				before := len(svc.listCalls())
				run(t, s, s.Update(done[0]))
				if len(svc.listCalls()) == before {
					t.Error("DoneMsg didn't reload the list")
				}
				if got := shownState(s, target.Number); got != tt.wantAfter {
					t.Errorf("after the answer #%d shows %q, want %q", target.Number, got, tt.wantAfter)
				}
			})
		}
	}
}

// question returns what the open question asks, on the issue's last line
// or in a modal of its own, or "" when none is open.
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
		// lead brings the question up, and answers arrive before any of
		// what they start runs, as a repeated key or a paste does.
		lead, answers []string
	}{
		{"y y from the list", []string{"down", "x"}, []string{"y", "y"}},
		{"y enter from the list", []string{"down", "x"}, []string{"y", "enter"}},
		{"y y in the modal", []string{"down", "enter", "x"}, []string{"y", "y"}},
		{"y enter in the modal", []string{"down", "enter", "x"}, []string{"y", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			s := started(t, svc, 80, 20)
			press(t, s, tt.lead...)
			var cmds []tea.Cmd
			for _, k := range tt.answers {
				cmds = append(cmds, s.Update(keyMsg(k)))
			}
			for _, c := range cmds {
				run(t, s, c)
			}
			if got := svc.changeCalls(); !slices.Equal(got, []string{"close 999"}) {
				t.Errorf("changes = %v, want one close", got)
			}
		})
	}
}

func TestYesAsksAgain(t *testing.T) {
	t.Run("access lost while asking", func(t *testing.T) {
		svc := newFakeService(sampleIssues(12))
		s := started(t, svc, 80, 20, WithViewer(me))
		// #1000 is another's, which triage may close.
		run(t, s, s.Update(ui.CapsMsg{Repo: testRepo, Caps: triageCaps}))
		press(t, s, "x")
		run(t, s, s.Update(ui.CapsMsg{Repo: testRepo, Caps: readCaps}))
		msgs := press(t, s, "y")
		if len(svc.changeCalls()) != 0 || !slices.Contains(msgs, info("You can't close #1000 in eggzec/gh-tui (read access).")) {
			t.Errorf("sent %v and showed %v, want only the refusal", svc.changeCalls(), msgs)
		}
	})
	t.Run("the cursor moved while asking", func(t *testing.T) {
		svc := newFakeService(sampleIssues(12))
		s := started(t, svc, 80, 20)
		press(t, s, "down", "x")
		// As a reload that reorders the list would, behind the question.
		run(t, s, s.Section.Update(keyMsg("down")))
		if it, _ := s.target(); it.Number == 999 {
			t.Fatal("the cursor didn't move")
		}
		msgs := press(t, s, "y")
		if len(svc.changeCalls()) != 0 || !slices.Contains(msgs, info("#999 changed meanwhile, so nothing was sent.")) {
			t.Errorf("sent %v and showed %v, want only the notice", svc.changeCalls(), msgs)
		}
	})
	t.Run("closed elsewhere while asking", func(t *testing.T) {
		svc := newFakeService(sampleIssues(12))
		s := started(t, svc, 80, 20)
		press(t, s, "down", "enter", "x")
		m := s.modal()
		m.issue.State = core.StateClosed
		msgs := press(t, s, "y")
		if len(svc.changeCalls()) != 0 || !slices.Contains(msgs, info("#999 changed meanwhile, so nothing was sent.")) {
			t.Errorf("sent %v and showed %v, want only the notice", svc.changeCalls(), msgs)
		}
	})
}

// shownState returns the state shown for issue number: the header's in the
// modal, or the selected row's, which follows the issue.
func shownState(s *host, number int) core.State {
	if m := s.modal(); m != nil {
		v := ansi.Strip(m.header(m.issue))
		if strings.Contains(v, ui.NewIcons(config.IconsNerd).State(ui.IssueOpen)+" Open") {
			return core.StateOpen
		}
		return core.StateClosed
	}
	it, ok := s.list.Selected()
	if !ok || it.Number != number {
		return ""
	}
	return it.State
}

// A change in the modal shows in the modal and in the list behind it at
// once, and both show GitHub's answer.
func TestChangeInModalShowsInBoth(t *testing.T) {
	for _, sendErr := range []error{nil, errors.New("403 Forbidden")} {
		name := "confirmed"
		if sendErr != nil {
			name = "refused"
		}
		t.Run(name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			svc.sendErr = sendErr
			// All issues, so the closed one stays in the list.
			h := started(t, svc, 80, 20)
			press(t, h, "]", "]", "down", "enter")
			m := h.modal()
			press(t, h, "x")
			done := runHolding(t, h, h.Update(keyMsg("y")))
			row := func() core.State {
				it, _ := h.list.Selected()
				return it.State
			}
			if got := shownState(h, 999); got != core.StateClosed {
				t.Fatalf("before the answer the modal shows %q, want closed", got)
			}
			// A refused change is rolled back as soon as it is sent, which
			// the test runs before the list reads the cache again.
			if sendErr == nil && row() != core.StateClosed {
				t.Fatalf("before the answer the list shows %q, want closed", row())
			}
			if len(done) != 1 {
				t.Fatalf("done = %v, want one", done)
			}
			run(t, h, h.Update(done[0]))
			want := core.StateClosed
			if sendErr != nil {
				want = core.StateOpen
			}
			if got := shownState(h, 999); got != want || row() != want || m.issue.State != want {
				t.Errorf("after the answer the modal shows %q and the list %q, want both %q", got, row(), want)
			}
		})
	}
}

func TestDoneOfOthersIsIgnored(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := started(t, svc, 80, 20)
	before := len(svc.listCalls())
	run(t, s, s.Update(ui.DoneMsg{From: ui.PullsTitle, What: "merge #3"}))
	if len(svc.listCalls()) != before {
		t.Error("another section's DoneMsg reloaded the list")
	}
}

func TestHelpOffersTheApplicableChange(t *testing.T) {
	s := started(t, newFakeService(sampleIssues(12)), 80, 20)
	offered := func() (closeOn, reopenOn bool) {
		for _, b := range s.Help().ShortHelp() {
			switch {
			case b.Help().Desc == "close":
				closeOn = b.Enabled()
			case b.Help().Desc == "reopen":
				reopenOn = b.Enabled()
			}
		}
		return closeOn, reopenOn
	}
	if c, r := offered(); !c || r {
		t.Errorf("open issue offers close %v, reopen %v; want only close", c, r)
	}
	press(t, s, "]")
	if c, r := offered(); c || !r {
		t.Errorf("closed issue offers close %v, reopen %v; want only reopen", c, r)
	}
	if !s.keys.Close.Enabled() {
		t.Error("Help disabled the section's own close binding")
	}
}
