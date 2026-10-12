package pulls

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// review makes a thread of the path and line, whose first comment is by
// author, with the comments that answer it.
func review(id, path string, line int, author, body string, replies ...core.ThreadComment) core.Thread {
	first := core.ThreadComment{ID: id + "c1", Author: core.User{Login: author}, Body: body, At: clock.Add(-26 * time.Hour)}
	return core.Thread{
		ID: id, Path: path, Line: line, Side: core.SideRight,
		Comments:      append([]core.ThreadComment{first}, replies...),
		TotalComments: 1 + len(replies),
	}
}

// reply makes a comment that answers a thread, two hours old.
func reply(author, body string) core.ThreadComment {
	return core.ThreadComment{ID: "r-" + author, Author: core.User{Login: author}, Body: body, At: clock.Add(-2 * time.Hour)}
}

// firstPage is a page of threads with one of each kind: on a line with a
// reply, outdated, on a whole file, and one that is resolved and so isn't
// shown. It has more pages after it.
func firstPage() core.ThreadPage {
	outdated := review("t2", "internal/legacy.go", 0, "monalisa", "This is gone in the next commit.")
	outdated.Outdated, outdated.OriginalLine = true, 12
	file := review("t3", "docs/design.md", 0, "octocat", "Needs a section on the disk layout.")
	file.File = true
	done := review("t4", "cmd/gh-tui/main.go", 12, "hubot", "Unused.")
	done.Resolved = true
	return core.ThreadPage{
		Threads: []core.Thread{
			review("t1", "internal/cache/disk.go", 45, "hubot", "Swallowing err hides a full disk.", reply("octocat", "Fixed, it is returned now.")),
			outdated, file, done,
		},
		Total: 6, Next: "p2",
	}
}

// secondPage is the page after firstPage, the last.
func secondPage() core.ThreadPage {
	return core.ThreadPage{
		Threads: []core.Thread{review("t5", "cmd/gh-tui/main.go", 12, "monalisa", "Why is the disk closed here?")},
		Total:   6,
	}
}

// threaded returns the modal of #142 on its Conversation tab, which read the
// threads svc serves, at width by height.
func threaded(tb testing.TB, svc *fakeService, width, height int, icons string) (*host, *detailModal) {
	tb.Helper()
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	h := started(tb, svc, width, height, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(icons)))
	h.SetTheme(theme(true))
	press(tb, h, "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter opened no pull request")
	}
	drain(tb, h, m.switchTo(conversationTab))
	return h, m
}

func TestThreadsBlock(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
		icons         string
	}{
		{"120x40", 116, 37, config.IconsUnicode},
		{"80x24", 76, 21, config.IconsUnicode},
		{"ascii", 76, 21, config.IconsASCII},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
			_, m := threaded(t, svc, tt.width, tt.height, tt.icons)
			v := m.View()
			lines := strings.Split(v, "\n")
			if len(lines) != tt.height {
				t.Errorf("the view has %d lines, want %d", len(lines), tt.height)
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != tt.width {
					t.Errorf("line %d is %d cells wide, want %d", i+1, w, tt.width)
				}
			}
			golden.RequireEqual(t, v)
		})
	}
}

func TestThreadsBlockContent(t *testing.T) {
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
	_, m := threaded(t, svc, 116, 37, config.IconsUnicode)
	v := ansi.Strip(m.View())
	for _, want := range []string{
		"Unresolved threads (3+)",
		"internal/cache/disk.go:45", "hubot", "Swallowing err hides a full disk.", "Fixed, it is returned now.",
		"internal/legacy.go:12 outdated", "This is gone in the next commit.",
		"docs/design.md", "Needs a section on the disk layout.",
		"Read more threads",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("the block lacks %q:\n%s", want, v)
		}
	}
	for _, not := range []string{"Unused.", "docs/design.md:"} {
		if strings.Contains(v, not) {
			t.Errorf("the block shows %q, which a resolved thread or a file has no use for:\n%s", not, v)
		}
	}
	if n := svc.threadReadCount(); n != 1 {
		t.Errorf("showing the tab read %d pages of threads, want 1", n)
	}
	if q := svc.threadReads[0]; q.Head != "a1b2c3d" || q.Number != 142 || q.Cursor != "" {
		t.Errorf("read %+v, want the first page of #142 at its head", q)
	}
}

