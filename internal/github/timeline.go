package github

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// timelineNode is the JSON shape of an item of a pull request's or an issue's
// timeline query. A member of the union has only some of its fields, and one
// the query doesn't select has only its Typename.
type timelineNode struct {
	Typename    string        `json:"__typename"`
	ID          string        `json:"id"`
	Actor       *actor        `json:"actor"`
	Author      *reviewAuthor `json:"author"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	SubmittedAt time.Time     `json:"submittedAt"`
	Body        string        `json:"body"`
	URL         string        `json:"url"`

	ViewerCanUpdate bool `json:"viewerCanUpdate"`
	ViewerCanDelete bool `json:"viewerCanDelete"`

	ReactionGroups []struct {
		Content string `json:"content"`
		Users   struct {
			TotalCount int `json:"totalCount"`
		} `json:"users"`
	} `json:"reactionGroups"`

	State      string          `json:"state"`
	Commit     *timelineCommit `json:"commit"`
	PullCommit *timelineCommit `json:"pullCommit"`

	BeforeCommit *timelineCommit `json:"beforeCommit"`
	AfterCommit  *timelineCommit `json:"afterCommit"`

	Label             *label           `json:"label"`
	Assignee          *timelineAccount `json:"assignee"`
	RequestedReviewer *timelineAccount `json:"requestedReviewer"`

	MergeRefName    string `json:"mergeRefName"`
	HeadRefName     string `json:"headRefName"`
	PreviousRefName string `json:"previousRefName"`
	CurrentRefName  string `json:"currentRefName"`
	PreviousTitle   string `json:"previousTitle"`
	CurrentTitle    string `json:"currentTitle"`

	WillCloseTarget  bool            `json:"willCloseTarget"`
	Source           *timelineSource `json:"source"`
	DismissalMessage string          `json:"dismissalMessage"`
	LockReason       string          `json:"lockReason"`
	MilestoneTitle   string          `json:"milestoneTitle"`
}

type timelineCommit struct {
	OID             string    `json:"oid"`
	AbbreviatedOID  string    `json:"abbreviatedOid"`
	MessageHeadline string    `json:"messageHeadline"`
	CommittedDate   time.Time `json:"committedDate"`
	Author          *struct {
		Name string `json:"name"`
		User *user  `json:"user"`
	} `json:"author"`
}

// timelineAccount is a person, an app or a team that an event is about.
type timelineAccount struct {
	Typename     string `json:"__typename"`
	Login        string `json:"login"`
	Name         string `json:"name"`
	CombinedSlug string `json:"combinedSlug"`
}

type timelineSource struct {
	Typename   string `json:"__typename"`
	Number     int    `json:"number"`
	Title      string `json:"title"`
	State      string `json:"state"`
	IssueState string `json:"issueState"`
	URL        string `json:"url"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

// eventKinds are the kinds of the events, by __typename.
var eventKinds = map[string]core.TimelineKind{
	"HeadRefForcePushedEvent":   core.TimelineForcePushed,
	"LabeledEvent":              core.TimelineLabeled,
	"UnlabeledEvent":            core.TimelineUnlabeled,
	"AssignedEvent":             core.TimelineAssigned,
	"UnassignedEvent":           core.TimelineUnassigned,
	"ReviewRequestedEvent":      core.TimelineReviewRequested,
	"ReviewRequestRemovedEvent": core.TimelineReviewRequestRemoved,
	"ReadyForReviewEvent":       core.TimelineReadyForReview,
	"ConvertToDraftEvent":       core.TimelineConvertedToDraft,
	"MergedEvent":               core.TimelineMerged,
	"ClosedEvent":               core.TimelineClosed,
	"ReopenedEvent":             core.TimelineReopened,
	"RenamedTitleEvent":         core.TimelineRenamed,
	"BaseRefChangedEvent":       core.TimelineBaseChanged,
	"CrossReferencedEvent":      core.TimelineCrossReferenced,
	"HeadRefDeletedEvent":       core.TimelineHeadDeleted,
	"ReviewDismissedEvent":      core.TimelineReviewDismissed,
	"LockedEvent":               core.TimelineLocked,
	"UnlockedEvent":             core.TimelineUnlocked,
	"MilestonedEvent":           core.TimelineMilestoned,
	"DemilestonedEvent":         core.TimelineDemilestoned,
}

func (c timelineCommit) core() core.TimelineCommit {
	out := core.TimelineCommit{OID: c.OID, Short: c.AbbreviatedOID, Headline: c.MessageHeadline}
	if c.Author != nil {
		out.Author = core.User{Name: c.Author.Name}
		if u := c.Author.User; u != nil {
			out.Author = core.User{Login: u.Login, Name: cmp.Or(u.Name, c.Author.Name)}
		}
	}
	return out
}

func (a timelineAccount) core() core.TimelineSubject {
	if a.CombinedSlug != "" {
		return core.TimelineSubject{Team: a.CombinedSlug}
	}
	return core.TimelineSubject{User: core.User{Login: a.Login, Name: a.Name, Bot: a.Typename == "Bot"}}
}

func (s timelineSource) core() core.TimelineSource {
	out := core.TimelineSource{
		Number: s.Number, Title: s.Title, URL: s.URL, Repo: s.Repository.NameWithOwner,
		IsPullRequest: s.Typename == "PullRequest",
		State:         core.State(strings.ToLower(cmp.Or(s.State, s.IssueState))),
	}
	return out
}

// core converts the item, and reports false for one that the timeline
// doesn't show: a member of the union it doesn't know, or a pending review,
// which only its author sees and which isn't part of the conversation yet.
func (n timelineNode) core() (core.TimelineItem, bool) {
	it := core.TimelineItem{ID: n.ID, At: n.CreatedAt, Body: n.Body, URL: n.URL}
	if n.Actor != nil {
		it.Actor = n.Actor.core()
	}
	for _, g := range n.ReactionGroups {
		if g.Users.TotalCount > 0 {
			it.Reactions = append(it.Reactions, core.Reaction{Content: g.Content, Count: g.Users.TotalCount})
		}
	}
	switch n.Typename {
	case "IssueComment":
		it.Kind = core.TimelineComment
		it.UpdatedAt = n.UpdatedAt
		it.CanUpdate, it.CanDelete = n.ViewerCanUpdate, n.ViewerCanDelete
	case "PullRequestReview":
		if strings.EqualFold(n.State, "PENDING") {
			return core.TimelineItem{}, false
		}
		it.Kind = core.TimelineReview
		it.Review = core.ReviewState(strings.ToLower(n.State))
		if !n.SubmittedAt.IsZero() {
			it.At = n.SubmittedAt
		}
		if n.Commit != nil {
			it.Commit = n.Commit.core()
		}
	case "PullRequestCommit":
		if n.PullCommit == nil {
			return core.TimelineItem{}, false
		}
		it.Kind = core.TimelinePushed
		it.Commit = n.PullCommit.core()
		it.Actor = it.Commit.Author
		it.At = n.PullCommit.CommittedDate
	default:
		kind, ok := eventKinds[n.Typename]
		if !ok {
			return core.TimelineItem{}, false
		}
		it.Kind = kind
		n.fillEvent(&it)
	}
	if n.Author != nil {
		it.Actor, it.AvatarURL = n.Author.core(), n.Author.AvatarURL
	}
	return it, true
}

// fillEvent sets what an event of it.Kind says besides who and when.
func (n timelineNode) fillEvent(it *core.TimelineItem) {
	switch it.Kind {
	case core.TimelineForcePushed:
		if n.BeforeCommit != nil {
			it.From = n.BeforeCommit.OID
		}
		if n.AfterCommit != nil {
			it.To = n.AfterCommit.OID
		}
	case core.TimelineLabeled, core.TimelineUnlabeled:
		if n.Label != nil {
			it.Label = n.Label.core()
		}
	case core.TimelineAssigned, core.TimelineUnassigned:
		if n.Assignee != nil {
			it.Subject = n.Assignee.core()
		}
	case core.TimelineReviewRequested, core.TimelineReviewRequestRemoved:
		if n.RequestedReviewer != nil {
			it.Subject = n.RequestedReviewer.core()
		}
	case core.TimelineMerged:
		it.Ref = n.MergeRefName
		if n.Commit != nil {
			it.Commit = n.Commit.core()
		}
	case core.TimelineRenamed:
		it.From, it.To = n.PreviousTitle, n.CurrentTitle
	case core.TimelineBaseChanged:
		it.From, it.To = n.PreviousRefName, n.CurrentRefName
	case core.TimelineCrossReferenced:
		if n.Source != nil {
			it.Source = n.Source.core()
		}
		it.Source.WillClose = n.WillCloseTarget
	case core.TimelineHeadDeleted:
		it.Ref = n.HeadRefName
	case core.TimelineReviewDismissed:
		it.Body = n.DismissalMessage
	case core.TimelineLocked:
		it.Ref = strings.ToLower(n.LockReason)
	case core.TimelineMilestoned, core.TimelineDemilestoned:
		it.Ref = n.MilestoneTitle
	case core.TimelineComment, core.TimelineReview, core.TimelinePushed,
		core.TimelineReadyForReview, core.TimelineConvertedToDraft,
		core.TimelineClosed, core.TimelineReopened, core.TimelineUnlocked:
		// Who and when say all there is to these.
	}
}

// timelineItems converts the nodes of a timeline page, leaving out the
// items of kinds the client doesn't know, so a page may hold fewer than
// asked for while another follows.
func timelineItems(ctx context.Context, nodes []timelineNode, next pageInfo) core.Page[core.TimelineItem] {
	items := make([]core.TimelineItem, 0, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		it, ok := n.core()
		if !ok {
			slog.DebugContext(ctx, "timeline item skipped", "span", "github.timeline", "type", n.Typename)
			continue
		}
		items = append(items, it)
	}
	return core.Page[core.TimelineItem]{Items: items, Next: next.next()}
}

// timelineErr wraps err of a timeline read, and makes a missing parent
// core.ErrNotFound.
func timelineErr(err error, found bool, kind string, repo core.RepoRef, number int) error {
	if err == nil && !found {
		err = core.ErrNotFound
	}
	if err == nil {
		return nil
	}
	return fmt.Errorf("list timeline of %s %s#%d: %w", kind, repo, number, err)
}
