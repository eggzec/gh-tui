package refs

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// has fails the test unless the view has every want, a line each.
func has(t *testing.T, s *Step, want ...string) {
	t.Helper()
	v := strings.Join(lines(s), "\n")
	for _, w := range want {
		if !strings.Contains(v, w) {
			t.Errorf("view lacks %q:\n%s", w, v)
		}
	}
}

// lacks fails the test if the view has any of unwanted.
func lacks(t *testing.T, s *Step, unwanted ...string) {
	t.Helper()
	v := strings.Join(lines(s), "\n")
	for _, w := range unwanted {
		if strings.Contains(v, w) {
			t.Errorf("view has %q:\n%s", w, v)
		}
	}
}

// The groups start as the user wants them: what the item closes open, what
// its texts write open unless there are many, the mentions closed and
// unread, and the cursor on the first item, not on a group.
func TestRefsGroupsFold(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, narrowW, narrowH)
	has(t, s, "- Closes (2)", "- Written here (5)", "+ Mentioned in (47)", "#198 Token refresh fails", "#187 Keep")
	if got := selected(s); !strings.HasPrefix(got, "o #198") {
		t.Errorf("the cursor is on %q, want the first item", got)
	}
	if _, m := f.counts(); m != 0 {
		t.Errorf("the mentions were read %d times before they opened", m)
	}

	// h folds the group the cursor is in, and l opens it again.
	h.keys("h")
	if got := selected(s); !strings.Contains(got, "Closes (2)") {
		t.Fatalf("h left the cursor on %q, want the group", got)
	}
	h.keys("h")
	lacks(t, s, "#198")
	has(t, s, "+ Closes (2)", "- Written here (5)")
	h.keys("l")
	has(t, s, "#198")

	// * opens every group, reading the mentions, once, and then folds
	// them all.
	h.keys("*")
	has(t, s, "- Closes (2)", "- Written here (5)", "- Mentioned in (47)", "#198", "#187", "#240")
	if _, m := f.counts(); m != 1 {
		t.Errorf("opening every group read the mentions %d times, want 1", m)
	}
	h.keys("*")
	lacks(t, s, "#198", "#187", "#240")
	has(t, s, "+ Closes (2)", "+ Written here (5)", "+ Mentioned in (47)")
}

// A group of many links starts closed, so that the busy item's own links
// aren't buried.
func TestRefsBusyWrittenStartsClosed(t *testing.T) {
	f := newFake()
	r := testRefs()
	for n := range 11 {
		r.Written = append(r.Written, issue(repo, 300+n, "Written about "+strconv.Itoa(n), core.StateOpen, "", body()))
	}
	f.set(r)
	s, _ := newStep(t, f, narrowW, narrowH)
	has(t, s, "- Closes (2)", "+ Written here (16)")
	lacks(t, s, "#187")
}

// Mentioned in reads a page of them when it first opens, none before, and
// the row that ends the page reads the next.
func TestRefsLazyMentions(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, narrowW, narrowH)
	if _, m := f.counts(); m != 0 {
		t.Fatalf("%d reads of the mentions before they opened", m)
	}
	h.keys("G", "enter")
	_, m := f.counts()
	if m != 1 {
		t.Fatalf("%d reads of the mentions after opening them, want 1", m)
	}
	has(t, s, "Mentioned in (47)", "#240 Refresh", "#239", "... 43 more: enter reads the next 4")
	if q := f.mentionQueries[0]; q.Cursor != "" || q.Number != 231 || !q.Pull || q.Repo != repo {
		t.Errorf("the page read is %+v", q)
	}

	// Closing the group and opening it again costs nothing; the cache has it.
	h.keys("h", "l")
	if _, m := f.counts(); m != 1 {
		t.Errorf("opening again read the mentions %d times, want 1", m)
	}

	// The row at the end reads the next page, and the rows of the first
	// stay.
	h.keys("G", "enter")
	_, m = f.counts()
	if m != 2 {
		t.Fatalf("%d reads of the mentions after asking for more, want 2", m)
	}
	if c := f.mentionQueries[1].Cursor; c != "p2" {
		t.Errorf("the next page read is %q, want p2", c)
	}
	has(t, s, "#240 Refresh", "#241 Another", "#242 A last")
	lacks(t, s, "more: enter reads")
	// With every page read, the heading counts the rows listed.
	has(t, s, "Mentioned in (6")
}

