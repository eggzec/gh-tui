package pulls

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/merge"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// changesService serves a conversation in which bob asked for changes twice,
// carl once, and each review has threads of its own.
func changesService() *fakeService {
	svc := convService()
	before := convAt.Add(-time.Hour)
	svc.timeline.pages = map[string]core.Page[core.TimelineItem]{"": {Items: []core.TimelineItem{
		{Kind: core.TimelineReview, ID: "R0", Actor: core.User{Login: "bob"}, At: before, Review: core.ReviewStateChangesRequested, Body: "The first complaint."},
		{Kind: core.TimelineReview, ID: "R1", Actor: core.User{Login: "bob"}, At: convAt, Review: core.ReviewStateChangesRequested, Body: "Please fix the loop."},
		{Kind: core.TimelineReview, ID: "R2", Actor: core.User{Login: "carl"}, At: convAt, Review: core.ReviewStateChangesRequested, Body: "Carl wants docs."},
	}}}
	thread := func(id, review, path, body string) core.Thread {
		return core.Thread{
			ID: id, ReviewID: review, Path: path, Line: 7, TotalComments: 1,
			Comments: []core.ThreadComment{{ID: id + "-a", Author: core.User{Login: "bob"}, At: convAt, Body: body}},
		}
	}
	svc.threadPages = map[string]core.ThreadPage{"": {Total: 4, Threads: []core.Thread{
		thread("T0", "R0", "old.go", "An old remark."),
		thread("T1", "R1", "a/b.go", "This loop never ends."),
		thread("T2", "R1", "c/d.go", "Name this better."),
		thread("T3", "R2", "e/f.go", "Carl's remark."),
	}}}
	return svc
}

// changesModal is a modal over svc with its conversation read, the Merge pane
// focused and a pull request that bob and carl asked changes of.
func changesModal(t *testing.T, svc *fakeService, dark bool) *panesModal {
	t.Helper()
	h, m := convModal(t, svc)
	p, err := config.Default().Palette(dark)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(ui.NewTheme(p, dark))
	modalScreen(t, h)
	m.caps = writeCaps
	m.detail.ReviewDecision = core.ReviewChangesRequested
	m.detail.Reviewers.Verdicts = []core.Verdict{
		{Author: core.User{Login: "bob"}, State: core.ReviewStateChangesRequested, SubmittedAt: convAt},
		{Author: core.User{Login: "carl"}, State: core.ReviewStateChangesRequested, SubmittedAt: convAt},
	}
	m.focus = paneMerge
	m.merge.restyle(m)
	m.merge.refresh(m)
	m.View()
	return m
}

// changesPreview moves the cursor of the Merge pane to the Changes requested row
// of login and returns the preview of the row, 80 columns wide.
func changesPreview(t *testing.T, m *panesModal, login string) merge.Preview {
	t.Helper()
	for range 12 {
		if got := m.hover[paneMerge-1]; strings.Contains(strings.ToLower(got.title), login) && strings.HasSuffix(got.title, "changes requested") {
			p, ok := m.mergePreviewOf(panes.Box{Pane: paneMerge, Rect: panes.Rect{W: 82, H: 30}})
			if !ok {
				t.Fatal("the row has no preview")
			}
			return p
		}
		m.Update(keyMsg("j"))
	}
	t.Fatalf("no Changes requested row of %s; hovered %+v", login, m.hover[paneMerge-1])
	return merge.Preview{}
}

func TestChangesPreviewShowsOnlyTheLatestReviewOfTheReviewer(t *testing.T) {
	t.Parallel()
	svc := changesService()
	m := changesModal(t, svc, true)
	timeline, threads := len(svc.timelineReads()), len(svc.threadReadsSoFar())
	p := changesPreview(t, m, "bob")
	body := ansi.Strip(p.Body)
	for _, want := range []string{"Please fix the loop.", "This loop never ends.", "Name this better."} {
		if !strings.Contains(body, want) {
			t.Errorf("the preview lacks %q:\n%s", want, body)
		}
	}
	for _, not := range []string{"The first complaint.", "An old remark.", "Carl wants docs.", "Carl's remark."} {
		if strings.Contains(body, not) {
			t.Errorf("the preview shows %q, which is not bob's latest review:\n%s", not, body)
		}
	}
	if p.Info == "" {
		t.Error("the preview says nothing of the age of the review")
	}
	if got := len(svc.timelineReads()); got != timeline {
		t.Errorf("the preview read the timeline %d more times, want none", got-timeline)
	}
	if got := len(svc.threadReadsSoFar()); got != threads {
		t.Errorf("the preview read the threads %d more times, want none", got-threads)
	}

	body = ansi.Strip(changesPreview(t, m, "carl").Body)
	if !strings.Contains(body, "Carl wants docs.") || strings.Contains(body, "Please fix the loop.") {
		t.Errorf("the preview of carl's row:\n%s", body)
	}
}

func TestChangesPreviewWithoutTheReview(t *testing.T) {
	t.Parallel()
	t.Run("loading", func(t *testing.T) {
		t.Parallel()
		m := changesModal(t, changesService(), true)
		m.conv.started = false
		m.conv.version++
		body := ansi.Strip(changesPreview(t, m, "bob").Body)
		if !strings.Contains(body, "Reading the conversation") {
			t.Errorf("before the conversation is read the preview is:\n%s", body)
		}
		m.conv.started = true
		m.conv.version++
		if body := ansi.Strip(changesPreview(t, m, "bob").Body); !strings.Contains(body, "Please fix the loop.") {
			t.Errorf("once the conversation is read the preview is:\n%s", body)
		}
	})
	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		m := changesModal(t, convService(), true)
		body := strings.Join(strings.Fields(ansi.Strip(changesPreview(t, m, "bob").Body)), " ")
		if !strings.Contains(body, "not in the conversation") || !strings.Contains(body, "enter opens the Conversation") {
			t.Errorf("the preview of a review that is not loaded:\n%s", body)
		}
	})
}

func TestChangesPreviewEnterOpensTheConversation(t *testing.T) {
	t.Parallel()
	m := changesModal(t, changesService(), true)
	changesPreview(t, m, "bob")
	run(m, m.mergeUpdate(merge.FocusReviewMsg{Login: "bob"}))
	if m.focus != paneConversation || convCursor(t, m) != "r:R1" {
		t.Errorf("focus %v, cursor %q; want the Conversation on bob's latest review r:R1", m.focus, convCursor(t, m))
	}
}

// TestChangesPreviewGolden holds the preview in both themes, with the review
// and its two threads, while the conversation loads, and without the review.
func TestChangesPreviewGolden(t *testing.T) {
	t.Parallel()
	states := map[string]struct {
		svc     func() *fakeService
		loading bool
	}{
		"review":    {changesService, false},
		"loading":   {changesService, true},
		"not found": {convService, false},
	}
	for state, c := range states {
		for name, dark := range map[string]bool{"dark": true, "light": false} {
			t.Run(state+"/"+name, func(t *testing.T) {
				t.Parallel()
				m := changesModal(t, c.svc(), dark)
				if c.loading {
					m.conv.started = false
					m.conv.version++
				}
				p := changesPreview(t, m, "bob")
				// The test clock is fixed, so the ages are kept as they are.
				golden.RequireEqual(t, p.Title+" | "+p.Info+"\n"+p.Body+"\n")
			})
		}
	}
}
