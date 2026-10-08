package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// The links of a pull request or issue are read in three ways: the
// item's own links and the texts that may name others, in one query; the
// items a text names, which only a number says, in batches of slots; and
// the items that mention it, a page at a time.
//
// refIssue and refPull are what is read of an item. state is aliased,
// since a selection of both under one name, as a union gets, refuses two
// fields of different types.
const refFragments = `
fragment refIssue on Issue {
  __typename number title url
  issueState: state
  stateReason
  repository { name owner { login } }
}
fragment refPull on PullRequest {
  __typename number title url isDraft
  pullState: state
  repository { name owner { login } }
}`

// refSides are the two ends of a link of the sidebar.
const refSides = `createdAt source { ...refIssue ...refPull } subject { ...refIssue ...refPull }`

// pullReferencesQuery reads what a pull request links: its texts, what it
// closes, the links of the sidebar, and how many items mention it. It
// reads the last page of the comments and reviews, which is the newest
// 100, and the first page of the other connections: GitHub's largest.
const pullReferencesQuery = `query PullReferences($owner: String!, $name: String!, $number: Int!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      body
      comments(last: 100) {
        totalCount
        nodes { author { __typename login } body }
      }
      reviews(last: 100) {
        totalCount
        nodes { author { __typename login } body }
      }
      closingIssuesReferences(first: 100) {
        nodes { ...refIssue }
      }
      linked: timelineItems(first: 100, itemTypes: [CONNECTED_EVENT, DISCONNECTED_EVENT]) {
        nodes {
          __typename
          ... on ConnectedEvent { ` + refSides + ` }
          ... on DisconnectedEvent { ` + refSides + ` }
        }
      }
      mentions: timelineItems(itemTypes: [CROSS_REFERENCED_EVENT]) { filteredCount }
    }
  }
}` + refFragments

// issueReferencesQuery reads what an issue links, as pullReferencesQuery
// does, with the pull requests that close it, those that did, and no
// reviews.
const issueReferencesQuery = `query IssueReferences($owner: String!, $name: String!, $number: Int!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      body
      comments(last: 100) {
        totalCount
        nodes { author { __typename login } body }
      }
      closedByPullRequestsReferences(first: 100, includeClosedPrs: true) {
        nodes { ...refPull }
      }
      linked: timelineItems(first: 100, itemTypes: [CONNECTED_EVENT, DISCONNECTED_EVENT, CLOSED_EVENT]) {
        nodes {
          __typename
          ... on ConnectedEvent { ` + refSides + ` }
          ... on DisconnectedEvent { ` + refSides + ` }
          ... on ClosedEvent { createdAt closer { ...refPull } }
        }
      }
      mentions: timelineItems(itemTypes: [CROSS_REFERENCED_EVENT]) { filteredCount }
    }
  }
}` + refFragments

// refMentions is the selection of the pages of items that mention one.
const refMentions = `timelineItems(last: $last, before: $before, itemTypes: [CROSS_REFERENCED_EVENT]) {
        pageInfo { hasPreviousPage startCursor }
        nodes { ... on CrossReferencedEvent { willCloseTarget actor { __typename login } source { ...refIssue ...refPull } } }
      }`

// pullMentionsQuery reads a page of the items that mention a pull request,
// the newest last, ending before a cursor.
const pullMentionsQuery = `query PullMentions($owner: String!, $name: String!, $number: Int!, $last: Int!, $before: String) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      ` + refMentions + `
    }
  }
}` + refFragments

// issueMentionsQuery is pullMentionsQuery for an issue.
const issueMentionsQuery = `query IssueMentions($owner: String!, $name: String!, $number: Int!, $last: Int!, $before: String) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      ` + refMentions + `
    }
  }
}` + refFragments

// refSlots is how many items one query resolves. Its text never changes,
// so the schema check holds it and the budget learns its cost.
const refSlots = 20

// RefSlots is how many items one request of ResolveReferences reads, so
// the size of the batches that callers send it.
const RefSlots = refSlots

// resolveReferencesQuery reads refSlots items by repository and number,
// each of which may be an issue or a pull request. A slot that isn't
// used is skipped by its w variable.
var resolveReferencesQuery = refSlotsQuery(refSlots)

