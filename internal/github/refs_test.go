package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

var refsHere = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}

func TestPullReferences(t *testing.T) {
	c, reqs := serveFixture(t, "graphql_pull_references.json")

	got, err := c.References(t.Context(), refsHere, 231, true)
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	req := <-reqs
	if req.Query != pullReferencesQuery {
		t.Errorf("query = %q, want pullReferencesQuery", req.Query)
	}
	if v := req.Variables; len(v) != 3 || v["owner"] != "eggzec" || v["name"] != "gh-tui" || v["number"] != 231.0 {
		t.Errorf("variables = %v, want eggzec, gh-tui and 231", v)
	}
	wantTexts := []core.RefOrigin{
		{Group: core.RefWritten, Where: "body"},
		{Group: core.RefWritten, Where: "comment", By: "bob"},
		{Group: core.RefWritten, Where: "comment", By: "ghost"},
		{Group: core.RefWritten, Where: "comment", By: "dependabot[bot]", Bot: true},
		{Group: core.RefWritten, Where: "comment", By: "carol"},
		{Group: core.RefWritten, Where: "review", By: "alice"},
	}
	origins := make([]core.RefOrigin, 0, len(got.Texts))
	for _, x := range got.Texts {
		origins = append(origins, x.Origin)
		if x.Text == "" {
			t.Errorf("a text of %+v is empty, want only texts with something written", x.Origin)
		}
	}
	if !slices.Equal(origins, wantTexts) {
		t.Errorf("text origins = %+v, want %+v", origins, wantTexts)
	}
	if n := len(got.Closing); n != 1 || got.Closing[0].Target != (core.Target{Repo: refsHere, Number: 198, Kind: core.KindIssue}) ||
		got.Closing[0].State != core.StateOpen || got.Closing[0].Title != "Token refresh fails on GHES 3.16" {
		t.Errorf("Closing = %+v, want the open issue #198", got.Closing)
	}
	// The other end of each event, whichever side of it the item is on.
	linked := make([]string, 0, len(got.Linked))
	for _, l := range got.Linked {
		linked = append(linked, fmt.Sprintf("%s %v", l.Ref.Target, l.Disconnected))
	}
	if want := []string{"eggzec/gh-tui#40 false", "eggzec/gh-tui#40 true", "eggzec/gh-tui#41 false", "eggzec/gh-tui#198 false"}; !slices.Equal(linked, want) {
		t.Errorf("Linked = %q, want %q", linked, want)
	}
	if got.Mentioned != 47 || got.CommentsRead != 4 || got.CommentsTotal != 340 || got.ReviewsRead != 2 || got.ReviewsTotal != 2 {
		t.Errorf("counts = %d mentioned, %d of %d comments, %d of %d reviews; want 47, 4 of 340 and 2 of 2",
			got.Mentioned, got.CommentsRead, got.CommentsTotal, got.ReviewsRead, got.ReviewsTotal)
	}
}

func TestIssueReferences(t *testing.T) {
	c, reqs := serveFixture(t, "graphql_issue_references.json")

	got, err := c.References(t.Context(), refsHere, 231, false)
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	req := <-reqs
	if req.Query != issueReferencesQuery {
		t.Errorf("query = %q, want issueReferencesQuery", req.Query)
	}
	// Without it GitHub leaves out the pull requests that were closed or
	// merged, which are the ones that closed the issue.
	if !strings.Contains(req.Query, "closedByPullRequestsReferences(first: 100, includeClosedPrs: true)") {
		t.Error("the request doesn't ask for closed pull requests")
	}
	pr300 := core.Target{Repo: refsHere, Number: 300, Kind: core.KindPull}
	if len(got.Closing) != 1 || got.Closing[0].Target != pr300 || got.Closing[0].State != core.StateMerged {
		t.Errorf("Closing = %+v, want the merged pull request #300", got.Closing)
	}
	// A closer that is a commit, or nothing, is no pull request.
	if len(got.Closers) != 1 || got.Closers[0].Target != pr300 {
		t.Errorf("Closers = %+v, want only #300", got.Closers)
	}
	if len(got.Linked) != 1 || got.Linked[0].Ref.Target.Number != 301 || got.Linked[0].Ref.State != core.StateClosed || got.Linked[0].Ref.Target.Kind != core.KindPull {
		t.Errorf("Linked = %+v, want the closed pull request #301", got.Linked)
	}
	if got.Mentioned != 3 || got.ReviewsTotal != 0 || len(got.Texts) != 2 {
		t.Errorf("sources = %+v, want 3 mentions, no reviews and 2 texts", got)
	}
}

