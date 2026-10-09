package pulls

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
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

// fakeRefs serves the links of every pull request: an issue it closes and a
// pull request its text writes.
type fakeRefs struct {
	mu    sync.Mutex
	reads []int
}

func (f *fakeRefs) value() core.References {
	return core.References{
		Closing: []core.Reference{{
			Target: core.Target{Repo: repo, Number: 88, Kind: core.KindIssue}, Title: "Crash on startup", State: core.StateOpen,
			Origins: []core.RefOrigin{{Group: core.RefClosing, Where: "closes"}},
		}},
		Written: []core.Reference{{
			Target: core.Target{Repo: repo, Number: 91, Kind: core.KindPull}, Title: "Retry the read", State: core.StateMerged,
			Origins: []core.RefOrigin{{Group: core.RefWritten, Where: "body"}},
		}},
	}
}

func (f *fakeRefs) CachedReferences(refssvc.Query) (core.References, bool) {
	return core.References{}, false
}
func (f *fakeRefs) References(_ context.Context, q refssvc.Query) (core.References, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, q.Number)
	return f.value(), nil
}
func (f *fakeRefs) CachedMentions(refssvc.MentionsQuery) (core.Page[core.Reference], bool) {
	return core.Page[core.Reference]{}, false
}
func (f *fakeRefs) Mentions(context.Context, refssvc.MentionsQuery) (core.Page[core.Reference], error) {
	return core.Page[core.Reference]{}, nil
}
func (f *fakeRefs) Invalidate(core.RepoRef, int) {}

func (f *fakeRefs) numbers() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.reads...)
}

var _ refs.Service = (*fakeRefs)(nil)

// startedRefs is a section with a service for the links, opened on #142.
func startedRefs(t *testing.T, opts ...Option) (*host, *fakeRefs) {
	t.Helper()
	f := &fakeRefs{}
	opts = append([]Option{WithReferences(f), WithIcons(ui.NewIcons(config.IconsASCII))}, opts...)
	h := started(t, newFakeService(), 100, 30, opts...)
	press(t, h, "enter")
	if h.modal() == nil {
		t.Fatal("enter didn't open the modal")
	}
	return h, f
}

func TestPullModalReferencesKey(t *testing.T) {
	h, f := startedRefs(t)
	m := h.modal()
	if m.refs != nil {
		t.Fatal("the links show before their key")
	}
	press(t, h, "#")
	if m.refs == nil {
		t.Fatal("# didn't show the links")
	}
	if v := modalText(h); !strings.Contains(v, "Closes (1)") || !strings.Contains(v, "#88 Crash on startup") || !strings.Contains(v, "Written here (1)") {
		t.Errorf("the modal doesn't show the links:\n%s", v)
	}
	if got := f.numbers(); len(got) == 0 || got[0] != 142 {
		t.Errorf("the links of %v were read, want #142", got)
	}
	// The keys of the pull request don't act on the links behind them.
	press(t, h, "M")
	if m.ask != nil {
		t.Error("M asked to merge while the links show")
	}
	// Backspace steps back to the conversation, and esc closes the modal.
	if _, ok := m.Act(ui.ActBack); !ok || m.refs != nil {
		t.Fatalf("back left the links open (%v), or wasn't taken", m.refs != nil)
	}
	if h.modal() != m {
		t.Error("back closed the modal")
	}
	press(t, h, "#")
	press(t, h, "esc")
	if h.modal() != nil {
		t.Error("esc over the links didn't close the modal")
	}
}

// Over the Checks tab, the links come and go and leave the tab as it was.
func TestPullModalReferencesOverChecks(t *testing.T) {
	c := &fakeChecks{}
	h, _ := startedRefs(t, WithChecks(c))
	m := h.modal()
	press(t, h, "C")
	if !m.onChecks() {
		t.Fatal("C didn't show the checks")
	}
	press(t, h, "#")
	if m.refs == nil || !m.onChecks() {
		t.Fatal("# over the checks didn't show the links over them")
	}
	if v := modalText(h); strings.Contains(v, "codecov") {
		t.Errorf("the checks show through the links:\n%s", v)
	}
	if cmd, ok := m.Act(ui.ActBack); !ok || m.refs != nil {
		t.Fatal("back didn't step back from the links")
	} else {
		drain(t, h, cmd)
	}
	if !m.onChecks() {
		t.Error("back left the Checks tab")
	}
	if v := modalText(h); !strings.Contains(v, "codecov") {
		t.Errorf("the checks don't show again:\n%s", v)
	}
}

// The links open an item in place of the modal, which the back key
// returns to.
func TestPullModalReferencesPickOpensWithBack(t *testing.T) {
	h, _ := startedRefs(t)
	m := h.modal()
	press(t, h, "#")
	got := press(t, h, "enter")
	var opened []ui.OpenIssueMsg
	for _, msg := range got {
		if o, ok := msg.(ui.OpenIssueMsg); ok {
			opened = append(opened, o)
		}
	}
	if len(opened) != 1 || opened[0].Number != 88 || opened[0].Repo != repo || opened[0].Back != m {
		t.Errorf("enter opened %+v, want #88 with the modal to go back to", opened)
	}
	// The modal, hidden, goes on showing the links when it is back.
	drain(t, h, h.Update(ui.ReopenedMsg{Modal: m}))
	if m.refs == nil {
		t.Error("the links closed when the modal was reopened")
	}
}

