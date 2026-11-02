package merge

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/codeowners"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// OwnersState says how far the read of the code owners has come.
type OwnersState int

// The states of the read of the code owners.
const (
	// OwnersOff is where nothing reads them: the preview of the row has no
	// body.
	OwnersOff OwnersState = iota
	// OwnersLoading is while they are being read.
	OwnersLoading
	// OwnersLoaded is once they were read.
	OwnersLoaded
	// OwnersFailed is once the read failed: the preview says so, since it
	// can't tell whether any file has an owner.
	OwnersFailed
)

// Owners is what the preview of the Review required row shows.
type Owners struct {
	State OwnersState
	// Files are the files that the pull request changes.
	Files []core.CommitFile
	// Owned are the code owners of those files, each with the files they own.
	Owned []codeowners.Owned
	// Members are the members of the teams among the owners, by the team as
	// the CODEOWNERS file writes it without its @, such as "acme/core". A team
	// whose members could not be read has no entry.
	Members map[string][]core.Person
}

// AddMembers sets the members of a team, named as the CODEOWNERS file writes
// it without its @. A team that is unavailable, or has no member, gets none,
// and its preview has no members line.
func (o *Owners) AddMembers(team string, members []core.Person, unavailable bool) {
	if unavailable || len(members) == 0 {
		return
	}
	if o.Members == nil {
		o.Members = map[string][]core.Person{}
	}
	o.Members[team] = members
}

// ownerState is what the pull request knows of an owner's review.
type ownerState int

const (
	ownerPlain ownerState = iota
	ownerRequired
	ownerApproved
)

// owner is a code owner of a changed file, as the preview shows it.
type owner struct {
	codeowners.Owned
	name    string
	state   ownerState
	members []core.Person
}

// ownersPreview is the preview of the Review required row: who must still
// review, as the code owners of the changed files, and what each owns.
func ownersPreview(d Data, o Options) Preview {
	st := &o.Styles
	p := Preview{Title: "Required review"}
	switch d.Owners.State {
	case OwnersOff:
		return p
	case OwnersLoading:
		p.Body = st.dimWrap("Reading the code owners"+st.ellipsis, o.Width)
		return p
	case OwnersFailed:
		p.Body = st.dimWrap("Could not read the code owners.", o.Width)
		return p
	case OwnersLoaded:
	}
	owners := ownersOf(d)
	if len(owners) == 0 {
		p.Body = wrap("GitHub requires a review before this can be merged, and no code owner was found for the changed files.", o.Width)
		return p
	}
	// Only an owner who still has to review is named: the first one is that
	// owner, as the required ones come first.
	if owners[0].state == ownerRequired {
		p.Title += o.Styles.separator + owners[0].name
	}
	p.Info = "CODEOWNERS"

	out := []string{wrap(ownersIntro(owners, len(d.Owners.Files)), o.Width)}
	byPath := make(map[string]core.CommitFile, len(d.Owners.Files))
	for _, f := range d.Owners.Files {
		byPath[f.Path] = f
	}
	fo := files.Options{Width: o.Width, Styles: st.Files}
	for _, w := range owners {
		out = append(out, "", cut(st.bold.Render(w.name)+" "+st.ownerState(w.state), o))
		if len(w.members) > 0 {
			out = append(out, strings.Split(st.dimWrap("members: "+people(w.members), o.Width), "\n")...)
		}
		fs := make([]core.CommitFile, 0, len(w.Paths))
		for _, path := range w.Paths {
			if f, ok := byPath[path]; ok {
				fs = append(fs, f)
			}
		}
		out = append(out, st.treeLines(nodesOf(files.Tree(fs, 0, fo)), o.Width)...)
	}
	p.Body = strings.Join(out, "\n")
	return p
}

// ownersIntro says that the owners must review, and how much the one who has
// not yet owns of the changed files.
func ownersIntro(owners []owner, changed int) string {
	var required []owner
	for _, w := range owners {
		if w.state == ownerRequired {
			required = append(required, w)
		}
	}
	const lead = "Review is required from the owners of the changed files"
	switch len(required) {
	case 0:
		return lead + "."
	case 1:
		noun := "files"
		if changed == 1 {
			noun = "file"
		}
		return lead + ". " + required[0].name + " owns " + strconv.Itoa(len(required[0].Paths)) +
			" of the " + strconv.Itoa(changed) + " changed " + noun + " and has not reviewed yet."
	}
	return lead + ", and " + strconv.Itoa(len(required)) + " of them have not reviewed yet."
}

