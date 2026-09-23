package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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
	return core.Notification{
		ID:   n.ID,
		Repo: core.RepoRef{Owner: n.Repository.Owner.Login, Name: n.Repository.Name},
		Subject: core.Subject{
			Title:  n.Subject.Title,
			Type:   core.SubjectType(n.Subject.Type),
			URL:    n.Subject.URL,
			WebURL: subjectWebURL(n.Repository.HTMLURL, n.Subject.URL, core.SubjectType(n.Subject.Type)),
		},
		Reason:    n.Reason,
		Unread:    n.Unread,
		UpdatedAt: n.UpdatedAt,
	}
}

// subjectWebURL turns a subject's API URL, such as
// https://api.github.com/repos/o/r/pulls/42, into its page under the
// repository's web URL. Building on the repository's URL keeps the right
// host on GitHub Enterprise Server.
func subjectWebURL(repoURL, apiURL string, typ core.SubjectType) string {
	// After "/repos/": owner, name, kind and ID.
	if _, rest, ok := strings.Cut(apiURL, "/repos/"); ok {
		if parts := strings.SplitN(rest, "/", 4); len(parts) == 4 {
			switch parts[2] {
			case "pulls":
				return repoURL + "/pull/" + parts[3]
			case "issues":
				return repoURL + "/issues/" + parts[3]
			case "commits":
				return repoURL + "/commit/" + parts[3]
			case "releases":
				// The API names releases by ID, which the web doesn't use.
				return repoURL + "/releases"
			}
		}
	}
	if typ == core.SubjectDiscussion {
		return repoURL + "/discussions"
	}
	return repoURL
}

// ListNotifications returns a page of the user's notification threads,
// newest first. Cursor is the Next of the previous page, or empty for the
// first page; it already carries the filter. If cond is current, the
// Response has NotModified set and the page is empty.
func (c *Client) ListNotifications(ctx context.Context, filter core.NotificationFilter, cursor string, cond Conditional) (core.Page[core.Notification], Response, error) {
	path := cursor
	if path == "" {
		path = notificationsPath(filter)
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

func notificationsPath(filter core.NotificationFilter) string {
	q := url.Values{}
	if filter.All {
		q.Set("all", "true")
	}
	if filter.Participating {
		q.Set("participating", "true")
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
