package issues

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
)

func TestComment(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	full := m.thread.Height()

	press(t, h, "c")
	if m.composing != composeComment || !m.prompt.Focused() || m.thread.Focused() {
		t.Fatal("c should open a focused comment prompt and blur the thread")
	}
	if h := m.thread.Height(); h >= full || h+m.prompt.Height() != full {
		t.Errorf("thread %d + prompt %d rows, want them to share %d", h, m.prompt.Height(), full)
	}
	assertFits(t, m.View(), 80, 30)

	typeText(t, h, "Same here, fixed by the patch.")
	done := runHolding(t, h, h.Update(keyMsg("ctrl+s")))
	if got := svc.changeCalls(); !slices.Equal(got, []string{"comment 999: Same here, fixed by the patch."}) {
		t.Fatalf("changes = %v, want the comment", got)
	}
	if m.composing != composeNone || !m.thread.Focused() || m.thread.Height() != full {
		t.Error("submitting should close the prompt and give the thread its room and focus back")
	}
	press(t, h, "G")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "you · sending…") || !strings.Contains(v, "Same here, fixed by the patch.") {
		t.Errorf("the pending comment isn't shown as sending:\n%s", v)
	}
	if len(done) != 1 || done[0].What != "comment on #999" || done[0].From != ui.IssuesTitle {
		t.Fatalf("done = %+v, want one for comment on #999", done)
	}

	run(t, h, h.Update(done[0]))
	press(t, h, "G")
	v = ansi.Strip(m.View())
	if strings.Contains(v, "sending…") || !strings.Contains(v, "octocat") || !strings.Contains(v, "Same here, fixed") {
		t.Errorf("after GitHub answered, the thread doesn't show the real comment:\n%s", v)
	}
}

func TestCommentRolledBack(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.sendErr = errors.New("403 Forbidden")
	h, m := opened(t, svc, 30)
	press(t, h, "c")
	typeText(t, h, "Nope")
	run(t, h, h.Update(keyMsg("ctrl+s")))
	press(t, h, "G")
	if v := ansi.Strip(m.View()); strings.Contains(v, "Nope") {
		t.Errorf("a refused comment is still shown:\n%s", v)
	}
}

func TestEmptyCommentIsIgnored(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	press(t, h, "c")
	typeText(t, h, "  ")
	press(t, h, "enter", "ctrl+s")
	if got := svc.changeCalls(); len(got) != 0 {
		t.Errorf("an empty comment was sent: %v", got)
	}
	if m.composing != composeComment {
		t.Error("an empty comment closed the prompt")
	}
}

// While the prompt is open, the modal's keys are text, esc included, which
// cancels the prompt before a second esc closes the modal.
func TestKeysWhileComposing(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	press(t, h, "c")
	msgs := typeText(t, h, "qxXrofclG")
	if got := m.prompt.Value(); got != "qxXrofclG" {
		t.Errorf("prompt value = %q, want every key typed", got)
	}
	if _, ok := has[ui.OpenMsg](msgs); ok || len(svc.changeCalls()) != 0 || len(svc.getCalls()) != 1 {
		t.Errorf("keys acted on the issue: changes %v, gets %v", svc.changeCalls(), svc.getCalls())
	}

	press(t, h, "esc")
	if h.modal() != m || m.composing != composeNone || !m.thread.Focused() {
		t.Error("esc should cancel the prompt and focus the thread, keeping the modal open")
	}
	if got := svc.changeCalls(); len(got) != 0 {
		t.Errorf("cancel sent %v", got)
	}
	press(t, h, "esc")
	if h.modal() != nil {
		t.Error("esc after cancel should close the modal")
	}
}

func TestComposeResize(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 30)
	press(t, h, "c")
	if !m.prompt.Focused() || m.thread.Focused() {
		t.Error("the prompt should have the focus, not the thread")
	}
	m.SetSize(60, 12)
	assertFits(t, m.View(), 60, 12)
	m.SetSize(40, 4)
	assertFits(t, m.View(), 40, 4)
	m.SetTheme(testTheme())
	press(t, h, "esc")
	m.SetSize(80, 30)
	assertFits(t, m.View(), 80, 30)
}