// refSlotsQuery builds the query that resolves n items.
func refSlotsQuery(n int) string {
	var vars, slots strings.Builder
	for i := range n {
		s := strconv.Itoa(i)
		if i > 0 {
			vars.WriteString(", ")
		}
		fmt.Fprintf(&vars, "$o%s: String!, $n%s: String!, $i%s: Int!, $w%s: Boolean!", s, s, s, s)
		fmt.Fprintf(&slots, "\n  r%[1]s: repository(owner: $o%[1]s, name: $n%[1]s) @include(if: $w%[1]s) { issueOrPullRequest(number: $i%[1]s) { ...refIssue ...refPull } }", s)
	}
	return "query ResolveReferences(" + vars.String() + ") {\n  " + rateLimitField + slots.String() + "\n}" + refFragments
}

// refItem is an issue or pull request as the fragments read it. A closer
// that is a commit has none of the fields.
type refItem struct {
	Typename    string `json:"__typename"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	IsDraft     bool   `json:"isDraft"`
	IssueState  string `json:"issueState"`
	PullState   string `json:"pullState"`
	StateReason string `json:"stateReason"`
	Repository  struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

var refStates = map[string]core.State{"OPEN": core.StateOpen, "CLOSED": core.StateClosed, "MERGED": core.StateMerged}

var refReasons = map[string]core.StateReason{
	"COMPLETED": core.ReasonCompleted, "NOT_PLANNED": core.ReasonNotPlanned,
	"DUPLICATE": core.ReasonDuplicate, "REOPENED": core.ReasonReopened,
}

// core returns the item, or false if it is not one: a closer that is a
// commit, or nothing.
func (i *refItem) core() (core.Reference, bool) {
	if i == nil || i.Number == 0 || i.Typename != "Issue" && i.Typename != "PullRequest" {
		return core.Reference{}, false
	}
	t := core.Target{
		Repo:   core.RepoRef{Owner: i.Repository.Owner.Login, Name: i.Repository.Name},
		Number: i.Number,
		Kind:   core.KindIssue,
	}
	state := i.IssueState
	if i.Typename == "PullRequest" {
		t.Kind, state = core.KindPull, i.PullState
	}
	return core.Reference{
		Target: t, Title: oneLine(i.Title), State: refStates[state], Draft: i.IsDraft,
		Reason: refReasons[i.StateReason], URL: i.URL,
	}, true
}

// refCloser is the origin of an item that closes the one on view, or that
// it closes.
func refCloser(pull bool) core.RefOrigin {
	if pull {
		return core.RefOrigin{Group: core.RefClosing, Where: "closes"}
	}
	return core.RefOrigin{Group: core.RefClosing, Where: "closed by"}
}

// refEvent is an event of the timeline that links the item.
type refEvent struct {
	Typename string   `json:"__typename"`
	Source   *refItem `json:"source"`
	Subject  *refItem `json:"subject"`
	Closer   *refItem `json:"closer"`
}

// refText is a comment or a review.
type refText struct {
	Author *actor `json:"author"`
	Body   string `json:"body"`
}

// refConn is a connection with the count of its items.
type refConn[T any] struct {
	TotalCount int `json:"totalCount"`
	Nodes      []T `json:"nodes"`
}

// refsItem is a pull request or issue as the two queries read it.
type refsItem struct {
	Body     string            `json:"body"`
	Comments refConn[refText]  `json:"comments"`
	Reviews  refConn[refText]  `json:"reviews"`
	Closing  *refConn[refItem] `json:"closingIssuesReferences"`
	ClosedBy *refConn[refItem] `json:"closedByPullRequestsReferences"`
	Linked   refConn[refEvent] `json:"linked"`
	Mentions struct {
		FilteredCount int `json:"filteredCount"`
	} `json:"mentions"`
}

// References reads what the pull request or issue number of repo links:
// its texts, which may name other items, what GitHub says it closes or is
// closed by, the links of the sidebar, and how many items mention it. Each
// part is one page of what exists, GitHub's largest: the newest comments
// and reviews, and the first of the rest. It returns an error matching
// core.ErrNotFound if there is no such item.
func (c *Client) References(ctx context.Context, repo core.RepoRef, number int, pull bool) (core.RefSources, error) {
	if pull {
		return c.readReferences(ctx, pullReferencesQuery, repo, number, true)
	}
	return c.readReferences(ctx, issueReferencesQuery, repo, number, false)
}

// readReferences sends query, the one of a pull request if pull is set,
// and the one of an issue otherwise.
func (c *Client) readReferences(ctx context.Context, query string, repo core.RepoRef, number int, pull bool) (core.RefSources, error) {
	what := refWhat(pull)
	var data struct {
		Repository *struct {
			Pull  *refsItem `json:"pullRequest"`
			Issue *refsItem `json:"issue"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	err := c.Query(ctx, query, vars, &data)
	var item *refsItem
	if data.Repository != nil {
		item = data.Repository.Issue
		if pull {
			item = data.Repository.Pull
		}
	}
	if err != nil && (item == nil || !refPartial(err)) {
		return core.RefSources{}, fmt.Errorf("read links of %s %s#%d: %w", what, repo, number, err)
	}
	if item == nil {
		return core.RefSources{}, fmt.Errorf("read links of %s %s#%d: %w", what, repo, number, core.ErrNotFound)
	}
	return item.sources(core.Target{Repo: repo, Number: number}, pull), nil
}

