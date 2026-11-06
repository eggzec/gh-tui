package issues

import (
	"context"
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
	full := m.convRect().H

	press(t, h, "c")
	if m.composing != composeComment || !m.prompt.Focused() {
		t.Fatal("c should open a focused comment prompt")
	}
	if h := m.convRect().H; h >= full || h+m.prompt.Height() != full {
		t.Errorf("conversation %d + prompt %d rows, want them to share %d", h, m.prompt.Height(), full)
	}
	assertFits(t, m.View(), 80, 30)

	typeText(t, h, "Same here, fixed by the patch.")
	press(t, h, "ctrl+s")
	if got := question(h); got != "Post this comment on #999?" || len(svc.changeCalls()) != 0 {
		t.Fatalf("submit asked %q and sent %v, want only the question", got, svc.changeCalls())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Post this comment on #999?") || !strings.Contains(v, "Same here, fixed by the patch.") {
		t.Errorf("the question isn't shown under the comment:\n%s", v)
	}
	assertFits(t, m.View(), 80, 30)
	done := runHolding(t, h, h.Update(keyMsg("y")))
	if got := svc.changeCalls(); !slices.Equal(got, []string{"comment 999: Same here, fixed by the patch."}) {
		t.Fatalf("changes = %v, want the comment", got)
	}
	if m.composing != composeNone || m.convRect().H != full {
		t.Error("submitting should close the prompt and give the conversation its room back")
	}
	// The rebuilt conversation shows its comments collapsed again.
	press(t, h, "*", "G")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Same here, fixed by the patch.") {
		t.Errorf("the pending comment isn't shown:\n%s", v)
	}
	if len(done) != 1 || done[0].What != "comment on #999" || done[0].From != ui.IssuesTitle {
		t.Fatalf("done = %+v, want one for comment on #999", done)
	}

	run(t, h, h.Update(done[0]))
	press(t, h, "*", "G")
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "Same here, fixed") {
		t.Errorf("after GitHub answered, the conversation doesn't show the comment:\n%s", v)
	}
}

