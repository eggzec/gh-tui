package pulls

import (
	"errors"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/merge"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// jumpModal is the modal of convModal with its conversation read, and another
// pane focused.
func jumpModal(t *testing.T) *panesModal {
	t.Helper()
	h, m := convModal(t, convService())
	modalScreen(t, h)
	m.focus = paneDescription
	m.syncConvFocus()
	return m
}

func convCursor(t *testing.T, m *panesModal) string {
	t.Helper()
	id, _, ok := m.conv.model.Selected()
	if !ok {
		t.Fatal("no row under the cursor of the conversation")
	}
	return id
}

func TestJumpPersonLatest(t *testing.T) {
	t.Parallel()
	m := jumpModal(t)
	run(m, m.jumpConv(convWant{login: "bob"}))
	if m.focus != paneConversation || convCursor(t, m) != "r:R1" {
		t.Errorf("focus %v, cursor %q; want the Conversation on bob's review r:R1", m.focus, convCursor(t, m))
	}
	run(m, m.jumpConv(convWant{login: "dee"}))
	if got := convCursor(t, m); got != "p:T1-a" && got != "t:T1" {
		t.Errorf("cursor %q, want dee's thread t:T1", got)
	}
	run(m, m.jumpConv(convWant{login: "ann"}))
	if got := convCursor(t, m); got != "c:C1" {
		t.Errorf("cursor %q, want ann's comment c:C1", got)
	}
}

func TestJumpClearsFilter(t *testing.T) {
	t.Parallel()
	m := jumpModal(t)
	var only [conv.NumGroups]bool
	only[conv.GroupReviews] = true
	m.conv.model.SetShown(only)
	toasts := run(m, m.jumpConv(convWant{id: "C1"}))
	if !slices.Equal(toasts, []string{"filter cleared to show it"}) || convCursor(t, m) != "c:C1" || m.conv.model.FilterName() != "" {
		t.Errorf("toasts %q, cursor %q, filter %q; want the filter cleared and the comment under the cursor",
			toasts, convCursor(t, m), m.conv.model.FilterName())
	}
	if toasts := run(m, m.jumpConv(convWant{id: "R1"})); len(toasts) != 0 {
		t.Errorf("a jump that nothing hides toasted %q", toasts)
	}
}

// reads is the queue of what a modal's conversation asks for, run one step at
// a time so that a test sees the state between two pages.
type reads struct {
	m      *panesModal
	queue  []tea.Cmd
	toasts []string
}

// lateModal opens the first pull request over svc and gives it the detail
// without running the reads that starts: the conversation has begun and holds
// no page.
func lateModal(t *testing.T, svc *fakeService) *reads {
	t.Helper()
	svc.getErr = errors.New("boom")
	h, m := convModal(t, svc)
	cmd := m.receive(panesDetailMsg{id: m.id, detail: svc.detail(m.number)})
	if !m.conv.started {
		t.Fatal("the detail didn't start the conversation")
	}
	modalScreen(t, h) // lays the panes out, which sizes the conversation
	return &reads{m: m, queue: []tea.Cmd{cmd}}
}

// step runs the next command of the queue, and reports whether there was one.
func (r *reads) step() bool {
	if len(r.queue) == 0 {
		return false
	}
	cmd := r.queue[0]
	r.queue = r.queue[1:]
	if cmd == nil {
		return true
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		r.queue = append(r.queue, msg...)
	case ui.NotifyMsg:
		r.toasts = append(r.toasts, msg.Text)
	default:
		r.queue = append(r.queue, r.m.Update(msg))
	}
	return true
}

// until runs commands until done reports true.
func (r *reads) until(t *testing.T, done func() bool) {
	t.Helper()
	for !done() {
		if !r.step() {
			t.Fatal("the reads ended before the state the test waits for")
		}
	}
}

// drain runs every command that is left.
func (r *reads) drain() {
	for r.step() {
	}
}

func TestJumpBeforeLoad(t *testing.T) {
	t.Parallel()
	r := lateModal(t, convService())
	m := r.m
	r.queue = append(r.queue, m.jumpConv(convWant{login: "ann"}))
	if m.conv.want == nil || m.focus != paneConversation {
		t.Fatalf("want %v, focus %v; want the jump kept until the timeline loads, with the Conversation focused", m.conv.want, m.focus)
	}
	for r.step() {
		if !m.conv.model.Ready() && m.conv.want == nil {
			t.Fatal("the jump was dropped before the conversation could show the comment")
		}
	}
	if got := convCursor(t, m); got != "c:C1" || m.conv.want != nil {
		t.Errorf("cursor %q, want %v; want ann's comment c:C1 and the want met", got, m.conv.want)
	}
}

func TestJumpBeforeLoadKeepsFilter(t *testing.T) {
	t.Parallel()
	r := lateModal(t, convService())
	m := r.m
	var only [conv.NumGroups]bool
	only[conv.GroupReviews] = true
	m.conv.model.SetShown(only)
	r.queue = append(r.queue, m.jumpConv(convWant{id: "C1"}))
	// The timeline page comes before the threads', so the rows aren't built.
	for r.step() {
		if !m.conv.model.Ready() && (len(r.toasts) != 0 || m.conv.model.FilterName() == "" || m.conv.want == nil) {
			t.Fatalf("before the rows are built: toasts %q, filter %q, want %v; want the filter kept and the jump waiting",
				r.toasts, m.conv.model.FilterName(), m.conv.want)
		}
	}
	if got := convCursor(t, m); got != "c:C1" || m.conv.model.FilterName() != "" ||
		!slices.Equal(r.toasts, []string{"filter cleared to show it"}) {
		t.Errorf("cursor %q, filter %q, toasts %q; want c:C1, the filter cleared once, with a toast",
			got, m.conv.model.FilterName(), r.toasts)
	}
}

func TestJumpLatestWaitsForLaterPages(t *testing.T) {
	t.Parallel()
	svc := convService()
	svc.timeline.pages = map[string]core.Page[core.TimelineItem]{
		"": {Items: convTimeline, Next: "2"},
		"2": {Items: []core.TimelineItem{
			// A review between the comments keeps C9 out of the run of C1.
			{Kind: core.TimelineReview, ID: "R2", Actor: core.User{Login: "bob"}, At: convAt, Review: core.ReviewStateApproved},
			{Kind: core.TimelineComment, ID: "C9", Actor: core.User{Login: "ann"}, At: convAt, Body: "Later."}}},
	}
	r := lateModal(t, svc)
	m := r.m
	r.until(t, func() bool { return m.conv.model.Ready() && m.conv.model.Status().Loading })
	r.queue = append(r.queue, m.jumpConv(convWant{login: "ann"}))
	if m.conv.want == nil {
		t.Fatal("the jump was met on the first page, while a later page may hold ann's latest comment")
	}
	r.drain()
	if got := convCursor(t, m); got != "c:C9" || m.conv.want != nil {
		t.Errorf("cursor %q, want %v; want ann's latest comment c:C9", got, m.conv.want)
	}
}

func TestMergeFocusReviewLandsOnReview(t *testing.T) {
	t.Parallel()
	svc := convService()
	svc.timeline.pages = map[string]core.Page[core.TimelineItem]{"": {Items: []core.TimelineItem{
		{Kind: core.TimelineReview, ID: "R1", Actor: core.User{Login: "bob"}, At: convAt, Review: core.ReviewStateChangesRequested},
		{Kind: core.TimelineComment, ID: "C9", Actor: core.User{Login: "bob"}, At: convAt, Body: "Will you rebase?"},
	}}}
	h, m := convModal(t, svc)
	modalScreen(t, h)
	m.focus = paneMerge
	run(m, m.mergeUpdate(merge.FocusReviewMsg{Login: "bob"}))
	if m.focus != paneConversation || convCursor(t, m) != "r:R1" {
		t.Errorf("focus %v, cursor %q; want the Conversation on the review r:R1, not bob's later comment", m.focus, convCursor(t, m))
	}
}

func TestJumpKeepsZoom(t *testing.T) {
	t.Parallel()
	m := jumpModal(t)
	m.zoom.SetZoom(true)
	run(m, m.jumpConv(convWant{login: "ann"}))
	if !m.zoom.Zoomed() || m.focus != paneConversation {
		t.Errorf("zoom %v, focus %v; want the Conversation, zoomed", m.zoom.Zoomed(), m.focus)
	}
}

func TestShowReview(t *testing.T) {
	t.Parallel()
	m := jumpModal(t)
	run(m, m.showReview("R1"))
	if m.focus != paneConversation || convCursor(t, m) != "r:R1" {
		t.Errorf("focus %v, cursor %q; want the Conversation on r:R1", m.focus, convCursor(t, m))
	}
}

func TestEnterCommitShowsFiles(t *testing.T) {
	t.Parallel()
	_, m, _ := commitPanes(t, sampleCommits(nil))
	m.focus = paneConversation
	m.Update(conv.EnterMsg{Kind: conv.KindCommit, SHA: "ccc3333"})
	if m.focus != paneFiles || m.files.rng.First != 2 || m.files.rng.Last != 2 {
		t.Errorf("focus %v, range %+v; want Files with only the third commit", m.focus, m.files.rng)
	}
}

func TestEnterCIFocusesMerge(t *testing.T) {
	t.Parallel()
	m := jumpModal(t)
	m.Update(conv.EnterMsg{Kind: conv.KindCI, SHA: "ccc3333"})
	if m.focus != paneMerge {
		t.Errorf("focus %v, want Merge", m.focus)
	}
}
