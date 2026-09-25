package github

import (
	"net/http"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestListLabels(t *testing.T) {
	c := serveIssueFixture(t, "labels_list.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/octo-org/hello/labels", map[string]string{"per_page": "100"})
		if got := r.Header.Get("If-None-Match"); got != `W/"e0"` {
			t.Errorf("If-None-Match = %q, want the ETag given", got)
		}
	})
	labels, res, err := c.ListLabels(t.Context(), issueRepo, Conditional{ETag: `W/"e0"`})
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 14 || labels[0] != (core.Label{Name: "bug", Color: "d73a4a", Description: "Something isn't working"}) {
		t.Errorf("labels = %d, first %+v; want 14, bug first", len(labels), labels[0])
	}
	if res.ETag != `W/"e1"` {
		t.Errorf("ETag = %q, want the response's", res.ETag)
	}
}

func TestListMilestones(t *testing.T) {
	c := serveIssueFixture(t, "milestones_list.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/octo-org/hello/milestones",
			map[string]string{"state": "open", "sort": "due_on", "direction": "asc", "per_page": "100"})
	})
	ms, _, err := c.ListMilestones(t.Context(), issueRepo, Conditional{})
	if err != nil {
		t.Fatalf("ListMilestones: %v", err)
	}
	want := []core.Milestone{{Number: 2, Title: "v2.0.0", State: core.StateOpen}, {Number: 3, Title: "v2.1.0", State: core.StateOpen}}
	if len(ms) != len(want) || ms[0] != want[0] || ms[1] != want[1] {
		t.Errorf("milestones = %+v, want %+v", ms, want)
	}
}

func TestMilestoneDueOn(t *testing.T) {
	due := time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC)
	if got := (restMilestone{Number: 1, Title: "v1", State: "open", DueOn: &due}).core(); !got.DueOn.Equal(due) {
		t.Errorf("DueOn = %v, want %v", got.DueOn, due)
	}
}
