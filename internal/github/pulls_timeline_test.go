package github

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// timelineServer answers a request after the cursor "CUR1" with the second
// fixture and any other with the first.
func timelineServer(t *testing.T) (client *Client, requests func() []pullQuery) {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	first, second := read("pulls_timeline_1.json"), read("pulls_timeline_2.json")
	var (
		mu   sync.Mutex
		reqs []pullQuery
	)
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req pullQuery
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		reqs = append(reqs, req)
		mu.Unlock()
		if req.Variables["after"] == "CUR1" {
			_, _ = w.Write(second)
			return
		}
		_, _ = w.Write(first)
	}))
	return c, func() []pullQuery {
		mu.Lock()
		defer mu.Unlock()
		return reqs
	}
}

func TestListPullRequestTimelineFirstPage(t *testing.T) {
	c, reqs := timelineServer(t)
	page, err := c.ListPullRequestTimeline(t.Context(), pullsRepo, 42, "", 100)
	if err != nil {
		t.Fatalf("ListPullRequestTimeline: %v", err)
	}
	if got := reqs(); len(got) != 1 || !strings.Contains(got[0].Query, "timelineItems(first: $first, after: $after, itemTypes: [") {
		t.Fatalf("requests = %+v, want one that reads the timeline items of some types", got)
	}
	if page.Next != "CUR1" {
		t.Errorf("next = %q, want CUR1", page.Next)
	}
	mona := core.User{Login: "monalisa", Name: "Mona Lisa"}
	hubot := core.User{Login: "hubot"}
	want := []core.TimelineItem{
		{
			Kind: core.TimelinePushed, Actor: mona, At: pullTime("2026-09-20T09:00:00Z"), URL: "https://github.com/eggzec/gh-tui/pull/42/commits/aaa111",
			Commit: core.TimelineCommit{OID: "aaa111aaa111", Short: "aaa111a", Headline: "feat: first", Author: mona},
		},
		{
			Kind: core.TimelinePushed, Actor: core.User{Name: "Git Only"}, At: pullTime("2026-09-20T09:30:00Z"), URL: "u",
			Commit: core.TimelineCommit{OID: "bbb222", Short: "bbb222", Headline: "fix: second", Author: core.User{Name: "Git Only"}},
		},
		{
			Kind: core.TimelineComment, ID: "IC_1", Actor: core.User{Login: "hubot", Name: "Hubot"}, AvatarURL: "https://avatars/hubot",
			At: pullTime("2026-09-20T10:00:00Z"), UpdatedAt: pullTime("2026-09-20T10:05:00Z"), Body: "Looks **good**",
			URL: "https://github.com/eggzec/gh-tui/pull/42#issuecomment-1", CanUpdate: true,
		},
		{
			Kind: core.TimelineReview, ID: "PRR_1", Actor: core.User{Login: "copilot", Bot: true}, AvatarURL: "https://avatars/copilot",
			At: pullTime("2026-09-20T11:02:00Z"), Body: "Please fix", Review: core.ReviewStateChangesRequested,
			URL:    "https://github.com/eggzec/gh-tui/pull/42#pullrequestreview-1",
			Commit: core.TimelineCommit{OID: "bbb222", Short: "bbb222"},
		},
		{Kind: core.TimelineForcePushed, Actor: mona, At: pullTime("2026-09-20T12:00:00Z"), From: "bbb222", To: "ccc333"},
		{Kind: core.TimelineLabeled, Actor: hubot, At: pullTime("2026-09-20T12:10:00Z"), Label: core.Label{Name: "bug", Color: "d73a4a", Description: "Something broke"}},
		{Kind: core.TimelineUnlabeled, Actor: hubot, At: pullTime("2026-09-20T12:11:00Z"), Label: core.Label{Name: "wip", Color: "ededed"}},
		{Kind: core.TimelineAssigned, Actor: hubot, At: pullTime("2026-09-20T12:20:00Z"), Subject: core.TimelineSubject{User: mona}},
		{Kind: core.TimelineUnassigned, Actor: hubot, At: pullTime("2026-09-20T12:21:00Z"), Subject: core.TimelineSubject{User: core.User{Login: "dependabot", Bot: true}}},
		{Kind: core.TimelineReviewRequested, Actor: hubot, At: pullTime("2026-09-20T12:30:00Z"), Subject: core.TimelineSubject{User: core.User{Login: "octocat", Name: "The Octocat"}}},
		{Kind: core.TimelineReviewRequestRemoved, Actor: hubot, At: pullTime("2026-09-20T12:31:00Z"), Subject: core.TimelineSubject{Team: "eggzec/core"}},
		{Kind: core.TimelineConvertedToDraft, Actor: core.User{Login: "monalisa"}, At: pullTime("2026-09-20T13:00:00Z")},
		{Kind: core.TimelineReadyForReview, Actor: core.User{Login: "monalisa"}, At: pullTime("2026-09-20T13:05:00Z")},
	}
	if len(page.Items) != len(want) {
		t.Fatalf("got %d items, want %d (a subscription and a pending review are left out): %+v", len(page.Items), len(want), page.Items)
	}
	for i := range want {
		if !reflect.DeepEqual(page.Items[i], want[i]) {
			t.Errorf("item %d =\n%+v\nwant\n%+v", i, page.Items[i], want[i])
		}
	}
}