func TestReferencesStateReasons(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"repository":{"issue":{"body":"","comments":{"totalCount":0,"nodes":[]},
"closedByPullRequestsReferences":{"nodes":[]},"mentions":{"filteredCount":0},"linked":{"nodes":[
{"__typename":"ConnectedEvent","source":{"__typename":"Issue","number":5,"title":"a\u001b[31m\nb","url":"u","issueState":"CLOSED","stateReason":"DUPLICATE","repository":{"name":"gh-tui","owner":{"login":"EggZec"}}},
 "subject":{"__typename":"Issue","number":9,"title":"self","url":"u","issueState":"OPEN","stateReason":null,"repository":{"name":"GH-TUI","owner":{"login":"eggzec"}}}}]}}}}}`)
	}))

	got, err := c.References(t.Context(), refsHere, 9, false)
	if err != nil || len(got.Linked) != 1 {
		t.Fatalf("References = %+v, %v; want one link", got, err)
	}
	r := got.Linked[0].Ref
	if r.Reason != core.ReasonDuplicate || r.State != core.StateClosed || r.Title != "a b" {
		t.Errorf("link = %+v, want a closed duplicate with its title on one line", r)
	}
}

func TestReferencesNotFound(t *testing.T) {
	for _, body := range []string{
		`{"data":{"repository":{"pullRequest":null}}}`,
		`{"data":{"repository":null}}`,
	} {
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
		if _, err := c.References(t.Context(), refsHere, 1, true); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("References on %s = %v, want core.ErrNotFound", body, err)
		}
		if _, err := c.Mentions(t.Context(), refsHere, 1, true, "", 10); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("Mentions on %s = %v, want core.ErrNotFound", body, err)
		}
	}
}

func TestMentionsPage(t *testing.T) {
	c, reqs := serveFixture(t, "graphql_issue_mentions.json")

	got, err := c.Mentions(t.Context(), refsHere, 231, false, "Y3Vyc29yOjUw", 4)
	if err != nil {
		t.Fatalf("Mentions: %v", err)
	}
	req := <-reqs
	if req.Query != issueMentionsQuery {
		t.Errorf("query = %q, want issueMentionsQuery", req.Query)
	}
	v := req.Variables
	if v["last"] != 4.0 || v["before"] != "Y3Vyc29yOjUw" || v["number"] != 231.0 {
		t.Errorf("variables = %v, want last 4 before the cursor", v)
	}
	// GitHub lists the page oldest first; the newest comes first here.
	keys := make([]string, 0, len(got.Items))
	for _, r := range got.Items {
		keys = append(keys, core.RefKey(r.Target))
	}
	if want := []string{"acme/infra#77", "eggzec/gh-tui#239", "eggzec/gh-tui#240"}; !slices.Equal(keys, want) {
		t.Errorf("items = %q, want %q", keys, want)
	}
	if got.Next != "Y3Vyc29yOjQw" {
		t.Errorf("Next = %q, want the cursor of the page before", got.Next)
	}
	bot := []core.RefOrigin{{Group: core.RefMentioned, Where: "mentioned", By: "ci-bot[bot]", Bot: true}}
	if !slices.Equal(got.Items[0].Origins, bot) {
		t.Errorf("origins = %+v, want %+v", got.Items[0].Origins, bot)
	}
	// A mention that closes the item says so besides.
	want := []core.RefOrigin{
		{Group: core.RefClosing, Where: "closed by", By: "bob"},
		{Group: core.RefMentioned, Where: "mentioned", By: "bob"},
	}
	if !slices.Equal(got.Items[1].Origins, want) {
		t.Errorf("origins = %+v, want %+v", got.Items[1].Origins, want)
	}
}

func TestMentionsFirstPageLast(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req gqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if b, ok := req.Variables["before"]; !ok || b != nil {
			t.Errorf("before = %v, want null for the newest page", b)
		}
		if !strings.Contains(req.Query, "pullRequest(number: $number)") {
			t.Errorf("query %q doesn't ask for a pull request", req.Query)
		}
		_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"timelineItems":{"pageInfo":{"hasPreviousPage":false,"startCursor":"x"},"nodes":[]}}}}}`)
	}))

	got, err := c.Mentions(t.Context(), refsHere, 1, true, "", 100)
	if err != nil || got.Next != "" || len(got.Items) != 0 {
		t.Errorf("Mentions = %+v, %v; want an empty last page", got, err)
	}
}

func resolveTargets(n int) []core.Target {
	out := make([]core.Target, n)
	for i := range out {
		out[i] = core.Target{Repo: refsHere, Number: i + 1}
	}
	return out
}

