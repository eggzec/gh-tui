package github

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eggzec/gh-tui/internal/core"
)

// The detail of a pull request is one read, which holds what the head of
// its modal and its Overview need: the merge state and what stands in its
// way, the reviewers, a summary of the review threads and the checks that
// failed. Its comments, reviews and the checks themselves are read apart,
// since they can be long.

// pullExcerpt bounds the text kept of a comment or a check.
const (
	// pullExcerpt is how many characters of a thread's first comment, or
	// of a failing check's reason, are kept.
	pullExcerpt = 160
)

// getPullQuery reads what the detail view shows on top of pullFields. The
// checks of the head commit are counted by outcome, and only those that
// failed are kept, with why: the checks step lists them all with
// PullChecks. The rules of the base branch are those of its branch
// protection, which refUpdateRule gives to any token that reads the
// repository, and those of the rulesets that apply to it.
var getPullQuery = fmt.Sprintf(`query GetPull($owner: String!, $name: String!, $number: Int!, $threads: Int!, $reviewers: Int!, $rules: Int!) {
  `+rateLimitField+`
  repository(owner: $owner, name: $name) {
    mergeCommitAllowed
    squashMergeAllowed
    rebaseMergeAllowed
    autoMergeAllowed
    pullRequest(number: $number) {
      ...pullFields
      body
      baseRefOid
      isCrossRepository
      headRepository { nameWithOwner }
      milestone { title }
      mergeable
      mergeStateStatus
      isInMergeQueue
      isMergeQueueEnabled
      mergeQueueEntry { position state enqueuedAt }
      autoMergeRequest { mergeMethod enabledAt enabledBy { login } }
      viewerCanEnableAutoMerge
      viewerCanDisableAutoMerge
      viewerCanMergeAsAdmin
      baseRef {
        refUpdateRule {
          requiredApprovingReviewCount requiredStatusCheckContexts
          requiresCodeOwnerReviews requiresConversationResolution requiresLinearHistory requiresSignatures
        }
        rules(first: $rules) {
          nodes {
            type
            parameters {
              ... on PullRequestParameters { requiredApprovingReviewCount requireCodeOwnerReview requiredReviewThreadResolution }
              ... on RequiredStatusChecksParameters { requiredStatusChecks { context } }
            }
          }
        }
      }
      reviewRequests(first: $reviewers) {
        totalCount
        nodes {
          asCodeOwner
          requestedReviewer {
            __typename
            ... on User { login name }
            ... on Bot { login }
            ... on Mannequin { login }
            ... on Team { combinedSlug }
          }
        }
      }
      latestOpinionatedReviews(first: $reviewers) {
        totalCount
        nodes { author { __typename login ... on User { name } } state submittedAt }
      }
      viewerLatestReview { state }
      reviewThreads(first: $threads) {
        totalCount
        pageInfo { hasNextPage endCursor }
        nodes {
          id isResolved isOutdated path line originalLine
          comments(first: 1) {
            totalCount
            nodes { author { __typename login ... on User { name } } createdAt bodyText }
          }
        }
      }
      headCommit: commits(last: 1) {
        nodes { commit { statusCheckRollup { contexts(first: %d) {
          totalCount
          checkRunCountsByState { state count }
          statusContextCountsByState { state count }
          nodes {
            __typename
            ... on CheckRun {
              databaseId name status conclusion title
              isRequired(pullRequestNumber: $number)
            }
            ... on StatusContext { context state description isRequired(pullRequestNumber: $number) }
          }
        } } } }
      }
    }
  }
}
`, checksContexts) + pullFields

