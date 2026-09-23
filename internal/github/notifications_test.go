package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

const lastModified = "Tue, 22 Sep 2026 14:03:11 GMT"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestListNotifications(t *testing.T) {
	tests := []struct {
		name      string
		filter    core.NotificationFilter
		wantQuery string
	}{
		{"unread", core.NotificationFilter{}, ""},
		{"all", core.NotificationFilter{All: true}, "all=true"},
		{"participating", core.NotificationFilter{Participating: true}, "participating=true"},
		{"all participating", core.NotificationFilter{All: true, Participating: true}, "all=true&participating=true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fixture(t, "notifications_list.json")
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/notifications" || r.URL.RawQuery != tt.wantQuery {
					t.Errorf("request = %s %s?%s, want GET /notifications?%s", r.Method, r.URL.Path, r.URL.RawQuery, tt.wantQuery)
				}
				if h := r.Header.Get("If-Modified-Since"); h != "" {
					t.Errorf("If-Modified-Since = %q on an unconditional request", h)
				}
				w.Header().Set("Last-Modified", lastModified)
				w.Header().Set("X-Poll-Interval", "60")
				w.Header().Set("Link", `<http://`+r.Host+`/notifications?page=2>; rel="next", <http://`+r.Host+`/notifications?page=3>; rel="last"`)
				_, _ = w.Write(body)
			}))

			page, res, err := c.ListNotifications(t.Context(), tt.filter, "", Conditional{})
			if err != nil {
				t.Fatalf("ListNotifications: %v", err)
			}
			if res.LastModified != lastModified || res.PollInterval != time.Minute {
				t.Errorf("response = %+v, want Last-Modified %q and a 60s poll interval", res, lastModified)
			}
			if page.Next == "" || page.Next != res.Next {
				t.Errorf("Next = %q, want the next link %q", page.Next, res.Next)
			}
			if !slices.Equal(page.Items, wantNotifications) {
				t.Errorf("items =\n%+v\nwant\n%+v", page.Items, wantNotifications)
			}
		})
	}
}

var wantNotifications = []core.Notification{
	{
		ID:   "14230157283",
		Repo: core.RepoRef{Owner: "eggzec", Name: "gh-tui"},
		Subject: core.Subject{
			Title: "cache: add MutateTag for optimistic updates",
			Type:  core.SubjectPullRequest,
			URL:   "https://api.github.com/repos/eggzec/gh-tui/pulls/42",
		},
		Reason:    "review_requested",
		Unread:    true,
		UpdatedAt: time.Date(2026, 9, 22, 14, 3, 11, 0, time.UTC),
	},
	{
		ID:   "14229874410",
		Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"},
		Subject: core.Subject{
			Title: "Keys arrive as KeyPressMsg in v2",
			Type:  core.SubjectIssue,
			URL:   "https://api.github.com/repos/charmbracelet/bubbletea/issues/1402",
		},
		Reason:    "mention",
		UpdatedAt: time.Date(2026, 9, 21, 9, 47, 2, 0, time.UTC),
	},
	{
		ID:        "14229011975",
		Repo:      core.RepoRef{Owner: "charmbracelet", Name: "huh"},
		Subject:   core.Subject{Title: "How do you theme huh forms?", Type: core.SubjectDiscussion},
		Reason:    "subscribed",
		Unread:    true,
		UpdatedAt: time.Date(2026, 9, 20, 18, 30, 55, 0, time.UTC),
	},
	{
		ID:        "14228450321",
		Repo:      core.RepoRef{Owner: "eggzec", Name: "gh-tui"},
		Subject:   core.Subject{Title: "CI workflow run failed for main branch", Type: "CheckSuite"},
		Reason:    "ci_activity",
		Unread:    true,
		UpdatedAt: time.Date(2026, 9, 20, 7, 15, 20, 0, time.UTC),
	},
}

