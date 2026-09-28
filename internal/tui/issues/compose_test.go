package issues

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
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
		// question is what the submit asks, "" when it changes nothing.
		question string
		want     []string
		// wantShown are the labels the header shows before GitHub answers.
		wantShown []string
	}{
		{"unchanged", "enhancement, help wanted", "", nil, []string{"enhancement", "help wanted"}},
		{"case and space differ", " Enhancement ,HELP WANTED,, ", "", nil, []string{"enhancement", "help wanted"}},
		{
			"added", "enhancement, help wanted, bug, ui", "Add the labels bug, ui to #999?",
			[]string{"label 999 +bug,ui"}, []string{"enhancement", "help wanted", "bug", "ui"},
		},
		{"removed", "help wanted", "Remove the label enhancement from #999?", []string{"unlabel 999 -enhancement"}, []string{"help wanted"}},
		{
			"all removed", "", "Remove the labels enhancement, help wanted from #999?",
			[]string{"unlabel 999 -enhancement", "unlabel 999 -help wanted"}, nil,
		},
		{
			"added and removed", "bug, Help Wanted, BUG", "Add the label bug to #999 and remove enhancement?",
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
			press(t, h, "enter")
			if got := question(h); got != tt.question || len(svc.changeCalls()) != 0 {
				t.Fatalf("submit asked %q and sent %v, want the question %q", got, svc.changeCalls(), tt.question)
			}
			var done []ui.DoneMsg
			if tt.question != "" {
				done = runHolding(t, h, h.Update(keyMsg("y")))
			}
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
	if cmd := h.Update(submit); cmd != nil || question(h) == "" {
		t.Fatal("submitting didn't ask first")
	}
	batch, ok := h.Update(keyMsg("y"))().(tea.BatchMsg)
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
		{"labels", "l", "bug", "Add the label bug to #999 and remove enhancement, help wanted?", []string{
			"label 999 +bug", "unlabel 999 -enhancement", "unlabel 999 -help wanted",
		}},
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
				// paste or a second submit.
				press(t, h, "enter", "x", "q", "ctrl+s")
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
		// setup readies the issue before it opens, and meddle changes what
		// the question is about before the answer.
		setup  func(svc *fakeService)
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
			name: "labels changed elsewhere", open: "l", typed: "bug",
			meddle: func(t *testing.T, h *host, svc *fakeService, _ *detailModal) {
				t.Helper()
				svc.set(999, func(it *core.Issue) { it.Labels = append(it.Labels, core.Label{Name: "ui"}) })
				run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
			},
			want: info("#999 changed meanwhile, so nothing was sent."),
		},
		{
			name: "labels made what was typed elsewhere", open: "l", typed: "bug",
			meddle: func(t *testing.T, h *host, svc *fakeService, _ *detailModal) {
				t.Helper()
				svc.set(999, func(it *core.Issue) { it.Labels = []core.Label{{Name: "bug"}} })
				run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
			},
			want: info("#999 changed meanwhile, so nothing was sent."),
		},
		{
			// Removing "a, b" and "c" reads as removing "a" and "b, c".
			name: "labels that read the same", open: "l", typed: "",
			meddle: func(t *testing.T, h *host, svc *fakeService, _ *detailModal) {
				t.Helper()
				svc.set(999, func(it *core.Issue) { it.Labels = []core.Label{{Name: "a"}, {Name: "b, c"}} })
				run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
			},
			setup: func(svc *fakeService) {
				svc.set(999, func(it *core.Issue) { it.Labels = []core.Label{{Name: "a, b"}, {Name: "c"}} })
			},
			want: info("#999 changed meanwhile, so nothing was sent."),
		},
		{
			name: "labels in an archived repository", open: "l", typed: "bug",
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
			if tt.setup != nil {
				tt.setup(svc)
			}
			h, m := opened(t, svc, 30)
			press(t, h, tt.open)
			m.prompt.SetValue(tt.typed)
			press(t, h, "ctrl+s")
			if question(h) == "" {
				t.Fatal("the submit asked nothing")
			}
			asked := question(h)
			tt.meddle(t, h, svc, m)
			if tt.setup != nil {
				if again, _, _ := m.labels(tt.typed); again.Question != asked {
					t.Fatalf("the change now asks %q, want it to read as %q", again.Question, asked)
				}
			}
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
