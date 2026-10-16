package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// reviewerTeamsQuery lists the teams of an organization that match text, by
// name. Only members see them, so it asks whether the viewer is one.
const reviewerTeamsQuery = `query ReviewerTeams($login: String!, $query: String, $first: Int!) {
  ` + rateLimitField + `
  organization(login: $login) {
    viewerIsAMember
    teams(first: $first, query: $query, orderBy: {field: NAME, direction: ASC}) {
      nodes { name slug }
    }
  }
}`

// reviewersBody is the body of the calls that request or withdraw reviews.
// REST marks reviewers as required, so a withdrawal always sends it.
type reviewersBody struct {
	Reviewers     []string `json:"reviewers"`
	TeamReviewers []string `json:"team_reviewers,omitempty"`
}

// reviewersPath is the path of the review requests of pull request number.
func reviewersPath(repo core.RepoRef, number int) string {
	return commitsBase(repo) + "/pulls/" + strconv.Itoa(number) + "/requested_reviewers"
}

// RequestReviewers asks the users logins and the teams, given by their slugs
// without the organization, to review pull request number of repo. The
// request adds to the pending ones, and asks again someone who already
// reviewed, as the web's re-request does. GitHub refuses the author of the
// pull request, and a user who can't review it, with a 422.
//
// It uses REST because GraphQL's requestReviews takes node IDs, which the
// reads of the candidates don't return. The answer is the REST pull, which
// has none of the fields of our detail, so nothing is decoded.
func (c *Client) RequestReviewers(ctx context.Context, repo core.RepoRef, number int, logins, teams []string) error {
	body := reviewersBody{Reviewers: logins, TeamReviewers: teams}
	if _, err := c.Do(ctx, http.MethodPost, reviewersPath(repo, number), body, nil); err != nil {
		return fmt.Errorf("request reviewers of pull %s#%d: %w", repo, number, err)
	}
	return nil
}

// RemoveReviewers withdraws the pending requests of the users logins and the
// teams, given by their slugs, on pull request number of repo, as
// RequestReviewers names them. GraphQL has no mutation to withdraw a
// request, so it uses REST.
func (c *Client) RemoveReviewers(ctx context.Context, repo core.RepoRef, number int, logins, teams []string) error {
	body := reviewersBody{Reviewers: logins, TeamReviewers: teams}
	if body.Reviewers == nil {
		body.Reviewers = []string{}
	}
	if _, err := c.Do(ctx, http.MethodDelete, reviewersPath(repo, number), body, nil); err != nil {
		return fmt.Errorf("withdraw reviewers of pull %s#%d: %w", repo, number, err)
	}
	return nil
}

// teamsRefused reports whether err says that GitHub refused the token the
// organization or its teams: a token without read:org gets INSUFFICIENT_SCOPES,
// as a root error, or on a field below it with the data that is left, which
// carries no cause of its own.
func teamsRefused(err error) bool {
	e, ok := errors.AsType[*GraphQLError](err)
	if !ok {
		return false
	}
	return slices.ContainsFunc(e.Errors, func(item GraphQLErrorItem) bool {
		refusal := item.Type == "FORBIDDEN" || item.Type == "INSUFFICIENT_SCOPES"
		return refusal && (len(item.Path) == 0 || item.Path[0] == "organization")
	})
}

// ReviewerTeams returns up to first teams of the organization org whose name
// matches text, by name, to ask for a review. Only members see the teams, and
// a token needs read:org: for anyone else it returns an error matching
// core.ErrForbidden, which a refused scope matches too. It returns
// an error matching core.ErrNotFound if there is no such organization,
// which is what a user who owns the repository is.
func (c *Client) ReviewerTeams(ctx context.Context, org, text string, first int) ([]core.Team, error) {
	var data struct {
		Org *struct {
			Member bool `json:"viewerIsAMember"`
			Teams  struct {
				Nodes []struct {
					Name string `json:"name"`
					Slug string `json:"slug"`
				} `json:"nodes"`
			} `json:"teams"`
		} `json:"organization"`
	}
	vars := map[string]any{"login": org, "first": first}
	if text != "" {
		vars["query"] = text
	}
	if err := c.Query(ctx, reviewerTeamsQuery, vars, &data); err != nil {
		if teamsRefused(err) {
			return nil, fmt.Errorf("list teams of %s: %w: %w", org, core.ErrForbidden, err)
		}
		return nil, fmt.Errorf("list teams of %s: %w", org, err)
	}
	if data.Org == nil {
		return nil, fmt.Errorf("list teams of %s: %w", org, core.ErrNotFound)
	}
	if !data.Org.Member {
		return nil, fmt.Errorf("list teams of %s: %w", org, errMembersOnly)
	}
	teams := make([]core.Team, 0, len(data.Org.Teams.Nodes))
	for _, n := range data.Org.Teams.Nodes {
		teams = append(teams, core.Team{Name: n.Name, Slug: n.Slug})
	}
	return teams, nil
}
