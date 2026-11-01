package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Notifications have no GraphQL API, so these methods use REST, which also
// supports Last-Modified revalidation and X-Poll-Interval.

type notification struct {
	ID         string `json:"id"`
	Repository struct {
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
		Owner   struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Subject struct {
		Title string `json:"title"`
		// URL is null for some subjects, such as discussions, which
		// decodes to "".
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"subject"`
	Reason    string    `json:"reason"`
	Unread    bool      `json:"unread"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (n notification) core() core.Notification {
	typ := core.SubjectType(n.Subject.Type)
	sub := parseSubject(n.Repository.HTMLURL, n.Subject.URL)
	sub.Title, sub.Type, sub.URL = n.Subject.Title, typ, n.Subject.URL
	return core.Notification{
		ID:        n.ID,
		Repo:      repoRef(n.Repository.Owner.Login, n.Repository.Name),
		Subject:   sub,
		Reason:    n.Reason,
		Unread:    n.Unread,
		UpdatedAt: n.UpdatedAt,
	}
}

// parseSubject reads what a subject's API URL, such as
// https://api.github.com/repos/o/r/pulls/42, names, and its page under the
// repository's web URL. Building on the repository's URL keeps the right
// host on GitHub Enterprise Server.
func parseSubject(repoURL, apiURL string) core.Subject {
	// After "/repos/": owner, name, kind and ID.
	if _, rest, ok := strings.Cut(apiURL, "/repos/"); ok {
		if parts := strings.SplitN(rest, "/", 4); len(parts) == 4 {
			id := parts[3]
			n, _ := strconv.Atoi(id)
			switch parts[2] {
			case "pulls":
				return core.Subject{Number: n, WebURL: repoURL + "/pull/" + id}
			case "issues":
				return core.Subject{Number: n, WebURL: repoURL + "/issues/" + id}
			case "discussions":
				return core.Subject{Number: n, WebURL: repoURL + "/discussions/" + id}
			case "commits":
				return core.Subject{SHA: id, WebURL: repoURL + "/commit/" + id}
			case "releases":
				rid, _ := strconv.ParseInt(id, 10, 64)
				// The API names releases by ID, which the web doesn't
				// use.
				return core.Subject{ReleaseID: rid, WebURL: repoURL + "/releases"}
			case "check-suites":
				sid, _ := strconv.ParseInt(id, 10, 64)
				// A check suite has no page of its own; ResolveSubjectURL
				// finds its run.
				return core.Subject{CheckSuiteID: sid, WebURL: repoURL + "/actions"}
			}
		}
	}
	// A subject with no URL, or one this doesn't know, opens the
	// repository's page.
	return core.Subject{WebURL: repoURL}
}

// ListNotifications returns a page of up to perPage of the user's
// notification threads, newest first; zero perPage leaves GitHub's default.
// Cursor is the Next of the previous page, or empty for the first page; it
// already carries the filter and the page size, so both are ignored then. If
// cond is current, the Response has NotModified set and the page is empty.
func (c *Client) ListNotifications(ctx context.Context, filter core.NotificationFilter, perPage int, cursor string, cond Conditional) (core.Page[core.Notification], Response, error) {
	path := cursor
	if path == "" {
		path = notificationsPath(filter, perPage, time.Now())
	}
	var threads []notification
	res, err := c.Get(ctx, path, cond, &threads)
	if err != nil {
		return core.Page[core.Notification]{}, res, fmt.Errorf("list notifications: %w", err)
	}
	if res.NotModified {
		return core.Page[core.Notification]{}, res, nil
	}
	return core.Page[core.Notification]{Items: convert(threads, notification.core), Next: res.Next}, res, nil
}

// ResolveSubjectURL returns the page that a release or check suite
// notification is about: the release's page, or the page of the first run of
// the check suite. The web URL of such a subject names only the repository's
// list of them, since the API names them by IDs the web doesn't use. A
// subject of another kind, or a check suite with no run, is an error.
func (c *Client) ResolveSubjectURL(ctx context.Context, n core.Notification) (string, error) {
	sub := n.Subject
	switch {
	case sub.ReleaseID > 0:
		var r restRelease
		if _, err := c.Get(ctx, releasePath(n.Repo, sub.ReleaseID), Conditional{}, &r); err != nil {
			return "", fmt.Errorf("resolve release %d of %s: %w", sub.ReleaseID, n.Repo, err)
		}
		if r.HTMLURL == "" {
			return "", fmt.Errorf("resolve release %d of %s: no page", sub.ReleaseID, n.Repo)
		}
		return r.HTMLURL, nil
	case sub.CheckSuiteID > 0:
		q := url.Values{"check_suite_id": {strconv.FormatInt(sub.CheckSuiteID, 10)}, "per_page": {"1"}}
		var body restRuns
		if _, err := c.Get(ctx, actionsPath(n.Repo)+"/runs?"+q.Encode(), Conditional{}, &body); err != nil {
			return "", fmt.Errorf("resolve check suite %d of %s: %w", sub.CheckSuiteID, n.Repo, err)
		}
		if len(body.WorkflowRuns) == 0 || body.WorkflowRuns[0].HTMLURL == "" {
			return "", fmt.Errorf("resolve check suite %d of %s: no run", sub.CheckSuiteID, n.Repo)
		}
		return body.WorkflowRuns[0].HTMLURL, nil
	case sub.Type == core.SubjectCheckSuite:
		return c.runOfTitle(ctx, n)
	default:
		return "", fmt.Errorf("resolve subject of notification %s: not a release or check suite", n.ID)
	}
}

// runTitle is the title GitHub gives a notification of a workflow run, such
// as "CI workflow run failed for main branch".
var runTitle = regexp.MustCompile(`^(.+) workflow run (\w+) for (.+) branch$`)

// runOfTitle finds the run that the title of a check suite notification
// names, which GitHub gives no URL: among the runs of that branch that ended
// as the title says, the workflow's run last updated nearest to the
// notification.
func (c *Client) runOfTitle(ctx context.Context, n core.Notification) (string, error) {
	m := runTitle.FindStringSubmatch(n.Subject.Title)
	if m == nil {
		return "", fmt.Errorf("resolve run of notification %s: title names no run", n.ID)
	}
	q := url.Values{"branch": {m[3]}, "per_page": {strconv.Itoa(actionsMaxPerPage)}}
	switch m[2] {
	case "failed":
		q.Set("status", string(core.ConclusionFailure))
	case "succeeded":
		q.Set("status", string(core.ConclusionSuccess))
	case "cancelled":
		q.Set("status", string(core.ConclusionCancelled))
	}
	var body restRuns
	if _, err := c.Get(ctx, actionsPath(n.Repo)+"/runs?"+q.Encode(), Conditional{}, &body); err != nil {
		return "", fmt.Errorf("resolve run of notification %s in %s: %w", n.ID, n.Repo, err)
	}
	var best *restRun
	for i := range body.WorkflowRuns {
		r := &body.WorkflowRuns[i]
		if r.HTMLURL == "" || !strings.EqualFold(r.Name, m[1]) {
			continue
		}
		if best == nil || absDuration(r.UpdatedAt.Sub(n.UpdatedAt)) < absDuration(best.UpdatedAt.Sub(n.UpdatedAt)) {
			best = r
		}
	}
	if best == nil {
		return "", fmt.Errorf("resolve run of notification %s in %s: no matching run", n.ID, n.Repo)
	}
	return best.HTMLURL, nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// notificationsPath is the path of the first page of the filter; now is
// where its Since window is counted back from.
func notificationsPath(filter core.NotificationFilter, perPage int, now time.Time) string {
	q := url.Values{}
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(perPage))
	}
	if filter.All {
		q.Set("all", "true")
	}
	if filter.Participating {
		q.Set("participating", "true")
	}
	if t := filter.SinceTime(now); !t.IsZero() {
		q.Set("since", t.UTC().Format(time.RFC3339))
	}
	if len(q) == 0 {
		return "notifications"
	}
	return "notifications?" + q.Encode()
}

// MarkThreadRead marks the notification thread id as read.
func (c *Client) MarkThreadRead(ctx context.Context, id string) error {
	if _, err := c.Do(ctx, http.MethodPatch, threadPath(id), nil, nil); err != nil {
		return fmt.Errorf("mark thread %s read: %w", id, err)
	}
	return nil
}

// MarkThreadDone marks the notification thread id as done, which removes it
// from the inbox until there is new activity.
func (c *Client) MarkThreadDone(ctx context.Context, id string) error {
	if _, err := c.Do(ctx, http.MethodDelete, threadPath(id), nil, nil); err != nil {
		return fmt.Errorf("mark thread %s done: %w", id, err)
	}
	return nil
}

func threadPath(id string) string {
	return "notifications/threads/" + url.PathEscape(id)
}

// MarkNotificationsRead marks every thread last updated at or before
// lastReadAt as read. GitHub answers 205 when it is done, or 202 when there
// are too many threads and it marks them in the background; both count as
// success, so threads may briefly still list as unread after a 202.
func (c *Client) MarkNotificationsRead(ctx context.Context, lastReadAt time.Time) error {
	body := struct {
		LastReadAt string `json:"last_read_at"`
		Read       bool   `json:"read"`
	}{lastReadAt.UTC().Format(time.RFC3339), true}
	if _, err := c.Do(ctx, http.MethodPut, "notifications", body, nil); err != nil {
		return fmt.Errorf("mark notifications read: %w", err)
	}
	return nil
}
