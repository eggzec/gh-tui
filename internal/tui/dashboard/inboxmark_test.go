package dashboard

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// question returns the confirmation that app opened.
func question(tb testing.TB, app []tea.Msg) *ui.ConfirmModal {
	tb.Helper()
	for _, msg := range app {
		if open, ok := msg.(ui.OpenModalMsg); ok {
			if m, ok := open.Modal.(*ui.ConfirmModal); ok {
				return m
			}
		}
	}
	tb.Fatalf("no question was asked: %v", app)
	return nil
}

// answer arms the question m and presses k, returning what that sent.
func answer(tb testing.TB, s *Section, m *ui.ConfirmModal, k string) []tea.Msg {
	tb.Helper()
	m.Update(ui.Arm()())
	return run(tb, s, m.Update(keyPress(k)))
}

func TestInboxMarkReadAsksAndMarks(t *testing.T) {
	in := &fakeInbox{threads: inboxThreads()}
	s := newSection(t, newFake(), in, 140, 38)
	app := press(t, s, "5", "U")
	if len(in.marks()) != 0 {
		t.Fatalf("U marked %v read before the answer", in.marks())
	}
	q := question(t, app)
	if want := "Mark bubbletea#1 as read?"; !strings.Contains(q.Question(), "as read?") {
		t.Errorf("question = %q, want one like %q", q.Question(), want)
	}
	if got := answer(t, s, q, "enter"); !slices.Contains(got, tea.Msg(ui.DoneMsg{From: ui.NotificationsTitle, What: "mark read"})) {
		t.Errorf("answering sent %v, want the thread marked read", got)
	}
	if !slices.Equal(in.marks(), []string{"1"}) {
		t.Errorf("marked %v read, want thread 1", in.marks())
	}
}

func TestInboxMarkReadNoDoesNothing(t *testing.T) {
	in := &fakeInbox{threads: inboxThreads()}
	s := newSection(t, newFake(), in, 140, 38)
	q := question(t, press(t, s, "5", "U"))
	answer(t, s, q, "n")
	if len(in.marks()) != 0 {
		t.Errorf("no marked %v read", in.marks())
	}
}

func TestInboxMarkDoneAsksAndMarks(t *testing.T) {
	in := &fakeInbox{threads: inboxThreads()}
	s := newSection(t, newFake(), in, 140, 38)
	q := question(t, press(t, s, "5", "D"))
	if !strings.Contains(q.Question(), "as done?") {
		t.Errorf("question = %q, want one about marking done", q.Question())
	}
	if len(in.doneMarks()) != 0 {
		t.Fatalf("D marked %v done before the answer", in.doneMarks())
	}
	answer(t, s, q, "y")
	if !slices.Equal(in.doneMarks(), []string{"1"}) {
		t.Errorf("marked %v done, want thread 1", in.doneMarks())
	}
}

// The pane draws each kind and state with the mark of the notifications list,
// in every icon set.
func TestInboxRowDrawsTheMarkOfTheSubject(t *testing.T) {
	t.Parallel()
	open := core.SubjectState{State: core.StateOpen}
	tests := []struct {
		name  string
		typ   core.SubjectType
		state *core.SubjectState
		want  ui.Mark
	}{
		{"open pull", core.SubjectPullRequest, &open, ui.Mark(ui.PullOpen)},
		{"merged pull", core.SubjectPullRequest, &core.SubjectState{State: core.StateMerged}, ui.Mark(ui.PullMerged)},
		{"pull before its state", core.SubjectPullRequest, nil, ui.MarkPullUnknown},
		{"open issue", core.SubjectIssue, &open, ui.Mark(ui.IssueOpen)},
		{"issue not planned", core.SubjectIssue, &core.SubjectState{State: core.StateClosed, Reason: core.ReasonNotPlanned}, ui.Mark(ui.IssueNotPlanned)},
		{"release", core.SubjectRelease, nil, ui.MarkRelease},
		{"commit", core.SubjectCommit, nil, ui.MarkCommit},
		{"check suite", core.SubjectCheckSuite, nil, ui.MarkCheckSuite},
	}
	for _, set := range []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII} {
		for _, tt := range tests {
			t.Run(set+" "+tt.name, func(t *testing.T) {
				t.Parallel()
				n := thread("7", "octocat/hello-world", "title", true, 0)
				n.Subject.Type = tt.typ
				in := &fakeInbox{threads: []core.Notification{n}}
				s := newSection(t, newFake(), in, 140, 38, WithIcons(ui.NewIcons(set)))
				if tt.state != nil {
					s.states = map[core.SubjectRef]core.SubjectState{{Repo: n.Repo, Number: 7, Type: tt.typ}: *tt.state}
				}
				cell := s.icons.Mark(tt.want)
				if cell == "" {
					t.Fatalf("mark %d has no glyph in %s", tt.want, set)
				}
				rows := ansi.Strip(strings.Join(s.inboxBody(80, 5), "\n"))
				if !strings.Contains(rows, cell) {
					t.Errorf("the pane lacks %q for mark %d in %s:\n%s", cell, tt.want, set, rows)
				}
			})
		}
	}
}
