package pulls

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// onFirstLine moves the cursor of the diff to its first line, the context line.
func onFirstLine(t *testing.T, h *host, o *diffOverlay) {
	t.Helper()
	for range 10 {
		if _, ok := o.d.Position(); ok {
			return
		}
		press(t, h, "j")
	}
	t.Fatal("the cursor reached no line")
}

// keptDrafts returns the drafts of the modal's pull request.
func keptDrafts(m *panesModal) []tray.Draft { return m.reviewDrafts() }

// writeDraft writes a draft on the range from the first line to the one two
// rows down, with body.
func writeDraft(t *testing.T, h *host, m *panesModal, o *diffOverlay, body string) {
	t.Helper()
	onFirstLine(t, h, o)
	press(t, h, "V")
	press(t, h, "j")
	press(t, h, "j")
	press(t, h, "c")
	typeText(m, body)
	press(t, h, "ctrl+s")
}

func TestDraftRangeIsKeptOnItsLastLine(t *testing.T) {
	t.Parallel()
	h, m, o := diffPanes(t, "a.go")
	writeDraft(t, h, m, o, "rename it")
	got := keptDrafts(m)
	if len(got) != 1 || got[0].Body != "rename it" || got[0].StartLine != 1 || got[0].Line != 2 || got[0].Path != "a.go" {
		t.Fatalf("drafts = %+v, want one on a.go L1-2", got)
	}
	if _, ok := o.d.Anchor(); ok {
		t.Error("the range stayed after the draft was saved")
	}
	screen := modalScreen(t, h)
	if !strings.Contains(screen, "✎ draft L1–2: rename it") || !strings.Contains(screen, "1 draft") {
		t.Errorf("screen has no draft row or count:\n%s", screen)
	}
}

func TestDraftEscInTheComposerSavesNothing(t *testing.T) {
	t.Parallel()
	h, m, o := diffPanes(t, "a.go")
	onFirstLine(t, h, o)
	press(t, h, "c")
	typeText(m, "lost")
	press(t, h, "esc")
	if got := keptDrafts(m); len(got) != 0 {
		t.Errorf("drafts = %+v after esc, want none", got)
	}
	if m.top() != o {
		t.Errorf("top = %T after esc, want the diff", m.top())
	}
}

func TestDraftEditAndDelete(t *testing.T) {
	t.Parallel()
	h, m, o := diffPanes(t, "a.go")
	writeDraft(t, h, m, o, "one")
	press(t, h, ")")
	if _, ok := o.draftAt(); !ok {
		t.Fatal(") did not reach the draft")
	}
	press(t, h, "c")
	typeText(m, " more")
	press(t, h, "ctrl+s")
	got := keptDrafts(m)
	if len(got) != 1 || got[0].Body != "one more" {
		t.Fatalf("drafts = %+v, want the one edited", got)
	}
	press(t, h, "D")
	press(t, h, "n")
	if len(keptDrafts(m)) != 1 {
		t.Fatal("n deleted the draft")
	}
	press(t, h, "D")
	press(t, h, "y")
	if got := keptDrafts(m); len(got) != 0 {
		t.Errorf("drafts = %+v after y, want none", got)
	}
}

func TestDraftPreviousAndNextMoveBetweenDrafts(t *testing.T) {
	t.Parallel()
	h, m, o := diffPanes(t, "a.go")
	writeDraft(t, h, m, o, "one")
	press(t, h, "g")
	press(t, h, ")")
	if d, ok := o.draftAt(); !ok || d.Body != "one" {
		t.Fatalf("draft at the cursor = %+v, %v after )", d, ok)
	}
	press(t, h, "g")
	press(t, h, "G")
	press(t, h, "(")
	if _, ok := o.draftAt(); !ok {
		t.Error("( did not reach the draft")
	}
}

func TestDraftOnAFileHeaderIsOnTheFile(t *testing.T) {
	t.Parallel()
	h, m, o := diffPanes(t, "a.go")
	press(t, h, "g")
	if !o.d.OnHeader() {
		t.Fatal("g did not reach the header of the file")
	}
	press(t, h, "c")
	typeText(m, "about the file")
	press(t, h, "ctrl+s")
	got := keptDrafts(m)
	if len(got) != 1 || got[0].Line != 0 || got[0].Path != "a.go" {
		t.Fatalf("drafts = %+v, want one on the file", got)
	}
}

func TestDraftRangeStaysWithinOneHunk(t *testing.T) {
	t.Parallel()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.changed = patched()
		for i := range svc.changed {
			if svc.changed[i].Path == "a.go" {
				svc.changed[i].Patch = "@@ -1,2 +1,2 @@\n keep\n-old\n+new\n@@ -20,2 +20,2 @@\n ctx\n-o2\n+n2\n"
			}
		}
	})
	drain(t, h, m.openDiff("a.go"))
	o, ok := m.top().(*diffOverlay)
	if !ok {
		t.Fatalf("top overlay = %T, want the diff", m.top())
	}
	modalScreen(t, h)
	onFirstLine(t, h, o)
	press(t, h, "V")
	// Past the deleted and the added line, and the header of the second hunk.
	for range 4 {
		press(t, h, "j")
	}
	if p, ok := o.d.Position(); !ok || p.Line != 20 {
		t.Fatalf("cursor at %+v, %v; want the first line of the second hunk", p, ok)
	}
	n, ok := find[ui.NotifyMsg](o.comment())
	if !ok || n.Text != "A range stays within one hunk." {
		t.Fatalf("notice %q, %v; want the refusal of a range over two hunks", n.Text, ok)
	}
	if m.top() != o {
		t.Errorf("top = %T, want the diff without a composer", m.top())
	}
	if got := keptDrafts(m); len(got) != 0 {
		t.Errorf("drafts = %+v, want none", got)
	}
}

func TestDraftEscClearsTheRangeFirst(t *testing.T) {
	t.Parallel()
	h, _, o := diffPanes(t, "a.go")
	onFirstLine(t, h, o)
	press(t, h, "V")
	if _, ok := o.d.Anchor(); !ok {
		t.Fatal("V started no range")
	}
	if !o.clear() {
		t.Error("clear did nothing with a range")
	}
	if _, ok := o.d.Anchor(); ok {
		t.Error("the range stayed")
	}
}

func TestDraftStaleOnesAreNotSent(t *testing.T) {
	t.Parallel()
	drafts := []tray.Draft{{ID: "a", Commit: "old"}, {ID: "b", Commit: "head"}, {ID: "c"}}
	if got := staleDrafts(drafts, "head"); got != 1 {
		t.Errorf("staleDrafts = %d, want 1", got)
	}
}
