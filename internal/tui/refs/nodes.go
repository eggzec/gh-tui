package refs

import (
	"context"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	refssvc "github.com/eggzec/gh-tui/internal/service/refs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// The IDs of the groups, which are the branches of the tree.
const (
	closingID   = "closing"
	writtenID   = "written"
	mentionedID = "mentioned"
	// moreID is the row that reads the next page of mentions.
	moreID = "mentioned:more"
)

// maxWrittenOpen is how many links the group of what the texts write may
// hold and still start open: a busy item shouldn't bury its closing links.
const maxWrittenOpen = 10

// itemValue is what an item's row stands for.
type itemValue struct {
	ref core.Reference
	// hay is the text the filter looks in, lower-cased.
	hay string
}

// noteValue is an inert row that says what the group leaves out.
type noteValue struct{}

// moreValue is the row that reads the next page of mentions.
type moreValue struct{}

// group is a branch of the tree and what is below it, once read.
type group struct {
	id    string
	title string
	rows  []tree.Node
}

// snapshot is what one read of the links gives the tree: the groups, whose
// rows are made once. Children reads it from the goroutine of a command, so
// it is never changed after it is stored.
type snapshot struct {
	refs   core.References
	groups []group
}

// mentionsRead is what the pages of mentions read so far say.
type mentionsRead struct {
	// rows are the items listed, and more reports that pages follow.
	rows int
	more bool
}

// build makes the groups of r. A group with nothing to list isn't one.
func (s *Step) build(r core.References) *snapshot {
	snap := &snapshot{refs: r}
	closing := "Closes"
	if !s.pull {
		closing = "Closed by"
	}
	if len(r.Closing) > 0 {
		snap.groups = append(snap.groups, group{id: closingID, title: closing, rows: s.itemNodes(closingID, r.Closing)})
	}
	if written := s.writtenNodes(r); len(written) > 0 {
		snap.groups = append(snap.groups, group{id: writtenID, title: "Written here", rows: written})
	}
	if r.Mentioned > 0 {
		snap.groups = append(snap.groups, group{id: mentionedID, title: "Mentioned in"})
	}
	return snap
}

// writtenNodes lists what the texts write, and notes what they leave out.
func (s *Step) writtenNodes(r core.References) []tree.Node {
	if len(r.Written) == 0 && r.Unresolved == 0 {
		return nil
	}
	nodes := s.itemNodes(writtenID, r.Written)
	note := func(id, text string) {
		nodes = append(nodes, tree.Node{ID: writtenID + ":" + id, Name: text, Value: noteValue{}})
	}
	if r.CommentsTotal > r.CommentsRead {
		note("comments", "searched the newest "+strconv.Itoa(r.CommentsRead)+" of "+strconv.Itoa(r.CommentsTotal)+" comments")
	}
	if r.ReviewsTotal > r.ReviewsRead {
		note("reviews", "searched the newest "+strconv.Itoa(r.ReviewsRead)+" of "+strconv.Itoa(r.ReviewsTotal)+" reviews")
	}
	if r.Unresolved > 0 {
		note("unresolved", strconv.Itoa(r.Unresolved)+" more "+plural(r.Unresolved, "link")+" not read")
	}
	return nodes
}

func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// itemNodes returns the rows of refs in the group.
func (s *Step) itemNodes(group string, refs []core.Reference) []tree.Node {
	nodes := make([]tree.Node, 0, len(refs))
	for i := range refs {
		// The item doesn't link to itself, whatever its text says.
		if !refs[i].Target.Same(s.self) {
			nodes = append(nodes, s.itemNode(group, refs[i]))
		}
	}
	return nodes
}

// itemNode returns the row of r: its repository when it isn't the item on
// view's, number and title, and the state and where it was found at the
// right. An item that can't be read says why instead of a title.
func (s *Step) itemNode(group string, r core.Reference) tree.Node {
	name := s.ident(r.Target)
	if r.Problem != "" {
		name += " can't be read: " + r.Problem
	} else if r.Title != "" {
		name += " " + r.Title
	}
	state := stateWord(r)
	origin := originWords(r.Origins)
	hay := strings.Join([]string{core.RefKey(r.Target), s.ident(r.Target), r.Title, r.Problem, state, origin}, " ")
	detail := origin
	if state != "" {
		detail = state + s.opts.icons.Separator + origin
	}
	return tree.Node{
		ID:     group + ":" + core.RefKey(r.Target),
		Name:   name,
		Detail: detail,
		Link:   r.URL,
		Value:  itemValue{ref: r, hay: strings.ToLower(hay)},
	}
}

// ident names the target as the row does: by number alone in the
// repository of the item on view, and with its repository in another.
func (s *Step) ident(t core.Target) string {
	n := "#" + strconv.Itoa(t.Number)
	if t.Repo.Same(s.self.Repo) {
		return n
	}
	return t.Repo.String() + n
}

// stateWord says how the item stands: "open", "draft", "merged", "closed",
// and for an issue why it was closed.
func stateWord(r core.Reference) string {
	switch {
	case r.Problem != "":
		return ""
	case r.State == core.StateOpen && r.Draft && r.Target.Kind == core.KindPull:
		return "draft"
	case r.State == core.StateMerged:
		return "merged"
	case r.State == core.StateClosed && r.Target.Kind != core.KindPull:
		switch r.Reason {
		case core.ReasonCompleted:
			return "closed as completed"
		case core.ReasonNotPlanned:
			return "closed as not planned"
		case core.ReasonDuplicate:
			return "closed as duplicate"
		case core.ReasonReopened:
		}
	}
	return string(r.State)
}

// originWords says where an item was found: the first two places, with
// how many more there are.
func originWords(origins []core.RefOrigin) string {
	var parts []string
	for i, o := range origins {
		if i == 2 {
			parts = append(parts, "+"+strconv.Itoa(len(origins)-2))
			break
		}
		parts = append(parts, originWord(o))
	}
	return strings.Join(parts, ", ")
}

func originWord(o core.RefOrigin) string {
	if o.By == "" {
		return o.Where
	}
	return o.Where + " by " + o.By
}

// stateOf returns the state the icon of r shows.
func stateOf(r core.Reference) ui.State {
	if r.Target.Kind == core.KindPull {
		return ui.PullState(r.State, r.Draft)
	}
	return ui.IssueState(core.Issue{State: r.State, Reason: r.Reason})
}

// children is the tree's source: the groups, the rows of each, and for the
// mentions the pages read so far, from the cache.
func (s *Step) children(ctx context.Context, parent tree.Node) ([]tree.Node, error) {
	snap := s.snap.Load()
	if snap == nil {
		return nil, nil
	}
	if parent.ID == "" {
		nodes := make([]tree.Node, len(snap.groups))
		for i, g := range snap.groups {
			nodes[i] = tree.Node{ID: g.id, Name: g.title, Branch: true, Value: g.id}
		}
		return nodes, nil
	}
	if parent.ID == mentionedID {
		return s.mentions(ctx)
	}
	for _, g := range snap.groups {
		if g.id == parent.ID {
			return g.rows, nil
		}
	}
	return nil, nil
}

// mentions reads the pages of the items that mention this one, as many as
// were asked for, each from the cache if it is there, and makes their rows,
// with the row that reads the next page after them.
func (s *Step) mentions(ctx context.Context) ([]tree.Node, error) {
	again := s.again.Swap(false)
	var nodes []tree.Node
	cursor, next := "", ""
	for range max(s.pages.Load(), 1) {
		q := refssvc.MentionsQuery{Repo: s.self.Repo, Number: s.self.Number, Pull: s.pull, Cursor: cursor, Again: again}
		p, ok := core.Page[core.Reference]{}, false
		if !again {
			p, ok = s.svc.CachedMentions(q)
		}
		if !ok {
			var err error
			if p, err = s.svc.Mentions(ctx, q); err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, s.itemNodes(mentionedID, p.Items)...)
		next = p.Next
		if next == "" {
			break
		}
		cursor = next
	}
	s.mread.Store(&mentionsRead{rows: len(nodes), more: next != ""})
	if next != "" {
		nodes = append(nodes, tree.Node{ID: moreID, Name: s.moreText(), Value: moreValue{}})
	}
	return nodes, nil
}

// moreText words the row that reads the next page.
func (s *Step) moreText() string {
	left := s.snap.Load().refs.Mentioned - int(s.pages.Load())*s.opts.pageSize
	size := s.opts.pageSize
	if left <= 0 {
		return s.opts.icons.Ellipsis + " more: enter reads the next page"
	}
	return s.opts.icons.Ellipsis + " " + strconv.Itoa(left) + " more: enter reads the next " + strconv.Itoa(min(size, left))
}
