package github

import (
	"cmp"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// answer is what a fake GitHub answers to one request.
type answer struct {
	status int
	header map[string]string
	body   string
}

// answers is a base transport that answers each request to a path below
// the REST root, or to graphql, with what is sent on the path's channel,
// so that a test decides when and in what order requests are answered.
type answers map[string]chan answer

func (a answers) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	ch, ok := a[strings.TrimPrefix(req.URL.Path, "/")]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	var ans answer
	select {
	case ans = <-ch:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	h := make(http.Header)
	for k, v := range ans.header {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: cmp.Or(ans.status, http.StatusOK),
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(ans.body)),
		Request:    req,
	}, nil
}

// newAnswered returns a client whose requests the paths of a answer.
func newAnswered(t *testing.T, a answers) *Client {
	t.Helper()
	c, err := New(WithBaseURL("https://api.github.com/"), WithToken("t"), WithHTTPClient(&http.Client{Transport: a}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return retryAtOnce(c)
}

// quotaHeader is the rate limit of resource in a response.
func quotaHeader(resource string, limit, remaining int, reset time.Time) map[string]string {
	return map[string]string{
		"X-RateLimit-Limit":     strconv.Itoa(limit),
		"X-RateLimit-Remaining": strconv.Itoa(remaining),
		"X-RateLimit-Reset":     strconv.FormatInt(reset.Unix(), 10),
		"X-RateLimit-Resource":  resource,
	}
}

// getAsync sends a GET of path from c in the background, and returns
// where its error arrives, once the request is on its way to the base.
func getAsync(c *Client, path string) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := c.Get(context.Background(), path, Conditional{}, nil)
		done <- err
	}()
	synctest.Wait()
	return done
}

func est(t *testing.T, c *Client, resource string) int {
	t.Helper()
	st, ok := c.budget.status(resource)
	if !ok {
		t.Fatalf("no quota of %s", resource)
	}
	return st.est
}

func TestBudgetClassify(t *testing.T) {
	tests := []struct {
		root, url, want string
	}{
		{"/", "https://api.github.com/repos/o/r/pulls", resourceCore},
		{"/", "https://api.github.com/user", resourceCore},
		{"/", "https://api.github.com/search/issues?q=x", resourceSearch},
		{"/", "https://api.github.com/search/repositories?q=x", resourceSearch},
		{"/", "https://api.github.com/search/code?q=x", resourceCodeSearch},
		{"/", "https://api.github.com/graphql", resourceGraphQL},
		{"/", "https://api.github.com/rate_limit", ""},
		{"/api/v3/", "https://ghe.example.com/api/v3/search/code?q=x", resourceCodeSearch},
		{"/api/v3/", "https://ghe.example.com/api/v3/repos/o/r", resourceCore},
		{"/api/v3/", "https://ghe.example.com/api/graphql", resourceGraphQL},
		{"/api/v3/", "https://ghe.example.com/other", ""},
	}
	for _, tt := range tests {
		gql := "/graphql"
		if tt.root != "/" {
			gql = "/api/graphql"
		}
		b := newBudget(tt.root, gql)
		if got := b.classify(httptest.NewRequest(http.MethodGet, tt.url, http.NoBody)); got != tt.want {
			t.Errorf("classify(%s) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// TestBudgetExternalNotCounted checks that a download from a host outside
// the API, such as the storage a job log redirects to, is counted against
// nothing.
func TestBudgetExternalNotCounted(t *testing.T) {
	b := newBudget("/", "/graphql")
	ctx := withCall(t.Context(), &call{external: true})
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://storage.example.com/logs", http.NoBody)
	if r := b.reserve(req); r != nil {
		t.Errorf("reserve = %+v, want nothing for an external download", r)
	}
}

// TestBudgetResourcesApart spends code search, and checks that core is
// counted on its own.
func TestBudgetResourcesApart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Minute)
		a := answers{"search/code": make(chan answer, 1), "user": make(chan answer, 2)}
		c := newAnswered(t, a)

		a["search/code"] <- answer{status: http.StatusForbidden, header: quotaHeader(resourceCodeSearch, 10, 0, reset)}
		if _, err := c.Get(t.Context(), "search/code", Conditional{}, nil); err == nil {
			t.Fatal("code search succeeded, want a rate limit")
		}
		a["user"] <- answer{header: quotaHeader(resourceCore, 5000, 4999, reset.Add(time.Hour))}
		if _, err := c.Get(t.Context(), "user", Conditional{}, nil); err != nil {
			t.Fatalf("Get user: %v", err)
		}

		done := getAsync(c, "user")
		if got := est(t, c, resourceCore); got != 4998 {
			t.Errorf("core est with a request in flight = %d, want 4998", got)
		}
		if got := est(t, c, resourceCodeSearch); got != 0 {
			t.Errorf("code search est = %d, want 0, untouched by core", got)
		}
		a["user"] <- answer{header: quotaHeader(resourceCore, 5000, 4998, reset.Add(time.Hour))}
		if err := <-done; err != nil {
			t.Fatalf("Get user: %v", err)
		}
		if got := c.RateLimit(resourceCodeSearch); got.Remaining != 0 || got.Limit != 10 {
			t.Errorf("code search = %+v, want 0 of 10", got)
		}
		if got := c.RateLimit(resourceCore); got.Remaining != 4998 {
			t.Errorf("core = %+v, want 4998 left", got)
		}
	})
}