func TestListNotificationsCursor(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/notifications" || r.URL.RawQuery != "all=true&page=2" {
			t.Errorf("request = %s?%s, want /notifications?all=true&page=2", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	cursor, err := c.resolve("notifications?all=true&page=2")
	if err != nil {
		t.Fatal(err)
	}

	// The filter is ignored: the cursor already carries it.
	page, _, err := c.ListNotifications(t.Context(), core.NotificationFilter{}, cursor, Conditional{})
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(page.Items) != 0 || !page.Last() {
		t.Errorf("page = %+v, want an empty last page", page)
	}
}

func TestListNotificationsNotModified(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("If-Modified-Since"); got != lastModified {
			t.Errorf("If-Modified-Since = %q, want %q", got, lastModified)
		}
		w.Header().Set("X-Poll-Interval", "120")
		w.WriteHeader(http.StatusNotModified)
	}))

	page, res, err := c.ListNotifications(t.Context(), core.NotificationFilter{}, "", Conditional{LastModified: lastModified})
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if !res.NotModified || res.PollInterval != 2*time.Minute {
		t.Errorf("response = %+v, want NotModified with a 120s poll interval", res)
	}
	if page.Items != nil || page.Next != "" {
		t.Errorf("page = %+v, want the zero page", page)
	}
}

func TestListNotificationsErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header http.Header
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, nil, core.ErrUnauthorized},
		{"rate limited", http.StatusForbidden, http.Header{
			"X-Ratelimit-Limit":     {"5000"},
			"X-Ratelimit-Remaining": {"0"},
			"X-Ratelimit-Reset":     {"1790000000"},
		}, core.ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				maps.Copy(w.Header(), tt.header)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"message":"no"}`)
			}))
			_, _, err := c.ListNotifications(t.Context(), core.NotificationFilter{}, "", Conditional{})
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestMarkThread(t *testing.T) {
	tests := []struct {
		name       string
		mark       func(c *Client, ctx context.Context, id string) error
		method     string
		status     int
		wantErr    error
		wantFailed bool
	}{
		{"read", (*Client).MarkThreadRead, http.MethodPatch, http.StatusResetContent, nil, false},
		{"read already", (*Client).MarkThreadRead, http.MethodPatch, http.StatusNotModified, nil, false},
		{"read forbidden", (*Client).MarkThreadRead, http.MethodPatch, http.StatusForbidden, nil, true},
		{"done", (*Client).MarkThreadDone, http.MethodDelete, http.StatusNoContent, nil, false},
		{"done missing", (*Client).MarkThreadDone, http.MethodDelete, http.StatusNotFound, core.ErrNotFound, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != "/notifications/threads/14230157283" {
					t.Errorf("request = %s %s, want %s /notifications/threads/14230157283", r.Method, r.URL.Path, tt.method)
				}
				w.WriteHeader(tt.status)
			}))

			err := tt.mark(c, t.Context(), "14230157283")
			if (err != nil) != tt.wantFailed {
				t.Fatalf("error = %v, want failure %v", err, tt.wantFailed)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestMarkNotificationsRead(t *testing.T) {
	accepted := fixture(t, "notifications_mark_all_accepted.json")
	tests := []struct {
		name    string
		status  int
		body    []byte
		wantErr error
	}{
		{"done", http.StatusResetContent, nil, nil},
		{"in the background", http.StatusAccepted, accepted, nil},
		{"unauthorized", http.StatusUnauthorized, []byte(`{"message":"Requires authentication"}`), core.ErrUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/notifications" {
					t.Errorf("request = %s %s, want PUT /notifications", r.Method, r.URL.Path)
				}
				var got map[string]any
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
				}
				want := map[string]any{"last_read_at": "2026-09-23T10:00:00Z", "read": true}
				if !maps.Equal(got, want) {
					t.Errorf("body = %v, want %v", got, want)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write(tt.body)
			}))

			// A non-UTC time must still be sent in UTC.
			at := time.Date(2026, 9, 23, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
			err := c.MarkNotificationsRead(t.Context(), at)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("MarkNotificationsRead: %v", err)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