func TestCommentRolledBack(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.sendErr = errors.New("403 Forbidden")
	h, m := opened(t, svc, 30)
	press(t, h, "c")
	typeText(t, h, "Nope")
	press(t, h, "ctrl+s", "y")
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
	if got := svc.changeCalls(); len(got) != 0 || question(h) != "" {
		t.Errorf("an empty comment asked %q and sent %v", question(h), got)
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
	if h.modal() != m || m.composing != composeNone {
		t.Error("esc should cancel the prompt, keeping the modal open")
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
	if !m.prompt.Focused() {
		t.Error("the prompt should have the focus")
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

// The labels picker ticks the issue's labels, and applying a change asks
// before it sends.
func TestLabelsPicker(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	press(t, h, "L")
	if m.over == nil {
		t.Fatal("L opened no picker")
	}
	// The ticked labels come first, so the cursor is on enhancement.
	press(t, h, "space", "enter")
	if got, want := question(h), "Remove the label enhancement from #999?"; got != want {
		t.Fatalf("applying asked %q, want %q", got, want)
	}
	if len(svc.changeCalls()) != 0 {
		t.Fatalf("sent %v before the answer", svc.changeCalls())
	}
	for _, d := range runHolding(t, h, h.Update(keyMsg("y"))) {
		run(t, h, h.Update(d))
	}
	if got := svc.changeCalls(); !slices.Equal(got, []string{"labels 999 + -enhancement"}) {
		t.Errorf("changes = %v", got)
	}
}

// Applying the picker with nothing changed asks nothing.
func TestLabelsPickerUnchanged(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	press(t, h, "L", "enter")
	if question(h) != "" || len(svc.changeCalls()) != 0 || m.over != nil {
		t.Errorf("asked %q, sent %v, picker open %v", question(h), svc.changeCalls(), m.over != nil)
	}
}

// failingLabels is a source of facets whose read of the labels fails.
type failingLabels struct{ fakeFacets }

func (*failingLabels) Labels(context.Context, core.RepoRef) ([]core.Label, error) {
	return nil, errors.New("boom")
}

// openedWith is opened for a section that has f as its source of facets.
func openedWith(t *testing.T, f Facets) (*host, *detailModal) {
	t.Helper()
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, sampleComments(3)...)
	h := started(t, svc, 80, 30, WithFacets(f))
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "*")
	return h, m
}

// The picker offers the labels of the repository read from the facets along
// with the issue's own.
func TestLabelsPickerFromFacets(t *testing.T) {
	t.Parallel()
	h, m := openedWith(t, &fakeFacets{})
	press(t, h, "L")
	o, ok := m.over.(*labelsOverlay)
	if !ok {
		t.Fatalf("L opened %T, want the labels picker", m.over)
	}
	// bug and ui from the repository, the rest from the issue.
	if want := 2 + len(m.issue.Labels); o.Rows != want {
		t.Errorf("the picker offers %d labels, want %d", o.Rows, want)
	}
}

// A failed read of the labels shows an error and opens no picker.
func TestLabelsPickerReadFails(t *testing.T) {
	t.Parallel()
	h, m := openedWith(t, &failingLabels{})
	msgs := press(t, h, "L")
	if m.over != nil {
		t.Error("the picker opened though the read failed")
	}
	if len(msgs) < 2 {
		t.Errorf("fed %v, want the read and an error toast", msgs)
	}
}

// Labels read while something else was opened do not open the picker over it.
func TestLabelsReadLateOpensNothing(t *testing.T) {
	t.Parallel()
	h, m := openedWith(t, &fakeFacets{})
	read := h.Update(keyMsg("L"))
	press(t, h, "c")
	if m.composing == composeNone {
		t.Fatal("c opened no prompt")
	}
	run(t, h, read)
	if m.over != nil || m.composing != composeComment {
		t.Errorf("picker open %v, composing %v", m.over != nil, m.composing)
	}
}

// A question about what the prompt sends takes every key: y sends it
// once, and n or esc go back to the prompt with the text kept.
func TestComposeAnswers(t *testing.T) {
	tests := []struct {
		name string
		// open opens the prompt, and typed is what the test submits.
		open, typed, question string
		want                  []string
	}{
		{"comment", "c", "Same here.", "Post this comment on #999?", []string{"comment 999: Same here."}},
	}
	for _, tt := range tests {
		for _, answer := range [][]string{{"y"}, {"n"}, {"esc"}, {"y", "y"}} {
			t.Run(tt.name+"/"+strings.Join(answer, " "), func(t *testing.T) {
				svc := newFakeService(sampleIssues(12))
				h, m := opened(t, svc, 30)
				press(t, h, tt.open)
				m.prompt.SetValue(tt.typed)
				press(t, h, "ctrl+s")
				if got := question(h); got != tt.question {
					t.Fatalf("asks %q, want %q", got, tt.question)
				}
				// Other keys, even the submit and the prompt's text, do
				// nothing while the question is open, and neither does a
				// paste or a second submit. Enter is not among them: it
				// answers the default, which is no here.
				press(t, h, "x", "q", "ctrl+s")
				run(t, h, h.Update(tea.PasteMsg{Content: "pasted"}))
				run(t, h, h.Update(prompt.SubmitMsg{ID: m.prompt.ID(), Value: tt.typed}))
				if got := question(h); got != tt.question || len(svc.changeCalls()) != 0 || m.prompt.Value() != tt.typed {
					t.Fatalf("after other keys asks %q with %q typed and changes %v", got, m.prompt.Value(), svc.changeCalls())
				}
				// The answers arrive before what the first starts runs, as
				// a repeated key does.
				var cmds []tea.Cmd
				for _, k := range answer {
					cmds = append(cmds, h.Update(keyMsg(k)))
				}
				for _, c := range cmds {
					run(t, h, c)
				}
				if question(h) != "" {
					t.Fatalf("%v left the question open", answer)
				}
				got := svc.changeCalls()
				slices.Sort(got)
				if answer[0] == "y" {
					if !slices.Equal(got, tt.want) || m.composing != composeNone {
						t.Errorf("sent %v with the prompt open %v, want %v sent once and the prompt closed", got, m.composing != composeNone, tt.want)
					}
					return
				}
				if len(got) != 0 {
					t.Errorf("%v sent %v", answer, got)
				}
				if m.composing == composeNone || !m.prompt.Focused() || m.prompt.Value() != tt.typed {
					t.Fatalf("%v didn't go back to the prompt with %q, has %q", answer, tt.typed, m.prompt.Value())
				}
				// Back at the prompt, keys are text again.
				press(t, h, "!")
				if m.prompt.Value() != tt.typed+"!" {
					t.Errorf("typing after %v gave %q", answer, m.prompt.Value())
				}
			})
		}
	}
}

func TestComposeAsksAgain(t *testing.T) {
	archived := writeCaps
	archived.Archived = true
	tests := []struct {
		name  string
		open  string
		typed string
		// meddle changes what the question is about before the answer.
		meddle func(t *testing.T, h *host, svc *fakeService, m *detailModal)
		want   tea.Msg
	}{
		{
			name: "comment in an archived repository", open: "c", typed: "Same here.",
			meddle: func(t *testing.T, h *host, _ *fakeService, _ *detailModal) {
				t.Helper()
				run(t, h, h.Update(ui.CapsMsg{Repo: testRepo, Caps: archived}))
			},
			want: info("eggzec/gh-tui is archived, so it's read-only."),
		},
		{
			name: "the comment changed behind the question", open: "c", typed: "Same here.",
			meddle: func(t *testing.T, _ *host, _ *fakeService, m *detailModal) {
				t.Helper()
				m.prompt.SetValue("Something else.")
			},
			want: info("#999 changed meanwhile, so nothing was sent."),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			h, m := opened(t, svc, 30)
			press(t, h, tt.open)
			m.prompt.SetValue(tt.typed)
			press(t, h, "ctrl+s")
			if question(h) == "" {
				t.Fatal("the submit asked nothing")
			}
			tt.meddle(t, h, svc, m)
			msgs := press(t, h, "y")
			if len(svc.changeCalls()) != 0 || !slices.Contains(msgs, tt.want) {
				t.Errorf("sent %v and showed %v, want only %v", svc.changeCalls(), msgs, tt.want)
			}
			// Nothing typed is lost.
			if m.composing == composeNone || question(h) != "" {
				t.Errorf("the prompt closed, or still asks %q", question(h))
			}
		})
	}
}

func TestLabelQuestion(t *testing.T) {
	tests := []struct {
		added, removed []string
		want           string
	}{
		{[]string{"bug"}, nil, "Add the label bug to #12?"},
		{[]string{"bug", "help wanted"}, nil, "Add the labels bug, help wanted to #12?"},
		{nil, []string{"wontfix"}, "Remove the label wontfix from #12?"},
		{nil, []string{"wontfix", "ui"}, "Remove the labels wontfix, ui from #12?"},
		{[]string{"bug"}, []string{"wontfix", "ui"}, "Add the label bug to #12 and remove wontfix, ui?"},
		{[]string{"a\x1b[31mb"}, nil, "Add the label ab to #12?"},
	}
	for _, tt := range tests {
		if got := labelQuestion("#12", tt.added, tt.removed); got != tt.want {
			t.Errorf("labelQuestion(+%v -%v) = %q, want %q", tt.added, tt.removed, got, tt.want)
		}
	}
}

func TestHelpWhileComposing(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 20)
	if got := enabled(m.KeyLayers()); !slices.Contains(got, "comment") || !slices.Contains(got, "labels") {
		t.Errorf("modal help = %v, want comment and labels", got)
	}
	press(t, h, "c")
	if got := enabled(m.KeyLayers()); !slices.Equal(got, []string{"submit", "cancel"}) {
		t.Errorf("help while composing = %v, want submit and cancel", got)
	}
	press(t, h, "esc", "esc")
	if got := enabled(h.KeyLayers()); slices.Contains(got, "comment") {
		t.Errorf("list help = %v, want no comment", got)
	}
}

// Comment and label act only in the modal.
func TestComposeKeysInTheList(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)
	press(t, h, "c", "L")
	if h.modal() != nil {
		t.Error("c or l opened something in the list")
	}
}