// TestBudgetOutOfOrder answers three requests in another order than they
// were sent, and checks that what an earlier request reported doesn't
// undo what a later one did, and that the requests not answered yet are
// still counted, whatever GitHub counted first.
func TestBudgetOutOfOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Hour)
		a := answers{"a": make(chan answer, 1), "b": make(chan answer, 1), "c": make(chan answer, 1), "d": make(chan answer, 1)}
		c := newAnswered(t, a)
		left := func(n int) answer { return answer{header: quotaHeader(resourceCore, 100, n, reset)} }

		a["a"] <- left(100)
		if _, err := c.Get(t.Context(), "a", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		b, cc, d := getAsync(c, "b"), getAsync(c, "c"), getAsync(c, "d")
		if got := est(t, c, resourceCore); got != 97 {
			t.Fatalf("est with three in flight = %d, want 97", got)
		}

		// d is answered first. GitHub may not have counted b and c yet,
		// if they still wait for a slot, so they still count.
		a["d"] <- left(99)
		<-d
		if got := est(t, c, resourceCore); got != 97 {
			t.Errorf("est after d's answer = %d, want 99 less b and c", got)
		}
		// The answer of b comes late, with more left than d's.
		a["b"] <- left(100)
		<-b
		if got := est(t, c, resourceCore); got != 98 {
			t.Errorf("est after b's late answer = %d, want 99 less c", got)
		}
		a["c"] <- left(97)
		<-cc
		if got := est(t, c, resourceCore); got != 97 {
			t.Errorf("est after all answers = %d, want 97", got)
		}
		if got := c.RateLimit(resourceCore).Remaining; got != 97 {
			t.Errorf("Remaining = %d, want 97", got)
		}
	})
}

// TestBudgetSettlesAnswersWithoutLimits checks that an answer that reports
// no rate limit still settles its request.
func TestBudgetSettlesAnswersWithoutLimits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Hour)
		a := answers{"x": make(chan answer, 2)}
		c := newAnswered(t, a)
		a["x"] <- answer{header: quotaHeader(resourceCore, 5000, 4000, reset)}
		a["x"] <- answer{}
		for range 2 {
			if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
				t.Fatal(err)
			}
		}
		if got := est(t, c, resourceCore); got != 4000 {
			t.Errorf("est = %d, want 4000", got)
		}
		if n := len(c.budget.pending); n != 0 {
			t.Errorf("%d reservations pending after their answers, want none", n)
		}
	})
}