func TestThreadsEnterShowsTheLine(t *testing.T) {
	t.Run("on a line", func(t *testing.T) {
		svc := newFakeService()
		svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
		h, m := threaded(t, svc, 100, 30, config.IconsUnicode)
		press(t, h, "enter")
		want := diff.Pos{Path: "internal/cache/disk.go", Side: diff.NewSide, Line: 45}
		if got, ok := m.files.diff.Position(); m.tab != filesTab || !ok || got != want {
			t.Errorf("enter showed tab %d with the cursor on %v (a line %v), want %v", m.tab, got, ok, want)
		}
	})
	t.Run("outdated and on a file go to the header", func(t *testing.T) {
		for i, path := range []string{"internal/legacy.go", "docs/design.md"} {
			svc := newFakeService()
			svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
			h, m := threaded(t, svc, 100, 30, config.IconsUnicode)
			for range i + 1 {
				press(t, h, "j")
			}
			press(t, h, "enter")
			if _, ok := m.files.diff.Position(); ok {
				t.Errorf("%s: the cursor is on a line, want it on the header of the file", path)
			}
			if cur, ok := m.files.diff.CurrentFile(); m.tab != filesTab || !ok || cur.Path != path {
				t.Errorf("enter showed tab %d on %q, want the file %s", m.tab, cur.Path, path)
			}
		}
	})
}

func TestThreadsMoreRowReadsThePage(t *testing.T) {
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": firstPage(), "p2": secondPage()}
	h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
	for range 3 {
		press(t, h, "j")
	}
	press(t, h, "enter")
	if m.tab != conversationTab {
		t.Fatalf("enter on the more row showed tab %d, want the conversation", m.tab)
	}
	if n := svc.threadReadCount(); n != 2 || svc.threadReads[1].Cursor != "p2" {
		t.Fatalf("threads were read %d times as %+v, want the first page and then p2", n, svc.threadReads)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Unresolved threads (4)", "cmd/gh-tui/main.go:12", "Why is the disk closed here?"} {
		if !strings.Contains(v, want) {
			t.Errorf("the block lacks %q after the second page:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Read more threads") {
		t.Errorf("the more row stays after the last page:\n%s", v)
	}
}

func TestThreadsMoreRowFails(t *testing.T) {
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
	h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
	svc.threadsErr = errors.New("boom")
	for range 3 {
		press(t, h, "j")
	}
	press(t, h, "enter")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Couldn't read more threads") || !strings.Contains(v, "Unresolved threads (3+)") {
		t.Errorf("a failed page isn't shown on the more row, with the threads kept:\n%s", v)
	}
	svc.threadsErr = nil
	svc.threadPages["p2"] = secondPage()
	press(t, h, "enter")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Unresolved threads (4)") {
		t.Errorf("enter on the more row didn't read the page again:\n%s", v)
	}
}

func TestThreadsEmptyAndError(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		svc := newFakeService()
		resolved := firstPage().Threads[3]
		svc.threadPages = map[string]core.ThreadPage{"": {Threads: []core.Thread{resolved}, Total: 1}}
		_, m := threaded(t, svc, 100, 30, config.IconsUnicode)
		if v := ansi.Strip(m.View()); strings.Contains(v, "Unresolved") || strings.Contains(v, "Unused.") {
			t.Errorf("a pull request with no unresolved threads shows a block:\n%s", v)
		}
	})
	t.Run("error", func(t *testing.T) {
		svc := newFakeService()
		svc.threadsErr = errors.New("boom")
		h, m := threaded(t, svc, 100, 30, config.IconsUnicode)
		v := ansi.Strip(m.View())
		if !strings.Contains(v, "Unresolved threads") || !strings.Contains(v, "Something went wrong · r to retry") {
			t.Errorf("the failure isn't shown on one line with how to retry:\n%s", v)
		}
		svc.threadsErr = nil
		svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
		press(t, h, "r")
		if v := ansi.Strip(m.View()); strings.Contains(v, "went wrong") || !strings.Contains(v, "Unresolved threads (3+)") {
			t.Errorf("r didn't read the threads again:\n%s", v)
		}
	})
}

