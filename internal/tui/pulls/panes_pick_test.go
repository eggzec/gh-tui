package pulls

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// EditLabels shows the labels in add and takes those in remove off the pull
// request at once.
func (f *fakeService) EditLabels(_ core.RepoRef, number int, add []core.Label, remove []string) *optimistic.Op {
	var what strings.Builder
	what.WriteString("labels")
	for _, l := range add {
		fmt.Fprintf(&what, " +%s", l.Name)
	}
	for _, n := range remove {
		fmt.Fprintf(&what, " -%s", n)
	}
	return f.change(what.String(), number, func(pr *core.PullRequest) {
		kept := make([]core.Label, 0, len(pr.Labels)+len(add))
		for _, l := range pr.Labels {
			if !slices.Contains(remove, l.Name) {
				kept = append(kept, l)
			}
		}
		pr.Labels = slices.Concat(kept, add)
	})
}

// labelFacets offers four labels, one with a description.
type labelFacets struct{ fakeFacets }

func (labelFacets) Labels(context.Context, core.RepoRef) ([]core.Label, error) {
	return []core.Label{
		{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		{Name: "cache", Color: "0e8a16"},
		{Name: "docs", Color: "0075ca", Description: "Words about the code"},
		{Name: "Zed", Color: "5319e7"},
	}, nil
}

func (f *fakeService) labelsOf(number int) []string {
	return labelNames(f.state(number).Labels)
}

// labeling opens the modal of the first pull request, which has the label
// cache, over svc, with the keys that write.
func labeling(t *testing.T, svc *fakeService) (*host, *panesModal) {
	t.Helper()
	for i := range svc.pulls {
		svc.pulls[i].Labels = []core.Label{{Name: "cache", Color: "0e8a16"}}
	}
	h := started(t, svc, 160, 30, WithFacets(labelFacets{}))
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the modal")
	}
	return h, m
}

func labelsOverlayOf(t *testing.T, m *panesModal) *labelsOverlay {
	t.Helper()
	o, ok := m.top().(*labelsOverlay)
	if !ok {
		t.Fatalf("the top overlay is %T, want the labels picker", m.top())
	}
	return o
}

func selectedLabel(t *testing.T, o *labelsOverlay) string {
	t.Helper()
	it, ok := o.p.Selected()
	if !ok {
		t.Fatal("no label is selected")
	}
	return it.Value.(string)
}

func TestLabelsOpenFromPanes(t *testing.T) {
	t.Parallel()
	for _, focus := range []panes.Pane{panes.Conversation, panes.Files} {
		t.Run(focus.Name(), func(t *testing.T) {
			t.Parallel()
			h, m := labeling(t, newFakeService())
			m.focus = focus
			press(t, h, "L")
			labelsOverlayOf(t, m)
		})
	}
}

func TestLabelsView(t *testing.T) {
	t.Parallel()
	h, m := labeling(t, newFakeService())
	press(t, h, "L")
	o := labelsOverlayOf(t, m)
	if got := o.title(); got != "Labels · 1 selected" {
		t.Errorf("title is %q", got)
	}
	golden.RequireEqual(t, m.View())
}

// The footer names the moves once, as the decision says, and words the picker's
// keys for labels.
func TestLabelsFooter(t *testing.T) {
	t.Parallel()
	h, m := labeling(t, newFakeService())
	press(t, h, "L")
	help := labelsHelp{&labelsOverlayOf(t, m).p}
	shown := help.ShortHelp()
	hints := make([]string, 0, len(shown))
	for _, b := range shown {
		hints = append(hints, b.Help().Key+" "+b.Help().Desc)
	}
	want := []string{"j/k move", "/ search", "space toggle", "↵ apply", "esc cancel"}
	if !slices.Equal(hints, want) {
		t.Errorf("footer hints are %q, want %q", hints, want)
	}
}

// A read of the labels that answers for another modal, or after a picker is
// open, opens nothing.
func TestLabelsStaleReadIsDropped(t *testing.T) {
	t.Parallel()
	h, m := labeling(t, newFakeService())
	labels := []core.Label{{Name: "bug"}}
	m.labelsRead(labelsReadMsg{id: m.id + 1, labels: labels})
	if m.top() != nil {
		t.Fatal("a read for another modal opened the picker")
	}
	press(t, h, "L")
	o := labelsOverlayOf(t, m)
	m.labelsRead(labelsReadMsg{id: m.id, labels: labels})
	if m.top() != o {
		t.Error("a read over an open picker replaced it")
	}
}

func TestLabelsSearchMatchesDescription(t *testing.T) {
	t.Parallel()
	h, m := labeling(t, newFakeService())
	press(t, h, "L")
	press(t, h, "/")
	for k := range strings.SplitSeq("words", "") {
		press(t, h, k)
	}
	if got := selectedLabel(t, labelsOverlayOf(t, m)); got != "docs" {
		t.Errorf("a word of the description selected %q, want docs", got)
	}
}