// Picking an item opens it in place of the modal the step is in, as what
// it is, which the back key returns from.
func TestRefsPickOpensWithBack(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want tea.Msg
	}{
		{"an issue", nil, ui.OpenIssueMsg{Repo: repo, Number: 198, Back: ret}},
		{"a pull request", []string{"j", "j", "j"}, ui.OpenPullMsg{Repo: repo, Number: 187, Back: ret}},
		{"one of another repository", []string{"j", "j", "j", "j"}, ui.OpenPullMsg{Repo: other, Number: 412, Back: ret}},
		{"a mention", []string{"G", "enter", "j"}, ui.OpenPullMsg{Repo: repo, Number: 240, Back: ret}},
		{"a mention of another repository", []string{"G", "enter", "j", "j", "j"}, ui.OpenIssueMsg{Repo: infra, Number: 77, Back: ret}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, h := newStep(t, newFake(), narrowW, narrowH)
			h.keys(tt.keys...)
			h.keys("enter")
			got := h.take()
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("enter sent %v, want %v\n%s", got, tt.want, strings.Join(lines(s), "\n"))
			}
		})
	}
}

// Enter on a group folds it, and does not open anything.
func TestRefsEnterOnGroupFolds(t *testing.T) {
	s, h := newStep(t, newFake(), narrowW, narrowH)
	h.keys("k")
	if got := selected(s); !strings.Contains(got, "Closes") {
		t.Fatalf("the cursor is on %q", got)
	}
	h.keys("enter")
	if got := h.take(); len(got) != 0 {
		t.Errorf("enter on a group sent %v", got)
	}
	lacks(t, s, "#198")
}

// Where an item was found reads as the first two places, then how many
// more; a bot's login keeps its mark; and an item found twice is one row.
func TestRefsOrigins(t *testing.T) {
	f := newFake()
	r := testRefs()
	r.Written = []core.Reference{
		issue(repo, 50, "Found three times", core.StateOpen, "", body(), comment("bob"), origin(core.RefWritten, "review", "alice")),
	}
	r.Closing = []core.Reference{issue(repo, 198, "Closed and written", core.StateOpen, "", closes(), body())}
	f.set(r)
	s, _ := newStep(t, f, wideW, wideH)
	has(t, s, "open - closes, body", "open - body, comment by bob, +1")
	// The comment of a bot.
	f2 := newFake()
	s2, _ := newStep(t, f2, wideW, wideH)
	has(t, s2, "draft - comment by dependabot[bot]")
	if n := strings.Count(text(s), "#198"); n != 1 {
		t.Errorf("#198 is %d rows, want 1", n)
	}
}

// The item never links to itself, whatever its texts say.
func TestRefsSelfNeverShows(t *testing.T) {
	f := newFake()
	r := testRefs()
	r.Written = append([]core.Reference{pull(repo, 231, "This very pull request", core.StateOpen, false, body())}, r.Written...)
	f.set(r)
	s, _ := newStep(t, f, narrowW, narrowH)
	lacks(t, s, "This very")
	has(t, s, "Written here (5)")
}

// An item that can't be read has its row, which says why; enter says it in
// a toast and does not try to open it, and the open key goes to the page
// of the host the session talks to.
func TestRefsUnreadable(t *testing.T) {
	s, h := newStep(t, newFake(), wideW, wideH)
	has(t, s, "? octo/secret#9 can't be read: not found, or private")
	h.keys("j", "j", "j", "j", "j", "j", "j")
	if got := selected(s); !strings.HasPrefix(got, "? octo/secret#9") {
		t.Fatalf("the cursor is on %q", got)
	}
	h.keys("enter")
	got := h.take()
	want := ui.NotifyMsg{Level: toast.Warning, Text: "Can't read octo/secret#9: not found, or private. o opens it in the browser."}
	if len(got) != 1 || got[0] != want {
		t.Errorf("enter sent %v, want %v", got, want)
	}
	h.keys("o")
	if got := h.take(); len(got) != 1 || got[0] != (ui.OpenMsg{URL: "https://github.com/octo/secret/issues/9"}) {
		t.Errorf("o sent %v", got)
	}
}

