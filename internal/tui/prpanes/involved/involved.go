// Package involved turns the people on a pull request into the rows of the
// involved pane of the pull request modal: the author, then the folding groups
// of the reviewers, the assignees and the commenters.
//
// The package does no I/O and holds no keys. What the pane shows arrives in
// [Data], and what enter on a person asks for leaves as an intent message from
// [Model], for the host to act on.
package involved

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// foldAt is the most commenters whose group starts open.
const foldAt = 3

// The IDs of the rows, which stay the same across refreshes so that the
// cursor and the folds follow them.
const (
	idAuthor     = "author"
	idReviewers  = "group:reviewers"
	idAssignees  = "group:assignees"
	idCommenters = "group:commenters"

	idEmptyReviewers  = "empty:reviewers"
	idEmptyAssignees  = "empty:assignees"
	idEmptyCommenters = "empty:commenters"
)

// reviewerID is the ID of the row of a reviewer, a person or a team.
func reviewerID(r Person) string {
	if r.Team != "" {
		return "reviewer:team:" + r.Team
	}
	return "reviewer:user:" + r.Login
}

// assigneeID and commenterID are the IDs of the rows of the person with login.
func assigneeID(login string) string  { return "assignee:" + login }
func commenterID(login string) string { return "commenter:" + login }

// State is where a reviewer stands on a pull request. The zero State is that
// of a person who is no reviewer.
type State int

// The states of a reviewer.
const (
	None State = iota
	Requested
	Rerequested
	Approved
	ChangesRequested
	Commented
	Dismissed
)

// String returns the word the row shows for s.
func (s State) String() string {
	switch s {
	case None:
		return ""
	case Requested:
		return "requested"
	case Rerequested:
		return "re-requested"
	case Approved:
		return "approved"
	case ChangesRequested:
		return "changes requested"
	case Commented:
		return "commented"
	case Dismissed:
		return "dismissed"
	}
	return ""
}

// Reviewed reports whether s is the state of a reviewer who has reviewed, and
// whose review may be asked for again.
func (s State) Reviewed() bool {
	return s >= Approved
}

// Data is what the pane shows.
type Data struct {
	Author    core.User
	Assignees []core.User
	Reviewers core.Reviewers
	// Items and Threads are the conversation as far as it is known: the
	// commenters are those who wrote in it.
	Items   []core.TimelineItem
	Threads []core.Thread
}

// Options say how the rows are drawn.
type Options struct {
	Styles Styles
	// Width is the cells that a preview is drawn in, and zero for no limit.
	Width int
}

// Person is the person or team that a row stands for.
type Person struct {
	// Login names a person, and is empty for a team, which Team names by its
	// organization and slug.
	Login, Team string
	// Name is how the row names them.
	Name string
	// State is where a reviewer stands, and None for anybody else.
	State State
}

// FocusPersonMsg asks to focus the conversation on the latest comment of a
// person, or of a team, which has none.
type FocusPersonMsg struct {
	Login, Team, Name string
}

// people is who the pane lists, in the order of its rows.
type people struct {
	reviewers []Person
	// hidden counts the reviewers that have no row: requests of a team that
	// can't be named, and those that were left out of what was read.
	hidden     int
	assignees  []core.User
	commenters []core.User
}

// compose works out who is on the pull request.
func compose(d Data) people {
	var p people
	p.reviewers, p.hidden = reviewers(d.Reviewers, d.Author.Login)
	p.assignees = d.Assignees
	skip := map[string]bool{d.Author.Login: d.Author.Login != ""}
	for _, r := range p.reviewers {
		skip[r.Login] = true
	}
	for _, a := range p.assignees {
		skip[a.Login] = true
	}
	delete(skip, "")
	p.commenters = commenters(d, skip)
	return p
}

// Reviewers returns the people and teams asked for a review and those who gave
// one, as the pane lists them.
func Reviewers(r core.Reviewers, author string) []Person {
	out, _ := reviewers(r, author)
	return out
}

// reviewers returns the people and teams who were asked for a review, and
// those who gave one, and how many more there are than it can show. The
// author never is one: GitHub lists their replies in review threads as
// reviews, and a review can't be requested from them.
func reviewers(r core.Reviewers, author string) (out []Person, hidden int) {
	latest := map[string]core.ReviewState{}
	verdicts := slices.DeleteFunc(slices.Concat(r.Latest, r.Verdicts), func(v core.Verdict) bool {
		return author != "" && v.Author.Login == author
	})
	for _, v := range verdicts {
		if _, ok := latest[v.Author.Login]; !ok {
			latest[v.Author.Login] = v.State
		}
	}
	listed := map[string]bool{}
	for _, q := range r.Requested {
		switch {
		case q.Team != "":
			out = append(out, Person{Team: q.Team, Name: teamName(q), State: Requested})
		case q.Unknown || q.User.Login == "":
			hidden++
		case !listed[q.User.Login]:
			listed[q.User.Login] = true
			st := Requested
			if _, ok := latest[q.User.Login]; ok {
				st = Rerequested
			}
			out = append(out, Person{Login: q.User.Login, Name: conv.Name(q.User), State: st})
		}
	}
	for _, v := range verdicts {
		st := stateOf(v.State)
		if listed[v.Author.Login] || st == None {
			continue
		}
		listed[v.Author.Login] = true
		out = append(out, Person{Login: v.Author.Login, Name: conv.Name(v.Author), State: st})
	}
	hidden += max(0, r.RequestedTotal-len(r.Requested)) + max(0, r.LatestTotal-len(r.Latest))
	return out, hidden
}

