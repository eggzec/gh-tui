package conv

import (
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// isPush reports whether k is a kind of the pushes group: a commit that was
// pushed, or a force push.
func isPush(k core.TimelineKind) bool {
	return k == core.TimelinePushed || k == core.TimelineForcePushed
}

// pusher returns who pushed a commit: the account GitHub knows, else the
// author named in the commit.
func pusher(it *core.TimelineItem) core.User {
	if it.Actor.Login != "" {
		return it.Actor
	}
	return it.Commit.Author
}

// pusherKey tells one author from another: the login of the account, else the
// name in the commit.
func pusherKey(it *core.TimelineItem) string {
	if it.Actor.Login != "" {
		return it.Actor.Login
	}
	return it.Commit.Author.Name
}

// pushRun gathers the commits that one author pushed in a row into a push.
// Feed it the visible items in the order of the timeline: each pushed commit
// and force push to commit and force, and end for any other item.
type pushRun struct {
	open *node
	// held is a force push that waits for the push after it.
	held *core.TimelineItem
}

// commit adds the pushed commit it to the open push, or starts a push with
// it, which is fresh. A push that starts right after a force push is forced.
func (p *pushRun) commit(it *core.TimelineItem) (n *node, fresh bool) {
	k := &node{kind: KindCommit, id: "k:" + it.Commit.OID, item: it}
	key := pusherKey(it)
	if p.open != nil && p.open.author == key {
		p.open.children = append(p.open.children, k)
		return p.open, false
	}
	p.open = &node{
		kind: KindPush, id: "push:" + it.Commit.OID, item: it,
		author: key, forced: p.held != nil, children: []*node{k},
	}
	p.held = nil
	return p.open, true
}

// force takes in a force push, which ends the open push. It returns the line
// of the force push before it when no push came after that one.
func (p *pushRun) force(it *core.TimelineItem) (lone *node) {
	lone = p.end()
	p.held = it
	return lone
}

// end closes the open push, and returns the line of a force push that no
// push followed, or nil.
func (p *pushRun) end() (lone *node) {
	p.open = nil
	if p.held == nil {
		return nil
	}
	lone = eventNode(p.held)
	p.held = nil
	return lone
}

// push draws a push as a system line over its commits. Closed it is centred,
// as the other system lines are; open it is left-aligned, so the commits
// under it hang from its icon. The row carries the closed line, and the node
// both.
func (b *builder) push(n *node) treeview.Node {
	first, last := n.children[0].item, n.children[len(n.children)-1].item
	what := "1 commit"
	if k := len(n.children); k > 1 {
		what = strconv.Itoa(k) + " commits"
	}
	head := b.st.who(pusher(first), false) + " " + b.say("pushed "+what+" (") + b.st.bold.Render(shortSHA(first.Commit))
	if len(n.children) > 1 {
		head += b.say(b.st.Glyphs.Cut) + b.st.bold.Render(shortSHA(last.Commit))
	}
	head += b.say(")")
	if n.forced {
		head += b.st.warn.Render(" " + b.st.Glyphs.Dot + " forced")
	}
	line := b.st.dim.Render(b.st.icons.Glyph(ui.GlyphCommit)) + " " + head
	age := b.ageOf(last.At)

	r := b.system(line, age, true)
	n.forms = [2]string{r.Lines[0], b.system(line, age, false).Lines[0]}
	r.Branch, r.Closed = true, true
	return r
}

// commit draws a commit of a push: its short SHA and the first line of its
// message.
func (b *builder) commit(n *node) treeview.Node {
	c := n.item.Commit
	head := termtext.Truncate(termtext.OneLine(c.Headline), max(b.width-20, 8), b.st.Glyphs.Cut)
	r := treeview.Node{
		Lines:  []string{b.st.sha.Render(shortSHA(c)) + "  " + head},
		Action: true,
	}
	state, known := b.o.checks[strings.ToLower(c.OID)]
	r.Right = b.checkGlyph(state)
	// An unknown roll-up folds too: the read of the checks tells what shows.
	if b.o.kids != nil && (!known || state != core.CheckNone) {
		r.Branch, r.Closed = true, true
	}
	return r
}

// checkGlyph returns the glyph of a state of checks, or "" for none.
func (b *builder) checkGlyph(s core.CheckState) string {
	switch s {
	case core.CheckPassing:
		return b.st.glyphs.Render(ui.GlyphCheckPass)
	case core.CheckFailing:
		return b.st.glyphs.Render(ui.GlyphCheckFail)
	case core.CheckPending:
		return b.st.glyphs.Render(ui.GlyphCheckPending)
	case core.CheckNone:
	}
	return ""
}

// latestCommit returns the date of the newest commit of a push.
func latestCommit(n *node) (t time.Time) {
	for _, c := range n.children {
		t = latest(t, c.item.At)
	}
	return t
}
