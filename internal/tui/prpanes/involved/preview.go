package involved

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Preview is what a row of the pane has to show beside it. Title and Info are
// plain text for the frame to style. Head and Body are drawn.
//
// A person's preview has a Head, ending in a rule, and under it the
// conversation narrowed to what they wrote: Key names that view, and Narrow
// says what it keeps. Nothing is the text for when the view has no rows. Any
// other preview is its Body alone.
type Preview struct {
	Title, Info string
	Head        []string
	Body        string

	Key     string
	Narrow  conv.Narrow
	Nothing string
}

// PreviewOf returns the preview of the row with the ID id, as [Rows] names it,
// with the width and styles of o. ok is false for an ID that no row of d has.
// It does no I/O and reads only d.
func PreviewOf(d Data, id string, o Options) (Preview, bool) {
	st := &o.Styles
	p := compose(d)
	switch id {
	case idReviewers:
		lines := make([]string, 0, len(p.reviewers)+1)
		for _, r := range p.reviewers {
			lines = append(lines, st.reviewer(r))
		}
		if p.hidden > 0 {
			lines = append(lines, st.dim.Render("and "+strconv.Itoa(p.hidden)+" more"))
		}
		return group("Reviewers", lines, o), true
	case idAssignees:
		lines := make([]string, 0, len(p.assignees))
		for _, a := range p.assignees {
			lines = append(lines, conv.Name(a)+"  "+st.dim.Render(a.Login))
		}
		return group("Assignees", lines, o), true
	case idCommenters:
		lines := make([]string, 0, len(p.commenters))
		for _, c := range p.commenters {
			n := Count(d, c.Login)
			word := " comments or reviews"
			if n == 1 {
				word = " comment or review"
			}
			lines = append(lines, conv.Name(c)+"  "+st.dim.Render(strconv.Itoa(n)+word))
		}
		return group("Commenters", lines, o), true
	case idEmptyReviewers:
		return empty("Reviewers", "enter requests a review", o), true
	case idEmptyAssignees:
		return empty("Assignees", "enter assigns", o), true
	case idEmptyCommenters:
		return empty("Commenters", "", o), true
	}
	who, ok := personByID(d, p, id)
	if !ok {
		return Preview{}, false
	}
	return person(d, p, who, o), true
}

// group is the preview of the header of a group: one line per member, or
// nobody.
func group(title string, lines []string, o Options) Preview {
	if len(lines) == 0 {
		return Preview{Title: title, Body: o.Styles.dim.Render("nobody")}
	}
	for i, l := range lines {
		lines[i] = o.Styles.cut(l, o.Width)
	}
	return Preview{Title: title, Body: strings.Join(lines, "\n")}
}

// empty is the preview of the row of a group that has nobody yet.
func empty(title, hint string, o Options) Preview {
	body := "nobody yet"
	if hint != "" {
		body += "\n" + o.Styles.dim.Render(hint)
	}
	return Preview{Title: title, Body: body}
}

// person is the preview of a person or team: who they are on the pull request,
// and what they wrote in it.
func person(d Data, p people, who Person, o Options) Preview {
	st := &o.Styles
	pv := Preview{Title: who.Name, Info: strings.Join(roles(d, p, who), st.sep)}
	if who.Team != "" {
		pv.Head = append(pv.Head, st.bold.Render("Team members:"), st.dim.Render("  members hidden"), "")
	}
	if who.State != None {
		pv.Head = append(pv.Head, st.cut(st.bold.Render(st.glyphs.Render(glyphOf(who.State))+" review: "+who.State.String()), o.Width))
	}
	pv.Head = append(pv.Head, st.dim.Render(ui.Rule(st.icons, o.Width)))
	pv.Nothing = st.dim.Render("no comments or reviews from " + who.Name + " in this pull request")
	if who.Login != "" {
		pv.Key = "person:" + who.Login
		pv.Narrow = NarrowFor(d, who.Login)
	}
	return pv
}

// roles returns what the person is on the pull request.
func roles(d Data, p people, who Person) []string {
	if who.Team != "" {
		return []string{"reviewer"}
	}
	var out []string
	if who.Login != "" && who.Login == d.Author.Login {
		out = append(out, "author")
	}
	if slices.ContainsFunc(p.reviewers, func(r Person) bool { return r.Login == who.Login }) {
		out = append(out, "reviewer")
	}
	if slices.ContainsFunc(p.assignees, func(a core.User) bool { return a.Login == who.Login }) {
		out = append(out, "assignee")
	}
	return out
}

// personByID returns the person that the row with the ID id stands for.
func personByID(d Data, p people, id string) (Person, bool) {
	if id == idAuthor {
		return Person{Login: d.Author.Login, Name: conv.Name(d.Author)}, true
	}
	for _, r := range p.reviewers {
		if reviewerID(r) == id {
			return r, true
		}
	}
	for _, a := range p.assignees {
		if assigneeID(a.Login) == id {
			return Person{Login: a.Login, Name: conv.Name(a)}, true
		}
	}
	for _, c := range p.commenters {
		if commenterID(c.Login) == id {
			return Person{Login: c.Login, Name: conv.Name(c)}, true
		}
	}
	return Person{}, false
}

// Count returns how many comments, reviews and replies in threads the person
// with login wrote in d.
func Count(d Data, login string) int {
	n := 0
	for i := range d.Items {
		it := &d.Items[i]
		if (it.Kind == core.TimelineComment || it.Kind == core.TimelineReview) && it.Actor.Login == login {
			n++
		}
	}
	for i := range d.Threads {
		cs := d.Threads[i].Comments
		for j := range cs {
			if cs[j].Author.Login == login && !cs[j].Pending {
				n++
			}
		}
	}
	return n
}

// NarrowFor returns what the conversation of d keeps to show the part of the
// person with login: the comments and reviews they wrote, the reviews that
// hold a thread they wrote in, and only the threads they wrote in.
func NarrowFor(d Data, login string) conv.Narrow {
	holds := map[string]bool{}
	for i := range d.Threads {
		if t := &d.Threads[i]; t.ReviewID != "" && wroteIn(t, login) {
			holds[t.ReviewID] = true
		}
	}
	return conv.Narrow{
		Item: func(it core.TimelineItem) bool {
			if it.Kind == core.TimelineReview {
				return it.Actor.Login == login || holds[it.ID]
			}
			return it.Kind == core.TimelineComment && it.Actor.Login == login
		},
		Thread: func(t core.Thread) bool { return wroteIn(&t, login) },
	}
}

// wroteIn reports whether the person with login wrote a comment of t.
func wroteIn(t *core.Thread, login string) bool {
	return slices.ContainsFunc(t.Comments, func(c core.ThreadComment) bool {
		return c.Author.Login == login && !c.Pending
	})
}

// cut shortens line to width cells, which is zero for no limit.
func (s *Styles) cut(line string, width int) string {
	if width <= 0 {
		return line
	}
	return ansi.Truncate(line, width, s.ellipsis)
}