// The open key goes to the pages of the host of the session, and the
// address of an item that can't be read, with the scheme the host serves.
func TestRefsOpenUsesTheHost(t *testing.T) {
	for _, tt := range []struct{ host, want string }{
		{"ghe.example.com", "https://ghe.example.com/octo/secret/issues/9"},
		{"github.localhost", "http://github.localhost/octo/secret/issues/9"},
	} {
		_, h := newStep(t, newFake(), wideW, wideH, WithHost(tt.host))
		h.keys("j", "j", "j", "j", "j", "j", "j", "o")
		if got := h.take(); len(got) != 1 || got[0] != (ui.OpenMsg{URL: tt.want}) {
			t.Errorf("host %s: o sent %v, want %s", tt.host, got, tt.want)
		}
	}
	// An item that was read opens at its own address, and a group at none.
	_, h := newStep(t, newFake(), wideW, wideH)
	h.keys("o")
	if got := h.take(); len(got) != 1 || got[0] != (ui.OpenMsg{URL: "https://github.com/octo/gh-tui/issues/198"}) {
		t.Errorf("o on an item sent %v", got)
	}
	h.keys("k", "o")
	if got := h.take(); len(got) != 0 {
		t.Errorf("o on a group sent %v", got)
	}
}

// The notes under "Written here" say what was left out, and are no links:
// enter does nothing there.
func TestRefsNotesAreInert(t *testing.T) {
	f := newFake()
	r := testRefs()
	r.Unresolved, r.CommentsRead, r.CommentsTotal = 12, 100, 340
	f.set(r)
	s, h := newStep(t, f, wideW, wideH)
	has(t, s, "Written here (5)", "searched the newest 100 of 340 comments", "12 more links not read")
	h.keys("G")
	h.keys("enter")
	if got := h.take(); len(got) != 0 {
		t.Errorf("enter on a note sent %v", got)
	}
	// The note after the group is on the row before the mentions.
	h.keys("k")
	if got := selected(s); !strings.Contains(got, "12 more links not read") {
		t.Errorf("the cursor is on %q", got)
	}
	h.keys("enter")
	if got := h.take(); len(got) != 0 {
		t.Errorf("enter on a note sent %v", got)
	}
}

// Typing filters every group at once, reads the first page of the mentions
// once however many keys are typed, and shows a group with a match open,
// with how many of its rows match.
func TestRefsFilterAcrossGroups(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, wideW, wideH)
	h.keys("&")
	if !s.TakesKeys() {
		t.Fatal("& didn't open the prompt")
	}
	h.typed("bob")
	if _, m := f.counts(); m != 1 {
		t.Errorf("typing read the mentions %d times, want 1", m)
	}
	has(t, s, "Written here (2 of 5)", "#187 Keep GraphQL", "cli/go-gh#412", "Mentioned in (1 of the first 4 of 47)", "#239 Read the viewer's")
	lacks(t, s, "Closes", "#233", "#240")
	h.typed("x")
	has(t, s, "No link matches")
	lacks(t, s, "Written here")

	// Words match anywhere in a row, in any order: the number, the title,
	// the state and the origins.
	for _, q := range []string{"merged", "GHES", "#77", "acme/infra", "comment bob", "closed completed", "review alice"} {
		h.keys("esc", "&")
		h.typed(q)
		if got := h.take(); len(got) != 0 {
			t.Fatalf("typing %q sent %v", q, got)
		}
		if strings.Contains(text(s), "No link matches") {
			t.Errorf("nothing matches %q:\n%s", q, strings.Join(lines(s), "\n"))
		}
	}
	if _, m := f.counts(); m != 1 {
		t.Errorf("the mentions were read %d times in all, want 1", m)
	}
}

// Enter keeps the filter, which shows in the first line, and gives the keys
// to the list; esc clears it, and a second esc asks to close the modal.
// Clearing the filter puts the folds back as they were.
func TestRefsFilterKeptAndCleared(t *testing.T) {
	s, h := newStep(t, newFake(), wideW, wideH)
	h.keys("h", "h")
	before := strings.Join(lines(s), "\n")
	h.keys("&")
	h.typed("bob")
	h.keys("enter")
	if s.TakesKeys() {
		t.Fatal("enter left the prompt open")
	}
	if !strings.Contains(lines(s)[0], "& bob") {
		t.Errorf("the first line doesn't show the filter: %q", lines(s)[0])
	}
	// The keys move the list again.
	h.keys("j", "j")
	if got := selected(s); !strings.Contains(got, "cli/go-gh#412") {
		t.Errorf("j moved to %q", got)
	}
	if got := s.Filter(); got != "bob" {
		t.Errorf("Filter() = %q", got)
	}

	h.keys("esc")
	if got := h.take(); len(got) != 0 {
		t.Errorf("the first esc sent %v, want it to clear the filter", got)
	}
	if s.Filter() != "" {
		t.Errorf("the filter is %q after esc", s.Filter())
	}
	if after := strings.Join(lines(s), "\n"); after != before+"" && !strings.Contains(after, "+ Closes (2)") {
		t.Errorf("the folds weren't put back:\n%s", after)
	}
	has(t, s, "+ Closes (2)", "Written here (5)")
	h.keys("esc")
	if got := h.take(); len(got) != 1 || got[0] != (CloseMsg{ID: s.ID()}) {
		t.Errorf("the second esc sent %v, want to close the modal", got)
	}
}

