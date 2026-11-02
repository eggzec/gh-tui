package github

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

const issueTimelineJSON = `{"data":{"repository":{"issue":{"page":{
  "nodes":[
    {"__typename":"IssueComment","id":"IC_1","author":{"__typename":"User","login":"hubot","avatarUrl":"https://avatars/hubot"},
     "body":"hi","createdAt":"2026-09-20T10:00:00Z","updatedAt":"2026-09-20T10:05:00Z","url":"u","viewerCanUpdate":true,"viewerCanDelete":false},
    {"__typename":"RenamedTitleEvent","actor":{"__typename":"User","login":"mona"},"createdAt":"2026-09-20T11:00:00Z","previousTitle":"a","currentTitle":"b"},
    {"__typename":"SubscribedEvent"}
  ],
  "pageInfo":{"hasNextPage":true,"endCursor":"CUR1"}}}}}}`

func TestListIssueTimeline(t *testing.T) {
	var query string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		query = string(body)
		_, _ = w.Write([]byte(issueTimelineJSON))
	}))
	page, err := c.ListIssueTimeline(t.Context(), pullsRepo, 5, "", 30)
	if err != nil {
		t.Fatalf("ListIssueTimeline: %v", err)
	}
	if !strings.Contains(query, "issue(number: $number)") {
		t.Errorf("query = %q, want an issue read", query)
	}
	if page.Next != "CUR1" || len(page.Items) != 2 {
		t.Fatalf("page = %+v, want 2 items (the unknown one is left out) and next CUR1", page)
	}
	if got := page.Items[0]; got.Kind != core.TimelineComment || got.Body != "hi" || !got.CanUpdate {
		t.Errorf("comment = %+v", got)
	}
	if got := page.Items[1]; got.Kind != core.TimelineRenamed || got.From != "a" || got.To != "b" {
		t.Errorf("rename = %+v", got)
	}
}

func TestListIssueTimelineNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":{"issue":null}}}`))
	}))
	_, err := c.ListIssueTimeline(t.Context(), pullsRepo, 5, "", 30)
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
