package issues

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	refssvc "github.com/eggzec/gh-tui/internal/service/refs"
	"github.com/eggzec/gh-tui/internal/tui/refs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fakeRefs serves the links of every issue: a pull request that closed it.
type fakeRefs struct{ reads []refssvc.Query }

func (f *fakeRefs) CachedReferences(refssvc.Query) (core.References, bool) {
	return core.References{}, false
}
func (f *fakeRefs) References(_ context.Context, q refssvc.Query) (core.References, error) {
	f.reads = append(f.reads, q)
	return core.References{Closing: []core.Reference{{
		Target: core.Target{Repo: testRepo, Number: 91, Kind: core.KindPull}, Title: "Fix the crash", State: core.StateMerged,
		Origins: []core.RefOrigin{{Group: core.RefClosing, Where: "closed by"}},
	}}}, nil
}
func (f *fakeRefs) CachedMentions(refssvc.MentionsQuery) (core.Page[core.Reference], bool) {
	return core.Page[core.Reference]{}, false
}
func (f *fakeRefs) Mentions(context.Context, refssvc.MentionsQuery) (core.Page[core.Reference], error) {
	return core.Page[core.Reference]{}, nil
}
func (f *fakeRefs) Invalidate(core.RepoRef, int) {}

var _ refs.Service = (*fakeRefs)(nil)

// openedRefs opens an issue over a section with a service for the links.
func openedRefs(t *testing.T) (*host, *detailModal, *fakeRefs) {
	t.Helper()
	f := &fakeRefs{}
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, sampleComments(3)...)
	h := started(t, svc, 100, 30, WithReferences(f), WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	return h, m, f
}

func modalText(h *host) string {
	return strings.Join(strings.Fields(ansi.Strip(h.modals[len(h.modals)-1].View())), " ")
}

func TestIssueModalReferencesKey(t *testing.T) {
	h, m, f := openedRefs(t)
	if m.refs != nil {
		t.Fatal("the links show before their key")
	}
	press(t, h, "#")
	if m.refs == nil {
		t.Fatal("# didn't show the links")
	}
	if v := modalText(h); !strings.Contains(v, "Closed by (1)") || !strings.Contains(v, "#91 Fix the crash") {
		t.Errorf("the modal doesn't show the links:\n%s", v)
	}
	if len(f.reads) == 0 || f.reads[0].Pull || f.reads[0].Number != m.number {
		t.Errorf("the links were read as %+v, want those of the issue", f.reads)
	}
	// The keys of the issue don't act on the links behind them.
	press(t, h, "X", "c")
	if m.ask != nil || m.composing != composeNone {
		t.Error("a key of the issue acted while the links show")
	}
	// Back steps back to the thread, and esc closes the modal.
	if cmd, ok := m.Act(ui.ActBack); !ok || m.refs != nil {
		t.Fatalf("back left the links open (%v), or wasn't taken", m.refs != nil)
	} else {
		run(t, h, cmd)
	}
	if h.modal() != m {
		t.Error("back closed the modal")
	}
	press(t, h, "#", "esc")
	if h.modal() != nil {
		t.Error("esc over the links didn't close the modal")
	}
}

// Without a link to step back from, back is the app's, which returns to the
// modal this one replaced.
func TestIssueModalBackWithoutReferences(t *testing.T) {
	_, m, _ := openedRefs(t)
	if _, ok := m.Act(ui.ActBack); ok {
		t.Error("the modal took the back key with no links to step back from")
	}
}

func TestIssueModalReferencesPickOpensWithBack(t *testing.T) {
	h, m, _ := openedRefs(t)
	press(t, h, "#")
	var opened []ui.OpenPullMsg
	for _, msg := range press(t, h, "enter") {
		if o, ok := msg.(ui.OpenPullMsg); ok {
			opened = append(opened, o)
		}
	}
	if len(opened) != 1 || opened[0].Number != 91 || opened[0].Repo != testRepo || opened[0].Over != m {
		t.Errorf("enter opened %+v, want #91 with the modal to go back to", opened)
	}
	run(t, h, h.Update(ui.ReopenedMsg{Modal: m}))
	if m.refs == nil {
		t.Error("the links closed when the modal was reopened")
	}
}

// The key is typed, not taken, in the prompt of a comment, and doesn't
// answer a question.
func TestIssueModalReferencesKeyOffWhileComposing(t *testing.T) {
	h, m, _ := openedRefs(t)
	press(t, h, "c")
	if m.composing == composeNone {
		t.Fatal("c didn't open the prompt")
	}
	press(t, h, "#")
	if m.refs != nil {
		t.Error("# showed the links over the prompt")
	}
	if got := m.prompt.Value(); !strings.Contains(got, "#") {
		t.Errorf("# wasn't typed: %q", got)
	}
	if cmd, ok := m.ShowReferences(); !ok || cmd != nil || m.refs != nil {
		t.Error("ShowReferences showed the links over the prompt")
	}
	press(t, h, "esc")
	press(t, h, "X")
	if m.ask == nil {
		t.Fatal("X didn't ask")
	}
	press(t, h, "#")
	if m.refs != nil {
		t.Error("# showed the links over a question")
	}
}