// In the prompt, esc clears the filter and closes it, enter keeps it, and
// backspace on an empty line closes it, as in the pager.
func TestRefsPromptKeys(t *testing.T) {
	s, h := newStep(t, newFake(), wideW, wideH)
	h.keys("&")
	h.typed("bob")
	h.keys("esc")
	if s.TakesKeys() || s.Filter() != "" {
		t.Errorf("esc left the prompt open (%v) with the filter %q", s.TakesKeys(), s.Filter())
	}
	if got := h.take(); len(got) != 0 {
		t.Errorf("esc in the prompt sent %v; it must not close the modal", got)
	}
	h.keys("&", "backspace")
	if s.TakesKeys() {
		t.Error("backspace on an empty line left the prompt open")
	}
	// An empty filter, kept, is none.
	h.keys("&", "enter")
	if s.TakesKeys() || s.Filter() != "" {
		t.Errorf("enter on an empty line: open %v, filter %q", s.TakesKeys(), s.Filter())
	}
	// Typed keys are text, not commands: q and * go in the line.
	h.keys("&")
	h.typed("q*")
	if got := s.prompt.Value(); got != "q*" {
		t.Errorf("the line is %q, want q*", got)
	}
}

// Reading the mentions to filter them leaves the group closed as it was.
func TestRefsFilterLoadsMentionsOnce(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, wideW, wideH)
	h.keys("&")
	h.typed("alice")
	has(t, s, "Mentioned in (1 of the first 4 of 47)")
	h.keys("esc")
	has(t, s, "+ Mentioned in")
	lacks(t, s, "#240")
}

// A heading counts the links shown of those read, and a group without a
// match is left out.
func TestRefsFilterHeadings(t *testing.T) {
	s, h := newStep(t, newFake(), wideW, wideH)
	h.keys("&")
	h.typed("closes")
	has(t, s, "Closes (2 of 2)")
	lacks(t, s, "Written here", "Mentioned in")
}

// The read that fails with nothing to show says so, in the words of the
// other views, and the refresh key reads again.
func TestRefsRateLimited(t *testing.T) {
	f := newFake()
	f.refsErr = &core.RateLimitError{Reset: testNow.Add(5 * time.Minute)}
	s, h := newStep(t, f, narrowW, narrowH, WithVoice(ui.Voice{Loc: time.UTC}))
	has(t, s, "Rate limited until 12:05", "loads again then")
	lacks(t, s, "Closes")

	f.mu.Lock()
	f.refsErr = nil
	f.mu.Unlock()
	h.keys("r")
	has(t, s, "Closes (2)")
	lacks(t, s, "Couldn't read")
	if f.invalidated != 1 {
		t.Errorf("r invalidated %d times, want 1", f.invalidated)
	}
}

// The links kept while GitHub can't be reached show, with a note that says
// when they were read.
func TestRefsOffline(t *testing.T) {
	f := newFake()
	r := testRefs()
	r.Offline = true
	f.set(r)
	s, _ := newStep(t, f, narrowW, narrowH)
	has(t, s, "Offline: showing the links read at 10:00.", "Closes (2)")

	r.Offline, r.Limited = false, true
	f.set(r)
	s, _ = newStep(t, f, narrowW, narrowH)
	has(t, s, "Rate limited: showing the links read at 10:00.")
}

// A mention page that fails to read is the error of its group, with the
// key that reads it again, and the links stay.
func TestRefsMentionsFail(t *testing.T) {
	f := newFake()
	f.pagesErr = fmt.Errorf("list mentions: %w", core.ErrOffline)
	s, h := newStep(t, f, narrowW, narrowH)
	h.keys("G", "enter")
	has(t, s, "Closes (2)", "Can't reach GitHub", "r to retry")
	f.mu.Lock()
	f.pagesErr = nil
	f.mu.Unlock()
	h.keys("r")
	has(t, s, "#240 Refresh")
	lacks(t, s, "Can't reach GitHub")
}

