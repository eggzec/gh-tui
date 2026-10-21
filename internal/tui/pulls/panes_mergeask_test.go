package pulls

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// modalToMerge opens the modal of #142 over svc, once its detail is read and
// the viewer may write.
func modalToMerge(t *testing.T, svc *fakeService) (*host, *panesModal) {
	t.Helper()
	svc.mergeTexts = map[int]core.MergeTexts{142: mergeTexts}
	h := started(t, svc, 160, 40, WithDrafts(&memDrafts{}, "host/me"))
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))
	press(t, h, "enter")
	m := h.modal()
	if m == nil || !m.loaded {
		t.Fatal("enter opened no loaded modal")
	}
	m.caps = writeCaps
	return h, m
}

// pressMergeOn focuses pane p of the modal, and presses the merge key.
func pressMergeOn(t *testing.T, h *host, p panes.Pane) {
	t.Helper()
	m := h.modal()
	m.focus = p
	m.resetSub()
	press(t, h, "M")
}

func TestModalMergeKeyOpensTheListsDialogFromAnyPane(t *testing.T) {
	t.Parallel()
	_, listDialog := merging(t, newFakeService(), &memDrafts{})
	for _, p := range []panes.Pane{panes.Description, panes.Files, panes.Conversation, panes.Merge} {
		t.Run(p.Name(), func(t *testing.T) {
			t.Parallel()
			h, _ := modalToMerge(t, newFakeService())
			pressMergeOn(t, h, p)
			dlg := dialogOf(h)
			if dlg == nil || !dlg.loaded {
				t.Fatalf("M on %v opened no loaded merge dialog", p)
			}
			if !dlg.ask.c.of.same(listDialog.ask.c.of) || !slices.Equal(dlg.methods, listDialog.methods) {
				t.Errorf("the dialog asks %+v of %v, the list's asks %+v of %v",
					dlg.ask.c.of, dlg.methods, listDialog.ask.c.of, listDialog.methods)
			}
		})
	}
}

func TestModalMergeKeyWorksInAPreview(t *testing.T) {
	t.Parallel()
	h, m := modalToMerge(t, newFakeService())
	m.focus = panes.Files
	m.zoom.SetZoom(true)
	m.sub[panes.Files-1] = panes.Preview
	if !m.inPreview(m.layout()) {
		t.Fatal("the modal isn't in a preview")
	}
	press(t, h, "M")
	if dialogOf(h) == nil {
		t.Error("M in a preview opened no merge dialog")
	}
}

func TestModalMergeDialogSendsOneMergeWithWhatItShows(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, _ := modalToMerge(t, svc)
	pressMergeOn(t, h, panes.Files)
	dlg := dialogOf(h)
	if dlg == nil || !dlg.loaded {
		t.Fatal("M opened no loaded merge dialog")
	}
	dlg.title.SetValue("Add things")
	press(t, h, "y")
	got := svc.changes()
	if len(got) != 1 || !strings.HasPrefix(got[0], "merge squash ") || !strings.Contains(got[0], " title=Add things body=squash body") {
		t.Errorf("changes = %q, want one squash with the dialog's message", got)
	}
}

func TestModalMergeDialogDeclinedSendsNothing(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, _ := modalToMerge(t, svc)
	pressMergeOn(t, h, panes.Merge)
	press(t, h, "n")
	if got := svc.changes(); len(got) != 0 {
		t.Errorf("changes = %q after a no, want none", got)
	}
}

func TestModalMergeQuestionIsAskedInTheModal(t *testing.T) {
	t.Parallel()
	queue := func() *fakeService {
		svc := newFakeService()
		mi := core.MergeInfo{Status: core.MergeClean}
		mi.Queue.Enabled = true
		svc.merges = map[int]core.MergeInfo{142: mi}
		return svc
	}
	t.Run("yes", func(t *testing.T) {
		t.Parallel()
		svc := queue()
		h, m := modalToMerge(t, svc)
		pressMergeOn(t, h, panes.Description)
		o, ok := m.top().(*confirmOverlay)
		if !ok {
			t.Fatalf("M left %T on top, want the question", m.top())
		}
		if !strings.Contains(o.c.Question, "merge queue") {
			t.Errorf("the question is %q, want the one for the queue", o.c.Question)
		}
		if len(svc.changes()) != 0 {
			t.Fatal("the pull request was changed before the answer")
		}
		press(t, h, "y")
		if got := svc.changes(); len(got) != 1 {
			t.Errorf("changes = %q, want one", got)
		}
	})
	t.Run("no", func(t *testing.T) {
		t.Parallel()
		svc := queue()
		h, _ := modalToMerge(t, svc)
		pressMergeOn(t, h, panes.Description)
		press(t, h, "n")
		if got := svc.changes(); len(got) != 0 {
			t.Errorf("changes = %q after a no, want none", got)
		}
	})
}

// A merge the user confirmed goes out once, though the modal closes before
// the read that it waits for comes back.
func TestModalMergeConfirmedSurvivesClosingTheModal(t *testing.T) {
	t.Parallel()
	queue := func() *fakeService {
		svc := newFakeService()
		mi := core.MergeInfo{Status: core.MergeClean}
		mi.Queue.Enabled = true
		svc.merges = map[int]core.MergeInfo{142: mi}
		return svc
	}
	// finish closes the modal while the read is held, then lets it come back.
	finish := func(t *testing.T, h *host, svc *fakeService, held tea.Cmd) {
		t.Helper()
		press(t, h, "esc")
		if h.modal() != nil {
			t.Fatal("esc left the modal open")
		}
		drain(t, h, held)
		if got := svc.changes(); len(got) != 1 {
			t.Errorf("changes = %q, want the one merge that was confirmed", got)
		}
	}
	t.Run("question", func(t *testing.T) {
		t.Parallel()
		svc := queue()
		h, _ := modalToMerge(t, svc)
		pressMergeOn(t, h, panes.Description)
		finish(t, h, svc, h.Update(keyMsg("y")))
	})
	t.Run("dialog", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService()
		h, _ := modalToMerge(t, svc)
		pressMergeOn(t, h, panes.Description)
		if dlg := dialogOf(h); dlg == nil || !dlg.loaded {
			t.Fatal("M opened no loaded merge dialog")
		}
		yes := h.Update(h.key("y"))
		if yes == nil {
			t.Fatal("y in the merge dialog started nothing")
		}
		seq, ok := sequence(yes())
		if !ok || len(seq) < 2 {
			t.Fatalf("y sent %d commands, want the dialog's sequence", len(seq))
		}
		// Everything but the read is done at once; the read is held.
		for _, c := range seq[:len(seq)-1] {
			drain(t, h, c)
		}
		finish(t, h, svc, seq[len(seq)-1])
	})
}

func TestModalMergeCommandReachesTheModal(t *testing.T) {
	t.Parallel()
	h, _ := modalToMerge(t, newFakeService())
	drain(t, h, h.Update(keymap.ActionPress("pull_modal.merge")))
	if dialogOf(h) == nil {
		t.Error("the merge command opened no merge dialog in the modal")
	}
}
