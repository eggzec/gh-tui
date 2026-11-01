package pulls

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/merge"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// updaterService is a fake service that can update a branch, which the shared
// fake can't.
type updaterService struct {
	*fakeService
	mu      sync.Mutex
	updates []string
	err     error
}

func (u *updaterService) UpdateBranch(_ core.RepoRef, _ int, head string) *optimistic.Op {
	u.mu.Lock()
	u.updates = append(u.updates, head)
	err := u.err
	u.mu.Unlock()
	return optimistic.New(func(context.Context) error { return err })
}

func (u *updaterService) sent() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.updates...)
}

// mergePanes returns a modal with the Merge pane focused, over a loaded pull
// request that edit has changed.
func mergePanes(t *testing.T, edit func(*core.PullRequestDetail)) (*panesModal, *updaterService) {
	t.Helper()
	m := sizedPanes(t, 160, 40)
	svc := &updaterService{fakeService: m.svc.(*fakeService)}
	m.svc = svc
	m.caps = writeCaps
	m.loaded = true
	m.detail.HeadSHA = "abc123"
	m.detail.BaseSHA = "def456"
	m.detail.HeadRef, m.detail.BaseRef = "feature", "main"
	m.detail.Draft = false
	m.detail.Merge = core.MergeInfo{}
	m.detail.ReviewDecision = ""
	if edit != nil {
		edit(&m.detail)
	}
	m.nav.SetFocus(paneMerge)
	m.merge.restyle(m)
	m.merge.refresh(m)
	m.View()
	return m, svc
}

// enter presses the select key and feeds the row the tree chose back to the
// modal, as the program does with the command the key returned.
func enter(t *testing.T, m *panesModal) {
	t.Helper()
	cmd := m.Update(keymap.ActionPress("global.select"))
	msg, ok := find[treeview.OpenMsg](cmd)
	if !ok {
		t.Fatal("enter chose no row")
	}
	cmd = m.Update(msg)
	if cmd == nil {
		return
	}
	// The tree answers with the intent of the row, which the program feeds back.
	if got, ok := find[merge.UpdateBranchMsg](cmd); ok {
		m.Update(got)
	} else if got, ok := find[merge.MarkReadyMsg](cmd); ok {
		m.Update(got)
	} else if got, ok := find[merge.FocusReviewMsg](cmd); ok {
		m.Update(got)
	}
}

func behindEdit(d *core.PullRequestDetail) {
	d.Merge.Status = core.MergeBehind
}

func TestMergePaneShowsTheBlockers(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, func(d *core.PullRequestDetail) {
		behindEdit(d)
		d.Draft = true
	})
	screen := ansi.Strip(m.View())
	for _, want := range []string{"2 blockers", "Draft", "behind", "feature"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the screen lacks %q:\n%s", want, screen)
		}
	}
}

func TestMergePaneReadyHasNoCount(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, nil)
	if n := merge.Count(m.merge.data); n != 0 {
		t.Fatalf("%d blockers over a clean pull request", n)
	}
	if screen := ansi.Strip(m.View()); strings.Contains(screen, "blocker") {
		t.Errorf("a ready pull request shows a count:\n%s", screen)
	}
}

// checksFake keeps the checks of a commit once it has read them, as the
// service does.
type checksFake struct {
	mu   sync.Mutex
	kept map[string]core.CommitChecks
	err  error
}

func (c *checksFake) CachedCommitChecks(q actions.CommitChecksQuery) (core.CommitChecks, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	got, ok := c.kept[q.SHA]
	return got, ok
}

func (c *checksFake) CommitChecks(_ context.Context, q actions.CommitChecksQuery) (core.CommitChecks, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return core.CommitChecks{}, c.err
	}
	got := core.CommitChecks{SHA: q.SHA}
	if c.kept == nil {
		c.kept = map[string]core.CommitChecks{}
	}
	c.kept[q.SHA] = got
	return got, nil
}

