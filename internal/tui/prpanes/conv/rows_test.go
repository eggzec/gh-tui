package conv

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

var now = at.Add(3 * 24 * time.Hour)

func theme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

// opts returns the options of rows drawn in width cells with the icon set.
func opts(set string, width int) Options {
	ic := ui.NewIcons(set)
	md := markdown.New(theme().Thread(ic).Markdown)
	md.SetGlyphs(ic.Markdown)
	return Options{Width: width, Styles: NewStyles(theme(), ic, set), MD: md, Now: now}
}

// byID returns the rows of d by their IDs.
func byID(rows []treeview.Node) map[string]treeview.Node {
	m := make(map[string]treeview.Node, len(rows))
	for i := range rows {
		m[rows[i].ID] = rows[i]
	}
	return m
}

func ids(rows []treeview.Node) []string {
	out := make([]string, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].ID)
	}
	return out
}

func TestDefaultFolds(t *testing.T) {
	t.Parallel()
	open, shut := thread("T", "R", 2), thread("T", "R", 2)
	shut.Resolved = true
	for _, tc := range []struct {
		name   string
		state  core.ReviewState
		thread core.Thread
		closed bool
	}{
		{"changes requested", core.ReviewStateChangesRequested, shut, false},
		{"commented with an unresolved thread", core.ReviewStateCommented, open, false},
		{"commented with resolved threads", core.ReviewStateCommented, shut, true},
		{"approved", core.ReviewStateApproved, open, true},
		{"dismissed", core.ReviewStateDismissed, open, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows := Build(Data{Items: []core.TimelineItem{review("R", tc.state)}, Threads: []core.Thread{tc.thread}}, opts(config.IconsUnicode, 100))
			r := byID(rows)["r:R"]
			if !r.Branch || r.Closed != tc.closed {
				t.Errorf("review: fold %v closed %v, want a fold, closed %v", r.Branch, r.Closed, tc.closed)
			}
		})
	}

	rows := byID(Build(Data{
		Items:   []core.TimelineItem{review("R", core.ReviewStateApproved), review("S", core.ReviewStateCommented), comment("1"), comment("2")},
		Threads: []core.Thread{open, shut},
	}, opts(config.IconsUnicode, 100)))
	if r := rows["r:S"]; r.Branch {
		t.Errorf("a review with no thread is a fold: %+v", r)
	}
	if r := rows["run:1"]; !r.Branch || !r.Closed {
		t.Errorf("run: fold %v closed %v, want a closed fold", r.Branch, r.Closed)
	}
	if r := rows["c:1"]; r.Branch {
		t.Error("a comment is a fold")
	}

	open.ID, shut.ID = "A", "B"
	two := Build(Data{Items: []core.TimelineItem{review("R", core.ReviewStateChangesRequested)}, Threads: []core.Thread{open, shut}}, opts(config.IconsUnicode, 100))
	m := byID(two)
	if m["t:A"].Closed || !m["t:B"].Closed {
		t.Errorf("unresolved thread closed %v, resolved thread closed %v, want open and closed", m["t:A"].Closed, m["t:B"].Closed)
	}
}

func TestEdited(t *testing.T) {
	t.Parallel()
	c := comment("1")
	c.UpdatedAt = at.Add(time.Hour)
	same := comment("2")
	same.UpdatedAt = same.At
	r := review("R", core.ReviewStateApproved)
	r.UpdatedAt = at.Add(time.Hour)
	edited := thread("E", "R", 2)
	edited.Comments[0].EditedAt = at.Add(time.Hour)
	edited.Comments[1].EditedAt = at.Add(time.Hour)
	plain := thread("P", "R", 1)

	rows := byID(Build(Data{
		Items:   []core.TimelineItem{c, r, same},
		Threads: []core.Thread{edited, plain},
	}, opts(config.IconsUnicode, 100)))
	for id, want := range map[string]bool{
		"c:1": true, "c:2": false, "r:R": false, "t:E": true, "p:E-b": true, "t:P": false,
	} {
		r, ok := rows[id]
		if !ok {
			t.Errorf("%s: no such row", id)
			continue
		}
		if got := strings.Contains(ansi.Strip(r.Right), "edited"); got != want {
			t.Errorf("%s: edited %v in %q, want %v", id, got, ansi.Strip(r.Right), want)
		}
	}
}

func TestRightOrder(t *testing.T) {
	t.Parallel()
	th := thread("T", "R", 1)
	th.Comments[0].Pending, th.Comments[0].EditedAt = true, at.Add(time.Hour)
	rows := byID(Build(Data{
		Items:   []core.TimelineItem{review("R", core.ReviewStateApproved)},
		Threads: []core.Thread{th},
	}, opts(config.IconsUnicode, 100)))
	if got, want := ansi.Strip(rows["t:T"].Right), "pending  edited  3d"; got != want {
		t.Errorf("right = %q, want %q", got, want)
	}
	if got, want := ansi.Strip(rows["r:R"].Right), "3d"; got != want {
		t.Errorf("review right = %q, want %q", got, want)
	}
}

