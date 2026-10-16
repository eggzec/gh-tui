package pulls

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// reviewersAPI is what the requests for review and the team candidates read
// and send. Teams are named by their slugs, without the organization.
type reviewersAPI interface {
	RequestReviewers(ctx context.Context, repo core.RepoRef, number int, logins, teams []string) error
	RemoveReviewers(ctx context.Context, repo core.RepoRef, number int, logins, teams []string) error
	ReviewerTeams(ctx context.Context, org, text string, first int) ([]core.Team, error)
}

// reviewerTeamsPage is how many teams a read of the candidates returns.
const reviewerTeamsPage = 20

var (
	// ErrAlreadyRequested is returned for a request for review that is
	// pending already.
	ErrAlreadyRequested = errors.New("review is already requested")
	// ErrNotPending is returned for a withdrawal of a review request that
	// isn't pending: the person reviewed, or was never asked.
	ErrNotPending = errors.New("only a pending review request can be removed")
)

// Reviewer is a person or a team to ask for a review. Exactly one of Login and
// Team is set; a Reviewer with neither is left out of a change.
type Reviewer struct {
	Login string
	// Team is the team as org/slug.
	Team string
	// Name is the display name of the team, which its chip shows until the
	// next read. It defaults to the slug.
	Name string
}

// key names r, whatever the case of its login or team.
func (r Reviewer) key() string {
	if r.Team != "" {
		return "team:" + strings.ToLower(r.Team)
	}
	return "user:" + strings.ToLower(r.Login)
}

// distinctReviewers returns who without the zero values and the repeats,
// which are the same login or team in any case.
func distinctReviewers(who []Reviewer) []Reviewer {
	var out []Reviewer
	seen := make(map[string]bool, len(who))
	for _, r := range who {
		if r.Login == "" && r.Team == "" || seen[r.key()] {
			continue
		}
		seen[r.key()] = true
		out = append(out, r)
	}
	return out
}

// request returns the review request that r makes.
func (r Reviewer) request() core.ReviewRequest {
	if r.Team != "" {
		return core.ReviewRequest{Team: r.Team, TeamName: cmp.Or(r.Name, r.slug())}
	}
	return core.ReviewRequest{User: core.User{Login: r.Login}}
}

// slug returns the team without its organization.
func (r Reviewer) slug() string {
	_, slug, ok := strings.Cut(r.Team, "/")
	if !ok {
		return r.Team
	}
	return slug
}

// asks reports whether q is a review request of r. A request the token may
// not see matches nobody.
func (r Reviewer) asks(q core.ReviewRequest) bool {
	switch {
	case q.Unknown:
		return false
	case r.Team != "":
		return strings.EqualFold(q.Team, r.Team)
	default:
		return q.Team == "" && strings.EqualFold(q.User.Login, r.Login)
	}
}

// pending reports whether requests holds a request of r.
func pending(requests []core.ReviewRequest, r Reviewer) bool {
	return slices.ContainsFunc(requests, r.asks)
}

// reviewerNames splits who into the logins and the team slugs REST takes.
func reviewerNames(who []Reviewer) (logins, slugs []string) {
	for _, r := range who {
		if r.Team != "" {
			slugs = append(slugs, r.slug())
			continue
		}
		logins = append(logins, r.Login)
	}
	return logins, slugs
}

