package pulls

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// kept returns a store that holds drafts for #142.
func kept(drafts ...tray.Draft) *memTray {
	return &memTray{kept: map[string][]tray.Draft{tray.Key(repo.String(), 142): drafts}}
}

// byID returns the draft with id among drafts.
func byID(t *testing.T, drafts []tray.Draft, id string) tray.Draft {
	t.Helper()
	i := slices.IndexFunc(drafts, func(d tray.Draft) bool { return d.ID == id })
	if i < 0 {
		t.Fatalf("no draft %q in %+v", id, drafts)
	}
	return drafts[i]
}

func TestTrayListShowsAndJumps(t *testing.T) {
	t.Parallel()
	at := time.Unix(10, 0).UTC()
	h, m := drafting(t, kept(
		tray.Draft{ID: "a", Commit: "a1b2c3d", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 10, Body: "first\nsecond line", Created: at},
		tray.Draft{ID: "b", Commit: "a1b2c3d", Path: "internal/cache/disk.go", Side: tray.Right, Line: 45, Body: "second draft", Created: at},
	))
	press(t, h, "t")
	if m.tray.list == nil {
		t.Fatal("t opened no list")
	}
	v := modalText(h)
	for _, want := range []string{"Drafts (2)", "cmd/gh-tui/main.go:10", "first", "internal/cache/disk.go:45", "second draft"} {
		if !strings.Contains(v, want) {
			t.Errorf("the list lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "outdated") {
		t.Errorf("a draft on the head is shown as outdated:\n%s", v)
	}

	// The cursor moves, and stays in the list.
	press(t, h, "j")
	press(t, h, "j")
	if m.tray.list.id != "b" {
		t.Errorf("the cursor is on %q after two downs, want the last draft", m.tray.list.id)
	}
	press(t, h, "k")
	press(t, h, "k")
	if m.tray.list.id != "a" {
		t.Errorf("the cursor is on %q after two ups, want the first draft", m.tray.list.id)
	}

	// enter shows the draft under the cursor in the diff.
	press(t, h, "j")
	press(t, h, "enter")
	if m.tray.list != nil {
		t.Fatal("enter left the list open")
	}
	if b, ok := m.files.diff.CursorBlock(); !ok || b.ID != draftPrefix+"b" {
		t.Errorf("the diff's cursor is on %+v, want the draft b", b)
	}

	// esc and t close it.
	for _, k := range []string{"esc", "t"} {
		press(t, h, "t")
		press(t, h, k)
		if m.tray.list != nil {
			t.Errorf("%s left the list open", k)
		}
	}
}

func TestTrayListEmptyAndDelete(t *testing.T) {
	t.Parallel()
	store := kept(
		tray.Draft{ID: "a", Commit: "a1b2c3d", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 10, Body: "one"},
		tray.Draft{ID: "b", Commit: "a1b2c3d", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 10, Body: "two"},
	)
	h, m := drafting(t, store)
	press(t, h, "t")
	press(t, h, "D")
	if got := store.drafts(); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("D kept %+v, want only b", got)
	}
	if m.tray.list == nil || m.tray.list.id != "b" {
		t.Fatalf("the list = %+v after the delete, want it open on b", m.tray.list)
	}
	press(t, h, "D")
	if len(store.drafts()) != 0 || m.tray.list != nil {
		t.Errorf("deleting the last draft left %+v and the list %+v, want none and a closed list", store.drafts(), m.tray.list)
	}

	// With no drafts t says so and opens nothing.
	press(t, h, "t")
	if m.tray.list != nil {
		t.Error("t opened a list of no drafts")
	}
}

func TestDraftsOfAnOlderHeadSettleOnTheFiles(t *testing.T) {
	t.Parallel()
	// Find the text the diff shows for line 10, which a draft remembers.
	_, probe := drafting(t, nil)
	text, shown, _ := probe.files.diff.Line(diff.Pos{Path: "cmd/gh-tui/main.go", Side: diff.NewSide, Line: 10})
	if !shown || text == "" {
		t.Fatalf("line 10 of main.go is %q, shown %v", text, shown)
	}

	store := kept(
		// Its line reads the same: it moves to the head.
		tray.Draft{ID: "same", Commit: "old1", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 10, Text: text, Body: "a"},
		// Its line changed.
		tray.Draft{ID: "changed", Commit: "old1", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 10, Text: "something else", Body: "b"},
		// Its line is not in the diff.
		tray.Draft{ID: "gone", Commit: "old1", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 900, Text: "x", Body: "c"},
		// A draft with no text, from before it was kept, moves if its line shows.
		tray.Draft{ID: "bare", Commit: "old1", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 10, Body: "d"},
		// A reply is to a thread, whatever commit it was written on.
		tray.Draft{ID: "reply", Commit: "old1", Path: "cmd/gh-tui/main.go", ThreadID: "t1", Side: tray.Right, Line: 900, Body: "e"},
		// A draft on the head has nothing to settle.
		tray.Draft{ID: "head", Commit: "a1b2c3d", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 900, Text: "x", Body: "f"},
	)
	h, m := drafting(t, store)
	want := map[string]struct {
		commit   string
		outdated bool
	}{
		"same":    {"a1b2c3d", false},
		"changed": {"old1", true},
		"gone":    {"old1", true},
		"bare":    {"a1b2c3d", false},
		"reply":   {"old1", false},
		"head":    {"a1b2c3d", false},
	}
	for _, src := range [][]tray.Draft{m.tray.drafts, store.drafts()} {
		for id, w := range want {
			d := byID(t, src, id)
			if d.Commit != w.commit || d.Outdated != w.outdated {
				t.Errorf("draft %s = commit %q outdated %v, want %q and %v", id, d.Commit, d.Outdated, w.commit, w.outdated)
			}
		}
	}
	// The list tells which are outdated.
	press(t, h, "t")
	if v := modalText(h); strings.Count(v, "outdated") != 2 {
		t.Errorf("the list marks %d drafts outdated, want 2:\n%s", strings.Count(v, "outdated"), v)
	}
}

// Placing an outdated draft on a line again moves it to the head, with the
// text of the new line, and keeps its body and ID.
func TestTrayReplaceAnOutdatedDraft(t *testing.T) {
	t.Parallel()
	// The draft sits in another file, so that its row is not among those of main.go.
	store := kept(tray.Draft{ID: "o", Commit: "old1", Path: "internal/cache/disk.go", Side: tray.Right, Line: 900, Text: "old text", Body: "keep me", Outdated: true})
	h, m := drafting(t, store)
	press(t, h, "t")
	press(t, h, "c")
	if m.tray.list != nil || m.tray.replacing != "o" {
		t.Fatalf("c in the list left list %+v and replacing %q, want it closed and armed", m.tray.list, m.tray.replacing)
	}
	// Move to line 10 of main.go, the first line of its diff.
	press(t, h, "g")
	down(t, h, rowContext)
	press(t, h, "c")
	c := m.tray.comp
	if c == nil {
		t.Fatal("c on a line opened no composer")
	}
	d := c.d
	if d.ID != "o" || d.Commit != "a1b2c3d" || d.Path != "cmd/gh-tui/main.go" || d.Line != 10 || d.StartLine != 0 || d.Outdated || d.Text == "" || d.Text == "old text" || d.Body != "keep me" {
		t.Errorf("the draft in the composer = %+v, want o placed on line 10 at the head, with its text", d)
	}
	if m.tray.replacing != "" {
		t.Errorf("replacing = %q after the placement, want none", m.tray.replacing)
	}
	typeText(t, h, "!")
	press(t, h, "esc")
	got := store.drafts()
	if len(got) != 1 || got[0].ID != "o" || got[0].Line != 10 || got[0].Outdated || got[0].Commit != "a1b2c3d" || got[0].Body != "keep me!" {
		t.Errorf("kept %+v, want the one draft, placed", got)
	}
}

// With a placement armed, c on a line that has a thread places the draft
// there; it does not reply to the thread.
func TestTrayReplaceOnAThreadedLine(t *testing.T) {
	t.Parallel()
	store := kept(tray.Draft{ID: "o", Commit: "old1", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 900, Text: "old text", Body: "keep me", Outdated: true})
	h, m := drafting(t, store, review("t1", "internal/cache/disk.go", 45, "hubot", "Swallowing err hides a full disk."))
	press(t, h, "t")
	press(t, h, "c")
	if !m.files.diff.SeekBlock("t1") {
		t.Fatal("the thread is not in the diff")
	}
	press(t, h, "c")
	if m.tray.comp == nil {
		t.Fatal("c on the thread opened no composer")
	}
	if d := m.tray.comp.d; d.ID != "o" || d.ThreadID != "" || d.Path != "internal/cache/disk.go" || d.Line != 45 || d.Outdated {
		t.Errorf("the composer holds %+v, want o placed on line 45 of disk.go, not a reply", d)
	}
	press(t, h, "esc")

	// Without it, c on the thread replies.
	if !m.files.diff.SeekBlock("t1") {
		t.Fatal("the thread is not in the diff")
	}
	press(t, h, "c")
	if m.tray.comp == nil || m.tray.comp.d.ThreadID != "t1" {
		t.Errorf("c on a thread with nothing armed opened %+v, want a reply", m.tray.comp)
	}
}

// Only an outdated draft is placed again, and a reply stays with its thread.
func TestTrayListPlaceNeedsAnOutdatedDraft(t *testing.T) {
	t.Parallel()
	h, m := drafting(t, kept(
		tray.Draft{ID: "a", Commit: "a1b2c3d", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 10, Body: "fine"},
		tray.Draft{ID: "r", Commit: "old1", Path: "cmd/gh-tui/main.go", ThreadID: "t1", Side: tray.Right, Line: 10, Body: "reply"},
	))
	press(t, h, "t")
	press(t, h, "c")
	if m.tray.list == nil || m.tray.replacing != "" {
		t.Errorf("c on a draft that is not outdated left list %+v and replacing %q, want it open and nothing armed", m.tray.list, m.tray.replacing)
	}
	press(t, h, "j")
	press(t, h, "c")
	if m.tray.list == nil || m.tray.replacing != "" {
		t.Errorf("c on a reply left list %+v and replacing %q, want it open and nothing armed", m.tray.list, m.tray.replacing)
	}
}

// The list fills the modal: every line is as wide as it, so the frame keeps
// its size and the screen under it stays hidden.
func TestTrayListFillsTheModal(t *testing.T) {
	t.Parallel()
	h, m := drafting(t, kept(
		tray.Draft{ID: "a", Commit: "a1b2c3d", Path: "cmd/gh-tui/main.go", Side: tray.Right, StartLine: 10, Line: 12, Body: "first\nsecond line"},
		tray.Draft{ID: "b", Commit: "a1b2c3d", Path: "internal/cache/disk.go", Side: tray.Right, Line: 45, Body: strings.Repeat("long ", 60)},
	))
	press(t, h, "t")
	lines := strings.Split(m.trayListView(), "\n")
	if len(lines) != m.height {
		t.Fatalf("the list has %d lines, want %d", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d is %d cells wide, want %d: %q", i, w, m.width, l)
		}
	}
	if v := modalText(h); !strings.Contains(v, "cmd/gh-tui/main.go:10-12") {
		t.Errorf("the list shows no range as path:10-12:\n%s", v)
	}
}

// A draft on an empty line is told from a draft with no text kept: it goes
// outdated when its line is not empty on the head, and the first line of a
// range counts as well.
func TestDraftsSettleOnEmptyLinesAndRanges(t *testing.T) {
	t.Parallel()
	_, probe := drafting(t, nil)
	line := func(path string, n int) string {
		text, shown, _ := probe.files.diff.Line(diff.Pos{Path: path, Side: diff.NewSide, Line: n})
		if !shown {
			t.Fatalf("line %d of %s is not shown", n, path)
		}
		return text
	}
	const main = "cmd/gh-tui/main.go"
	store := kept(
		// Line 10 of main.go is not empty on the head.
		tray.Draft{ID: "was-empty", Commit: "old1", Path: main, Side: tray.Right, Line: 10, HasText: true, Body: "a"},
		// Line 2 of the new test file is empty, as it was.
		tray.Draft{ID: "still-empty", Commit: "old1", Path: "internal/cache/disk_test.go", Side: tray.Right, Line: 2, HasText: true, Body: "b"},
		// The last line of the range is as it was, and the first is not.
		tray.Draft{ID: "start", Commit: "old1", Path: main, Side: tray.Right, StartLine: 10, Line: 11, Text: line(main, 11), StartText: "changed", HasText: true, Body: "c"},
		tray.Draft{ID: "range", Commit: "old1", Path: main, Side: tray.Right, StartLine: 10, Line: 11, Text: line(main, 11), StartText: line(main, 10), HasText: true, Body: "d"},
	)
	_, m := drafting(t, store)
	for id, outdated := range map[string]bool{"was-empty": true, "still-empty": false, "start": true, "range": false} {
		if d := byID(t, m.tray.drafts, id); d.Outdated != outdated {
			t.Errorf("draft %s outdated = %v, want %v", id, d.Outdated, outdated)
		}
	}
}

// A placement that was left, by esc, another tab, the list or a range, does
// not move the draft at the next comment.
func TestTrayReplacingEnds(t *testing.T) {
	t.Parallel()
	store := kept(tray.Draft{ID: "o", Commit: "old1", Path: "internal/cache/disk.go", Side: tray.Right, Line: 900, Text: "old text", Body: "keep me", Outdated: true})
	h, m := drafting(t, store)
	arm := func() {
		t.Helper()
		press(t, h, "t")
		press(t, h, "c")
		if m.tray.replacing != "o" {
			t.Fatalf("replacing = %q, want armed", m.tray.replacing)
		}
	}

	// The hint shows in the status line while it is armed.
	arm()
	if v := modalText(h); !strings.Contains(v, "placing draft: c on a line") {
		t.Errorf("no hint while placing:\n%s", v)
	}
	press(t, h, "esc")
	if m.tray.replacing != "" || m.closed {
		t.Fatalf("esc left replacing %q, closed %v, want it ended and the modal open", m.tray.replacing, m.closed)
	}
	press(t, h, "g")
	down(t, h, rowContext)
	press(t, h, "c")
	if c := m.tray.comp; c == nil || c.d.ID == "o" {
		t.Fatalf("c after esc opened %+v, want a new draft", c)
	}
	press(t, h, "esc")

	// Another tab ends it.
	arm()
	drain(t, h, m.switchTo(overviewTab))
	if m.tray.replacing != "" {
		t.Errorf("leaving the Files tab left replacing %q", m.tray.replacing)
	}
	drain(t, h, m.switchTo(filesTab))

	// A range ends it, and a placement ends the range.
	arm()
	press(t, h, "g")
	down(t, h, rowContext)
	press(t, h, "V")
	if m.tray.replacing != "" || m.tray.from == nil {
		t.Errorf("a range left replacing %q and from %+v", m.tray.replacing, m.tray.from)
	}
	arm()
	if m.tray.from != nil {
		t.Errorf("a placement left the range %+v open", m.tray.from)
	}
}
