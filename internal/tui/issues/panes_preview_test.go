package issues

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// fakePreviews answers every preview with a body of body lines, and counts
// the reads.
type fakePreviews struct {
	mu         sync.Mutex
	body       int
	issueReads []int
	pullReads  []int
}

func (f *fakePreviews) CachedIssuePreview(pulls.PreviewQuery) (core.IssuePreview, bool) {
	return core.IssuePreview{}, false
}

func (f *fakePreviews) IssuePreview(_ context.Context, q pulls.PreviewQuery) (core.IssuePreview, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issueReads = append(f.issueReads, q.Number)
	return core.IssuePreview{
		Target: core.Target{Repo: q.Repo, Number: q.Number, Kind: core.KindIssue}, Title: "Issue title",
		Author: "ada", Body: strings.Repeat("a line of the body\n", f.body),
	}, nil
}

func (f *fakePreviews) CachedComments(pulls.CommentsQuery) (core.Page[core.Comment], bool) {
	return core.Page[core.Comment]{}, false
}

func (f *fakePreviews) Comments(context.Context, pulls.CommentsQuery) (core.Page[core.Comment], error) {
	return core.Page[core.Comment]{}, nil
}

func (f *fakePreviews) CachedPullPreview(pulls.PreviewQuery) (core.PullPreview, bool) {
	return core.PullPreview{}, false
}

func (f *fakePreviews) PullPreview(_ context.Context, q pulls.PreviewQuery) (core.PullPreview, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pullReads = append(f.pullReads, q.Number)
	return core.PullPreview{
		Target: core.Target{Repo: q.Repo, Number: q.Number, Kind: core.KindPull}, Title: "Pull title",
		Author: "ada", Head: "fix", Base: "main",
	}, nil
}

// openedBlockers opens the first issue with the Blockers pane zoomed and its
// blocker (#12) under the cursor.
func openedBlockers(t *testing.T, f *fakePreviews) (*host, *detailModal) {
	t.Helper()
	h := started(t, blockedSvc(), 120, 30, WithPreviews(f))
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "*", "5")
	if !m.ctl.Zoom.Zoomed() {
		press(t, h, "z")
	}
	m.View() // sizes the tree, which takes no row moves before it has a size
	press(t, h, "l", "down")
	return h, m
}

func TestBlockersPreviewShowsTheHoveredIssue(t *testing.T) {
	t.Parallel()
	f := &fakePreviews{body: 3}
	h, m := openedBlockers(t, f)
	if want := []int{12}; !slices.Equal(f.issueReads, want) {
		t.Fatalf("issues read = %v, want %v", f.issueReads, want)
	}
	if len(f.pullReads) != 0 {
		t.Errorf("pull requests read: %v", f.pullReads)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "a line of the body") {
		t.Errorf("the preview doesn't show the blocker:\n%s", v)
	}
	// Hovering the row again reads nothing more.
	press(t, h, "up", "down")
	if len(f.issueReads) != 1 {
		t.Errorf("issues read = %v, want one read", f.issueReads)
	}
}

func TestBlockersPreviewScrolls(t *testing.T) {
	t.Parallel()
	f := &fakePreviews{body: 80}
	h, m := openedBlockers(t, f)
	m.View()
	id := m.slots.Hover(paneBlockers).ID
	if id == "" {
		t.Fatal("no row hovered")
	}
	press(t, h, "tab")
	m.View()
	if !m.ctl.Nav.InPreview(m.paneLayout()) {
		t.Fatal("tab didn't reach the preview")
	}
	press(t, h, "j")
	m.View()
	press(t, h, "j")
	m.View()
	if got := m.side.zoom.Top(); got != 2 {
		t.Fatalf("offset = %d after two j, want 2", got)
	}
	press(t, h, "tab")
	m.View()
	press(t, h, "tab")
	m.View()
	m.View()
	if got := m.side.zoom.Top(); got != 2 {
		t.Errorf("offset = %d after leaving and coming back, want 2", got)
	}
}

func TestDevelopmentPreviewReadsTheHoveredRow(t *testing.T) {
	t.Parallel()
	f := &fakePreviews{body: 3}
	refs := &fakeRefs{}
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 120, 30, WithReferences(refs), WithPreviews(f))
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "4")
	if !m.ctl.Zoom.Zoomed() {
		press(t, h, "z")
	}
	m.View()
	press(t, h, "down")
	m.View()
	if want := []int{91}; !slices.Equal(f.pullReads, want) {
		t.Fatalf("pull requests read = %v, want %v (issues %v)\n%s", f.pullReads, want, f.issueReads, ansi.Strip(m.View()))
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Pull title") {
		t.Errorf("the preview doesn't show the pull request:\n%s", v)
	}
}
