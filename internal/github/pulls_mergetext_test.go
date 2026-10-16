package github

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPullMergeTexts(t *testing.T) {
	c, requests := pullServer(t, "graphql_pull_merge_text.json")
	got, err := c.PullMergeTexts(t.Context(), pullsRepo, 142)
	if err != nil {
		t.Fatalf("PullMergeTexts: %v", err)
	}
	want := core.MergeTexts{
		Merge: core.MergeMessage{
			Title: "Merge pull request #142 from eggzec/merge-dialog", Body: "Add the merge dialog", SendBody: true,
		},
		Squash: core.MergeMessage{
			Title:    "Add the merge dialog (#142)",
			Body:     "* read GitHub's default message\n* send it with the merge\n\nCo-authored-by: Ada <ada@example.com>",
			SendBody: true,
		},
	}
	if got != want {
		t.Errorf("PullMergeTexts = %+v, want %+v", got, want)
	}
	reqs := requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	vars := map[string]any{"owner": "eggzec", "name": "gh-tui", "number": float64(142)}
	if !reflect.DeepEqual(reqs[0].Variables, vars) {
		t.Errorf("variables = %v, want %v", reqs[0].Variables, vars)
	}
	for _, field := range []string{"viewerMergeHeadlineText(mergeType: MERGE)", "viewerMergeBodyText(mergeType: SQUASH)"} {
		if !strings.Contains(reqs[0].Query, field) {
			t.Errorf("query lacks %s", field)
		}
	}
}

func TestPullMergeTextsNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(pullNotFound))
	}))
	if _, err := c.PullMergeTexts(t.Context(), pullsRepo, 142); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}
