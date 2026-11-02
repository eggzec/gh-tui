package pulls

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

func sampleCommits(review *core.ViewerReview) core.PullCommits {
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	return core.PullCommits{
		Commits: []core.PullCommit{
			{OID: "aaa1111", Headline: "first", CommittedAt: at},
			{OID: "bbb2222", Headline: "second", CommittedAt: at.Add(time.Hour)},
			{OID: "ccc3333", Headline: "third", CommittedAt: at.Add(2 * time.Hour)},
			{OID: "ddd4444", Headline: "fourth", CommittedAt: at.Add(3 * time.Hour)},
		},
		LastReview: review,
	}
}

// commitPanes opens the modal of the first pull request over commits and
// focuses the Files pane once the commits are read.
func commitPanes(tb testing.TB, pc core.PullCommits) (*host, *panesModal, *fakeCommits) {
	tb.Helper()
	h, m, svc := rangePanes(tb, pc, nil)
	return h, m, svc.pullCommits
}

// rangePanes is commitPanes with what the service serves set by setup first.
// The base of the pull requests is "base000".
func rangePanes(tb testing.TB, pc core.PullCommits, setup func(svc *fakeService), opts ...Option) (*host, *panesModal, *fakeService) {
	tb.Helper()
	svc := newFakeService()
	svc.tweaks = map[int]func(*core.PullRequestDetail){}
	for i := range svc.pulls {
		svc.pulls[i].HeadSHA = "ddd4444"
		svc.tweaks[svc.pulls[i].Number] = func(d *core.PullRequestDetail) { d.BaseSHA = "base000" }
	}
	svc.pullCommits = &fakeCommits{commits: pc}
	if setup != nil {
		setup(svc)
	}
	h := started(tb, svc, 160, 30, opts...)
	press(tb, h, "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter didn't open the modal")
	}
	if !m.files.loaded {
		tb.Fatalf("the commits were not read: %+v", m.files)
	}
	m.nav.SetFocus(paneFiles)
	modalScreen(tb, h)
	return h, m, svc
}

func markCommit(m *panesModal, oid string) {
	m.filesMsg(treeview.MarkMsg{Tree: m.files.tree.ID(), ID: files.CommitID(oid), Kind: files.KindCommit})
}

func TestFilesPaneReadsCommitsOnce(t *testing.T) {
	t.Parallel()
	_, m, fc := commitPanes(t, sampleCommits(nil))
	if got := m.files.rng; got != files.All(4) {
		t.Errorf("range = %+v, want every commit selected", got)
	}
	if len(fc.reads) != 1 || fc.reads[0].Head != "ddd4444" {
		t.Errorf("reads = %+v, want one read at the head", fc.reads)
	}
}

func TestFilesPaneSpaceExtendsRange(t *testing.T) {
	t.Parallel()
	_, m, _ := commitPanes(t, sampleCommits(nil))
	f := &m.files
	f.rng = files.Range{First: 1, Last: 1}
	markCommit(m, "ddd4444")
	if f.rng != (files.Range{First: 1, Last: 3}) {
		t.Errorf("marking a commit past the range gave %+v, want 1..3", f.rng)
	}
	markCommit(m, "bbb2222")
	if f.rng != (files.Range{First: 2, Last: 3}) {
		t.Errorf("marking the first commit gave %+v, want 2..3", f.rng)
	}
}

func TestFilesPaneSpaceKeepsOneCommit(t *testing.T) {
	t.Parallel()
	_, m, _ := commitPanes(t, sampleCommits(nil))
	m.files.rng = files.Range{First: 2, Last: 2}
	cmd := m.filesMsg(treeview.MarkMsg{Tree: m.files.tree.ID(), ID: files.CommitID("ccc3333"), Kind: files.KindCommit})
	if cmd == nil {
		t.Error("marking the only selected commit said nothing")
	}
	if m.files.rng != (files.Range{First: 2, Last: 2}) {
		t.Errorf("range = %+v, want it unchanged", m.files.rng)
	}
}

func TestFilesPaneIgnoresOtherTrees(t *testing.T) {
	t.Parallel()
	_, m, _ := commitPanes(t, sampleCommits(nil))
	m.files.rng = files.Range{First: 1, Last: 1}
	m.filesMsg(treeview.MarkMsg{Tree: m.files.tree.ID() + 1, ID: files.CommitID("ddd4444"), Kind: files.KindCommit})
	if m.files.rng != (files.Range{First: 1, Last: 1}) {
		t.Errorf("a mark of another tree changed the range to %+v", m.files.rng)
	}
}

func TestFilesPaneSelectAll(t *testing.T) {
	t.Parallel()
	h, m, _ := commitPanes(t, sampleCommits(nil))
	m.files.rng = files.Range{First: 2, Last: 2}
	press(t, h, "A")
	if m.files.rng != files.All(4) {
		t.Errorf("A gave %+v, want every commit", m.files.rng)
	}
}

func TestFilesPaneSinceReview(t *testing.T) {
	t.Parallel()
	review := &core.ViewerReview{Commit: "bbb2222", SubmittedAt: time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC)}
	h, m, _ := commitPanes(t, sampleCommits(review))
	press(t, h, "w")
	if m.files.rng != (files.Range{First: 2, Last: 3}) {
		t.Errorf("w gave %+v, want the commits after the reviewed one", m.files.rng)
	}
}

