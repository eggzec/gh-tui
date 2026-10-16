package github

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// commitChecksNode is a commit of the answer, with the rollup state (empty
// for none), written as GitHub writes them.
func commitChecksNode(oid, state string) string {
	rollup := "null"
	if state != "" {
		rollup = fmt.Sprintf(`{"state":%q}`, state)
	}
	return fmt.Sprintf(`{"commit":{"oid":%q,"messageHeadline":"commit %[1]s","committedDate":"2026-03-01T12:00:00Z",
	  "author":{"name":"Ada","user":{"login":"ada"}},"statusCheckRollup":%s}}`, oid, rollup)
}

// commitChecksAnswer is the answer to the read of a page of commits.
func commitChecksAnswer(total int, next, review string, nodes ...string) string {
	pageInfo := `{"hasNextPage":false,"endCursor":"end"}`
	if next != "" {
		pageInfo = fmt.Sprintf(`{"hasNextPage":true,"endCursor":%q}`, next)
	}
	if review == "" {
		review = `{"nodes":[]}`
	}
	return fmt.Sprintf(`{"data":{"repository":{"pullRequest":{"commits":{"totalCount":%d,"pageInfo":%s,"nodes":[%s]},"reviews":%s}}}}`,
		total, pageInfo, strings.Join(nodes, ","), review)
}

func TestListPullCommits(t *testing.T) {
	review := `{"nodes":[{"commit":{"oid":"c1"},"submittedAt":"2026-03-02T08:00:00Z"}]}`
	c, reqs := sequenceServer(t, commitChecksAnswer(3, "", review,
		commitChecksNode("c1", "SUCCESS"), commitChecksNode("c2", "FAILURE"), commitChecksNode("c3", "")))
	got, err := c.ListPullCommits(t.Context(), pendingRepo, 42, "me")
	if err != nil {
		t.Fatalf("ListPullCommits: %v", err)
	}
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	commit := func(oid string, checks core.CheckState) core.PullCommit {
		return core.PullCommit{OID: oid, Headline: "commit " + oid, CommittedAt: at, Author: "Ada", Login: "ada", Checks: checks}
	}
	want := core.PullCommits{
		Commits:    []core.PullCommit{commit("c1", core.CheckPassing), commit("c2", core.CheckFailing), commit("c3", core.CheckNone)},
		LastReview: &core.ViewerReview{Commit: "c1", SubmittedAt: time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("commits =\n%+v\nwant\n%+v", got, want)
	}
	checkPullQuery(t, reqs(), "reviews(author: $viewer, last: 1, states: [APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED])", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "number": float64(42), "first": float64(100), "after": nil,
		"withReview": true, "viewer": "me",
	})
}

func TestListPullCommitsRollupStates(t *testing.T) {
	c, _ := sequenceServer(t, commitChecksAnswer(5, "", "",
		commitChecksNode("a", "PENDING"), commitChecksNode("b", "EXPECTED"), commitChecksNode("c", "ERROR"),
		commitChecksNode("d", "SUCCESS"), commitChecksNode("e", "SOMETHING_NEW")))
	got, err := c.ListPullCommits(t.Context(), pendingRepo, 1, "me")
	if err != nil {
		t.Fatal(err)
	}
	states := make([]core.CheckState, 0, len(got.Commits))
	for _, cm := range got.Commits {
		states = append(states, cm.Checks)
	}
	want := []core.CheckState{core.CheckPending, core.CheckPending, core.CheckFailing, core.CheckPassing, core.CheckPending}
	if !reflect.DeepEqual(states, want) {
		t.Errorf("states = %v, want %v", states, want)
	}
}

func TestListPullCommitsNoReview(t *testing.T) {
	c, _ := sequenceServer(t, commitChecksAnswer(1, "", "", commitChecksNode("c1", "SUCCESS")))
	got, err := c.ListPullCommits(t.Context(), pendingRepo, 1, "me")
	if err != nil || got.LastReview != nil || got.Capped || len(got.Commits) != 1 {
		t.Errorf("ListPullCommits = %+v, %v; want one commit, no review, not capped", got, err)
	}
}

func TestListPullCommitsReviewOfAGoneCommit(t *testing.T) {
	review := `{"nodes":[{"commit":null,"submittedAt":"2026-03-02T08:00:00Z"}]}`
	c, _ := sequenceServer(t, commitChecksAnswer(1, "", review, commitChecksNode("c1", "")))
	got, err := c.ListPullCommits(t.Context(), pendingRepo, 1, "me")
	if err != nil || got.LastReview == nil || got.LastReview.Commit != "" || got.LastReview.SubmittedAt.IsZero() {
		t.Errorf("review = %+v, %v; want one with no commit and a time", got.LastReview, err)
	}
}