func TestThreadsReadOnShowAndRefresh(t *testing.T) {
	svc := newFakeService()
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
	h := started(t, svc, 100, 30, WithChecks(&fakeChecks{}))
	press(t, h, "enter")
	m := h.modal()
	if n := svc.threadReadCount(); n != 0 {
		t.Fatalf("the overview read %d pages of threads, want none", n)
	}
	drain(t, h, m.switchTo(conversationTab))
	if n := svc.threadReadCount(); n != 1 {
		t.Fatalf("showing the conversation read %d pages, want 1", n)
	}
	drain(t, h, m.switchTo(filesTab))
	drain(t, h, m.switchTo(conversationTab))
	if n := svc.threadReadCount(); n != 1 {
		t.Errorf("showing the conversation again read %d pages, want it kept", n)
	}
	press(t, h, "r")
	if n := svc.threadReadCount(); n != 2 {
		t.Errorf("refresh read %d pages in all, want 2", n)
	}
}

// threadIDs returns the ids of the threads the block lists.
func threadIDs(m *detailModal) []string {
	ids := make([]string, 0, len(m.th.list))
	for i := range m.th.list {
		ids = append(ids, m.th.list[i].ID)
	}
	return ids
}

// onePage is a page of a single thread, the last.
func onePage(id string) core.ThreadPage {
	return core.ThreadPage{Threads: []core.Thread{review(id, "x/"+id+".go", 3, "monalisa", "Look at "+id+".")}, Total: 1}
}

func TestThreadsStaleResultsDropped(t *testing.T) {
	want := []string{"t1", "t2", "t3"}
	setup := func(t *testing.T) (*fakeService, *host, *detailModal) {
		t.Helper()
		svc := newFakeService()
		svc.threadPages = map[string]core.ThreadPage{"": firstPage(), "p2": secondPage()}
		h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
		return svc, h, m
	}
	t.Run("a first page of a read that was replaced", func(t *testing.T) {
		svc, h, m := setup(t)
		old := m.fetchThreads("", m.th.gen)()
		svc.threadPages[""] = onePage("t7")
		press(t, h, "r")
		if got := threadIDs(m); !slices.Equal(got, []string{"t7"}) {
			t.Fatalf("refresh shows %v, want t7", got)
		}
		drain(t, h, m.receiveThreads(old.(threadsMsg)))
		if got := threadIDs(m); !slices.Equal(got, []string{"t7"}) {
			t.Errorf("the page of the replaced read changed the threads to %v", got)
		}
	})
	t.Run("a next page of a read that was replaced", func(t *testing.T) {
		_, h, m := setup(t)
		old := m.fetchThreads("p2", m.th.gen)()
		press(t, h, "r")
		cmd := m.receiveThreads(old.(threadsMsg))
		if got := threadIDs(m); !slices.Equal(got, want) || cmd != nil {
			t.Errorf("a next page of the replaced read gave %v, want %v kept", got, want)
		}
	})
	t.Run("a page of a head the pull request left", func(t *testing.T) {
		svc, h, m := setup(t)
		old := m.fetchThreads("", m.th.gen)()
		m.detail.HeadSHA = "f00d"
		svc.threadPages[""] = onePage("t7")
		drain(t, h, m.reloadThreads())
		if q := svc.threadReads[len(svc.threadReads)-1]; q.Head != "f00d" {
			t.Fatalf("the read after the head changed was %+v, want the new head", q)
		}
		drain(t, h, m.receiveThreads(old.(threadsMsg)))
		if got := threadIDs(m); !slices.Equal(got, []string{"t7"}) {
			t.Errorf("the page of the old head changed the threads to %v", got)
		}
	})
	t.Run("a page for another thread", func(t *testing.T) {
		_, _, m := setup(t)
		cmd := m.receiveThreads(threadsMsg{thread: m.thread.ID() + 1, gen: m.th.gen, page: onePage("t9")})
		if got := threadIDs(m); !slices.Equal(got, want) || cmd != nil {
			t.Errorf("a page for another modal gave %v, want %v kept", got, want)
		}
	})
}

