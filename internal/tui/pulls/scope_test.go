package pulls

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tray"
)

// scoped opens the Files tab of a pull request whose head is a1b2c3d, within
// scope, with one file for the range from "old" and one for the commit "c0ffee".
func scoped(t *testing.T, s filesScope) (*host, *detailModal, *fakeService) {
	t.Helper()
	return scopedAt(t, s, "a.go", 100)
}

// scopedAt is scoped with the path of the one file of the scope, and the
// width of the modal.
func scopedAt(t *testing.T, s filesScope, path string, width int) (*host, *detailModal, *fakeService) {
	t.Helper()
	svc := newFakeService()
	file := core.CommitFile{Path: path, Status: "modified", SHA: "blob", Additions: 1, Deletions: 1, Patch: "@@ -1 +1 @@\n-a\n+b\n"}
	svc.ranges = map[string]core.CompareDetail{"old": {Files: []core.CommitFile{file}}}
	svc.commitFiles = map[string][]core.CommitFile{"c0ffee": {file}}
	h, m := filed(t, svc, width, 30)
	m.tab = filesTab
	drain(t, h, m.setFilesScope(s))
	return h, m, svc
}

var sinceOld = filesScope{kind: scopeSince, base: "old", commits: 3}

func TestJumpToAThreadTheScopeLacksShowsAll(t *testing.T) {
	t.Parallel()
	oldSide := review("t1", "internal/cache/disk.go", 43, "hubot", "Swallowing err hides a full disk.")
	oldSide.Side = core.SideLeft
	elsewhere := review("t2", "cmd/gh-tui/main.go", 12, "hubot", "Why is the disk closed here?")
	for _, tt := range []struct {
		name string
		th   core.Thread
	}{
		{"on an old line", oldSide},
		{"in a file that didn't change", elsewhere},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, m, _ := scopedAt(t, sinceOld, "internal/cache/disk.go", 100)
			m.th.all = []core.Thread{tt.th}
			m.applyThreadBlocks()
			msgs := drain(t, h, m.showThreadAt(&m.th.all[0]))
			if m.files.scope.kind != scopeAll {
				t.Fatalf("scope = %v, want all to reach the thread", m.files.scope)
			}
			if !strings.Contains(fmt.Sprint(msgs), "Showing all the changes") {
				t.Errorf("msgs = %v, want the toast", msgs)
			}
			if got := ansi.Strip(m.filesView()); !strings.Contains(got, tt.th.Comments[0].Body) {
				t.Errorf("the diff doesn't show the thread:\n%s", got)
			}
			if cur, ok := m.files.diff.CurrentFile(); !ok || cur.Path != tt.th.Path {
				t.Errorf("the cursor is in %q, want the file of the thread", cur.Path)
			}
		})
	}
}

func TestJumpToAThreadTheScopeHasStaysInIt(t *testing.T) {
	t.Parallel()
	h, m, _ := scopedAt(t, sinceOld, "internal/cache/disk.go", 100)
	m.th.all = []core.Thread{review("t1", "internal/cache/disk.go", 45, "hubot", "Swallowing err hides a full disk.")}
	m.applyThreadBlocks()
	msgs := drain(t, h, m.showThreadAt(&m.th.all[0]))
	if m.files.scope.kind != scopeSince || strings.Contains(fmt.Sprint(msgs), "Showing all") {
		t.Errorf("scope = %v, msgs = %v, want the scope kept, with no toast", m.files.scope, msgs)
	}
}

func TestFinderOutsideTheScopeShowsAll(t *testing.T) {
	t.Parallel()
	h, m, _ := scopedAt(t, sinceOld, "internal/cache/disk.go", 100)
	m.findHead = m.detail.HeadSHA
	msgs := drain(t, h, m.chooseFile("cmd/gh-tui/main.go"))
	got := fmt.Sprint(msgs)
	if m.files.scope.kind != scopeAll || !strings.Contains(got, "Showing all the changes") || strings.Contains(got, "isn't changed") {
		t.Errorf("scope = %v, msgs = %v, want all, with the toast and no claim that the file isn't changed", m.files.scope, msgs)
	}
	if cur, ok := m.files.diff.CurrentFile(); !ok || cur.Path != "cmd/gh-tui/main.go" {
		t.Errorf("the diff is at %v, want main.go", cur.Path)
	}
}

func TestDraftJumpOutsideTheScopeShowsAll(t *testing.T) {
	t.Parallel()
	h, m, _ := scopedAt(t, sinceOld, "internal/cache/disk.go", 100)
	m.tray.drafts = []tray.Draft{{ID: "d1", Path: "cmd/gh-tui/main.go", Line: 12, Body: "Rename this."}}
	msgs := drain(t, h, m.showDraft("d1"))
	if m.files.scope.kind != scopeAll || !strings.Contains(fmt.Sprint(msgs), "Showing all the changes") {
		t.Fatalf("scope = %v, msgs = %v, want all with the toast", m.files.scope, msgs)
	}
	if cur, ok := m.files.diff.CurrentFile(); !ok || cur.Path != "cmd/gh-tui/main.go" {
		t.Errorf("the cursor is in %q, want the file of the draft", cur.Path)
	}
	if got := ansi.Strip(m.filesView()); !strings.Contains(got, "Rename this.") {
		t.Errorf("the diff doesn't show the draft:\n%s", got)
	}
}