func TestSpacing(t *testing.T) {
	t.Parallel()
	rows := Build(Data{
		Items:   []core.TimelineItem{comment("1"), review("R", core.ReviewStateChangesRequested), comment("2"), comment("3")},
		Threads: []core.Thread{thread("T", "R", 3)},
	}, opts(config.IconsUnicode, 100))
	want := "c:1 b:r:R r:R t:T p:T-b p:T-c b:run:2 run:2 rc:2 rc:3"
	if got := strings.Join(ids(rows), " "); got != want {
		t.Errorf("rows = %q, want %q", got, want)
	}
	for _, r := range rows {
		if blank := strings.HasPrefix(r.ID, "b:"); blank != r.Skip || blank != (r.Kind == KindBlank) {
			t.Errorf("%s: skip %v kind %q", r.ID, r.Skip, r.Kind)
		}
	}
	if got := Build(Data{}, opts(config.IconsUnicode, 100)); len(got) != 0 {
		t.Errorf("no data made rows: %v", ids(got))
	}
}

func TestGlyphWidths(t *testing.T) {
	t.Parallel()
	for _, set := range []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII} {
		ic := ui.NewIcons(set)
		g := newGlyphs(set, ic)
		marks := map[string]string{"dot": g.Dot, "dash": g.Dash}
		for name, gl := range map[string]ui.Glyph{
			"approved": ui.GlyphApproved, "changes": ui.GlyphChanges, "commented": ui.GlyphCommented,
			"dismissed": ui.GlyphDismissed, "unresolved": ui.GlyphUnresolved, "resolved": ui.GlyphResolved,
			"outdated": ui.GlyphOutdated, "pending": ui.GlyphCheckPending,
		} {
			marks[name] = ic.Glyph(gl)
		}
		for name, s := range marks {
			if w := ansi.StringWidth(s); w != 1 {
				t.Errorf("%s %s %q is %d cells wide", set, name, s, w)
			}
		}
		if set != config.IconsASCII {
			continue
		}
		marks["fold"], marks["cut"] = g.Folded, g.Cut
		for name, s := range marks {
			for _, r := range s {
				if r > 0x7e {
					t.Errorf("ascii %s %q is not ASCII", name, s)
				}
			}
		}
	}
}

func TestHostileText(t *testing.T) {
	t.Parallel()
	h := termtexttest.Hostile
	long := strings.Repeat("a line of a very long body\n\n", 5000)
	d := Data{
		Items: []core.TimelineItem{
			{Kind: core.TimelineComment, ID: "1", Actor: core.User{Login: h, Name: termtexttest.HostileLogin}, At: at, Body: h + "\n\tindented\there\r\n" + long},
			review("R", core.ReviewStateCommented),
		},
		Threads: []core.Thread{{
			ID: "T", ReviewID: "R", Path: "dir/" + h + ".go", Line: 4, TotalComments: 1,
			Comments: []core.ThreadComment{{ID: "c", Author: core.User{Login: h}, At: at, Body: "\t" + h}},
		}},
	}
	for _, set := range []string{config.IconsNerd, config.IconsASCII} {
		for _, w := range []int{40, 80, 160} {
			rows := Build(d, opts(set, w))
			m := treeview.New(treeview.Source{}, treeview.WithSize(w, 400))
			m.SetRows(rows)
			termtexttest.AssertClean(t, m.View(), w)
			for _, r := range rows {
				if len(r.Lines) > maxBodyLines+2 {
					t.Errorf("%s at %d: %s holds %d lines", set, w, r.ID, len(r.Lines))
				}
				for _, l := range r.Lines {
					if strings.Contains(l, "\t") {
						t.Errorf("%s at %d: %s has a tab: %q", set, w, r.ID, l)
					}
				}
			}
			if got := ansi.Strip(strings.Join(byID(rows)["c:1"].Lines, "\n")); !strings.Contains(got, "more lines") {
				t.Errorf("%s at %d: the long body isn't cut", set, w)
			}
		}
	}
}

// col returns the column of the first cell of word in the lines of text.
func col(t *testing.T, text, word string) int {
	t.Helper()
	for l := range strings.SplitSeq(text, "\n") {
		if before, _, ok := strings.Cut(l, word); ok {
			return ansi.StringWidth(before)
		}
	}
	t.Fatalf("%q isn't in\n%s", word, text)
	return 0
}