// pullDetail is the JSON shape of the pull request in getPullQuery.
type pullDetail struct {
	pull
	Body              string `json:"body"`
	BaseRefOid        string `json:"baseRefOid"`
	IsCrossRepository bool   `json:"isCrossRepository"`
	// HeadRepository is null once a fork is deleted.
	HeadRepository *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"headRepository"`
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`

	Mergeable           string `json:"mergeable"`
	MergeStateStatus    string `json:"mergeStateStatus"`
	IsInMergeQueue      bool   `json:"isInMergeQueue"`
	IsMergeQueueEnabled bool   `json:"isMergeQueueEnabled"`
	MergeQueueEntry     *struct {
		Position   int       `json:"position"`
		State      string    `json:"state"`
		EnqueuedAt time.Time `json:"enqueuedAt"`
	} `json:"mergeQueueEntry"`
	AutoMergeRequest *struct {
		MergeMethod string    `json:"mergeMethod"`
		EnabledAt   time.Time `json:"enabledAt"`
		EnabledBy   *user     `json:"enabledBy"`
	} `json:"autoMergeRequest"`
	ViewerCanEnableAutoMerge  bool `json:"viewerCanEnableAutoMerge"`
	ViewerCanDisableAutoMerge bool `json:"viewerCanDisableAutoMerge"`
	ViewerCanMergeAsAdmin     bool `json:"viewerCanMergeAsAdmin"`
	// BaseRef is null for a base branch that was deleted, and its rule for
	// a branch without rules or that the token may not read.
	BaseRef *struct {
		Rules nodes[struct {
			Type       string `json:"type"`
			Parameters struct {
				RequiredApprovingReviewCount   int  `json:"requiredApprovingReviewCount"`
				RequireCodeOwnerReview         bool `json:"requireCodeOwnerReview"`
				RequiredReviewThreadResolution bool `json:"requiredReviewThreadResolution"`
				RequiredStatusChecks           []struct {
					Context string `json:"context"`
				} `json:"requiredStatusChecks"`
			} `json:"parameters"`
		}] `json:"rules"`
		RefUpdateRule *struct {
			RequiredApprovingReviewCount   int      `json:"requiredApprovingReviewCount"`
			RequiredStatusCheckContexts    []string `json:"requiredStatusCheckContexts"`
			RequiresCodeOwnerReviews       bool     `json:"requiresCodeOwnerReviews"`
			RequiresConversationResolution bool     `json:"requiresConversationResolution"`
			RequiresLinearHistory          bool     `json:"requiresLinearHistory"`
			RequiresSignatures             bool     `json:"requiresSignatures"`
		} `json:"refUpdateRule"`
	} `json:"baseRef"`

	ReviewRequests struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			AsCodeOwner bool `json:"asCodeOwner"`
			// RequestedReviewer is null for a reviewer that is gone.
			RequestedReviewer *struct {
				actor
				CombinedSlug string `json:"combinedSlug"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	LatestOpinionatedReviews struct {
		TotalCount int      `json:"totalCount"`
		Nodes      []review `json:"nodes"`
	} `json:"latestOpinionatedReviews"`
	ViewerLatestReview *struct {
		State string `json:"state"`
	} `json:"viewerLatestReview"`
	ReviewThreads struct {
		TotalCount int            `json:"totalCount"`
		PageInfo   pageInfo       `json:"pageInfo"`
		Nodes      []reviewThread `json:"nodes"`
	} `json:"reviewThreads"`

	HeadCommit nodes[struct {
		Commit struct {
			StatusCheckRollup *struct {
				Contexts struct {
					pullCheckCounts
					TotalCount int             `json:"totalCount"`
					Nodes      []checksContext `json:"nodes"`
				} `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"commit"`
	}] `json:"headCommit"`
}

// reviewThread is a thread of review comments on a diff.
type reviewThread struct {
	ID           string `json:"id"`
	IsResolved   bool   `json:"isResolved"`
	IsOutdated   bool   `json:"isOutdated"`
	Path         string `json:"path"`
	Line         int    `json:"line"`
	OriginalLine int    `json:"originalLine"`
	Comments     struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			Author    *actor    `json:"author"`
			CreatedAt time.Time `json:"createdAt"`
			BodyText  string    `json:"bodyText"`
		} `json:"nodes"`
	} `json:"comments"`
}

func (t reviewThread) core() core.ReviewThread {
	out := core.ReviewThread{
		ID:       t.ID,
		Path:     oneLine(t.Path),
		Line:     t.Line,
		Resolved: t.IsResolved,
		Outdated: t.IsOutdated,
		Comments: t.Comments.TotalCount,
	}
	// An outdated thread has no line in the current diff: it keeps the
	// one it was made on.
	if out.Line == 0 {
		out.Line = t.OriginalLine
	}
	if len(t.Comments.Nodes) > 0 {
		c := t.Comments.Nodes[0]
		if c.Author != nil {
			out.Author = c.Author.core()
		}
		out.At = c.CreatedAt
		out.Excerpt = excerpt(c.BodyText)
	}
	return out
}

// excerpt returns the start of s on one line, at most pullExcerpt
// characters, ending in an ellipsis when it is cut.
func excerpt(s string) string {
	s = oneLine(s)
	if utf8.RuneCountInString(s) <= pullExcerpt {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:pullExcerpt-1]), " ") + "…"
}

func (d pullDetail) core() core.PullRequestDetail {
	pr := d.pull.core()
	pr.Body = d.Body
	out := core.PullRequestDetail{
		PullRequest: pr,
		BaseSHA:     d.BaseRefOid,
		CrossRepo:   d.IsCrossRepository,
	}
	if d.HeadRepository != nil {
		out.HeadRepo = oneLine(d.HeadRepository.NameWithOwner)
	}
	if d.Milestone != nil {
		out.Milestone = oneLine(d.Milestone.Title)
	}
	if len(d.HeadCommit.Nodes) > 0 {
		if r := d.HeadCommit.Nodes[0].Commit.StatusCheckRollup; r != nil {
			out.CheckCounts = r.Contexts.core()
			out.FailingChecks, out.Merge.RequiredChecks = failingChecks(r.Contexts.Nodes)
			out.ChecksTruncated = r.Contexts.TotalCount > len(r.Contexts.Nodes)
		}
	}
	d.mergeInto(&out.Merge)
	out.Reviewers = d.reviewers()
	out.Threads = d.threads()
	return out
}

// failingChecks returns the checks of contexts that failed, in order, and
// how the required ones fare.
func failingChecks(contexts []checksContext) (failing []core.FailingCheck, required core.CheckCounts) {
	for i := range contexts {
		c := &contexts[i]
		var f core.FailingCheck
		var outcome core.CheckOutcome
		switch c.Typename {
		case "CheckRun":
			run := c.checkRun()
			outcome = run.Outcome()
			f = core.FailingCheck{Name: oneLine(run.Name), Run: true, ID: run.ID, Required: run.Required}
			f.Reason = c.runReason()
		case "StatusContext":
			st := c.statusContext()
			outcome = st.Outcome()
			f = core.FailingCheck{Name: oneLine(st.Context), Required: st.Required, Reason: excerpt(st.Description)}
		default:
			continue
		}
		if f.Required {
			switch outcome {
			case core.CheckPassing:
				required.Passed++
			case core.CheckPending:
				required.Pending++
			case core.CheckFailing:
				required.Failed++
			}
		}
		if outcome == core.CheckFailing {
			failing = append(failing, f)
		}
	}
	return failing, required
}

// runReason returns why a check run failed to show beside its name: its
// title, which the check's app sets to say what happened.
func (c *checksContext) runReason() string {
	return excerpt(c.Title)
}

// mergeInto sets the merge state of the pull request in m.
func (d pullDetail) mergeInto(m *core.MergeInfo) {
	m.Mergeable = core.Mergeable(strings.ToLower(d.Mergeable))
	m.Status = core.MergeStatus(strings.ToLower(d.MergeStateStatus))
	m.CanAutoMerge = d.ViewerCanEnableAutoMerge
	m.CanDisableAutoMerge = d.ViewerCanDisableAutoMerge
	m.CanMergeAsAdmin = d.ViewerCanMergeAsAdmin
	if a := d.AutoMergeRequest; a != nil {
		m.AutoMerge = &core.AutoMerge{Method: core.MergeMethod(strings.ToLower(a.MergeMethod)), EnabledAt: a.EnabledAt}
		if a.EnabledBy != nil {
			m.AutoMerge.EnabledBy = oneLine(a.EnabledBy.Login)
		}
	}
	m.Queue = core.MergeQueue{Enabled: d.IsMergeQueueEnabled, Queued: d.IsInMergeQueue}
	if e := d.MergeQueueEntry; e != nil {
		m.Queue.Queued = true
		m.Queue.Position, m.Queue.State, m.Queue.EnqueuedAt = e.Position, strings.ToLower(e.State), e.EnqueuedAt
	}
	if b := d.BaseRef; b != nil && (b.RefUpdateRule != nil || len(b.Rules.Nodes) > 0) {
		m.Rules.Known = true
		if r := b.RefUpdateRule; r != nil {
			m.Rules.Approvals = r.RequiredApprovingReviewCount
			m.Rules.CodeOwners = r.RequiresCodeOwnerReviews
			m.Rules.Conversations = r.RequiresConversationResolution
			m.Rules.LinearHistory = r.RequiresLinearHistory
			m.Rules.Signatures = r.RequiresSignatures
			addRuleChecks(&m.Rules, r.RequiredStatusCheckContexts...)
		}
		// The rulesets' rules add to those of the branch protection.
		for _, rule := range b.Rules.Nodes {
			p := rule.Parameters
			switch rule.Type {
			case "PULL_REQUEST":
				m.Rules.Approvals = max(m.Rules.Approvals, p.RequiredApprovingReviewCount)
				m.Rules.CodeOwners = m.Rules.CodeOwners || p.RequireCodeOwnerReview
				m.Rules.Conversations = m.Rules.Conversations || p.RequiredReviewThreadResolution
			case "REQUIRED_STATUS_CHECKS":
				for _, c := range p.RequiredStatusChecks {
					addRuleChecks(&m.Rules, c.Context)
				}
			case "REQUIRED_LINEAR_HISTORY":
				m.Rules.LinearHistory = true
			case "REQUIRED_SIGNATURES":
				m.Rules.Signatures = true
			}
		}
	}
}

// addRuleChecks adds the names of required checks to r, once each.
func addRuleChecks(r *core.MergeRules, names ...string) {
	for _, n := range names {
		if n = oneLine(n); !slices.Contains(r.Checks, n) {
			r.Checks = append(r.Checks, n)
		}
	}
}

// reviewers returns who was asked to review the pull request and what they
// answered.
func (d pullDetail) reviewers() core.Reviewers {
	out := core.Reviewers{
		RequestedTotal: d.ReviewRequests.TotalCount,
		VerdictsTotal:  d.LatestOpinionatedReviews.TotalCount,
	}
	for _, n := range d.ReviewRequests.Nodes {
		r := n.RequestedReviewer
		req := core.ReviewRequest{CodeOwner: n.AsCodeOwner}
		switch {
		case r == nil:
			req.Unknown = true
		case r.Typename == "Team":
			req.Team = oneLine(r.CombinedSlug)
		case r.Login != "":
			req.User = r.core()
		default:
			req.Unknown = true
		}
		out.Requested = append(out.Requested, req)
	}
	for _, r := range d.LatestOpinionatedReviews.Nodes {
		c := r.core()
		out.Verdicts = append(out.Verdicts, core.Verdict{Author: c.Author, State: c.State, SubmittedAt: c.SubmittedAt})
	}
	if v := d.ViewerLatestReview; v != nil {
		out.Viewer = core.ReviewState(strings.ToLower(v.State))
	}
	return out
}

// threads summarizes the review threads that were read.
func (d pullDetail) threads() core.ThreadSummary {
	t := d.ReviewThreads
	out := core.ThreadSummary{
		Total:     t.TotalCount,
		Threads:   convert(t.Nodes, reviewThread.core),
		Truncated: t.TotalCount > len(t.Nodes),
	}
	for i := range out.Threads {
		th := &out.Threads[i]
		if th.Resolved {
			out.Resolved++
		} else {
			out.Unresolved++
		}
		if th.Outdated {
			out.Outdated++
		}
	}
	return out
}

// DetailSizes are how many of each nested list GetPullRequest reads, each
// 1 to 100: the review threads, the review requests and the latest reviews,
// and the rules of rulesets of the base branch. A size out of range is
// brought into it.
type DetailSizes struct {
	Threads, Reviewers, Rules int
}

// vars returns the variables of getPullQuery for sizes.
func (z DetailSizes) vars() map[string]any {
	clamp := func(n int) int { return min(max(n, 1), 100) }
	return map[string]any{"threads": clamp(z.Threads), "reviewers": clamp(z.Reviewers), "rules": clamp(z.Rules)}
}

// GetPullRequest returns pull request number of repo with its body, its
// merge state, reviewers, a summary of its review threads and the checks
// of its head commit that failed, with lists of the sizes. Its reviews are read a
// page at a time with ListPullRequestReviews, and its comments, which are
// those of its issue, with ListIssueComments.
//
// A token that may not read the base branch's rules or its merge queue
// still gets the rest: those fields stay unset. An Enterprise Server whose
// schema lacks a field of the detail leaves it out: see Client.Query.
func (c *Client) GetPullRequest(ctx context.Context, repo core.RepoRef, number int, sizes DetailSizes) (core.PullRequestDetail, error) {
	vars := sizes.vars()
	vars["owner"], vars["name"], vars["number"] = repo.Owner, repo.Name, number
	var data struct {
		Repository *struct {
			MergeCommitAllowed bool        `json:"mergeCommitAllowed"`
			SquashMergeAllowed bool        `json:"squashMergeAllowed"`
			RebaseMergeAllowed bool        `json:"rebaseMergeAllowed"`
			AutoMergeAllowed   bool        `json:"autoMergeAllowed"`
			PullRequest        *pullDetail `json:"pullRequest"`
		} `json:"repository"`
	}
	err := c.Query(ctx, getPullQuery, vars, &data)
	if err != nil && (data.Repository == nil || data.Repository.PullRequest == nil || !detailPartial(err)) {
		return core.PullRequestDetail{}, fmt.Errorf("get pull request %s#%d: %w", repo, number, err)
	}
	if data.Repository == nil || data.Repository.PullRequest == nil {
		return core.PullRequestDetail{}, fmt.Errorf("get pull request %s#%d: %w", repo, number, core.ErrNotFound)
	}
	r := data.Repository
	out := r.PullRequest.core()
	out.Merge.RepoAutoMerge = r.AutoMergeAllowed
	for _, m := range []struct {
		allowed bool
		method  core.MergeMethod
	}{{r.MergeCommitAllowed, core.MergeCommit}, {r.SquashMergeAllowed, core.MergeSquash}, {r.RebaseMergeAllowed, core.MergeRebase}} {
		if m.allowed {
			out.Merge.Methods = append(out.Merge.Methods, m.method)
		}
	}
	return out, nil
}

// detailOptional are the fields of the pull request in getPullQuery that
// only some tokens may read, and that are null when they are refused: the
// base branch with its rules, the pull request's place in a merge queue, and
// the review requests, whose teams a token without read:org may not see:
// such a reviewer reads as unknown.
var detailOptional = []string{"baseRef", "mergeQueueEntry", "reviewRequests"}

// detailPartial reports whether err only refuses fields of the detail that
// detailOptional names, so that the rest of what came with it is kept.
func detailPartial(err error) bool {
	gerr, ok := errors.AsType[*GraphQLError](err)
	if !ok || len(gerr.Errors) == 0 {
		return false
	}
	for _, item := range gerr.Errors {
		if item.Type != "FORBIDDEN" && item.Type != "INSUFFICIENT_SCOPES" {
			return false
		}
		if len(item.Path) < 3 || item.Path[0] != "repository" || item.Path[1] != "pullRequest" {
			return false
		}
		if name, _ := item.Path[2].(string); !slices.Contains(detailOptional, name) {
			return false
		}
	}
	return true
}