// The command works with the key unbound, and says so where there is no
// service for the links.
func TestPullModalShowReferences(t *testing.T) {
	h, _ := startedRefs(t)
	m := h.modal()
	cmd, ok := m.ShowReferences()
	if !ok {
		t.Fatal("the modal refuses to show the links")
	}
	drain(t, h, cmd)
	if m.refs == nil {
		t.Error("ShowReferences didn't show the links")
	}
	if cmd, ok := m.ShowReferences(); !ok || cmd != nil {
		t.Error("ShowReferences over the links did something")
	}
	if got := m.Commands(); !slices.Contains(got, ui.CommandReferences) {
		t.Errorf("Commands() = %v, want it to name the references command", got)
	}

	bare := started(t, newFakeService(), 100, 30)
	press(t, bare, "enter")
	if slices.Contains(bare.modal().Commands(), ui.CommandReferences) {
		t.Error("a modal without a service names the references command")
	}
	if _, ok := bare.modal().ShowReferences(); ok {
		t.Error("a modal without a service shows the links")
	}
	press(t, bare, "#")
	if bare.modal().refs != nil {
		t.Error("# showed links without a service")
	}
	if bare.modal().keys.References.Enabled() {
		t.Error("the key is enabled without a service")
	}
}

// The key waits for the answer of a question, and the command line with it.
func TestPullModalReferencesWhileAsking(t *testing.T) {
	h, _ := startedRefs(t)
	m := h.modal()
	press(t, h, "M")
	if m.ask == nil {
		t.Fatal("M didn't ask to merge")
	}
	press(t, h, "#")
	if m.refs != nil {
		t.Error("# showed the links over a question")
	}
	if cmd, ok := m.ShowReferences(); !ok || cmd != nil || m.refs != nil {
		t.Error("ShowReferences showed the links over a question")
	}
}

// The help lists the key of the links in the modal, and the keys of the
// links, and not those of the modal, while they show.
func TestPullModalReferencesKeyLayers(t *testing.T) {
	h, _ := startedRefs(t)
	m := h.modal()
	names := func() string {
		layers := m.KeyLayers()
		out := make([]string, 0, len(layers))
		for _, l := range layers {
			out = append(out, l.Context)
		}
		return strings.Join(out, ",")
	}
	if got := names(); got != "pull_modal,pull_conversation" {
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

// The modal gives the step what the item says of itself.
func TestPullModalReferencesReadsWithTheTitle(t *testing.T) {
	h, _ := startedRefs(t)
	press(t, h, "#")
	v := ansi.Strip(h.modals[len(h.modals)-1].View())
	if first, _, _ := strings.Cut(v, "\n"); !strings.Contains(first, "#142") {
		t.Errorf("the first line of the links is %q", first)
	}
}

// The links are read again when the pull request is read and found changed,
// and not when it is read and found as it was.
func TestPullModalRereadsTheLinksForAChange(t *testing.T) {
	h, f := startedRefs(t)
	m := h.modal()
	press(t, h, "#")
	reads := len(f.numbers())
	same := detailMsg{thread: m.thread.ID(), detail: m.detail}
	drain(t, h, m.Update(same))
	if got := len(f.numbers()); got != reads {
		t.Fatalf("an unchanged detail read the links %d more times", got-reads)
	}
	changed := m.detail
	changed.UpdatedAt = changed.UpdatedAt.Add(time.Hour)
	drain(t, h, m.Update(detailMsg{thread: m.thread.ID(), detail: changed}))
	if got := len(f.numbers()); got != reads+1 {
		t.Errorf("a changed detail read the links %d more times, want 1", got-reads)
	}
}

// The mouse scrolls the links, not the thread they cover.
func TestPullModalMouseDoesNotReachTheCoveredThread(t *testing.T) {
	svc := newFakeService()
	for n := range 40 {
		svc.thread = append(svc.thread, core.Comment{ID: strconv.Itoa(n), Body: "A comment, number " + strconv.Itoa(n)})
	}
	h := started(t, svc, 100, 20, WithReferences(&fakeRefs{}), WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, h, "enter")
	m := h.modal()
	wheel := tea.MouseWheelMsg{Button: tea.MouseWheelDown}
	top := ansi.Strip(m.thread.View())
	drain(t, h, m.Update(wheel))
	if ansi.Strip(m.thread.View()) == top {
		t.Fatal("the wheel doesn't scroll the thread, so the test shows nothing")
	}
	press(t, h, "g")
	top = ansi.Strip(m.thread.View())
	press(t, h, "#")
	drain(t, h, m.Update(wheel))
	if got := ansi.Strip(m.thread.View()); got != top {
		t.Error("the mouse scrolled the thread under the links")
	}
}

// A merge that waits for the detail is dropped when the links open, so its
// question doesn't pop up over them.
func TestPullModalReferencesDropAPendingMerge(t *testing.T) {
	h, _ := startedRefs(t)
	m := h.modal()
	merge := keyMsg("M")
	m.merging = &merge
	press(t, h, "#")
	if m.merging != nil {
		t.Fatal("the pending merge survived opening the links")
	}
	drain(t, h, m.Update(detailMsg{thread: m.thread.ID(), detail: m.detail}))
	if m.ask != nil {
		t.Error("the merge question popped up over the links")
	}
}