// refPartial reports whether err only refuses some nodes below the item,
// such as an issue in an organization that enforces SAML, or one that is
// gone: every error is FORBIDDEN or NOT_FOUND on a path deeper than the
// item itself. The item that came with it is kept, without those nodes.
func refPartial(err error) bool {
	gerr, ok := errors.AsType[*GraphQLError](err)
	if !ok || len(gerr.Errors) == 0 {
		return false
	}
	for _, item := range gerr.Errors {
		if item.Type != "FORBIDDEN" && item.Type != "NOT_FOUND" || !refNodePath(item.Path) {
			return false
		}
	}
	return true
}

// refNodePath reports whether path is that of one node of a connection,
// such as ["repository", "pullRequest", "closingIssuesReferences",
// "nodes", 0], or below one. The item itself, or a whole connection, is
// shorter or has no "nodes" or index, and fails the read.
func refNodePath(path []any) bool {
	for _, seg := range path[min(2, len(path)):] {
		if seg == "nodes" {
			return true
		}
		if _, ok := seg.(float64); ok {
			return true
		}
	}
	return false
}

// refWhat names the kind of the item for an error.
func refWhat(pull bool) string {
	if pull {
		return "pull request"
	}
	return "issue"
}

// sources turns what was read of the item self into its sources.
func (r *refsItem) sources(self core.Target, pull bool) core.RefSources {
	out := core.RefSources{
		Mentioned:    r.Mentions.FilteredCount,
		CommentsRead: len(r.Comments.Nodes), CommentsTotal: r.Comments.TotalCount,
		ReviewsRead: len(r.Reviews.Nodes), ReviewsTotal: r.Reviews.TotalCount,
	}
	if r.Body != "" {
		out.Texts = append(out.Texts, core.RefText{Origin: core.RefOrigin{Group: core.RefWritten, Where: "body"}, Text: r.Body})
	}
	for _, t := range []struct {
		where string
		nodes []refText
	}{{"comment", r.Comments.Nodes}, {"review", r.Reviews.Nodes}} {
		for _, n := range t.nodes {
			if n.Body == "" {
				continue
			}
			by, bot := refAuthor(n.Author)
			out.Texts = append(out.Texts, core.RefText{Origin: core.RefOrigin{Group: core.RefWritten, Where: t.where, By: by, Bot: bot}, Text: n.Body})
		}
	}
	closing := r.Closing
	if !pull {
		closing = r.ClosedBy
	}
	if closing != nil {
		for i := range closing.Nodes {
			if ref, ok := closing.Nodes[i].core(); ok {
				out.Closing = append(out.Closing, ref)
			}
		}
	}
	for _, e := range r.Linked.Nodes {
		switch e.Typename {
		case "ConnectedEvent", "DisconnectedEvent":
			if ref, ok := e.otherEnd(self); ok {
				out.Linked = append(out.Linked, core.RefLink{Ref: ref, Disconnected: e.Typename == "DisconnectedEvent"})
			}
		case "ClosedEvent":
			if ref, ok := e.Closer.core(); ok {
				out.Closers = append(out.Closers, ref)
			}
		}
	}
	return out
}