func TestScopeTitlesKeepTheViewedCountAndThePath(t *testing.T) {
	t.Parallel()
	const subject = "fix the parser so that a long subject can't push the count out"
	for _, tt := range []struct {
		name  string
		scope filesScope
	}{
		{"since", filesScope{kind: scopeSince, base: "old", commits: 3}},
		{"commit", filesScope{kind: scopeCommit, sha: "c0ffee", subject: subject}},
	} {
		for _, width := range []int{76, 116} {
			t.Run(fmt.Sprintf("%s %d", tt.name, width), func(t *testing.T) {
				t.Parallel()
				_, m, _ := scopedAt(t, tt.scope, "internal/cache/disk.go", width)
				m.files.viewed = 1
				lines := strings.Split(ansi.Strip(m.filesView()), "\n")
				if len(lines) != m.height {
					t.Errorf("the view has %d lines, want %d", len(lines), m.height)
				}
				if !strings.Contains(lines[0], "internal/cache/disk.go") {
					t.Errorf("the first line %q lacks the path", lines[0])
				}
				if !m.filesNarrow() && !strings.Contains(lines[0], "1/1 viewed") {
					t.Errorf("the first line %q lacks the viewed count", lines[0])
				}
				if w := ansi.StringWidth(lines[0]); w != m.width {
					t.Errorf("the first line is %d wide, want %d", w, m.width)
				}
			})
		}
	}
}

func TestFilesScopeSinceReadsTheRange(t *testing.T) {
	t.Parallel()
	_, m, svc := scoped(t, filesScope{kind: scopeSince, base: "old", commits: 3})
	if len(svc.rangeReads) != 1 || svc.rangeReads[0] != (pulls.RangeQuery{Repo: m.repo, Base: "old", Head: "a1b2c3d"}) {
		t.Fatalf("range reads = %v, want one from old to the head", svc.rangeReads)
	}
	if n := svc.fileReadCount(); n != 0 {
		t.Errorf("the scope read %d pages of all the files, want none", n)
	}
	if got := m.files.diff.Files(); got != 1 {
		t.Errorf("the diff holds %d files, want 1", got)
	}
	if got := m.scopedFiles(); !strings.Contains(got, "Files") || !strings.Contains(got, "since you looked (3 commits)") {
		t.Errorf("title = %q, want the scope in it", got)
	}
}

func TestFilesScopeCommitReadsTheCommitAndRefusesComments(t *testing.T) {
	t.Parallel()
	h, m, svc := scoped(t, filesScope{kind: scopeCommit, sha: "c0ffee", subject: "fix parser"})
	if len(svc.commitReads) != 1 || svc.commitReads[0].SHA != "c0ffee" {
		t.Fatalf("commit reads = %v, want one of c0ffee", svc.commitReads)
	}
	if got := m.scopedFiles(); !strings.Contains(got, "c0ffee fix parser") {
		t.Errorf("title = %q, want the commit in it", got)
	}
	for _, k := range []string{"c", "V"} {
		if got := fmt.Sprint(press(t, h, k)); !strings.Contains(got, "Comment from All changes") || m.tray.comp != nil {
			t.Errorf("%s in a commit: msgs %q, composer %v; want the toast and no composer", k, got, m.tray.comp)
		}
	}
}

func TestFilesScopeResetsWhenTheHeadMoves(t *testing.T) {
	t.Parallel()
	h, m, _ := scoped(t, filesScope{kind: scopeSince, base: "old", commits: 1})
	m.detail.HeadSHA = "b2c3d4e"
	msgs := drain(t, h, m.checkHead())
	if m.files.scope.kind != scopeAll || m.files.head != "b2c3d4e" {
		t.Errorf("scope = %v at %q, want all at the new head", m.files.scope, m.files.head)
	}
	if !strings.Contains(fmt.Sprint(msgs), "head moved") {
		t.Errorf("msgs = %v, want the toast about the head", msgs)
	}
}

func TestFilesScopeResetsWhenTheHistoryWasRewritten(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.scopeErr = core.ErrRewritten
	h, m := filed(t, svc, 100, 30)
	m.tab = filesTab
	msgs := drain(t, h, m.setFilesScope(filesScope{kind: scopeSince, base: "old", commits: 1}))
	if m.files.scope.kind != scopeAll {
		t.Errorf("scope = %v, want all after a rewritten read", m.files.scope)
	}
	if !strings.Contains(fmt.Sprint(msgs), "rewritten") {
		t.Errorf("msgs = %v, want the toast about the rewrite", msgs)
	}
}

func TestCatchupRowOpensFilesSinceYouLooked(t *testing.T) {
	t.Parallel()
	h, m, svc := catching(t, lastSeen("old"), 100, aheadBy(2))
	rows := m.attentionRows()
	if len(rows) == 0 || rows[0].kind != attSince {
		t.Fatalf("rows = %v, want the since row first", rows)
	}
	for onRow(m) != rows[0].subject {
		before := m.cursorRow(rows)
		press(t, h, "k")
		if m.cursorRow(m.attentionRows()) == before {
			t.Fatalf("the cursor never reached the row")
		}
	}
	press(t, h, "enter")
	if m.tab != filesTab || m.files == nil || m.files.scope.kind != scopeSince || m.files.scope.commits != 2 {
		t.Fatalf("tab %v, files %v: want Files in the since scope of 2 commits", m.tab, m.files)
	}
	if len(svc.rangeReads) != 1 || svc.rangeReads[0].Base != "old" {
		t.Errorf("range reads = %v, want one from old", svc.rangeReads)
	}
}