// A head that changed while another tab showed is read when the Conversation
// tab shows again.
func TestThreadsReadAgainAfterHeadChange(t *testing.T) {
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
	h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
	drain(t, h, m.switchTo(filesTab))
	m.detail.HeadSHA = "f00d"
	svc.threadPages[""] = onePage("t7")
	drain(t, h, m.switchTo(conversationTab))
	if q := svc.threadReads[len(svc.threadReads)-1]; svc.threadReadCount() != 2 || q.Head != "f00d" {
		t.Fatalf("reads %+v, want a second one of the new head", svc.threadReads)
	}
	if got := threadIDs(m); !slices.Equal(got, []string{"t7"}) {
		t.Errorf("the threads are %v, want those of the new head", got)
	}
}

func TestThreadsKeptReadAgainOnline(t *testing.T) {
	offline := fmt.Errorf("github: POST /graphql: %w", core.ErrOffline)
	t.Run("a first page served kept", func(t *testing.T) {
		svc := newFakeService()
		kept := firstPage()
		kept.Offline = true
		svc.threadPages = map[string]core.ThreadPage{"": kept}
		h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
		if !m.th.kept {
			t.Fatal("a page served offline isn't kept")
		}
		svc.threadPages[""] = onePage("t7")
		drain(t, h, h.Update(ui.OnlineMsg{}))
		drain(t, h, h.Update(ui.OnlineMsg{}))
		if n := svc.threadReadCount(); n != 2 {
			t.Errorf("two OnlineMsg read %d pages in all, want one more than the first", n)
		}
		if got := threadIDs(m); !slices.Equal(got, []string{"t7"}) || m.th.kept {
			t.Errorf("online shows %v (kept %v), want the fresh page", got, m.th.kept)
		}
	})
	t.Run("a next page served kept", func(t *testing.T) {
		svc := newFakeService()
		more := secondPage()
		more.Limited = true
		svc.threadPages = map[string]core.ThreadPage{"": firstPage(), "p2": more}
		h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
		for range 3 {
			press(t, h, "j")
		}
		press(t, h, "enter")
		if !m.th.kept {
			t.Fatal("a next page served rate limited isn't kept")
		}
		drain(t, h, h.Update(ui.OnlineMsg{}))
		if n := svc.threadReadCount(); n != 3 {
			t.Errorf("online read %d pages in all, want the first again", n)
		}
	})
	t.Run("a first page that couldn't be reached", func(t *testing.T) {
		svc := newFakeService()
		svc.threadsErr = offline
		h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
		if m.th.err == nil {
			t.Fatal("the failed read left no error")
		}
		svc.threadsErr = nil
		svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
		drain(t, h, h.Update(ui.OnlineMsg{}))
		if m.th.err != nil || len(m.th.list) != 3 {
			t.Errorf("online left error %v and %d threads, want the page", m.th.err, len(m.th.list))
		}
	})
	t.Run("a refusal isn't read again", func(t *testing.T) {
		svc := newFakeService()
		svc.threadsErr = core.ErrNotFound
		h, _ := threaded(t, svc, 116, 37, config.IconsUnicode)
		drain(t, h, h.Update(ui.OnlineMsg{}))
		if n := svc.threadReadCount(); n != 1 {
			t.Errorf("online read %d pages after a refusal, want none more", n)
		}
	})
	t.Run("a cached page shows at once", func(t *testing.T) {
		svc := newFakeService()
		svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
		h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
		svc.threadCache = map[string]core.ThreadPage{"": onePage("t8")}
		cmd := m.readThreads()
		if got := threadIDs(m); !slices.Equal(got, []string{"t8"}) {
			t.Fatalf("the cached page isn't shown before the read: %v", got)
		}
		drain(t, h, cmd)
		if got := threadIDs(m); !slices.Equal(got, []string{"t1", "t2", "t3"}) {
			t.Errorf("the read didn't replace the cached page: %v", got)
		}
	})
}

func TestThreadsCursorFollowsItsThread(t *testing.T) {
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
	h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
	press(t, h, "j")
	if id := m.th.rowIDAt(m.th.cursorRow()); id != "t2" {
		t.Fatalf("the cursor is on %q, want t2", id)
	}
	above := firstPage()
	above.Threads = append([]core.Thread{review("t0", "a/new.go", 1, "hubot", "New.")}, above.Threads...)
	svc.threadPages[""] = above
	press(t, h, "r")
	if row, id := m.th.cursorRow(), m.th.rowIDAt(m.th.cursorRow()); row != 2 || id != "t2" {
		t.Errorf("after a thread came above, the cursor is on row %d (%q), want t2 on row 2", row, id)
	}
	svc.threadPages[""] = onePage("t1")
	press(t, h, "r")
	if row := m.th.cursorRow(); row != 0 {
		t.Errorf("with its thread gone the cursor is on row %d, want it clamped to 0", row)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Unresolved threads (1)") {
		t.Errorf("the block of the one thread isn't shown:\n%s", v)
	}
}