func TestIssueModalShowReferences(t *testing.T) {
	h, m, _ := openedRefs(t)
	cmd, ok := m.ShowReferences()
	if !ok {
		t.Fatal("the modal refuses to show the links")
	}
	run(t, h, cmd)
	if m.refs == nil {
		t.Error("ShowReferences didn't show the links")
	}
	if got := m.Commands(); !slices.Contains(got, ui.CommandReferences) {
		t.Errorf("Commands() = %v, want it to name the references command", got)
	}

	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, sampleComments(3)...)
	bare := started(t, svc, 100, 30)
	press(t, bare, "down", "enter")
	if slices.Contains(bare.modal().Commands(), ui.CommandReferences) {
		t.Error("a modal without a service names the references command")
	}
	if _, ok := bare.modal().ShowReferences(); ok {
		t.Error("a modal without a service shows the links")
	}
	press(t, bare, "#")
	if bare.modal().refs != nil || bare.modal().keys.References.Enabled() {
		t.Error("# works without a service")
	}
}

func TestIssueModalReferencesKeyLayers(t *testing.T) {
	h, m, _ := openedRefs(t)
	names := func() string {
		layers := m.KeyLayers()
		out := make([]string, 0, len(layers))
		for _, l := range layers {
			out = append(out, l.Context)
		}
		return strings.Join(out, ",")
	}
	if got := names(); got != "issue_modal" {
		t.Fatalf("layers = %q", got)
	}
	var listed bool
	for _, b := range m.KeyLayers()[0].Bindings {
		if b.Help().Desc == "linked items" && b.Enabled() {
			listed = true
		}
	}
	if !listed {
		t.Error("the modal's help doesn't list the key of the links")
	}
	press(t, h, "#")
	if got := names(); got != "references" {
		t.Errorf("layers over the links = %q, want references alone", got)
	}
	press(t, h, "&")
	if got := names(); got != "search_prompt" {
		t.Errorf("layers while the filter types = %q", got)
	}
}

// The links are read again when the issue is read and found changed, and
// not when it is read and found as it was.
func TestIssueModalRereadsTheLinksForAChange(t *testing.T) {
	h, m, f := openedRefs(t)
	press(t, h, "#")
	reads := len(f.reads)
	run(t, h, m.Update(issueMsg{thread: m.id, issue: m.issue}))
	if got := len(f.reads); got != reads {
		t.Fatalf("an unchanged issue read the links %d more times", got-reads)
	}
	changed := m.issue
	changed.UpdatedAt = changed.UpdatedAt.Add(time.Hour)
	run(t, h, m.Update(issueMsg{thread: m.id, issue: changed}))
	// The pane's read and the open step's.
	if got := len(f.reads); got != reads+2 {
		t.Errorf("a changed issue read the links %d more times, want 2", got-reads)
	}
	if last := f.reads[len(f.reads)-1]; !last.Updated.Equal(changed.UpdatedAt) && !f.reads[len(f.reads)-2].Updated.Equal(changed.UpdatedAt) {
		t.Errorf("no read carried the update time %v: %+v", changed.UpdatedAt, f.reads[reads:])
	}
}

// A kept value that comes back stale is read again with Again set, and the
// pane lists the pull requests of other repositories by their repository.
func TestIssueModalLinksStaleReadsAgain(t *testing.T) {
	_, m, f := openedRefs(t)
	other := core.RepoRef{Owner: "acme", Name: "lib"}
	r := core.References{Stale: true, Closing: []core.Reference{{
		Target: core.Target{Repo: other, Number: 5, Kind: core.KindPull}, Title: "Tracks it", State: core.StateOpen,
	}}}
	cmd := m.receiveLinks(linksMsg{id: m.id, refs: r})
	if cmd == nil {
		t.Fatal("a stale value wasn't read again")
	}
	cmd()
	if last := f.reads[len(f.reads)-1]; !last.Again {
		t.Errorf("the read after a stale value = %+v, want Again", last)
	}
	m.side.dev.SetSize(60, 6)
	if v := ansi.Strip(m.side.dev.View()); !strings.Contains(v, "acme/lib#5 Tracks it") {
		t.Errorf("the pane doesn't name the other repository:\n%s", v)
	}
}

// The mouse scrolls the links, not the thread they cover.
func TestIssueModalMouseDoesNotReachTheCoveredThread(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, sampleComments(40)...)
	h := started(t, svc, 100, 20, WithReferences(&fakeRefs{}), WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, h, "down", "enter", "*")
	m := h.modal()
	wheel := tea.MouseWheelMsg{Button: tea.MouseWheelDown}
	top := ansi.Strip(m.conv.View())
	press(t, h, "j")
	if ansi.Strip(m.conv.View()) == top {
		t.Fatal("the key doesn't move the thread, so the test shows nothing")
	}
	press(t, h, "g")
	top = ansi.Strip(m.conv.View())
	press(t, h, "#")
	run(t, h, m.Update(wheel))
	if got := ansi.Strip(m.conv.View()); got != top {
		t.Error("the mouse scrolled the thread under the links")
	}
}