// The refresh key reads the links again, and what a text no longer links
// goes.
func TestRefsRefreshDropsEditedLinks(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, wideW, wideH)
	has(t, s, "#233 Page the timeline")
	r := testRefs()
	r.Written = slices.DeleteFunc(slices.Clone(r.Written), func(x core.Reference) bool { return x.Target.Number == 233 })
	f.set(r)
	h.keys("r")
	lacks(t, s, "#233 Page the timeline")
	has(t, s, "Written here (4)", "#187")
	if r, _ := f.counts(); r != 2 {
		t.Errorf("the links were read %d times, want 2", r)
	}
}

// Refreshing reads the mentions that were read, past the cache.
func TestRefsRefreshReadsMentionsAgain(t *testing.T) {
	f := newFake()
	_, h := newStep(t, f, wideW, wideH)
	h.keys("G", "enter")
	h.keys("r")
	if _, m := f.counts(); m != 2 {
		t.Fatalf("the mentions were read %d times, want 2", m)
	}
	if !f.mentionQueries[1].Again {
		t.Error("the refresh read the mentions from the cache")
	}
}

// Links kept by an earlier session show at once, and are read again.
func TestRefsStaleIsReadAgain(t *testing.T) {
	f := newFake()
	f.stale = true
	s, _ := newStep(t, f, narrowW, narrowH)
	has(t, s, "Closes (2)")
	if len(f.queries) != 2 || f.queries[0].Again || !f.queries[1].Again {
		t.Errorf("the reads were %+v, want one, and then one that goes past the kept copy", f.queries)
	}
}

// A copy in the cache shows before the first read is back, and the read
// carries what the modal knows of when the item changed.
func TestRefsShowsCachedAtOnce(t *testing.T) {
	f := newFake()
	f.cached = true
	s := New(t.Context(), f, self, true, config.Default().Keys, WithIcons(ui.NewIcons(config.IconsASCII)),
		WithItem(func() Item { return Item{Title: "T", Updated: testNow} }))
	s.SetTheme(testTheme())
	s.SetSize(narrowW, narrowH)
	if !s.loaded {
		t.Fatal("the cached links weren't taken")
	}
	h := &host{s: s}
	h.run(s.Init())
	has(t, s, "Closes (2)")
	if q := f.queries[0]; !q.Updated.Equal(testNow) || !q.Pull || q.Number != 231 {
		t.Errorf("the read was %+v", q)
	}
}

// Nothing linked either way says so, and what a link looks like.
func TestRefsEmpty(t *testing.T) {
	f := newFake()
	f.set(core.References{})
	s, _ := newStep(t, f, narrowW, narrowH)
	has(t, s, "Nothing links to octo/gh-tui#231, and it links to nothing.", "Links are #12, owner/repo#12")
}

// The step opens in the width it is given and the keys of an issue's
// links read the same.
func TestRefsIssueClosedBy(t *testing.T) {
	f := newFake()
	s := New(t.Context(), f, core.Target{Repo: repo, Number: 5, Kind: core.KindIssue}, false, config.Default().Keys,
		WithIcons(ui.NewIcons(config.IconsASCII)))
	s.SetTheme(testTheme())
	s.SetSize(narrowW, narrowH)
	(&host{s: s}).run(s.Init())
	has(t, s, "Closed by (2)")
	if q := f.queries[0]; q.Pull {
		t.Errorf("the query of an issue is of a pull request: %+v", q)
	}
}

// The mentions are read for the item's kind.
func TestRefsMentionsQueryKind(t *testing.T) {
	f := newFake()
	s := New(t.Context(), f, core.Target{Repo: repo, Number: 5, Kind: core.KindIssue}, false, config.Default().Keys,
		WithIcons(ui.NewIcons(config.IconsASCII)))
	s.SetTheme(testTheme())
	s.SetSize(narrowW, narrowH)
	h := &host{s: s}
	h.run(s.Init())
	h.keys("G", "enter")
	if len(f.mentionQueries) != 1 || f.mentionQueries[0].Pull || f.mentionQueries[0].Number != 5 {
		t.Errorf("the mentions were read as %+v", f.mentionQueries)
	}
}