func TestLabels(t *testing.T) {
	// #999 has enhancement and help wanted.
	tests := []struct {
		name  string
		typed string
		want  []string
		// wantShown are the labels the header shows before GitHub answers.
		wantShown []string
	}{
		{"unchanged", "enhancement, help wanted", nil, []string{"enhancement", "help wanted"}},
		{"case and space differ", " Enhancement ,HELP WANTED,, ", nil, []string{"enhancement", "help wanted"}},
		{"added", "enhancement, help wanted, bug, ui", []string{"label 999 +bug,ui"}, []string{"enhancement", "help wanted", "bug", "ui"}},
		{"removed", "help wanted", []string{"unlabel 999 -enhancement"}, []string{"help wanted"}},
		{"all removed", "", []string{"unlabel 999 -enhancement", "unlabel 999 -help wanted"}, nil},
		{
			"added and removed", "bug, Help Wanted, BUG",
			[]string{"label 999 +bug", "unlabel 999 -enhancement"}, []string{"help wanted", "bug"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			h, m := opened(t, svc, 30)
			press(t, h, "l")
			if m.composing != composeLabels || m.prompt.Value() != "enhancement, help wanted" {
				t.Fatalf("l opened %v with %q, want the labels prompt with the issue's labels", m.composing, m.prompt.Value())
			}
			m.prompt.SetValue(tt.typed)
			cmd := h.Update(keyMsg("enter"))
			done := runHolding(t, h, cmd)
			if m.composing != composeNone {
				t.Error("submit didn't close the prompt")
			}
			got := svc.changeCalls()
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("changes = %v, want %v", got, tt.want)
			}
			if len(done) != len(tt.want) {
				t.Errorf("%d ops sent, want %d", len(done), len(tt.want))
			}
			shown := make([]string, 0, len(m.issue.Labels))
			for _, l := range m.issue.Labels {
				shown = append(shown, l.Name)
			}
			if !slices.Equal(shown, tt.wantShown) {
				t.Errorf("labels shown = %v, want %v", shown, tt.wantShown)
			}
			for _, d := range done {
				run(t, h, h.Update(d))
			}
		})
	}
}

// The label changes go to GitHub one after another: the added labels,
// then each removed one, in the issue's order.
func TestLabelOpsRunInOrder(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	press(t, h, "l")
	m.prompt.SetValue("bug")
	submit, ok := h.Update(keyMsg("enter"))().(prompt.SubmitMsg)
	if !ok {
		t.Fatal("enter didn't submit the labels")
	}
	batch, ok := h.Update(submit)().(tea.BatchMsg)
	if !ok {
		t.Fatal("submitting didn't return a batch of the reload and the changes")
	}
	var seq []tea.Cmd
	for _, c := range batch {
		if c == nil {
			continue
		}
		if cmds, ok := sequence(c()); ok {
			seq = cmds
		}
	}
	want := []string{"add bug to #999", "remove enhancement from #999", "remove help wanted from #999"}
	if len(seq) != len(want) {
		t.Fatalf("sequence of %d changes, want %d", len(seq), len(want))
	}
	for i, c := range seq {
		if d, ok := c().(ui.DoneMsg); !ok || d.What != want[i] {
			t.Errorf("change %d = %#v, want %q", i, d, want[i])
		}
	}
}

func TestLabelDiff(t *testing.T) {
	have := []core.Label{{Name: "bug"}, {Name: "Help Wanted"}, {Name: "ui"}}
	tests := []struct {
		name           string
		typed          string
		added, removed []string
	}{
		{"unchanged", "bug, Help Wanted, ui", nil, nil},
		{"reordered", "ui,bug,Help Wanted", nil, nil},
		{"case differs", "BUG, help wanted, UI", nil, nil},
		{"space differs", "  bug ,  Help Wanted,ui  ,", nil, nil},
		{"added", "bug, Help Wanted, ui, docs, good first issue", []string{"docs", "good first issue"}, nil},
		{"added once", "bug, Help Wanted, ui, docs, Docs", []string{"docs"}, nil},
		{"removed", "bug", nil, []string{"Help Wanted", "ui"}},
		{"all removed", " , ", nil, []string{"bug", "Help Wanted", "ui"}},
		{"both", "ui, docs", []string{"docs"}, []string{"bug", "Help Wanted"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, removed := labelDiff(have, tt.typed)
			if !slices.Equal(added, tt.added) || !slices.Equal(removed, tt.removed) {
				t.Errorf("labelDiff(%q) = +%v -%v, want +%v -%v", tt.typed, added, removed, tt.added, tt.removed)
			}
		})
	}
	if added, removed := labelDiff(nil, "bug"); !slices.Equal(added, []string{"bug"}) || removed != nil {
		t.Errorf("labelDiff(nil, bug) = +%v -%v", added, removed)
	}
}

func TestHelpWhileComposing(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 20)
	if got := enabled(m.Help()); !slices.Contains(got, "comment") || !slices.Contains(got, "labels") {
		t.Errorf("modal help = %v, want comment and labels", got)
	}
	press(t, h, "c")
	if got := enabled(m.Help()); !slices.Equal(got, []string{"submit", "cancel"}) {
		t.Errorf("help while composing = %v, want submit and cancel", got)
	}
	press(t, h, "esc", "esc")
	if got := enabled(h.Help()); slices.Contains(got, "comment") {
		t.Errorf("list help = %v, want no comment", got)
	}
}

// Comment and label act only in the modal.
func TestComposeKeysInTheList(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)
	press(t, h, "c", "l")
	if h.modal() != nil {
		t.Error("c or l opened something in the list")
	}
}