// RequestReviews asks who to review pull request number of repo, and asks
// again anyone who already reviewed. The cached detail lists them at once,
// and the change is undone if GitHub refuses it. Those already asked are
// left out; if that is everyone, the Op returns ErrAlreadyRequested without
// sending anything. A Reviewer that names nobody is dropped, a person named
// twice is asked once, and an empty who is an Op that does nothing.
func (s *Service) RequestReviews(repo core.RepoRef, number int, who []Reviewer) *optimistic.Op {
	who = distinctReviewers(who)
	if len(who) == 0 {
		return optimistic.New(func(context.Context) error { return nil })
	}
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("request review on pull %s#%d: %w", repo, number, err))
	}
	key := detailKey(repo, number)
	if d, ok := cached(s.details, key); ok {
		who = slices.DeleteFunc(who, func(r Reviewer) bool {
			return pending(d.Reviewers.Requested, r)
		})
		if len(who) == 0 {
			return optimistic.Refused(fmt.Errorf("request review on pull %s#%d: %w", repo, number, ErrAlreadyRequested))
		}
	}
	undo, _ := s.details.Mutate(key, func(d core.PullRequestDetail) core.PullRequestDetail {
		return withRequests(d, who)
	})
	logins, slugs := reviewerNames(who)
	return optimistic.New(func(ctx context.Context) error {
		if err := s.api.RequestReviewers(ctx, repo, number, logins, slugs); err != nil {
			return fmt.Errorf("request review on pull %s#%d: %w", repo, number, err)
		}
		// GitHub answers with nothing to reconcile with. A read that landed
		// while the request was in flight may have stored the old requests,
		// so the change is applied again.
		s.details.Mutate(key, func(d core.PullRequestDetail) core.PullRequestDetail {
			return withRequests(d, who)
		})
		s.staleReviewers(repo, number)
		return nil
	}, undo)
}

// withRequests returns d with a request of each of who that it doesn't hold,
// so it adds nothing to a detail that has them already. A list cut short may
// hold a request that its total counts, so then the total only grows to the
// length of the list.
func withRequests(d core.PullRequestDetail, who []Reviewer) core.PullRequestDetail {
	complete := len(d.Reviewers.Requested) >= d.Reviewers.RequestedTotal
	requests := slices.Clone(d.Reviewers.Requested)
	added := 0
	for _, r := range who {
		if pending(requests, r) {
			continue
		}
		requests = append(requests, r.request())
		added++
	}
	if complete {
		d.Reviewers.RequestedTotal += added
	} else {
		d.Reviewers.RequestedTotal = max(d.Reviewers.RequestedTotal, len(requests))
	}
	d.Reviewers.Requested = requests
	return d
}

// withoutRequests returns d without the requests of who.
func withoutRequests(d core.PullRequestDetail, who []Reviewer) core.PullRequestDetail {
	kept := make([]core.ReviewRequest, 0, len(d.Reviewers.Requested))
	for _, q := range d.Reviewers.Requested {
		if !slices.ContainsFunc(who, func(r Reviewer) bool { return r.asks(q) }) {
			kept = append(kept, q)
		}
	}
	d.Reviewers.RequestedTotal -= len(d.Reviewers.Requested) - len(kept)
	d.Reviewers.Requested = kept
	return d
}

// RemoveReviewRequests withdraws the pending requests for review of who on
// pull request number of repo. If the cached detail lists all the pending
// requests and one of who isn't there, the Op returns ErrNotPending without
// sending anything. A request the token may not see can't be matched, so it
// stays.
func (s *Service) RemoveReviewRequests(repo core.RepoRef, number int, who []Reviewer) *optimistic.Op {
	who = distinctReviewers(who)
	if len(who) == 0 {
		return optimistic.New(func(context.Context) error { return nil })
	}
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("withdraw review request on pull %s#%d: %w", repo, number, err))
	}
	key := detailKey(repo, number)
	if d, ok := cached(s.details, key); ok && len(d.Reviewers.Requested) >= d.Reviewers.RequestedTotal {
		// A list cut short may hold the request, so only a complete one can
		// tell that there is none.
		for _, r := range who {
			if !pending(d.Reviewers.Requested, r) {
				return optimistic.Refused(fmt.Errorf("withdraw review request on pull %s#%d: %w", repo, number, ErrNotPending))
			}
		}
	}
	undo, _ := s.details.Mutate(key, func(d core.PullRequestDetail) core.PullRequestDetail {
		return withoutRequests(d, who)
	})
	logins, slugs := reviewerNames(who)
	return optimistic.New(func(ctx context.Context) error {
		if err := s.api.RemoveReviewers(ctx, repo, number, logins, slugs); err != nil {
			return fmt.Errorf("withdraw review request on pull %s#%d: %w", repo, number, err)
		}
		// As with a request, a read that landed meanwhile may have brought
		// the withdrawn requests back.
		s.details.Mutate(key, func(d core.PullRequestDetail) core.PullRequestDetail {
			return withoutRequests(d, who)
		})
		s.staleReviewers(repo, number)
		return nil
	}, undo)
}