func TestLabelsSpaceSortsAndKeepsTheCursorByIdentity(t *testing.T) {
	t.Parallel()
	h, m := labeling(t, newFakeService())
	press(t, h, "L")
	o := labelsOverlayOf(t, m)
	if got := selectedLabel(t, o); got != "cache" {
		t.Fatalf("the picker starts on %q, want the ticked cache", got)
	}
	press(t, h, "down")
	if got := selectedLabel(t, o); got != "bug" {
		t.Fatalf("down selected %q, want bug", got)
	}
	press(t, h, "space")
	if got := selectedLabel(t, o); got != "docs" {
		t.Errorf("space on bug left the cursor on %q, want the one below it, docs", got)
	}
	if got := o.title(); got != "Labels · 2 selected" {
		t.Errorf("title is %q after a tick", got)
	}
	// The ticked labels sort first, so bug now sits above cache.
	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "[x] bug") || strings.Index(screen, "[x] bug") > strings.Index(screen, "[x] cache") {
		t.Errorf("bug isn't listed with the ticked labels, before cache:\n%s", screen)
	}
}

func TestLabelsEnterApplies(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, m := labeling(t, svc)
	press(t, h, "L")
	press(t, h, "down")
	press(t, h, "space")
	if got := svc.changes(); len(got) != 0 {
		t.Fatalf("space sent %q", got)
	}
	// cache is ticked first: untick it too.
	press(t, h, "up")
	press(t, h, "up")
	if got := selectedLabel(t, labelsOverlayOf(t, m)); got != "bug" {
		t.Fatalf("up moved to %q, want bug", got)
	}
	press(t, h, "down")
	press(t, h, "space")
	press(t, h, "enter")

	if m.top() != nil {
		t.Error("enter left the picker open")
	}
	if got := svc.changes(); len(got) != 1 || !strings.HasPrefix(got[0], "labels +bug -cache") {
		t.Errorf("sent %q, want one change that adds bug and removes cache", got)
	}
	if names := labelNames(m.detail.Labels); !slices.Equal(names, []string{"bug"}) {
		t.Errorf("the header has labels %v, want [bug]", names)
	}
}

func labelNames(ls []core.Label) []string {
	names := make([]string, 0, len(ls))
	for _, l := range ls {
		names = append(names, l.Name)
	}
	return names
}

func TestLabelsUnchangedSendsNothing(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, m := labeling(t, svc)
	press(t, h, "L")
	press(t, h, "down")
	press(t, h, "space")
	// The cursor moved below bug, which is listed first now: back up to it.
	press(t, h, "up")
	press(t, h, "up")
	press(t, h, "space")
	press(t, h, "enter")
	if m.top() != nil {
		t.Error("enter left the picker open")
	}
	if got := svc.changes(); len(got) != 0 {
		t.Errorf("an unchanged set sent %q", got)
	}
}

func TestLabelsFailureRollsBack(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, m := labeling(t, svc)
	svc.sendErr = errors.New("boom")
	press(t, h, "L")
	press(t, h, "down")
	press(t, h, "space")
	msgs := press(t, h, "enter")

	var failed bool
	for _, msg := range msgs {
		if done, ok := msg.(ui.DoneMsg); ok && done.Err != nil {
			failed = true
		}
	}
	if !failed {
		t.Error("no failed DoneMsg, so no error toast")
	}
	if names := labelNames(m.detail.Labels); !slices.Equal(names, []string{"cache"}) {
		t.Errorf("the header has labels %v after the failure, want [cache]", names)
	}
	if got := svc.labelsOf(m.number); !slices.Equal(got, []string{"cache"}) {
		t.Errorf("the cache has labels %v after the failure, want [cache]", got)
	}
}

func TestLabelsEscAndBackCloseWithoutApplying(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, m := labeling(t, svc)

	press(t, h, "L")
	press(t, h, "down")
	press(t, h, "space")
	press(t, h, "esc")
	if m.top() != nil {
		t.Error("esc left the picker open")
	}

	press(t, h, "L")
	press(t, h, "down")
	press(t, h, "space")
	if _, ok := m.Act(ui.ActBack); !ok {
		t.Error("back reached the app while the picker was open")
	}
	if m.top() != nil {
		t.Error("back left the picker open")
	}
	if got := svc.changes(); len(got) != 0 {
		t.Errorf("closing sent %q", got)
	}
}

func TestLabelsEscClearsTheQueryFirst(t *testing.T) {
	t.Parallel()
	h, m := labeling(t, newFakeService())
	press(t, h, "L")
	press(t, h, "/")
	press(t, h, "z")
	press(t, h, "enter")
	press(t, h, "esc")
	if m.top() == nil {
		t.Fatal("esc closed the picker while a query was kept")
	}
	press(t, h, "esc")
	if m.top() != nil {
		t.Error("a second esc left the picker open")
	}
}
