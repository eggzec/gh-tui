package github

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func subjectRef(typ core.SubjectType, n int) core.SubjectRef {
	return core.SubjectRef{Repo: refsHere, Number: n, Type: typ}
}

// subjectServer answers each used slot of a request with body(i), the body
// of the slot's number, and records the used slots of each request.
func subjectServer(t *testing.T, sizes *[]int, queries *[]string, body func(number int) string) *Client {
	t.Helper()
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req gqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		var parts []string
		for i := range subjectSlots {
			s := strconv.Itoa(i)
			if req.Variables["w"+s] != true {
				continue
			}
			parts = append(parts, `"s`+s+`":`+body(int(req.Variables["i"+s].(float64))))
		}
		if sizes != nil {
			*sizes = append(*sizes, len(parts))
		}
		if queries != nil {
			*queries = append(*queries, req.Query)
		}
		_, _ = io.WriteString(w, `{"data":{`+strings.Join(parts, ",")+`}}`)
	}))
}

func TestSubjectStatesDecode(t *testing.T) {
	slots := map[int]string{
		1: `{"issueOrPullRequest":{"__typename":"PullRequest","pullState":"OPEN","isDraft":true}}`,
		2: `{"issueOrPullRequest":{"__typename":"PullRequest","pullState":"MERGED","isDraft":false}}`,
		3: `{"issueOrPullRequest":{"__typename":"Issue","issueState":"CLOSED","stateReason":"NOT_PLANNED"}}`,
		4: `{"issueOrPullRequest":{"__typename":"Issue","issueState":"OPEN"}}`,
		5: `{"discussion":{"closed":false,"isAnswered":true}}`,
		6: `{"discussion":{"closed":true,"isAnswered":false}}`,
		7: `{"discussion":{"closed":false,"isAnswered":false}}`,
	}
	var reqVars []map[string]any
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req gqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		reqVars = append(reqVars, req.Variables)
		var parts []string
		for i := range subjectSlots {
			s := strconv.Itoa(i)
			if req.Variables["w"+s] == true {
				parts = append(parts, `"s`+s+`":`+slots[int(req.Variables["i"+s].(float64))])
			}
		}
		_, _ = io.WriteString(w, `{"data":{`+strings.Join(parts, ",")+`}}`)
	}))
	asked := []core.SubjectRef{
		subjectRef(core.SubjectPullRequest, 1), subjectRef(core.SubjectPullRequest, 2),
		subjectRef(core.SubjectIssue, 3), subjectRef(core.SubjectIssue, 4),
		subjectRef(core.SubjectDiscussion, 5), subjectRef(core.SubjectDiscussion, 6),
		subjectRef(core.SubjectDiscussion, 7),
	}

	got, err := c.SubjectStates(t.Context(), asked)
	if err != nil {
		t.Fatalf("SubjectStates: %v", err)
	}
	want := map[core.SubjectRef]core.SubjectState{
		asked[0]: {State: core.StateOpen, Draft: true},
		asked[1]: {State: core.StateMerged},
		asked[2]: {State: core.StateClosed, Reason: core.ReasonNotPlanned},
		asked[3]: {State: core.StateOpen},
		asked[4]: {State: core.StateOpen, Answered: true},
		asked[5]: {State: core.StateClosed},
		asked[6]: {State: core.StateOpen},
	}
	if len(got) != len(want) {
		t.Errorf("got %d states, want %d: %+v", len(got), len(want), got)
	}
	for ref, w := range want {
		if got[ref] != w {
			t.Errorf("state of %v = %+v, want %+v", ref, got[ref], w)
		}
	}
	// A discussion number never goes to issueOrPullRequest, nor the others
	// to discussion.
	v := reqVars[0]
	if v["p0"] != true || v["d0"] != false || v["p4"] != false || v["d4"] != true {
		t.Errorf("p0 d0 p4 d4 = %v %v %v %v, want true false false true", v["p0"], v["d0"], v["p4"], v["d4"])
	}
	// Every variable is required, so a slot left over sends placeholders.
	if v["o7"] != "-" || v["n7"] != "-" || v["i7"] != 1.0 || v["w7"] != false || v["p7"] != false || v["d7"] != false || len(v) != 6*subjectSlots {
		t.Errorf("slot 7 = %v with %d variables, want placeholders and false", v, len(v))
	}
}

func TestSubjectStatesBatches(t *testing.T) {
	for _, tt := range []struct {
		subjects int
		sizes    []int
	}{{60, []int{25, 25, 10}}, {25, []int{25}}, {1, []int{1}}} {
		t.Run(strconv.Itoa(tt.subjects), func(t *testing.T) {
			var sizes []int
			var queries []string
			c := subjectServer(t, &sizes, &queries, func(int) string {
				return `{"issueOrPullRequest":{"__typename":"Issue","issueState":"OPEN"}}`
			})
			asked := make([]core.SubjectRef, tt.subjects)
			for i := range asked {
				asked[i] = subjectRef(core.SubjectIssue, i+1)
			}
			got, err := c.SubjectStates(t.Context(), asked)
			if err != nil {
				t.Fatalf("SubjectStates: %v", err)
			}
			if !slices.Equal(sizes, tt.sizes) {
				t.Errorf("slots used by request = %v, want %v", sizes, tt.sizes)
			}
			if len(got) != tt.subjects {
				t.Errorf("got %d states, want %d", len(got), tt.subjects)
			}
			// The shape never changes, so that the budget learns its cost.
			for _, q := range queries {
				if q != subjectStatesQuery {
					t.Errorf("a query differs from subjectStatesQuery")
				}
			}
		})
	}
}

