package github

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// viewedAnswer is the answer to the read of a page of files, each with the
// state "VIEWED" except where state is empty, which leaves the field out as an
// older server does.
func viewedAnswer(total int, next, state string, paths ...string) string {
	pageInfo := `{"hasNextPage":false,"endCursor":"end"}`
	if next != "" {
		pageInfo = fmt.Sprintf(`{"hasNextPage":true,"endCursor":%q}`, next)
	}
	nodes := make([]string, len(paths))
	for i, p := range paths {
		nodes[i] = fmt.Sprintf(`{"path":%q}`, p)
		if state != "" {
			nodes[i] = fmt.Sprintf(`{"path":%q,"viewerViewedState":%q}`, p, state)
		}
	}
	return fmt.Sprintf(`{"data":{"repository":{"pullRequest":{"files":{"totalCount":%d,"pageInfo":%s,"nodes":[%s]}}}}}`,
		total, pageInfo, strings.Join(nodes, ","))
}

// filePaths returns n paths from first on, as "f<i>.go".
func filePaths(first, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("f%d.go", first+i)
	}
	return out
}

func TestListPullViewedStates(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "graphql_pull_viewed_states.json"))
	if err != nil {
		t.Fatal(err)
	}
	c, reqs := sequenceServer(t, string(b))
	got, err := c.ListPullViewed(t.Context(), pendingRepo, 42)
	if err != nil {
		t.Fatalf("ListPullViewed: %v", err)
	}
	want := core.PullViewed{
		States:    map[string]core.ViewedState{"a.go": core.Viewed, "dir/c.go": core.ViewedChanged},
		Supported: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("viewed = %+v, want %+v", got, want)
	}
	for path, st := range map[string]core.ViewedState{"a.go": core.Viewed, "b.go": core.Unviewed, "dir/c.go": core.ViewedChanged, "d.go": core.Unviewed} {
		if s := got.State(path); s != st {
			t.Errorf("State(%q) = %v, want %v", path, s, st)
		}
	}
	checkPullQuery(t, reqs(), "nodes { path viewerViewedState }", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "number": float64(42), "first": float64(100), "after": nil,
	})
}

func TestListPullViewedPages(t *testing.T) {
	c, reqs := sequenceServer(t,
		viewedAnswer(150, "cursor1", "VIEWED", filePaths(0, 100)...),
		viewedAnswer(150, "", "VIEWED", filePaths(100, 50)...),
	)
	got, err := c.ListPullViewed(t.Context(), pendingRepo, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.States) != 150 || got.State("f149.go") != core.Viewed || got.Capped || !got.Supported {
		t.Errorf("viewed has %d marks, f149.go %v, capped %v, supported %v; want all 150 of both pages",
			len(got.States), got.State("f149.go"), got.Capped, got.Supported)
	}
	rs := reqs()
	if len(rs) != 2 || rs[1].Variables["after"] != "cursor1" {
		t.Errorf("requests = %+v, want two, the second after cursor1", rs)
	}
}

func TestListPullViewedCapped(t *testing.T) {
	answers := make([]string, 0, core.MaxPullFiles/pullViewedPerPage+1)
	for p := range core.MaxPullFiles/pullViewedPerPage + 1 {
		answers = append(answers, viewedAnswer(core.MaxPullFiles+50, fmt.Sprintf("cursor%d", p), "VIEWED", filePaths(p*pullViewedPerPage, pullViewedPerPage)...))
	}
	c, reqs := sequenceServer(t, answers...)
	got, err := c.ListPullViewed(t.Context(), pendingRepo, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.States) != core.MaxPullFiles || !got.Capped {
		t.Errorf("viewed has %d marks, capped %v; want %d and capped", len(got.States), got.Capped, core.MaxPullFiles)
	}
	if n := len(reqs()); n != core.MaxPullFiles/pullViewedPerPage {
		t.Errorf("got %d requests, want %d", n, core.MaxPullFiles/pullViewedPerPage)
	}
}