// Once GitHub answers again, links that failed or were served kept are read
// again; fresh ones cost nothing.
func TestRefsOnline(t *testing.T) {
	f := newFake()
	f.refsErr = fmt.Errorf("list: %w", core.ErrOffline)
	s, h := newStep(t, f, narrowW, narrowH)
	has(t, s, "Can't reach GitHub")
	f.mu.Lock()
	f.refsErr = nil
	f.mu.Unlock()
	h.send(ui.OnlineMsg{})
	has(t, s, "Closes (2)")
	r, _ := f.counts()
	h.send(ui.OnlineMsg{})
	if again, _ := f.counts(); again != r {
		t.Errorf("an OnlineMsg read the fresh links again")
	}
}

// Back from the item the step opened, the links are read again behind what
// shows, and the folds, the filter and the cursor are as they were.
func TestRefsReopenedKeepsState(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, wideW, wideH)
	h.keys("G", "enter", "j", "j")
	h.keys("&")
	h.typed("alice")
	h.keys("enter")
	before := strings.Join(lines(s), "\n")
	h.send(ui.ReopenedMsg{Modal: ret})
	if after := strings.Join(lines(s), "\n"); after != before {
		t.Errorf("the view changed:\n%s\n---\n%s", before, after)
	}
	// Another modal's reopening isn't this step's.
	r, _ := f.counts()
	h.send(ui.ReopenedMsg{Modal: &retModal{}})
	if again, _ := f.counts(); again != r {
		t.Error("a message for another modal read the links")
	}
}

// Messages of other steps are dropped.
func TestRefsIgnoresOthers(t *testing.T) {
	s, h := newStep(t, newFake(), narrowW, narrowH)
	before := text(s)
	h.send(readMsg{id: s.ID() + 100, refs: core.References{}})
	if text(s) != before {
		t.Error("another step's links were taken")
	}
}

// A read for an older request doesn't replace a newer one.
func TestRefsDropsOldReads(t *testing.T) {
	s, h := newStep(t, newFake(), narrowW, narrowH)
	h.send(readMsg{id: s.ID(), seq: s.seq - 1, refs: core.References{}})
	has(t, s, "Closes (2)")
}

// Closing the step ends its reads.
func TestRefsCloseCancels(t *testing.T) {
	s, _ := newStep(t, newFake(), narrowW, narrowH)
	s.Close()
	if s.ctx.Err() == nil {
		t.Error("the step's reads go on after it closed")
	}
}

// The first page of the mentions is asked for once for a filter, so a read
// that fails isn't tried on every key; a new filter asks again.
func TestRefsFilterAsksForMentionsOncePerFilter(t *testing.T) {
	f := newFake()
	f.pagesErr = fmt.Errorf("list mentions: %w", core.ErrOffline)
	s, h := newStep(t, f, wideW, wideH)
	h.keys("&")
	h.typed("alice")
	if _, m := f.counts(); m != 1 {
		t.Errorf("typing five keys read the mentions %d times, want 1", m)
	}
	h.keys("esc", "&")
	h.typed("bob")
	if _, m := f.counts(); m != 2 {
		t.Errorf("a new filter read the mentions %d times in all, want 2", m)
	}
	_ = s
}

// Picking the row of more mentions puts the cursor on the first mention
// that the next page brought, in the place of the row that was under it.
func TestRefsMoreLandsOnTheFirstNewMention(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, wideW, wideH)
	h.keys("G", "enter")
	h.keys("G")
	if got := selected(s); !strings.Contains(got, "more: enter reads the next") {
		t.Fatalf("the cursor is on %q, want the row of more", got)
	}
	h.keys("enter")
	if got := selected(s); !strings.HasPrefix(got, "o #241 Another issue that mentions this one") {
		t.Errorf("the cursor is on %q, want the first mention of the second page", got)
	}
	// The cursor is the user's afterwards.
	h.keys("j")
	if got := selected(s); !strings.HasPrefix(got, "o #242") {
		t.Errorf("j moved to %q", got)
	}
}

// The links are read again when the modal asks, which costs a request only
// if the item changed since they were read.
func TestRefsRereadCarriesTheUpdateTime(t *testing.T) {
	f := newFake()
	updated := testNow
	s, h := newStep(t, f, narrowW, narrowH, WithItem(func() Item { return Item{Title: "T", Updated: updated} }))
	updated = testNow.Add(time.Hour)
	h.run(s.Reread())
	if len(f.queries) != 2 || !f.queries[1].Updated.Equal(updated) || f.queries[1].Again {
		t.Errorf("the reads were %+v, want a second with the new update time", f.queries)
	}
}