// teamName is how a team is named: its name, else its slug.
func teamName(q core.ReviewRequest) string {
	if n := strings.TrimSpace(termtext.OneLine(q.TeamName)); n != "" {
		return n
	}
	_, slug, _ := strings.Cut(q.Team, "/")
	return termtext.OneLine(slug)
}

// stateOf returns the state of a reviewer whose latest review is s.
func stateOf(s core.ReviewState) State {
	switch s {
	case core.ReviewStatePending:
		return None
	case core.ReviewStateApproved:
		return Approved
	case core.ReviewStateChangesRequested:
		return ChangesRequested
	case core.ReviewStateCommented:
		return Commented
	case core.ReviewStateDismissed:
		return Dismissed
	}
	return None
}

// commenters returns who wrote a comment, a review or a reply in a thread,
// bots included, in the order they first appeared, except those in skip.
func commenters(d Data, skip map[string]bool) []core.User {
	type said struct {
		by core.User
		at int64
	}
	var all []said
	for i := range d.Items {
		it := &d.Items[i]
		if it.Kind == core.TimelineComment || it.Kind == core.TimelineReview {
			all = append(all, said{it.Actor, it.At.UnixNano()})
		}
	}
	for i := range d.Threads {
		cs := d.Threads[i].Comments
		for j := range cs {
			if !cs[j].Pending {
				all = append(all, said{cs[j].Author, cs[j].At.UnixNano()})
			}
		}
	}
	slices.SortStableFunc(all, func(a, b said) int { return cmp.Compare(a.at, b.at) })
	var out []core.User
	seen := map[string]bool{}
	for _, s := range all {
		if skip[s.by.Login] || seen[s.by.Login] {
			continue
		}
		seen[s.by.Login] = true
		out = append(out, s.by)
	}
	return out
}

// HasComments reports whether the person with login wrote a comment, a review
// or a reply in d.
func HasComments(d Data, login string) bool {
	for i := range d.Items {
		it := &d.Items[i]
		if (it.Kind == core.TimelineComment || it.Kind == core.TimelineReview) && it.Actor.Login == login {
			return true
		}
	}
	for i := range d.Threads {
		cs := d.Threads[i].Comments
		for j := range cs {
			if cs[j].Author.Login == login && !cs[j].Pending {
				return true
			}
		}
	}
	return false
}

// DefaultFolds returns, by the ID of each group, whether it starts closed:
// the reviewers and the assignees start open, and the commenters only when
// they are few.
func DefaultFolds(d Data) map[string]bool {
	p := compose(d)
	return map[string]bool{
		idReviewers:  false,
		idAssignees:  false,
		idCommenters: len(p.commenters) > foldAt,
	}
}

// Rows returns the rows of the pane: the author, then each group with its
// people, or with a hint when it has none. Enter on the header of a group
// folds it, and on a person asks to focus their latest comment. The rows
// share nothing with d.
func Rows(d Data, o Options) []foldtree.Row {
	rows, _ := build(d, o)
	return rows
}

// build returns the rows, and the person of each row that stands for one.
func build(d Data, o Options) (rows []foldtree.Row, who map[string]Person) {
	st := &o.Styles
	p := compose(d)
	folds := DefaultFolds(d)
	who = map[string]Person{}

	rows = []foldtree.Row{{
		ID:     idAuthor,
		Lines:  []string{st.dim.Render("Author ") + conv.Name(d.Author)},
		Action: true,
	}}
	who[idAuthor] = Person{Login: d.Author.Login, Name: conv.Name(d.Author)}

	group := func(id, name string, n int) {
		rows = append(rows, foldtree.Row{
			ID:     id,
			Lines:  []string{st.bold.Render(name) + " " + st.dim.Render(strconv.Itoa(n))},
			Fold:   true,
			Arrow:  true,
			Action: true,
			Closed: folds[id],
		})
	}
	empty := func(id, hint string) {
		rows = append(rows, foldtree.Row{ID: id, Depth: 1, Lines: []string{st.dim.Render("none (enter to " + hint + ")")}})
	}

	n := len(p.reviewers) + p.hidden
	group(idReviewers, "Reviewers", n)
	for _, r := range p.reviewers {
		id := reviewerID(r)
		who[id] = r
		rows = append(rows, foldtree.Row{ID: id, Depth: 1, Lines: []string{st.reviewer(r)}, Action: true})
	}
	if n == 0 {
		empty(idEmptyReviewers, "request")
	}

	group(idAssignees, "Assignees", len(p.assignees))
	for _, a := range p.assignees {
		id := assigneeID(a.Login)
		who[id] = Person{Login: a.Login, Name: conv.Name(a)}
		rows = append(rows, foldtree.Row{ID: id, Depth: 1, Lines: []string{conv.Name(a)}, Action: true})
	}
	if len(p.assignees) == 0 {
		empty(idEmptyAssignees, "assign")
	}

	group(idCommenters, "Commenters", len(p.commenters))
	for _, c := range p.commenters {
		id := commenterID(c.Login)
		who[id] = Person{Login: c.Login, Name: conv.Name(c)}
		rows = append(rows, foldtree.Row{ID: id, Depth: 1, Lines: []string{conv.Name(c)}, Action: true})
	}
	if len(p.commenters) == 0 {
		rows = append(rows, foldtree.Row{ID: idEmptyCommenters, Depth: 1, Lines: []string{st.dim.Render("none")}})
	}
	return rows, who
}