// withChecks gives the pane of m a reader of the checks of the head, which has
// kept nothing, and returns the command that reads them.
func withChecks(m *panesModal, c *checksFake) tea.Cmd {
	p := m.merge
	p.reader, p.has = commitReader{c}, true
	p.loader, p.wanted = checktree.NewLoader(p.reader, m.repo), ""
	return p.refresh(m)
}

// results runs cmd, and the commands of a batch, and returns their messages.
func results(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	b, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range b {
		out = append(out, results(c)...)
	}
	return out
}

// readyShows reports whether the title of the Merge pane has the ready mark.
func readyShows(m *panesModal) bool {
	c := m.mergeContent(panes.Box{Rect: panes.Rect{W: 60, H: 12}})
	return slices.Contains(c.TopRight, m.merge.styles.Ready)
}

func TestMergePaneReadyWaitsForTheChecks(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, nil)
	c := &checksFake{}
	cmd := withChecks(m, c)
	if readyShows(m) {
		t.Error("ready shows before the checks are read")
	}
	for _, msg := range results(cmd) {
		m.Update(msg)
	}
	if !readyShows(m) {
		t.Error("ready doesn't show once the checks passed")
	}

	m, _ = mergePanes(t, nil)
	c = &checksFake{err: errors.New("offline")}
	for _, msg := range results(withChecks(m, c)) {
		m.Update(msg)
	}
	if readyShows(m) {
		t.Error("ready shows over checks that couldn't be read")
	}
}

func TestMergePaneTakesItsAnswersUnderAnOverlay(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, behindEdit)
	cmd := withChecks(m, &checksFake{})
	m.Update(keyMsg("U"))
	if _, ok := m.top().(*confirmOverlay); !ok {
		t.Fatalf("U left %T on top, want the question", m.top())
	}
	for _, msg := range results(cmd) {
		m.Update(msg)
	}
	if !m.merge.data.ChecksLoaded {
		t.Error("the checks that came under the question were lost")
	}
	n, ok := find[ui.NotifyMsg](m.Update(mergeSentMsg{owner: m, what: "update", done: "Updated feature with main"}))
	if !ok || !strings.Contains(n.Text, "Updated feature with main") {
		t.Errorf("the answer to the update was lost: %v, %v", n, ok)
	}
	m.Update(mergeBehindMsg{id: m.id, q: m.merge.asked, n: 3})
	if m.merge.behind != 3 {
		t.Errorf("behind %d, want the count that came under the question", m.merge.behind)
	}
}

func TestMergePaneUpdateBranchAsksFirst(t *testing.T) {
	t.Parallel()
	m, svc := mergePanes(t, behindEdit)
	m.Update(keyMsg("U"))
	if _, ok := m.top().(*confirmOverlay); !ok {
		t.Fatalf("U left %T on top, want the question", m.top())
	}
	if len(svc.sent()) != 0 {
		t.Fatal("the branch was updated before the answer")
	}
	cmd := m.Update(keyMsg("y"))
	msg, ok := find[mergeSentMsg](cmd)
	if !ok {
		t.Fatal("yes sent nothing")
	}
	if got := svc.sent(); len(got) != 1 || got[0] != "abc123" {
		t.Fatalf("updated %v, want the head it was asked at", got)
	}
	if !msg.invalidate || msg.settle != updateSettle || msg.err != nil {
		t.Errorf("answer %+v", msg)
	}
	cmd = m.Update(msg)
	if n, ok := find[ui.NotifyMsg](cmd); !ok || !strings.Contains(n.Text, "Updated feature with main") {
		t.Errorf("notice %v, %v", n, ok)
	}
}

func TestMergePaneNoAnswerUpdatesNothing(t *testing.T) {
	t.Parallel()
	m, svc := mergePanes(t, behindEdit)
	m.Update(keyMsg("U"))
	m.Update(keyMsg("n"))
	if len(svc.sent()) != 0 {
		t.Error("a no updated the branch")
	}
}