// staleReviewers marks stale what is cached of pull request number of repo
// that a change of its review requests touches, for a change GitHub accepted
// but did not return. The detail stays, so it shows until the next read
// refreshes it with the code owner flags and the names of the teams.
func (s *Service) staleReviewers(repo core.RepoRef, number int) {
	key := detailKey(repo, number)
	s.seen.Delete(key)
	// A review_requested event appears in the timeline.
	forget(s.timelines, s.keptTimelines, key)
	s.details.Invalidate(key)
	s.keptDetails.Delete(key)
	s.lists.InvalidateTag(repoTag(repo))
}

// ReviewerTeamsQuery asks for the teams of the organization that owns Repo
// whose name matches Text, to ask for a review.
type ReviewerTeamsQuery struct {
	Repo core.RepoRef
	Text string
}

// ReviewerTeams are the teams that may be asked for a review.
type ReviewerTeams struct {
	Teams []core.Team
	// Unavailable says that the teams can't be listed: the token lacks the
	// read:org scope, or the viewer isn't a member of the organization. The
	// view leaves the teams out, which is no error.
	Unavailable bool
	// Offline and Limited say that the teams were served in place of an
	// answer, as Page's are.
	Offline, Limited bool
}

func (q ReviewerTeamsQuery) key() string {
	return "pullreviewerteams:" + strings.ToLower(q.Repo.Owner) + "?" + q.Text
}

func reviewerTeamsMarks(t *ReviewerTeams) (offline, limited *bool) { return &t.Offline, &t.Limited }

// CachedReviewerTeams returns the teams of q if they are cached, fresh or
// stale, without reading them.
func (s *Service) CachedReviewerTeams(q ReviewerTeamsQuery) (ReviewerTeams, bool) {
	return cached(s.reviewerTeams, q.key())
}

// ReviewerTeams returns the teams of q, which are kept in memory only. A
// token that lacks the read:org scope, as far as the access service knows,
// is not sent to GitHub: the result is Unavailable. So is a refusal of its
// scopes, for a token whose scopes aren't known, and a 403, which is how
// GitHub answers someone who isn't a member. A user who owns the
// repository has no teams, which is no error. Each answer is cached for the
// TTL, and if GitHub can't be reached a cached list is served with Offline
// set.
func (s *Service) ReviewerTeams(ctx context.Context, q ReviewerTeamsQuery) (ReviewerTeams, error) {
	if s.access != nil && s.access.Check(core.NeedReadOrg) != nil {
		return ReviewerTeams{Unavailable: true}, nil
	}
	load := func(ctx context.Context, _ cache.Entry[ReviewerTeams], _ bool) (cache.Entry[ReviewerTeams], error) {
		teams, err := s.api.ReviewerTeams(ctx, q.Repo.Owner, q.Text, reviewerTeamsPage)
		switch {
		case errors.Is(err, core.ErrForbidden), errors.As(err, new(*core.ScopeError)):
			return cache.Entry[ReviewerTeams]{Value: ReviewerTeams{Unavailable: true}}, nil
		case errors.Is(err, core.ErrNotFound):
			return cache.Entry[ReviewerTeams]{}, nil
		case err != nil:
			return cache.Entry[ReviewerTeams]{}, err
		}
		return cache.Entry[ReviewerTeams]{Value: ReviewerTeams{Teams: teams}}, nil
	}
	e, err := fallback.Fetch(ctx, s.reviewerTeams, nil, q.key(), reviewerTeamsMarks, load)
	if err != nil {
		return ReviewerTeams{}, fmt.Errorf("list teams of %s: %w", q.Repo.Owner, err)
	}
	return e.Value, nil
}
