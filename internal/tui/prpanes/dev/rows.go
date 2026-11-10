package dev

import (
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Options say how the rows are drawn.
type Options struct {
	// Repo is the repository of the pull request: the items in it are named
	// by number alone, those in another by their repository too.
	Repo   core.RepoRef
	Styles Styles
	// HideCloses leaves out the header of what is closed, for an issue.
	HideCloses bool
}

// Rows are the rows of a tree, and what each stands for by the ID of the row.
type Rows struct {
	Rows  []treeview.Node
	Items map[string]Item
}

// The IDs of the headers and of the notes.
const (
	closesID    = "header:closes"
	linkedID    = "header:linked"
	mentionedID = "header:mentioned"
)

// Build returns the rows of links: a header for what the pull request closes
// with each issue and what blocks it, and a header for the pull requests
// linked to it.
func Build(links core.PullLinks, o Options) Rows {
	b := builder{o: o, st: o.Styles, items: map[string]Item{}}
	if !o.HideCloses {
		b.closes(links)
	}
	b.linked(links)
	b.mentioned(links)
	return Rows{Rows: b.rows, Items: b.items}
}

type builder struct {
	o     Options
	st    Styles
	rows  []treeview.Node
	items map[string]Item
}

func (b *builder) add(r treeview.Node, it Item) {
	b.rows = append(b.rows, r)
	b.items[r.ID] = it
}

func (b *builder) header(id, group, title string) {
	b.add(treeview.Node{ID: id, Lines: []string{b.st.bold.Render(title)}, Branch: true},
		Item{Kind: Header, Group: group, Title: title})
}

func (b *builder) note(id string, depth int, kind Kind, text string) {
	b.add(treeview.Node{ID: id, Depth: depth, Lines: []string{b.st.dim.Render(text)}}, Item{Kind: kind})
}

func (b *builder) closes(links core.PullLinks) {
	b.header(closesID, GroupCloses, "Closes "+strconv.Itoa(links.ClosesTotal))
	if len(links.Closes) == 0 {
		b.note("none:closes", 1, None, "none")
		return
	}
	for i := range links.Closes {
		c := &links.Closes[i]
		id := "issue:" + core.RefKey(c.Issue.Target)
		b.add(treeview.Node{ID: id, Depth: 1, Lines: []string{b.line(c.Issue)}, Branch: len(c.Blockers) > 0, Action: true},
			Item{Kind: Issue, Ref: c.Issue})
		if !links.BlockersSupported {
			continue
		}
		for j := range c.Blockers {
			bl := c.Blockers[j]
			b.add(treeview.Node{ID: "blocker:" + core.RefKey(c.Issue.Target) + ">" + core.RefKey(bl.Target), Depth: 2, Lines: []string{b.blocker(bl)}, Action: true},
				Item{Kind: Blocker, Ref: bl})
		}
		if more := c.BlockersTotal - len(c.Blockers); more > 0 {
			b.note("more:blockers:"+core.RefKey(c.Issue.Target), 2, More, "+"+strconv.Itoa(more)+" more")
		}
	}
	if more := links.ClosesTotal - len(links.Closes); more > 0 {
		b.note("more:closes", 1, More, "+"+strconv.Itoa(more)+" more")
	}
}

func (b *builder) linked(links core.PullLinks) {
	// Only pull requests are listed: an issue connected to this item is not
	// one, and an item of unknown kind is shown as it may be.
	var pulls []core.Reference
	for i := range links.Linked {
		if links.Linked[i].Target.Kind != core.KindIssue {
			pulls = append(pulls, links.Linked[i])
		}
	}
	b.header(linkedID, GroupLinked, "Linked PRs "+strconv.Itoa(len(pulls)))
	if len(pulls) == 0 && !links.LinkedCapped {
		b.note("none:linked", 1, None, "none")
	}
	for i := range pulls {
		r := &pulls[i]
		b.add(treeview.Node{ID: "pull:" + core.RefKey(r.Target), Depth: 1, Lines: []string{b.line(*r)}, Action: true},
			Item{Kind: Pull, Ref: *r})
	}
	if links.LinkedCapped {
		b.note("more:linked", 1, More, "more on GitHub")
	}
}

// mentioned lists the items that mention this one, each of its true kind, and
// is left out when none does. The header counts the rows loaded, with a plus
// while more may come.
func (b *builder) mentioned(links core.PullLinks) {
	if len(links.Mentioned) == 0 && links.MentionedTotal == 0 {
		return
	}
	b.header(mentionedID, GroupMentioned, "Mentioned "+ui.Count(len(links.Mentioned), links.MentionedCapped))
	for i := range links.Mentioned {
		r := &links.Mentioned[i]
		kind := Pull
		if r.Target.Kind == core.KindIssue {
			kind = Issue
		}
		b.add(treeview.Node{ID: "mention:" + core.RefKey(r.Target), Depth: 1, Lines: []string{b.line(*r)}, Action: true},
			Item{Kind: kind, Ref: *r})
	}
	if links.MentionedCapped {
		b.note("more:mentioned", 1, More, "more on GitHub")
	}
}

// ident names the target of r: by number alone in the repository of the pull
// request, and with its repository in another.
func (b *builder) ident(r core.Reference) string {
	n := "#" + strconv.Itoa(r.Target.Number)
	if r.Target.Repo.Same(b.o.Repo) {
		return n
	}
	return r.Target.Repo.String() + n
}

// glyph returns the state of r in its colour, or a dim question mark for an
// item that couldn't be read.
func (b *builder) glyph(r core.Reference) string {
	return b.st.ref(r)
}

// text is the text after the glyph: the name, and the title or what stopped it
// from being read.
func (b *builder) text(r core.Reference) string {
	t := b.ident(r)
	switch {
	case r.Problem != "":
		t += " can't be read: " + ui.OneLine(r.Problem)
	case r.Title != "":
		t += " " + ui.OneLine(r.Title)
	}
	return t
}

// line is the row of an item: glyph, name and title, as a link.
func (b *builder) line(r core.Reference) string {
	return b.glyph(r) + " " + termtext.Link(r.URL, b.st.Tree.Name.Render(b.text(r)))
}

// blocker is the row of what blocks an issue, dim.
func (b *builder) blocker(r core.Reference) string {
	return b.st.dim.Render("blocked by ") + b.glyph(r) + " " + termtext.Link(r.URL, b.st.dim.Render(b.text(r)))
}