// manyThreads is a page of n threads that each have a reply.
func manyThreads(n int) core.ThreadPage {
	p := core.ThreadPage{Total: n}
	for i := range n {
		id := "m" + strconv.Itoa(i)
		p.Threads = append(p.Threads, review(id, "pkg/"+id+".go", i+1, "hubot", "Comment "+id+".", reply("octocat", "Reply "+id+".")))
	}
	return p
}

func TestThreadsKeysFallThroughOffScreen(t *testing.T) {
	setup := func(t *testing.T) (*fakeService, *host, *detailModal) {
		t.Helper()
		svc := newFakeService()
		svc.threadPages = map[string]core.ThreadPage{"": manyThreads(20)}
		h, m := threaded(t, svc, 100, 14, config.IconsUnicode)
		return svc, h, m
	}
	t.Run("j scrolls when the next row is below the screen", func(t *testing.T) {
		_, h, m := setup(t)
		for range 40 {
			row, top := m.th.cursorRow(), m.thread.YOffset()
			press(t, h, "j")
			if m.th.cursorRow() != row {
				continue
			}
			if row == 0 || row >= len(m.th.list)-1 {
				t.Fatalf("the cursor stayed on row %d of %d", row, len(m.th.list))
			}
			if m.thread.YOffset() <= top {
				t.Errorf("j with the next row below the screen left the view at %d, want it scrolled", top)
			}
			return
		}
		t.Fatal("the cursor never reached the bottom of the screen")
	})
	t.Run("enter does nothing to a cursor off the screen", func(t *testing.T) {
		svc, h, m := setup(t)
		for range 80 {
			press(t, h, "j")
			if m.th.rows[m.th.cursorRow()] < m.thread.YOffset() {
				reads := svc.threadReadCount()
				press(t, h, "enter")
				if m.tab != conversationTab || svc.threadReadCount() != reads {
					t.Errorf("enter on a cursor above the screen switched to tab %d", m.tab)
				}
				return
			}
		}
		t.Fatal("scrolling never took the cursor row off the screen")
	})
}

func TestConversationKeyLayer(t *testing.T) {
	show := func(t *testing.T, m *detailModal) key.Binding {
		t.Helper()
		layers := m.KeyLayers()
		l := layers[len(layers)-1]
		if l.Context != ctxConversation {
			t.Fatalf("the last layer is %q, want the Conversation's", l.Context)
		}
		for _, b := range l.Bindings {
			if b.Help().Key == m.keys.showThread.Help().Key && b.Enabled() {
				return b
			}
		}
		return key.Binding{}
	}
	t.Run("rows", func(t *testing.T) {
		svc := newFakeService()
		svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
		h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
		if b := show(t, m); b.Help().Desc != "show in diff" {
			t.Errorf("the layer offers %+v on a thread, want show in diff", b.Help())
		}
		for range 3 {
			press(t, h, "j")
		}
		if b := show(t, m); b.Help().Desc != "read more" {
			t.Errorf("the layer offers %+v on the more row, want read more", b.Help())
		}
	})
	t.Run("none", func(t *testing.T) {
		svc := newFakeService()
		_, m := threaded(t, svc, 116, 37, config.IconsUnicode)
		if b := show(t, m); b.Enabled() {
			t.Errorf("the layer offers %+v with no threads", b.Help())
		}
	})
}

// A failed refresh keeps the threads that were read, with the failure above
// them.
func TestThreadsFailedRefreshKeepsRows(t *testing.T) {
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
	h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
	svc.threadsErr = errors.New("boom")
	press(t, h, "r")
	v := ansi.Strip(m.View())
	for _, want := range []string{"Unresolved threads (3+)", "went wrong", "internal/cache/disk.go:45"} {
		if !strings.Contains(v, want) {
			t.Errorf("the block lacks %q after a failed refresh:\n%s", want, v)
		}
	}
}