func TestResolvePartial(t *testing.T) {
	c, reqs := serveFixture(t, "graphql_resolve_references_partial.json")
	targets := []core.Target{
		{Repo: refsHere, Number: 12},
		{Repo: core.RepoRef{Owner: "octo", Name: "secret"}, Number: 9},
		{Repo: core.RepoRef{Owner: "acme", Name: "saml"}, Number: 3},
		{Repo: core.RepoRef{Owner: "cli", Name: "go-gh"}, Number: 233},
		{Repo: refsHere, Number: 5},
		{Repo: refsHere, Number: 6},
	}

	got, err := c.ResolveReferences(t.Context(), targets)
	if err != nil {
		t.Fatalf("ResolveReferences: %v", err)
	}
	req := <-reqs
	if req.Query != resolveReferencesQuery {
		t.Errorf("query = %q, want resolveReferencesQuery", req.Query)
	}
	v := req.Variables
	if v["o1"] != "octo" || v["n1"] != "secret" || v["i1"] != 9.0 || v["w1"] != true {
		t.Errorf("slot 1 = %v %v %v %v, want octo secret 9 true", v["o1"], v["n1"], v["i1"], v["w1"])
	}
	// Every variable is required, so a slot left over sends placeholders.
	if v["o6"] != "-" || v["n6"] != "-" || v["i6"] != 1.0 || v["w6"] != false || len(v) != 4*refSlots {
		t.Errorf("slot 6 = %v %v %v %v with %d variables, want placeholders and false", v["o6"], v["n6"], v["i6"], v["w6"], len(v))
	}
	if len(got) != len(targets) {
		t.Fatalf("got %d references, want %d", len(got), len(targets))
	}
	if r := got[0]; r.Problem != "" || r.State != core.StateClosed || r.Reason != core.ReasonNotPlanned || r.Target.Kind != core.KindIssue || r.Title != "Login loops on GHES" {
		t.Errorf("the issue = %+v, want it read, closed as not planned", r)
	}
	if r := got[3]; r.Problem != "" || !r.Draft || r.Target.Kind != core.KindPull || r.Target.Repo.Owner != "cli" {
		t.Errorf("the pull request = %+v, want a draft of cli/go-gh", r)
	}
	for i, want := range map[int]string{1: "not found, or private", 2: "access refused", 4: "Odd thing happened", 5: "not found, or private"} {
		if got[i].Problem != want || got[i].Target != targets[i] || got[i].Title != "" {
			t.Errorf("reference %d = %+v, want only its target and the problem %q", i, got[i], want)
		}
	}
}

func TestResolveBatches(t *testing.T) {
	for _, tt := range []struct {
		targets int
		sizes   []int
	}{{45, []int{20, 20, 5}}, {20, []int{20}}, {100, []int{20, 20, 20, 20, 20}}} {
		t.Run(strconv.Itoa(tt.targets), func(t *testing.T) { testResolveBatches(t, tt.targets, tt.sizes) })
	}
}

func testResolveBatches(t *testing.T, n int, want []int) {
	t.Helper()
	var sizes []int
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req gqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		used := 0
		var b strings.Builder
		b.WriteString(`{"data":{`)
		for i := range refSlots {
			s := strconv.Itoa(i)
			if req.Variables["w"+s] != true {
				continue
			}
			used++
			if used > 1 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `"r%s":{"issueOrPullRequest":{"__typename":"Issue","number":%v,"title":"t","url":"u","issueState":"OPEN","repository":{"name":"gh-tui","owner":{"login":"eggzec"}}}}`, s, req.Variables["i"+s])
		}
		b.WriteString(`}}`)
		sizes = append(sizes, used)
		_, _ = io.WriteString(w, b.String())
	}))

	got, err := c.ResolveReferences(t.Context(), resolveTargets(n))
	if err != nil {
		t.Fatalf("ResolveReferences: %v", err)
	}
	if !slices.Equal(sizes, want) {
		t.Errorf("slots used by request = %v, want %v, one request after another", sizes, want)
	}
	for i := range got {
		if got[i].Target.Number != i+1 {
			t.Fatalf("reference %d is #%d, want them in the order asked", i, got[i].Target.Number)
		}
	}
	if len(got) != n {
		t.Errorf("got %d references, want %d", len(got), n)
	}
}