func TestSubjectSlotsQuery(t *testing.T) {
	q := subjectSlotsQuery(2)
	for _, want := range []string{
		"$o1: String!, $n1: String!, $i1: Int!, $w1: Boolean!, $p1: Boolean!, $d1: Boolean!",
		"s1: repository(owner: $o1, name: $n1) @include(if: $w1)",
		"issueOrPullRequest(number: $i1) @include(if: $p1)",
		"discussion(number: $i1) @include(if: $d1)",
		rateLimitField,
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query for 2 slots lacks %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "$o2") {
		t.Errorf("query for 2 slots has a third:\n%s", q)
	}
	if got := strings.Count(subjectStatesQuery, "$w"+strconv.Itoa(subjectSlots-1)+")"); got != 1 {
		t.Errorf("subjectStatesQuery has the last slot %d times, want 1", got)
	}
	if strings.Contains(subjectStatesQuery, "$o"+strconv.Itoa(subjectSlots)+":") {
		t.Errorf("subjectStatesQuery has more than %d slots", subjectSlots)
	}
}

func TestSubjectStatesSkipsWhatItCannotRead(t *testing.T) {
	var sizes []int
	c := subjectServer(t, &sizes, nil, func(int) string {
		return `{"discussion":{"closed":false,"isAnswered":false}}`
	})
	got, err := c.SubjectStates(t.Context(), []core.SubjectRef{
		subjectRef(core.SubjectDiscussion, 0),
		subjectRef(core.SubjectRelease, 3),
		subjectRef(core.SubjectCommit, 4),
		{Repo: core.RepoRef{Owner: "eggzec"}, Number: 5, Type: core.SubjectIssue},
	})
	if err != nil || len(got) != 0 {
		t.Fatalf("SubjectStates = %+v, %v; want nothing", got, err)
	}
	if len(sizes) != 0 {
		t.Errorf("sent %d requests, want none when no subject can be read", len(sizes))
	}

	// What is asked twice takes one slot.
	d := subjectRef(core.SubjectDiscussion, 9)
	got, err = c.SubjectStates(t.Context(), []core.SubjectRef{d, d, subjectRef(core.SubjectDiscussion, 0)})
	if err != nil || len(got) != 1 {
		t.Fatalf("SubjectStates = %+v, %v; want one state", got, err)
	}
	if !slices.Equal(sizes, []int{1}) {
		t.Errorf("slots used = %v, want [1]", sizes)
	}
}

func TestSubjectStatesUnknown(t *testing.T) {
	asked := []core.SubjectRef{
		subjectRef(core.SubjectPullRequest, 1),
		subjectRef(core.SubjectIssue, 2),
		subjectRef(core.SubjectDiscussion, 3),
		subjectRef(core.SubjectIssue, 4),
	}
	for name, body := range map[string]string{
		"null slots": `{"data":{"s0":{"issueOrPullRequest":{"__typename":"PullRequest","pullState":"OPEN"}},"s1":null,"s2":{"discussion":null},"s3":{"issueOrPullRequest":null}}}`,
		"partial errors": `{"data":{"s0":{"issueOrPullRequest":{"__typename":"PullRequest","pullState":"OPEN"}},"s1":null,"s2":null,"s3":null},` +
			`"errors":[{"type":"NOT_FOUND","path":["s1","issueOrPullRequest"],"message":"gone"},` +
			`{"type":"FORBIDDEN","path":["s2"],"message":"no access"},{"type":"NOT_FOUND","path":["s3"],"message":"gone"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			got, err := c.SubjectStates(t.Context(), asked)
			if err != nil {
				t.Fatalf("SubjectStates: %v, want the unreadable ones left out", err)
			}
			want := map[core.SubjectRef]core.SubjectState{asked[0]: {State: core.StateOpen}}
			if len(got) != 1 || got[asked[0]] != want[asked[0]] {
				t.Errorf("states = %+v, want only the pull request", got)
			}
		})
	}
}

func TestSubjectStatesFails(t *testing.T) {
	for name, body := range map[string]string{
		"rate limit":        `{"data":null,"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}`,
		"no path with data": `{"data":{"s0":null},"errors":[{"type":"NOT_FOUND","message":"gone"}]}`,
		"other path":        `{"data":{"s0":null},"errors":[{"type":"NOT_FOUND","path":["viewer"],"message":"gone"}]}`,
		"other type":        `{"data":{"s0":null},"errors":[{"type":"INTERNAL","path":["s0"],"message":"oops"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", "1790000000")
				_, _ = io.WriteString(w, body)
			}))
			got, err := c.SubjectStates(t.Context(), []core.SubjectRef{subjectRef(core.SubjectIssue, 1)})
			if err == nil || got != nil {
				t.Fatalf("SubjectStates = %+v, %v; want the error", got, err)
			}
			if name == "rate limit" {
				if _, ok := errors.AsType[*core.RateLimitError](err); !ok {
					t.Errorf("error = %v, want a *core.RateLimitError", err)
				}
			}
		})
	}
}

func TestSubjectStatesTransportError(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Bad credentials"}`)
	}))
	got, err := c.SubjectStates(t.Context(), []core.SubjectRef{subjectRef(core.SubjectIssue, 1)})
	if err == nil || got != nil {
		t.Fatalf("SubjectStates = %+v, %v; want the error", got, err)
	}
}