func TestMergePaneRefusedUpdateShowsTheReason(t *testing.T) {
	t.Parallel()
	m, svc := mergePanes(t, behindEdit)
	svc.err = errors.New("merge conflict")
	m.Update(keyMsg("U"))
	msg, ok := find[mergeSentMsg](m.Update(keyMsg("y")))
	if !ok {
		t.Fatal("yes sent nothing")
	}
	f, ok := find[ui.FailMsg](m.Update(msg))
	if !ok || !strings.Contains(f.Err.Error(), "merge conflict") {
		t.Errorf("failure %v, %v", f, ok)
	}
}

func TestMergePaneEnterOnBehindAsks(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, behindEdit)
	enter(t, m)
	if _, ok := m.top().(*confirmOverlay); !ok {
		t.Fatalf("enter on the behind row left %T on top", m.top())
	}
}

func TestMergePaneEnterOnDraftAsksToMarkReady(t *testing.T) {
	t.Parallel()
	m, svc := mergePanes(t, func(d *core.PullRequestDetail) { d.Draft = true })
	// The service holds the draft too, so a cached read shows what its change did.
	svc.mu.Lock()
	for i := range svc.pulls {
		svc.pulls[i].Draft = svc.pulls[i].Number == m.number
	}
	svc.cached[m.number] = true
	svc.mu.Unlock()
	enter(t, m)
	before := merge.Count(m.merge.data)
	if _, ok := m.top().(*confirmOverlay); !ok {
		t.Fatalf("enter on the draft row left %T on top", m.top())
	}
	if len(svc.changes()) != 0 {
		t.Fatal("marked ready before the answer")
	}
	msg, ok := find[mergeSentMsg](m.Update(keyMsg("y")))
	if !ok {
		t.Fatal("yes sent nothing")
	}
	if got := svc.changes(); len(got) != 1 || !strings.HasPrefix(got[0], "ready ") {
		t.Errorf("changes %v", got)
	}
	if msg.err != nil || msg.invalidate {
		t.Errorf("answer %+v", msg)
	}
	// The mutation changed the cache, so the row goes before GitHub answers.
	if after := merge.Count(m.merge.data); before == 0 || after != before-1 {
		t.Errorf("%d blockers after yes, want %d: the row stays until GitHub answers", after, before-1)
	}
}

func TestMergePaneEnterOnChangesRequestedFocusesTheConversation(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, func(d *core.PullRequestDetail) {
		d.Reviewers.Verdicts = []core.Verdict{{Author: core.User{Login: "ann"}, State: core.ReviewStateChangesRequested}}
	})
	if merge.Count(m.merge.data) == 0 {
		t.Fatal("the fixture's verdict doesn't block")
	}
	enter(t, m)
	if m.nav.Focus() != paneConversation {
		t.Errorf("focus %v, want the conversation", m.nav.Focus())
	}
}

func TestMergePaneIgnoresSpace(t *testing.T) {
	t.Parallel()
	m, svc := mergePanes(t, behindEdit)
	for _, k := range []string{"space", "R"} {
		m.Update(keyMsg(k))
		if m.top() != nil || len(svc.sent()) != 0 {
			t.Errorf("%s did something: %T", k, m.top())
		}
	}
}

func TestMergePaneDrawKeyFollowsTheCursor(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, func(d *core.PullRequestDetail) {
		behindEdit(d)
		d.Draft = true
	})
	before := m.merge.version
	m.Update(keyMsg("down"))
	if m.merge.version == before {
		t.Error("moving the cursor didn't change what the pane draws")
	}
}

func TestMergePaneModalKeysWin(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, behindEdit)
	m.Update(keyMsg("tab"))
	if m.nav.Focus() == paneMerge {
		t.Error("tab didn't leave the Merge pane")
	}
}