func TestFilesPaneSinceWithoutReview(t *testing.T) {
	t.Parallel()
	h, m, _ := commitPanes(t, sampleCommits(nil))
	m.files.rng = files.Range{First: 1, Last: 2}
	press(t, h, "w")
	if m.files.rng != (files.Range{First: 1, Last: 2}) {
		t.Errorf("w without a review changed the range to %+v", m.files.rng)
	}
}

func TestFilesPaneNewHeadResets(t *testing.T) {
	t.Parallel()
	_, m, _ := commitPanes(t, sampleCommits(nil))
	m.files.rng = files.Range{First: 1, Last: 2}
	pc := sampleCommits(nil)
	m.setCommits("ddd4444", pc)
	if m.files.rng != (files.Range{First: 1, Last: 2}) {
		t.Errorf("the same head changed the range to %+v", m.files.rng)
	}
	pc.Commits = append(pc.Commits, core.PullCommit{OID: "eee5555", Headline: "fifth"})
	m.setCommits("eee5555", pc)
	if m.files.rng != files.All(5) {
		t.Errorf("a new head gave %+v, want every commit", m.files.rng)
	}
}

func TestFilesPaneKeysKeepTheStyles(t *testing.T) {
	t.Parallel()
	h, m, _ := commitPanes(t, sampleCommits(nil))
	gen, seq := m.gen, m.files.seq
	if cmd, ok := m.pressFiles(tea.KeyPressMsg{Code: '~', Text: "~"}); ok || cmd != nil {
		t.Errorf("a key nothing uses was taken: ok=%v", ok)
	}
	if m.files.seq != seq {
		t.Error("a key nothing uses counted as a change")
	}
	press(t, h, "A")
	if m.files.seq == seq {
		t.Error("select all didn't count as a change")
	}
	if m.gen != gen {
		t.Errorf("gen = %d, want %d: keys of the pane don't change the theme or icons", m.gen, gen)
	}
}

func TestFilesPaneCommitChecksOpen(t *testing.T) {
	t.Parallel()
	pc := sampleCommits(nil)
	pc.Commits[2].Checks = core.CheckFailing
	fc := &fakeChecks{}
	h, m, _ := rangePanes(t, pc, nil, WithChecks(fc))
	f := &m.files
	// The commit with checks is a closed fold and asks for nothing yet.
	f.tree.Reveal(files.CommitID("ccc3333"))
	modalScreen(t, h)
	if !f.tree.Folded(files.CommitID("ccc3333")) {
		t.Fatal("the commit with checks isn't a closed fold")
	}
	if len(fc.commits) != 0 {
		t.Fatalf("read the checks of %v before the commit opened", fc.commits)
	}
	press(t, h, "l")
	screen := modalScreen(t, h)
	if len(fc.commits) != 1 || fc.commits[0] != "ccc3333" {
		t.Fatalf("read the checks of %v, want ccc3333", fc.commits)
	}
	if !strings.Contains(screen, "build") {
		t.Errorf("the checks aren't under the commit:\n%s", screen)
	}
	// Enter on its job asks to open the job; on a commit without checks it
	// does nothing.
	if len(f.targets) != 1 {
		t.Fatalf("targets = %v, want the job's", f.targets)
	}
	for id := range f.targets {
		cmd := m.filesMsg(treeview.OpenMsg{Tree: f.tree.ID(), ID: id})
		if cmd == nil {
			t.Fatal("enter on the job did nothing")
		}
		if got, want := cmd(), (checktree.OpenJobMsg{SHA: "ccc3333", RunID: 9, JobID: 5}); got != want {
			t.Errorf("enter on the job sent %v, want %v", got, want)
		}
	}
	if cmd := m.filesMsg(treeview.OpenMsg{Tree: f.tree.ID(), ID: files.CommitID("aaa1111")}); cmd != nil {
		t.Error("enter on a commit without checks did something")
	}
	// Closing the modal ends the reads.
	m.Discard()
}

func TestFilesPaneChecksWithoutService(t *testing.T) {
	t.Parallel()
	pc := sampleCommits(nil)
	pc.Commits[2].Checks = core.CheckFailing
	h, m, _ := rangePanes(t, pc, nil)
	m.files.tree.Reveal(files.CommitID("ccc3333"))
	modalScreen(t, h)
	press(t, h, "enter")
	if m.files.tree.Folded(files.CommitID("ccc3333")) {
		t.Error("a commit opened with no checks service")
	}
}

func TestShowCommit(t *testing.T) {
	t.Parallel()
	h, m, _ := commitPanes(t, sampleCommits(nil))
	m.nav.SetFocus(paneConversation)
	m.showCommit("ccc3333")
	if m.nav.Focus() != paneFiles {
		t.Errorf("focus = %v, want the Files pane", m.nav.Focus())
	}
	if m.files.rng != (files.Range{First: 2, Last: 2}) {
		t.Errorf("range = %+v, want only the commit", m.files.rng)
	}
	if m.files.tree.Folded(files.IDCommits) {
		t.Error("the commits are folded")
	}
	if screen := modalScreen(t, h); !strings.Contains(screen, "third") {
		t.Errorf("the screen doesn't show the commit:\n%s", screen)
	}
	if m.files.tree.Folded(files.IDCommits) {
		t.Error("drawing folded the commits again")
	}
	rng := m.files.rng
	m.showCommit("zzz9999")
	if m.files.rng != rng {
		t.Errorf("an unknown commit changed the range to %+v", m.files.rng)
	}
}