// otherEnd returns the end of the connection that isn't self.
func (e refEvent) otherEnd(self core.Target) (core.Reference, bool) {
	for _, end := range []*refItem{e.Source, e.Subject} {
		if ref, ok := end.core(); ok && !ref.Target.Same(self) {
			return ref, true
		}
	}
	return core.Reference{}, false
}

// refAuthor returns the login of an author as the item's text shows it:
// "ghost" for a deleted account, and for an app GitHub names without the
// "[bot]" that its login has.
func refAuthor(a *actor) (login string, bot bool) {
	if a == nil || a.Login == "" {
		return "ghost", false
	}
	if a.Typename == "Bot" {
		return a.Login + "[bot]", true
	}
	return a.Login, false
}

// Mentions reads a page of the items that mention the pull request or
// issue number of repo, the newest first, of up to last: the page ends
// before the cursor before, or is the newest if it is empty. The Next of
// the page is the cursor of the page before it. An item whose mention
// closes this one has a closing origin besides the mention.
func (c *Client) Mentions(ctx context.Context, repo core.RepoRef, number int, pull bool, before string, last int) (core.Page[core.Reference], error) {
	if pull {
		return c.readMentions(ctx, pullMentionsQuery, repo, number, true, before, last)
	}
	return c.readMentions(ctx, issueMentionsQuery, repo, number, false, before, last)
}