// TestBudgetNotModifiedRefunds checks that a 304, which doesn't count
// against the rate limit, gives back what its request took.
func TestBudgetNotModifiedRefunds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Hour)
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		a["x"] <- answer{header: quotaHeader(resourceCore, 5000, 4000, reset)}
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}

		done := getAsync(c, "x")
		if got := est(t, c, resourceCore); got != 3999 {
			t.Errorf("est in flight = %d, want 3999", got)
		}
		a["x"] <- answer{status: http.StatusNotModified, header: quotaHeader(resourceCore, 5000, 4000, reset)}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if got := est(t, c, resourceCore); got != 4000 {
			t.Errorf("est after a 304 = %d, want 4000", got)
		}
	})
}

// TestBudgetForgetsFailures checks that a request that got no answer
// takes nothing.
func TestBudgetForgetsFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Hour)
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		a["x"] <- answer{header: quotaHeader(resourceCore, 5000, 4000, reset)}
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			_, err := c.Get(ctx, "x", Conditional{}, nil)
			done <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-done; err == nil {
			t.Fatal("canceled Get succeeded")
		}
		if got := est(t, c, resourceCore); got != 4000 {
			t.Errorf("est after a canceled request = %d, want 4000", got)
		}
	})
}

type panicking struct{}

func (panicking) RoundTrip(*http.Request) (*http.Response, error) { panic("base") }

// TestBudgetSettlesOnPanic checks that a request whose base panicked
// takes nothing.
func TestBudgetSettlesOnPanic(t *testing.T) {
	b := newBudget("/", "/graphql")
	rt := &rateTransport{base: panicking{}, budget: b}
	func() {
		defer func() { _ = recover() }()
		resp, err := rt.RoundTrip(httptest.NewRequest(http.MethodGet, "https://api.github.com/x", http.NoBody))
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	if n := len(b.pending); n != 0 {
		t.Errorf("%d reservations pending after a panic, want none", n)
	}
}

// TestBudgetGraphQLCost checks that a query reserves what its operation
// cost last time, and 1 before GitHub said.
func TestBudgetGraphQLCost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Hour)
		a := answers{"graphql": make(chan answer, 2)}
		c := newAnswered(t, a)
		const costly, cheap = "query Costly { rateLimit { cost } }", "query Cheap { viewer { login } }"
		reply := func(remaining int, data string) answer {
			return answer{header: quotaHeader(resourceGraphQL, 5000, remaining, reset), body: `{"data": ` + data + `}`}
		}
		query := func(q string) <-chan error {
			done := make(chan error, 1)
			go func() { done <- c.Query(context.Background(), q, nil, nil) }()
			synctest.Wait()
			return done
		}

		done := query(costly)
		if st, ok := c.budget.status(resourceGraphQL); ok {
			t.Fatalf("graphql quota before any answer = %+v, want none", st)
		}
		a["graphql"] <- reply(4993, `{"rateLimit": {"cost": 7}}`)
		if err := <-done; err != nil {
			t.Fatal(err)
		}

		costlyDone := query(costly)
		if got := est(t, c, resourceGraphQL); got != 4986 {
			t.Errorf("est with Costly in flight = %d, want 4993-7", got)
		}
		cheapDone := query(cheap)
		if got := est(t, c, resourceGraphQL); got != 4985 {
			t.Errorf("est with Costly and Cheap in flight = %d, want 4993-7-1", got)
		}
		a["graphql"] <- reply(4986, `{"rateLimit": {"cost": 7}}`)
		a["graphql"] <- reply(4985, `{"viewer": {"login": "x"}}`)
		for _, done := range []<-chan error{costlyDone, cheapDone} {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
		if got := est(t, c, resourceGraphQL); got != 4985 {
			t.Errorf("est after both = %d, want 4985", got)
		}
	})
}

// TestBudgetMutationCost checks that a mutation costs 1 of the primary
// limit, whatever a query of the same name cost.
func TestBudgetMutationCost(t *testing.T) {
	b := newBudget("/", "/graphql")
	b.learnCost("Star", 9)
	ctx := withCall(t.Context(), &call{op: "Star"})
	r := b.reserve(httptest.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", http.NoBody))
	if r.resource != resourceGraphQL || r.cost != 1 {
		t.Errorf("reservation = %+v, want 1 of graphql", r)
	}
}