func TestListPullRequestTimelineSecondPage(t *testing.T) {
	c, reqs := timelineServer(t)
	page, err := c.ListPullRequestTimeline(t.Context(), pullsRepo, 42, "CUR1", 100)
	if err != nil {
		t.Fatalf("ListPullRequestTimeline: %v", err)
	}
	if got := reqs(); len(got) != 1 || got[0].Variables["after"] != "CUR1" {
		t.Fatalf("requests = %+v, want one after CUR1", got)
	}
	if page.Next != "" {
		t.Errorf("next = %q, want none on the last page", page.Next)
	}
	mona := core.User{Login: "monalisa"}
	want := []core.TimelineItem{
		{Kind: core.TimelineRenamed, Actor: mona, At: pullTime("2026-09-21T09:00:00Z"), From: "wip", To: "feat: timeline"},
		{Kind: core.TimelineBaseChanged, Actor: mona, At: pullTime("2026-09-21T09:05:00Z"), From: "main", To: "release"},
		{
			Kind: core.TimelineCrossReferenced, Actor: core.User{Login: "octocat"}, At: pullTime("2026-09-21T09:10:00Z"),
			Source: core.TimelineSource{Number: 50, Title: "Follow up", State: core.StateMerged, URL: "https://github.com/eggzec/other/pull/50", Repo: "eggzec/other", IsPullRequest: true, WillClose: true},
		},
		{
			Kind: core.TimelineCrossReferenced, At: pullTime("2026-09-21T09:11:00Z"),
			Source: core.TimelineSource{Number: 7, Title: "A bug", State: core.StateOpen, URL: "https://github.com/eggzec/gh-tui/issues/7", Repo: "eggzec/gh-tui"},
		},
		{Kind: core.TimelineReviewDismissed, Actor: mona, At: pullTime("2026-09-21T09:20:00Z"), Body: "Addressed"},
		{Kind: core.TimelineLocked, Actor: mona, At: pullTime("2026-09-21T09:21:00Z"), Ref: "too_heated"},
		{Kind: core.TimelineUnlocked, Actor: mona, At: pullTime("2026-09-21T09:22:00Z")},
		{Kind: core.TimelineMilestoned, Actor: mona, At: pullTime("2026-09-21T09:30:00Z"), Ref: "v1"},
		{Kind: core.TimelineDemilestoned, Actor: mona, At: pullTime("2026-09-21T09:31:00Z"), Ref: "v0"},
		{Kind: core.TimelineClosed, Actor: mona, At: pullTime("2026-09-21T10:00:00Z")},
		{Kind: core.TimelineReopened, Actor: mona, At: pullTime("2026-09-21T10:05:00Z")},
		{
			Kind: core.TimelineMerged, Actor: mona, At: pullTime("2026-09-21T11:00:00Z"), Ref: "main",
			Commit: core.TimelineCommit{OID: "ddd444ddd444", Short: "ddd444d"},
		},
		{Kind: core.TimelineHeadDeleted, Actor: mona, At: pullTime("2026-09-21T11:01:00Z"), Ref: "feat/timeline"},
	}
	if len(page.Items) != len(want) {
		t.Fatalf("got %d items, want %d: %+v", len(page.Items), len(want), page.Items)
	}
	for i := range want {
		if !reflect.DeepEqual(page.Items[i], want[i]) {
			t.Errorf("item %d =\n%+v\nwant\n%+v", i, page.Items[i], want[i])
		}
	}
}

func TestListPullRequestTimelineMissingPull(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":null}}}`))
	}))
	if _, err := c.ListPullRequestTimeline(t.Context(), pullsRepo, 42, "", 100); err == nil {
		t.Fatal("ListPullRequestTimeline of a missing pull request = nil error")
	}
}