// readMentions sends query, the one of a pull request if pull is set, and
// the one of an issue otherwise.
func (c *Client) readMentions(ctx context.Context, query string, repo core.RepoRef, number int, pull bool, before string, last int) (core.Page[core.Reference], error) {
	what := refWhat(pull)
	var data struct {
		Repository *struct {
			Pull  *refMentionsItem `json:"pullRequest"`
			Issue *refMentionsItem `json:"issue"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number, "last": last, "before": nil}
	if before != "" {
		vars["before"] = before
	}
	err := c.Query(ctx, query, vars, &data)
	var item *refMentionsItem
	if data.Repository != nil {
		item = data.Repository.Issue
		if pull {
			item = data.Repository.Pull
		}
	}
	if err != nil && (item == nil || !refPartial(err)) {
		return core.Page[core.Reference]{}, fmt.Errorf("read mentions of %s %s#%d: %w", what, repo, number, err)
	}
	if item == nil {
		return core.Page[core.Reference]{}, fmt.Errorf("read mentions of %s %s#%d: %w", what, repo, number, core.ErrNotFound)
	}
	return item.Timeline.page(pull), nil
}

// refMentionsItem is the item of the pages of mentions.
type refMentionsItem struct {
	Timeline refMentionsTimeline `json:"timelineItems"`
}

// refMentionsTimeline is a page of the cross-references of an item.
type refMentionsTimeline struct {
	PageInfo struct {
		HasPreviousPage bool   `json:"hasPreviousPage"`
		StartCursor     string `json:"startCursor"`
	} `json:"pageInfo"`
	Nodes []struct {
		WillCloseTarget bool     `json:"willCloseTarget"`
		Actor           *actor   `json:"actor"`
		Source          *refItem `json:"source"`
	} `json:"nodes"`
}

// page returns the mentions, the newest first, which GitHub lists the
// oldest first.
func (t refMentionsTimeline) page(pull bool) core.Page[core.Reference] {
	var p core.Page[core.Reference]
	for _, n := range slices.Backward(t.Nodes) {
		ref, ok := n.Source.core()
		if !ok {
			continue
		}
		by, bot := refAuthor(n.Actor)
		if n.WillCloseTarget {
			o := refCloser(pull)
			o.By, o.Bot = by, bot
			ref.Origins = append(ref.Origins, o)
		}
		ref.Origins = append(ref.Origins, core.RefOrigin{Group: core.RefMentioned, Where: "mentioned", By: by, Bot: bot})
		p.Items = append(p.Items, ref)
	}
	if t.PageInfo.HasPreviousPage {
		p.Next = t.PageInfo.StartCursor
	}
	return p
}

// ResolveReferences reads the items that targets name, which say a
// number and a repository but not what they are, in the order of targets.
// It sends refSlots at a time, one request after another. An item that
// can't be read is returned with its Problem set, such as one in a
// private repository, and the others are read. It fails as a whole only
// on an error that names no item, such as a rate limit.
func (c *Client) ResolveReferences(ctx context.Context, targets []core.Target) ([]core.Reference, error) {
	out := make([]core.Reference, 0, len(targets))
	for batch := range refBatches(targets) {
		refs, err := c.resolveSlots(ctx, batch)
		if err != nil {
			return nil, err
		}
		out = append(out, refs...)
	}
	return out, nil
}

// refBatches splits targets into runs of at most refSlots.
func refBatches(targets []core.Target) func(yield func([]core.Target) bool) {
	return func(yield func([]core.Target) bool) {
		for len(targets) > 0 {
			n := min(refSlots, len(targets))
			if !yield(targets[:n]) {
				return
			}
			targets = targets[n:]
		}
	}
}

// resolveSlots reads up to refSlots targets in one query.
func (c *Client) resolveSlots(ctx context.Context, targets []core.Target) ([]core.Reference, error) {
	vars := make(map[string]any, 4*refSlots)
	for i := range refSlots {
		s := strconv.Itoa(i)
		// An unused slot is skipped, but every variable is required.
		o, n, num, used := "-", "-", 1, false
		if i < len(targets) {
			o, n, num, used = targets[i].Repo.Owner, targets[i].Repo.Name, targets[i].Number, true
		}
		vars["o"+s], vars["n"+s], vars["i"+s], vars["w"+s] = o, n, num, used
	}
	var data map[string]json.RawMessage
	err := c.Query(ctx, resolveReferencesQuery, vars, &data)
	problems := map[int]string{}
	if err != nil {
		gerr, ok := errors.AsType[*GraphQLError](err)
		if !ok || len(data) == 0 {
			return nil, fmt.Errorf("resolve references: %w", err)
		}
		for _, item := range gerr.Errors {
			slot, ok := refSlot(item)
			if !ok {
				return nil, fmt.Errorf("resolve references: %w", err)
			}
			if _, seen := problems[slot]; !seen {
				problems[slot] = refProblem(item)
			}
		}
	}
	out := make([]core.Reference, len(targets))
	for i, t := range targets {
		out[i] = core.Reference{Target: t}
		var slot struct {
			Item *refItem `json:"issueOrPullRequest"`
		}
		if raw := data["r"+strconv.Itoa(i)]; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &slot); err != nil {
				return nil, fmt.Errorf("resolve references: decode %s: %w", t, err)
			}
		}
		if ref, ok := slot.Item.core(); ok {
			out[i] = ref
			continue
		}
		out[i].Problem = refUnreadable(problems[i])
	}
	return out, nil
}

// refUnreadable is the problem of a slot that has none of its own: GitHub
// answered null and no error for it.
func refUnreadable(p string) string {
	if p == "" {
		return "not found, or private"
	}
	return p
}

// refSlot returns the slot that an error of ResolveReferences is about,
// by the first segment of its path, such as "r7".
func refSlot(item GraphQLErrorItem) (int, bool) {
	if len(item.Path) == 0 {
		return 0, false
	}
	alias, _ := item.Path[0].(string)
	n, err := strconv.Atoi(strings.TrimPrefix(alias, "r"))
	if err != nil || !strings.HasPrefix(alias, "r") || n < 0 || n >= refSlots {
		return 0, false
	}
	return n, true
}

// refProblem says why an item can't be read, for the user.
func refProblem(item GraphQLErrorItem) string {
	switch item.Type {
	case "NOT_FOUND":
		return "not found, or private"
	case "FORBIDDEN":
		return "access refused"
	}
	if m := oneLine(item.Message); m != "" {
		return m
	}
	return "can't be read"
}