// ownersOf returns the owners of the changed files, those whose review is
// required first, then by name.
func ownersOf(d Data) []owner {
	r := d.Detail.Reviewers
	out := make([]owner, 0, len(d.Owners.Owned))
	for _, ow := range d.Owners.Owned {
		w := owner{Owned: ow, name: ownerName(ow.Owner, r.Requested), members: d.Owners.Members[ow.Owner.Name]}
		w.state = stateOf(ow.Owner, r, w.members)
		out = append(out, w)
	}
	slices.SortStableFunc(out, func(a, b owner) int {
		switch {
		case a.state == ownerRequired && b.state != ownerRequired:
			return -1
		case a.state != ownerRequired && b.state == ownerRequired:
			return 1
		}
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})
	return out
}

// stateOf tells whether the review of o is still required of them, or an
// approval stands, from the reviews and requests the pull request has.
func stateOf(o codeowners.Owner, r core.Reviewers, members []core.Person) ownerState {
	for _, q := range r.Requested {
		if q.CodeOwner && !q.Unknown && asks(q, o) {
			return ownerRequired
		}
	}
	for _, v := range r.Verdicts {
		if v.State != core.ReviewStateApproved || v.Author.Login == "" {
			continue
		}
		if o.Kind == codeowners.OwnerUser && strings.EqualFold(v.Author.Login, o.Name) {
			return ownerApproved
		}
		for _, m := range members {
			if strings.EqualFold(v.Author.Login, m.Login) {
				return ownerApproved
			}
		}
	}
	return ownerPlain
}

// asks reports whether q asks o for a review.
func asks(q core.ReviewRequest, o codeowners.Owner) bool {
	if q.Team != "" {
		return o.Kind == codeowners.OwnerTeam && strings.EqualFold(q.Team, o.Name)
	}
	return o.Kind == codeowners.OwnerUser && q.User.Login != "" && strings.EqualFold(q.User.Login, o.Name)
}

// ownerName names an owner: a person by their login, a team by its display
// name when it was asked, else by its slug.
func ownerName(o codeowners.Owner, asked []core.ReviewRequest) string {
	if o.Kind != codeowners.OwnerTeam {
		return text(o.Name)
	}
	for _, q := range asked {
		if asks(q, o) && q.TeamName != "" {
			return text(q.TeamName)
		}
	}
	return text(o.Slug())
}

// people lists the logins of members.
func people(ms []core.Person) string {
	names := make([]string, 0, len(ms))
	for _, m := range ms {
		names = append(names, text(m.Login))
	}
	return strings.Join(names, ", ")
}

// ownerState draws the state of an owner.
func (s *Styles) ownerState(o ownerState) string {
	switch o {
	case ownerRequired:
		return s.warn.Render("required" + s.separator + "no review yet")
	case ownerApproved:
		return s.ok.Render("approved" + s.separator + "not required")
	case ownerPlain:
	}
	return s.dim.Render("code owner")
}

// dimWrap wraps text to width and draws every line dim.
func (s *Styles) dimWrap(text string, width int) string {
	lines := strings.Split(wrap(text, width), "\n")
	for i, l := range lines {
		lines[i] = s.dim.Render(l)
	}
	return strings.Join(lines, "\n")
}

// treeLines draws rows, which are one line each, as a tree that nothing has
// the cursor in, in width cells.
func (s *Styles) treeLines(rows []treeview.Node, width int) []string {
	if len(rows) == 0 {
		return nil
	}
	if width <= 0 {
		width = widest(rows)
	}
	lines := strings.Split(treeview.Render(rows, width, s.tree), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
		if ansi.StringWidth(lines[i]) == 0 {
			lines[i] = ""
		}
	}
	return lines
}

// nodesOf returns the rows as the rows of a treeview, for the
// file rows that the files pane still builds as a treeview.
func nodesOf(rows []treeview.Node) []treeview.Node {
	out := make([]treeview.Node, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = treeview.Node{
			ID: r.ID, Depth: r.Depth, Lines: r.Lines, Marker: r.Marker, Right: r.Right,
			Branch: r.Branch, Closed: r.Closed, Action: r.Action, Skip: r.Skip, Kind: r.Kind, Inline: r.Inline,
		}
	}
	return out
}

// widest is the width that cuts none of rows, for a tree where the host has
// no limit. Each level takes four cells for its guides, cursor and arrow, and
// two more are left for the marker's gap.
func widest(rows []treeview.Node) int {
	w := 0
	for j := range rows {
		r := &rows[j]
		for i, l := range r.Lines {
			n := (r.Depth+1)*4 + ansi.StringWidth(l)
			if i == 0 {
				n += 2 + ansi.StringWidth(r.Marker) + ansi.StringWidth(r.Right)
			}
			w = max(w, n)
		}
	}
	return w
}