func TestListPullViewedRepeatedCursor(t *testing.T) {
	c, reqs := sequenceServer(t,
		viewedAnswer(300, "same", "VIEWED", filePaths(0, 100)...),
		viewedAnswer(300, "same", "VIEWED", filePaths(100, 100)...),
	)
	got, err := c.ListPullViewed(t.Context(), pendingRepo, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(reqs()); n != 2 {
		t.Errorf("got %d requests, want 2: the repeated cursor must stop the read", n)
	}
	if !got.Capped {
		t.Errorf("viewed = %+v, want it capped: the files after the loop are missing", got)
	}
}

func TestListPullViewedNoPull(t *testing.T) {
	c, _ := sequenceServer(t, `{"data":{"repository":{"pullRequest":null}}}`)
	if _, err := c.ListPullViewed(t.Context(), pendingRepo, 1); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("err = %v, want core.ErrNotFound", err)
	}
	c, _ = sequenceServer(t, `{"data":{"repository":null}}`)
	if _, err := c.ListPullViewed(t.Context(), pendingRepo, 1); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("err = %v, want core.ErrNotFound for no repository", err)
	}
}

func TestListPullViewedUnsupported(t *testing.T) {
	srv := &enterpriseServer{
		t:     t,
		data:  []byte(viewedAnswer(2, "", "", "a.go", "b.go")),
		lacks: map[string]string{"viewerViewedState": "PullRequestChangedFile"},
	}
	c := newEnterpriseClient(t, srv)
	got, err := c.ListPullViewed(t.Context(), pendingRepo, 1)
	if err != nil {
		t.Fatalf("ListPullViewed: %v", err)
	}
	if got.Supported || len(got.States) != 0 {
		t.Errorf("viewed = %+v, want no marks and not supported", got)
	}
	sent := srv.sent()
	if len(sent) != 2 {
		t.Fatalf("sent %d queries, want the first and it again without the field:\n%q", len(sent), sent)
	}
	if !strings.Contains(sent[0], "viewerViewedState") || strings.Contains(sent[1], "viewerViewedState") {
		t.Errorf("the field was not struck from the second query:\n%q", sent)
	}
	if !strings.Contains(sent[1], "path") {
		t.Errorf("the second query lost the path of the files:\n%s", sent[1])
	}
}

func TestListPullViewedNoFiles(t *testing.T) {
	c, _ := sequenceServer(t, viewedAnswer(0, "", ""))
	got, err := c.ListPullViewed(t.Context(), pendingRepo, 1)
	if err != nil || !got.Supported || got.Capped {
		t.Errorf("ListPullViewed = %+v, %v; want supported, not capped", got, err)
	}
}

func TestSetFileViewed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		viewed bool
		op     string
	}{
		{"mark", true, "markFileAsViewed(input: {pullRequestId: $id, path: $path})"},
		{"unmark", false, "unmarkFileAsViewed(input: {pullRequestId: $id, path: $path})"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, reqs := sequenceServer(t, `{"data":{"result":{"pullRequest":{"id":"PR_1"}}}}`)
			if err := c.SetFileViewed(t.Context(), "PR_1", "dir/a.go", tc.viewed); err != nil {
				t.Fatalf("SetFileViewed: %v", err)
			}
			checkPullQuery(t, reqs(), tc.op, map[string]any{"id": "PR_1", "path": "dir/a.go"})
		})
	}
}

func TestSetFileViewedFails(t *testing.T) {
	c, _ := sequenceServer(t, `{"data":{"result":null},"errors":[{"type":"UNPROCESSABLE","message":"No such file"}]}`)
	err := c.SetFileViewed(t.Context(), "PR_1", "a.go", true)
	if err == nil || !strings.Contains(err.Error(), "mark file a.go as viewed on pull request PR_1") || !strings.Contains(err.Error(), "No such file") {
		t.Errorf("err = %v, want the file, the pull request and GitHub's message", err)
	}
	c, _ = sequenceServer(t, `{"data":{"result":null}}`)
	if err := c.SetFileViewed(t.Context(), "PR_1", "a.go", false); !errors.Is(err, errNoViewedPull) {
		t.Errorf("err = %v, want errNoViewedPull", err)
	}
}
