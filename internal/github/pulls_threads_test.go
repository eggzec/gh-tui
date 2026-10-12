package github

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestListPullRequestThreads(t *testing.T) {
	c, reqs := pullServer(t, "pulls_threads.json")
	page, err := c.ListPullRequestThreads(t.Context(), pullsRepo, 42, "", 50)
	if err != nil {
		t.Fatalf("ListPullRequestThreads: %v", err)
	}
	checkPullQuery(t, reqs(), "reviewThreads(first: $first, after: $after)", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "number": float64(42), "first": float64(50),
	})
	if q := reqs()[0].Query; !strings.Contains(q, "comments(first: 100)") {
		t.Errorf("query does not read 100 comments of each thread:\n%s", q)
	}
	if page.Total != 7 || page.Next != "Y3Vyc29yOnYyOpHOAAAF" || len(page.Threads) != 6 {
		t.Fatalf("page = total %d, next %q, %d threads; want 7, the cursor of the fixture, 6", page.Total, page.Next, len(page.Threads))
	}
	hubot := core.User{Login: "hubot", Name: "Hubot"}
	mona := core.User{Login: "monalisa", Name: "Mona Lisa"}

	t.Run("line", func(t *testing.T) {
		want := core.Thread{
			ID: "PRRT_kwDOLnBTf85Aa001", Path: "internal/tui/model.go",
			Line: 42, OriginalLine: 40,
			Side: core.SideRight, StartSide: core.SideRight,
			ViewerCanResolve: true,
			Comments: []core.ThreadComment{
				{
					ID: "PRRC_kwDOLnBTf86Ab001", Author: hubot, At: pullTime("2026-09-21T10:00:00Z"),
					Body: "Should this be a **pointer**?\n\n```go\nm := &Model{}\n```",
					URL:  "https://github.com/eggzec/gh-tui/pull/42#discussion_r1001",
				},
				{
					ID: "PRRC_kwDOLnBTf86Ab002", Author: mona, At: pullTime("2026-09-21T11:30:00Z"),
					Body: "No, a value keeps it copyable.", URL: "https://github.com/eggzec/gh-tui/pull/42#discussion_r1002",
					ReplyTo: "PRRC_kwDOLnBTf86Ab001", EditedAt: pullTime("2026-09-21T11:45:00Z"),
				},
			},
			TotalComments: 2,
		}
		if got := page.Threads[0]; !reflect.DeepEqual(got, want) {
			t.Errorf("thread =\n%+v\nwant\n%+v", got, want)
		}
	})

	t.Run("several lines", func(t *testing.T) {
		got := page.Threads[1]
		if got.StartLine != 10 || got.Line != 14 || got.OriginalStartLine != 10 || got.OriginalLine != 14 || got.StartSide != core.SideRight {
			t.Errorf("lines = %d-%d (originally %d-%d), start side %q; want 10-14 both ways, right", got.StartLine, got.Line, got.OriginalStartLine, got.OriginalLine, got.StartSide)
		}
		if !got.Comments[0].Author.Bot {
			t.Errorf("author = %+v, want an app", got.Comments[0].Author)
		}
	})

	t.Run("file", func(t *testing.T) {
		got := page.Threads[2]
		if !got.File || got.Line != 0 || got.OriginalLine != 0 || got.ViewerCanResolve {
			t.Errorf("thread = file %v, line %d, original %d, can resolve %v; want a file thread without lines that the viewer can't resolve", got.File, got.Line, got.OriginalLine, got.ViewerCanResolve)
		}
		if got.Comments[0].Author != (core.User{}) {
			t.Errorf("author = %+v, want none for a deleted account", got.Comments[0].Author)
		}
		if page.Threads[0].File {
			t.Error("a thread on a line reads as on its file")
		}
	})

	t.Run("outdated", func(t *testing.T) {
		got := page.Threads[3]
		if !got.Outdated || !got.Resolved || got.Line != 0 || got.OriginalLine != 7 || got.ResolvedBy != mona || got.ViewerCanResolve || !got.ViewerCanUnresolve {
			t.Errorf("thread = outdated %v, resolved %v, line %d, original %d, by %+v, can resolve %v, can unresolve %v; want an outdated resolved one made on line 7 and resolved by monalisa, that the viewer can only unresolve",
				got.Outdated, got.Resolved, got.Line, got.OriginalLine, got.ResolvedBy, got.ViewerCanResolve, got.ViewerCanUnresolve)
		}
	})

	t.Run("left side", func(t *testing.T) {
		got := page.Threads[4]
		if got.Side != core.SideLeft || got.StartSide != core.SideLeft || got.Line != 88 {
			t.Errorf("thread = side %q, start side %q, line %d; want left, left, 88", got.Side, got.StartSide, got.Line)
		}
	})

	t.Run("more comments than read", func(t *testing.T) {
		got := page.Threads[5]
		if !got.MoreComments || got.TotalComments != 130 || len(got.Comments) != 1 {
			t.Errorf("thread = more %v, total %d, read %d; want more, 130, 1", got.MoreComments, got.TotalComments, len(got.Comments))
		}
		if page.Threads[0].MoreComments {
			t.Error("a thread whose comments were all read is marked as having more")
		}
	})
}

func TestListPullRequestThreadsSecondPage(t *testing.T) {
	c, reqs := pullServer(t, "pulls_threads_2.json")
	page, err := c.ListPullRequestThreads(t.Context(), pullsRepo, 42, "Y3Vyc29yOnYyOpHOAAAF", 50)
	if err != nil {
		t.Fatalf("ListPullRequestThreads: %v", err)
	}
	checkPullQuery(t, reqs(), "reviewThreads(first: $first, after: $after)", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "number": float64(42), "first": float64(50), "after": "Y3Vyc29yOnYyOpHOAAAF",
	})
	if !page.Last() || page.Total != 7 || len(page.Threads) != 1 || page.Threads[0].Path != "go.mod" {
		t.Errorf("page = %+v, want the last page, of 7 threads in all, holding the one of go.mod", page)
	}
}

func TestListPullRequestThreadsSize(t *testing.T) {
	for _, tt := range []struct{ asked, sent int }{{50, 50}, {0, 1}, {500, 100}} {
		c, reqs := pullServer(t, "pulls_threads.json")
		if _, err := c.ListPullRequestThreads(t.Context(), pullsRepo, 42, "", tt.asked); err != nil {
			t.Fatalf("ListPullRequestThreads: %v", err)
		}
		if got := reqs()[0].Variables["first"]; got != float64(tt.sent) {
			t.Errorf("asked %d, sent %v, want %d", tt.asked, got, tt.sent)
		}
	}
}