func TestListPullCommitsPages(t *testing.T) {
	review := `{"nodes":[{"commit":{"oid":"c1"},"submittedAt":"2026-03-02T08:00:00Z"}]}`
	c, reqs := sequenceServer(t,
		commitChecksAnswer(3, "cursor1", review, commitChecksNode("c1", "SUCCESS"), commitChecksNode("c2", "SUCCESS")),
		commitChecksAnswer(3, "", `{"nodes":[]}`, commitChecksNode("c3", "PENDING")),
	)
	got, err := c.ListPullCommits(t.Context(), pendingRepo, 1, "me")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Commits) != 3 || got.Commits[2].OID != "c3" || got.Capped {
		t.Errorf("commits = %+v, want c1 to c3, not capped", got)
	}
	if got.LastReview == nil || got.LastReview.Commit != "c1" {
		t.Errorf("review = %+v, want the one of the first page", got.LastReview)
	}
	rs := reqs()
	if len(rs) != 2 {
		t.Fatalf("got %d requests, want 2", len(rs))
	}
	if rs[1].Variables["after"] != "cursor1" || rs[1].Variables["withReview"] != false {
		t.Errorf("second request variables = %#v, want the cursor and no review", rs[1].Variables)
	}
}

func TestListPullCommitsCap(t *testing.T) {
	page := func(from, n int) []string {
		nodes := make([]string, 0, n)
		for i := range n {
			nodes = append(nodes, commitChecksNode(fmt.Sprintf("c%03d", from+i), ""))
		}
		return nodes
	}
	tests := []struct {
		name      string
		total     int
		answers   []string
		wantLen   int
		wantCap   bool
		wantQuery int
	}{
		{
			"more than GitHub lists",
			400,
			[]string{
				commitChecksAnswer(400, "p1", "", page(0, 100)...),
				commitChecksAnswer(400, "p2", "", page(100, 100)...),
				commitChecksAnswer(400, "p3", "", page(200, 100)...),
			},
			core.MaxPullCommits, true, 3,
		},
		{
			"exactly as many as GitHub lists",
			250,
			[]string{
				commitChecksAnswer(250, "p1", "", page(0, 100)...),
				commitChecksAnswer(250, "p2", "", page(100, 100)...),
				commitChecksAnswer(250, "", "", page(200, 50)...),
			},
			250, false, 3,
		},
		{
			"fewer returned than counted",
			150,
			[]string{commitChecksAnswer(150, "", "", page(0, 100)...)},
			100, true, 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := sequenceServer(t, tt.answers...)
			got, err := c.ListPullCommits(t.Context(), pendingRepo, 1, "me")
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Commits) != tt.wantLen || got.Capped != tt.wantCap {
				t.Errorf("got %d commits, capped %v; want %d, %v", len(got.Commits), got.Capped, tt.wantLen, tt.wantCap)
			}
			if n := len(reqs()); n != tt.wantQuery {
				t.Errorf("got %d requests, want %d", n, tt.wantQuery)
			}
		})
	}
}

func TestListPullCommitsWithoutViewer(t *testing.T) {
	c, reqs := sequenceServer(t, commitChecksAnswer(1, "", "", commitChecksNode("c1", "")))
	if _, err := c.ListPullCommits(t.Context(), pendingRepo, 1, ""); err != nil {
		t.Fatal(err)
	}
	if v := reqs()[0].Variables["withReview"]; v != false {
		t.Errorf("withReview = %v, want false: there is no viewer to read a review of", v)
	}
}

func TestListPullCommitsStopsOnARepeatedCursor(t *testing.T) {
	c, reqs := sequenceServer(t, commitChecksAnswer(300, "same", "", commitChecksNode("c1", "")))
	got, err := c.ListPullCommits(t.Context(), pendingRepo, 1, "me")
	if err != nil || len(got.Commits) != 2 {
		t.Fatalf("ListPullCommits = %+v, %v; want two commits", got, err)
	}
	if n := len(reqs()); n != 2 {
		t.Errorf("got %d requests, want 2", n)
	}
}

func TestListPullCommitsNotFound(t *testing.T) {
	c, _ := sequenceServer(t, `{"data":{"repository":{"pullRequest":null}}}`)
	if _, err := c.ListPullCommits(t.Context(), pendingRepo, 1, "me"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListPullCommitsPageFailure(t *testing.T) {
	c, _ := sequenceServer(t, commitChecksAnswer(3, "cursor1", "", commitChecksNode("c1", "")), reviewRefused)
	if got, err := c.ListPullCommits(t.Context(), pendingRepo, 1, "me"); err == nil || len(got.Commits) != 0 {
		t.Errorf("ListPullCommits = %+v, %v; want the failure and nothing else", got, err)
	}
}