func TestHang(t *testing.T) {
	t.Parallel()
	rev := review("R", core.ReviewStateChangesRequested)
	rev.Body = "Firstpara of the review.\n\nSecondpara of the review."
	th := thread("T", "R", 2)
	th.Comments[0].Body = "Alphapara of the thread.\n\nBetapara of the thread."
	th.Comments[1].Body = "Gammapara of the reply.\n\nDeltapara of the reply."
	for _, w := range []int{60, 80, 120} {
		rows := Build(Data{Items: []core.TimelineItem{rev}, Threads: []core.Thread{th}}, opts(config.IconsASCII, w))
		out := text(rows, w, true)
		for _, tc := range [][2]string{{"Firstpara", "Secondpara"}, {"Alphapara", "Betapara"}, {"Gammapara", "Deltapara"}} {
			if head, next := col(t, out, tc[0]), col(t, out, tc[1]); head != next {
				t.Errorf("width %d: %s starts at column %d, %s at %d:\n%s", w, tc[0], head, tc[1], next, out)
			}
		}
	}
}

func TestDrafts(t *testing.T) {
	t.Parallel()
	o := opts(config.IconsUnicode, 100)
	th := thread("T", "", 2)
	th.Pending = true
	th.Comments[0].Pending, th.Comments[1].Pending = true, true
	posted := thread("P", "R", 2)
	rows := byID(Build(Data{
		Items:   []core.TimelineItem{review("R", core.ReviewStateCommented)},
		Threads: []core.Thread{th, posted},
	}, o))

	if got := ansi.Strip(rows["r:pending"].Lines[0]); !strings.Contains(got, "your pending review") {
		t.Errorf("pending review reads %q", got)
	}
	user := core.User{Login: "dee"}
	if got, want := rows["t:T"].Lines[0], o.Styles.review(core.ReviewStatePending)+" "+o.Styles.who(user, true); !strings.HasPrefix(got, want) {
		t.Errorf("pending thread head = %q, want it to start with %q", got, want)
	}
	if got, want := rows["p:T-b"].Lines[0], o.Styles.who(user, true); !strings.HasPrefix(got, want) {
		t.Errorf("pending reply head = %q, want it to start with %q", got, want)
	}
	if got, want := rows["t:P"].Lines[0], o.Styles.thread(false)+" "+o.Styles.who(user, false); !strings.HasPrefix(got, want) {
		t.Errorf("posted thread head = %q, want it to start with %q", got, want)
	}
}

func TestUnreadReplies(t *testing.T) {
	t.Parallel()
	th := thread("T", "R", 1)
	th.TotalComments = 4
	rows := byID(Build(Data{
		Items:   []core.TimelineItem{review("R", core.ReviewStateChangesRequested)},
		Threads: []core.Thread{th},
	}, opts(config.IconsUnicode, 100)))
	r := rows["t:T"]
	if !r.Branch || !r.Closed || !strings.Contains(ansi.Strip(r.Marker), "3 replies") {
		t.Errorf("fold %v closed %v marker %q, want a closed fold over 3 replies", r.Branch, r.Closed, ansi.Strip(r.Marker))
	}
}

func TestQuietHead(t *testing.T) {
	t.Parallel()
	o := opts(config.IconsUnicode, 100)
	th := thread("T", "R", 1)
	th.Resolved = true
	rows := byID(Build(Data{
		Items:   []core.TimelineItem{review("R", core.ReviewStateCommented)},
		Threads: []core.Thread{th},
	}, o))
	if head := rows["t:T"].Lines[0]; !strings.HasPrefix(head, o.Styles.thread(true)+" ") {
		t.Errorf("the head lost the glyph of a resolved thread: %q", head)
	}

	// Links are not made by the markdown but by the host, over the lines.
	const link = "\x1b]8;;https://example.com/docs\x1b\\docs\x1b]8;;\x1b\\"
	b := &builder{st: &o.Styles}
	got := b.quiet([]string{"\x1b[32mG\x1b[m ann  \x1b[1;31mSee\x1b[m " + link, "\x1b[1mmore\x1b[m"}, "\x1b[32mG\x1b[m ann")
	if !strings.HasPrefix(got[0], "\x1b[32mG\x1b[m ann  ") || !strings.Contains(got[0], link) {
		t.Errorf("the head lost its glyph colour or its link: %q", got[0])
	}
	for _, l := range got {
		if strings.Contains(l, "\x1b[1") {
			t.Errorf("the line is still bold: %q", l)
		}
	}
	if want := "See docs"; !strings.Contains(ansi.Strip(got[0]), want) {
		t.Errorf("the head reads %q, want %q in it", ansi.Strip(got[0]), want)
	}
}

func BenchmarkBuild(b *testing.B) {
	var d Data
	for i := range 300 {
		if i%3 == 2 {
			d.Items = append(d.Items, review(fmt.Sprint("R", i), core.ReviewStateCommented))
		} else {
			d.Items = append(d.Items, comment(strconv.Itoa(i)))
		}
	}
	for i := range 100 {
		d.Threads = append(d.Threads, thread(fmt.Sprint("T", i), fmt.Sprint("R", i*3+2), 3))
	}
	o := opts(config.IconsNerd, 160)
	b.ReportAllocs()
	for b.Loop() {
		Build(d, o)
	}
}