func TestResolveFailsOnAnErrorWithoutASlot(t *testing.T) {
	for name, body := range map[string]string{
		"rate limit":        `{"data":null,"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}`,
		"no path with data": `{"data":{"r0":null},"errors":[{"type":"NOT_FOUND","message":"gone"}]}`,
		"other path":        `{"data":{"r0":null},"errors":[{"type":"NOT_FOUND","path":["viewer"],"message":"gone"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", "1790000000")
				_, _ = io.WriteString(w, body)
			}))
			got, err := c.ResolveReferences(t.Context(), resolveTargets(1))
			if err == nil || got != nil {
				t.Fatalf("ResolveReferences = %+v, %v; want the error", got, err)
			}
			if name == "rate limit" {
				if _, ok := errors.AsType[*core.RateLimitError](err); !ok {
					t.Errorf("error = %v, want a *core.RateLimitError", err)
				}
			}
		})
	}
}

func TestRefSlotsQuery(t *testing.T) {
	q := refSlotsQuery(2)
	for _, want := range []string{"$o1: String!, $n1: String!, $i1: Int!, $w1: Boolean!", "r1: repository(owner: $o1, name: $n1) @include(if: $w1)"} {
		if !strings.Contains(q, want) {
			t.Errorf("query for 2 slots lacks %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "$o2") {
		t.Errorf("query for 2 slots has a third:\n%s", q)
	}
	if got := strings.Count(resolveReferencesQuery, "@include(if:"); got != refSlots {
		t.Errorf("resolveReferencesQuery has %d slots, want %d", got, refSlots)
	}
}

// An issue that an organization's SAML enforcement keeps from the token
// leaves the rest of the item.
func TestReferencesPartlyRefused(t *testing.T) {
	forbidden := `{"type":"FORBIDDEN","path":["repository","pullRequest","closingIssuesReferences","nodes",0],"message":"Resource protected by organization SAML enforcement."}`
	issue := `{"__typename":"Issue","number":198,"title":"t","url":"u","issueState":"OPEN","repository":{"name":"gh-tui","owner":{"login":"eggzec"}}}`
	answer := func(errs string) *Client {
		return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"body":"x","comments":{"totalCount":0,"nodes":[]},"reviews":{"totalCount":0,"nodes":[]},`+
				`"closingIssuesReferences":{"nodes":[null,`+issue+`]},"linked":{"nodes":[]},"mentions":{"filteredCount":2}}}},"errors":[`+errs+`]}`)
		}))
	}

	got, err := answer(forbidden).References(t.Context(), refsHere, 231, true)
	if err != nil || len(got.Closing) != 1 || got.Closing[0].Target.Number != 198 || got.Mentioned != 2 {
		t.Errorf("References = %+v, %v; want the item without the refused node", got, err)
	}
	// An error about the item itself, or of another kind, fails the read.
	for _, errs := range []string{
		`{"type":"FORBIDDEN","path":["repository","pullRequest"],"message":"no"}`,
		forbidden + `,{"type":"SOMETHING","path":["repository","pullRequest","body"],"message":"odd"}`,
		`{"type":"FORBIDDEN","message":"no path"}`,
		`{"type":"NOT_FOUND","path":["repository","pullRequest","closingIssuesReferences"],"message":"the whole connection"}`,
		`{"type":"NOT_FOUND","path":["repository","pullRequest","body"],"message":"a scalar"}`,
	} {
		if _, err := answer(errs).References(t.Context(), refsHere, 231, true); err == nil {
			t.Errorf("References with %s succeeded, want the error", errs)
		}
	}
}

func TestMentionsPartlyRefused(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"repository":{"issue":{"timelineItems":{"pageInfo":{"hasPreviousPage":false},"nodes":[null,`+
			`{"willCloseTarget":false,"actor":null,"source":{"__typename":"Issue","number":3,"title":"t","url":"u","issueState":"OPEN","repository":{"name":"r","owner":{"login":"o"}}}}]}}}},`+
			`"errors":[{"type":"NOT_FOUND","path":["repository","issue","timelineItems","nodes",0,"source"],"message":"gone"}]}`)
	}))
	got, err := c.Mentions(t.Context(), refsHere, 1, false, "", 10)
	if err != nil || len(got.Items) != 1 || got.Items[0].Target.Number != 3 {
		t.Errorf("Mentions = %+v, %v; want the readable mention", got, err)
	}
}

// The newest comments and reviews are read, for they are likelier to
// name what is linked now.
func TestReferencesReadTheNewest(t *testing.T) {
	for _, q := range []string{pullReferencesQuery, issueReferencesQuery} {
		if !strings.Contains(q, "comments(last: 100)") || strings.Contains(q, "comments(first") || strings.Contains(q, "reviews(first") {
			t.Errorf("query reads the oldest comments or reviews:\n%s", q)
		}
	}
}
