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
	s := opened(t, svc, 30)
	full := s.detail.Height()

	press(t, s, "c")
	if s.composing != composeComment || !s.prompt.Focused() || s.detail.Focused() {
		t.Fatal("c should open a focused comment prompt and blur the thread")
	}
	if h := s.detail.Height(); h >= full || h+s.prompt.Height() != full {
		t.Errorf("thread %d + prompt %d rows, want them to share %d", h, s.prompt.Height(), full)
	}
	assertFits(t, s.View(), 80, 30)

	typeText(t, s, "Same here, fixed by the patch.")
	done := runHolding(t, s, s.Update(keyMsg("ctrl+s")))
	if got := svc.changeCalls(); !slices.Equal(got, []string{"comment 999: Same here, fixed by the patch."}) {
		t.Fatalf("changes = %v, want the comment", got)
	}
	if s.composing != composeNone || !s.detail.Focused() || s.detail.Height() != full {
		t.Error("submitting should close the prompt and give the thread its room and focus back")
	}
	press(t, s, "G")
	v := ansi.Strip(s.View())
	if !strings.Contains(v, "you · sending…") || !strings.Contains(v, "Same here, fixed by the patch.") {
		t.Errorf("the pending comment isn't shown as sending:\n%s", v)
	}
	if len(done) != 1 || done[0].What != "comment on #999" || done[0].From != ui.IssuesTitle {
		t.Fatalf("done = %+v, want one for comment on #999", done)
	}

	run(t, s, s.Update(done[0]))
	press(t, s, "G")
	v = ansi.Strip(s.View())
	if strings.Contains(v, "sending…") || !strings.Contains(v, "octocat") || !strings.Contains(v, "Same here, fixed") {
		t.Errorf("after GitHub answered, the thread doesn't show the real comment:\n%s", v)
	}
}

func TestCommentRolledBack(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.sendErr = errors.New("403 Forbidden")
	s := opened(t, svc, 30)
	press(t, s, "c")
	typeText(t, s, "Nope")
	run(t, s, s.Update(keyMsg("ctrl+s")))
	press(t, s, "G")
	if v := ansi.Strip(s.View()); strings.Contains(v, "Nope") {
		t.Errorf("a refused comment is still shown:\n%s", v)
	}
}

func TestEmptyCommentIsIgnored(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := opened(t, svc, 30)
	press(t, s, "c")
	typeText(t, s, "  ")
	press(t, s, "enter", "ctrl+s")
	if got := svc.changeCalls(); len(got) != 0 {
		t.Errorf("an empty comment was sent: %v", got)
	}
	if s.composing != composeComment {
		t.Error("an empty comment closed the prompt")
	}
}

// While the prompt is open, the section's keys are text.
func TestKeysWhileComposing(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := opened(t, svc, 30)
	press(t, s, "c")
	if !s.Capturing() {
		t.Fatal("the section should capture keys while the prompt is open")
	}
	msgs := typeText(t, s, "qxXrofclG")
	if got := s.prompt.Value(); got != "qxXrofclG" {
		t.Errorf("prompt value = %q, want every key typed", got)
	}
	if _, ok := has[ui.OpenMsg](msgs); ok || len(svc.changeCalls()) != 0 || len(svc.getCalls()) != 1 {
		t.Errorf("keys acted on the issue: changes %v, gets %v", svc.changeCalls(), svc.getCalls())
	}

	press(t, s, "esc")
	if !s.inDetail || s.composing != composeNone || !s.detail.Focused() || s.Capturing() {
		t.Error("esc should cancel the prompt and focus the thread, staying in the detail")
	}
	if got := svc.changeCalls(); len(got) != 0 {
		t.Errorf("cancel sent %v", got)
	}
	press(t, s, "esc")
	if s.inDetail {
		t.Error("esc after cancel should go back to the list")
	}
}

func TestComposeFocus(t *testing.T) {
	s := opened(t, newFakeService(sampleIssues(12)), 30)
	press(t, s, "c")
	s.Blur()
	if s.prompt.Focused() || s.Capturing() {
		t.Error("a blurred section kept the prompt focused")
	}
	s.Focus()
	if !s.prompt.Focused() || s.detail.Focused() {
		t.Error("focus should go back to the prompt, not the thread")
	}
	s.SetSize(60, 12)
	assertFits(t, s.View(), 60, 12)
	s.SetSize(40, 4)
	assertFits(t, s.View(), 40, 4)
	s.SetTheme(testTheme())
	press(t, s, "esc")
	s.SetSize(80, 30)
	assertFits(t, s.View(), 80, 30)
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
			s := opened(t, svc, 30)
			press(t, s, "l")
			if s.composing != composeLabels || s.prompt.Value() != "enhancement, help wanted" {
				t.Fatalf("l opened %v with %q, want the labels prompt with the issue's labels", s.composing, s.prompt.Value())
			}
			s.prompt.SetValue(tt.typed)
			cmd := s.Update(keyMsg("enter"))
			done := runHolding(t, s, cmd)
			if s.composing != composeNone {
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
			shown := make([]string, 0, len(s.issue.Labels))
			for _, l := range s.issue.Labels {
				shown = append(shown, l.Name)
			}
			if !slices.Equal(shown, tt.wantShown) {
				t.Errorf("labels shown = %v, want %v", shown, tt.wantShown)
			}
			for _, d := range done {
				run(t, s, s.Update(d))
			}
		})
	}
}

// The label changes go to GitHub one after another: the added labels,
// then each removed one, in the issue's order.
func TestLabelOpsRunInOrder(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := opened(t, svc, 30)
	press(t, s, "l")
	s.prompt.SetValue("bug")
	submit, ok := s.Update(keyMsg("enter"))().(prompt.SubmitMsg)
	if !ok {
		t.Fatal("enter didn't submit the labels")
	}
	batch, ok := s.Update(submit)().(tea.BatchMsg)
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
	if got := descs(); !slices.Contains(got, "comment") || !slices.Contains(got, "labels") {
		t.Errorf("detail help = %v, want comment and labels", got)
	}
	press(t, s, "c")
	if got := descs(); !slices.Equal(got, []string{"submit", "cancel"}) {
		t.Errorf("help while composing = %v, want submit and cancel", got)
	}
	press(t, s, "esc", "esc")
	if got := descs(); slices.Contains(got, "comment") {
		t.Errorf("list help = %v, want no comment", got)
	}
}

// Comment and label act only in the detail.
func TestComposeKeysInTheList(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := started(t, svc, 80, 20)
	press(t, s, "c", "l")
	if s.composing != composeNone || s.Capturing() {
		t.Error("c or l opened a prompt in the list")
	}
}