// text draws rows in a tree of width cells, without colors or links, and
// with every fold opened when all is set.
func text(rows []treeview.Node, width int, all bool) string {
	m := treeview.New(treeview.Source{}, treeview.WithSize(width, 400))
	m.SetRows(rows)
	if all {
		for i := range rows {
			if rows[i].Branch {
				m.SetFolded(rows[i].ID, false)
			}
		}
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func TestRowsGolden(t *testing.T) {
	t.Parallel()
	d := fixture()
	for _, set := range []string{config.IconsNerd, config.IconsASCII} {
		for _, w := range []int{80, 120, 160} {
			for _, all := range []bool{false, true} {
				name := fmt.Sprintf("%s-%d", set, w)
				if all {
					name += "-open"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					golden.RequireEqual(t, text(Build(d, opts(set, w)), w, all))
				})
			}
		}
	}
}

// fixture is a conversation of every kind of row.
func fixture() Data {
	user := func(login, name string) core.User { return core.User{Login: login, Name: name} }
	bot := core.User{Login: "dependabot", Bot: true}
	long := "https://github.com/eggzec/gh-tui/pull/123/files#diff-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ago := func(h int) time.Time { return now.Add(-time.Duration(h) * time.Hour) }
	item := func(kind core.TimelineKind, id string, u core.User, h int, body string) core.TimelineItem {
		return core.TimelineItem{Kind: kind, ID: id, Actor: u, At: ago(h), Body: body}
	}
	rev := func(id string, u core.User, st core.ReviewState, h int, body string) core.TimelineItem {
		it := item(core.TimelineReview, id, u, h, body)
		it.Review = st
		return it
	}
	req := core.TimelineItem{Kind: core.TimelineReviewRequested, Actor: user("ann", ""), At: ago(60), Subject: core.TimelineSubject{Team: "eggzec/core"}}
	edited := item(core.TimelineComment, "c2", user("bob", "Bob Builder"), 50, "Thanks.\n\nI **changed** the layout, see `tree.go` and\n\n\tindented with a tab\n\n- one\n- two")
	edited.UpdatedAt = ago(49)
	th := func(id, rid string, resolved, outdated bool, n int, body string) core.Thread {
		t := core.Thread{ID: id, ReviewID: rid, Path: "internal/tui/tree.go", Line: 42, Resolved: resolved, Outdated: outdated, TotalComments: n}
		for i := range n {
			t.Comments = append(t.Comments, core.ThreadComment{
				ID: fmt.Sprint(id, "-", i), Author: user([]string{"ann", "bob"}[i%2], ""), At: ago(40 - i), Body: body,
			})
		}
		t.Comments[n-1].EditedAt = ago(1)
		return t
	}
	file := th("T4", "R3", false, false, 1, "Please add a header comment to the file.")
	file.File, file.Line, file.OriginalLine = true, 0, 3
	moved := th("T2", "R2", true, true, 2, "Fixed in the next push.")
	moved.Line, moved.OriginalLine = 0, 9
	more := th("T1", "R1", false, false, 3, "This loop runs once per row, which is slow for\n\nthe long trees we draw. Could it be cached?")
	more.TotalComments = 5
	drafts := th("T5", "RP", false, false, 1, "A draft.")
	drafts.Pending = true
	drafts.Comments[0].Pending = true

	return Data{
		Items: []core.TimelineItem{
			rev("R0", user("cy", ""), core.ReviewStateApproved, 70, ""),
			item(core.TimelineComment, "c1", bot, 65, "Bumps [golang.org/x/text](https://github.com/golang/text) from 0.3 to 0.4."),
			req,
			rev("R1", user("bob", "Bob Builder"), core.ReviewStateChangesRequested, 55, "Needs work on the tree.\n\nSee the threads below."),
			edited,
			rev("R2", user("dee", ""), core.ReviewStateCommented, 45, ""),
			rev("R3", user("ann", ""), core.ReviewStateCommented, 44, "A few remarks."),
			rev("R4", user("eve", "Eve"), core.ReviewStateDismissed, 43, "Stale."),
			rev("R5", user("fay", ""), core.ReviewStateApproved, 42, "Looks good to me."),
			item(core.TimelineComment, "c3", core.User{}, 30, "A comment of a deleted account: "+long),
			item(core.TimelineComment, "c4", user("yamada", "山田太郎"), 20, "漢字のコメントです。日本語の文章は折り返されます。"),
			item(core.TimelineComment, "c5", user("ann", ""), 2, "Merged soon."),
		},
		Threads: []core.Thread{more, moved, file, drafts},
	}
}
