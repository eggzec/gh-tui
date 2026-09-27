package github

import (
	"cmp"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
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
		{"/", "https://storage.example.com/repos/o/r", ""},
		{"/api/v3/", "https://api.github.com/repos/o/r", ""},
	}
	for _, tt := range tests {
		host, gql := "api.github.com", "/graphql"
		if tt.root != "/" {
			host, gql = "ghe.example.com", "/api/graphql"
		}
		b := newBudget(host, tt.root, gql)
		if got := b.classify(httptest.NewRequest(http.MethodGet, tt.url, http.NoBody)); got != tt.want {
			t.Errorf("classify(%s) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// TestBudgetExternalNotCounted checks that a download from a host outside
// the API, such as the storage a job log redirects to, is counted against
// nothing, whether the call says so or a redirect led there.
func TestBudgetExternalNotCounted(t *testing.T) {
	b := newBudget("api.github.com", "/", "/graphql")
	ctx := withCall(t.Context(), &call{external: true})
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://storage.example.com/logs", http.NoBody)
	if r := b.reserve(req); r != nil {
		t.Errorf("reserve = %+v, want nothing for an external download", r)
	}
	req = httptest.NewRequest(http.MethodGet, "https://storage.example.com/repos/o/r", http.NoBody)
	if r := b.reserve(req); r != nil {
		t.Errorf("reserve = %+v, want nothing for a redirect to another host", r)
	}
}

// TestBudgetResourcesApart spends code search, and checks that core is
// counted on its own.
func TestBudgetResourcesApart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Minute)
		a := answers{"search/code": make(chan answer, 1), "user": make(chan answer, 2)}
		c := newAnswered(t, a)

		a["search/code"] <- spent(resourceCodeSearch, reset, 0)
		if _, err := c.Get(t.Context(), "search/code", Conditional{}, nil); err == nil {
			t.Fatal("code search succeeded, want a rate limit")
		}
		if st, _ := c.budget.status(resourceCodeSearch); !st.release.Equal(reset.Add(minGuard)) {
			t.Errorf("code search release = %v, want %v", st.release, reset.Add(minGuard))
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
		if got := c.RateLimit(resourceCodeSearch); got.Remaining != 0 || got.Resource != resourceCodeSearch {
			t.Errorf("code search = %+v, want none left", got)
		}
		if got := c.RateLimit(resourceCore); got.Remaining != 4998 {
			t.Errorf("core = %+v, want 4998 left", got)
		}
		if st, _ := c.budget.status(resourceCore); !st.release.IsZero() {
			t.Errorf("core release = %v, want none", st.release)
		}
	})
}

// TestBudgetLearnsResources answers a REST route that the path says
// counts against core as counted against another resource, and checks
// that the next request of the route is counted against that one.
func TestBudgetLearnsResources(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const sbom = "dependency_sbom"
		reset := time.Now().Add(time.Hour)
		a := answers{"repos/o/r/dependency-graph/sbom": make(chan answer, 1), "repos/p/q/dependency-graph/sbom": make(chan answer, 1)}
		c := newAnswered(t, a)
		a["repos/o/r/dependency-graph/sbom"] <- answer{header: quotaHeader(sbom, 100, 50, reset)}
		if _, err := c.Get(t.Context(), "repos/o/r/dependency-graph/sbom", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}

		done := getAsync(c, "repos/p/q/dependency-graph/sbom")
		if got := est(t, c, sbom); got != 49 {
			t.Errorf("%s est with another repository's in flight = %d, want 49", sbom, got)
		}
		a["repos/p/q/dependency-graph/sbom"] <- answer{header: quotaHeader(sbom, 100, 49, reset)}
		if err := <-done; err != nil {
			t.Fatal(err)
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
	b := newBudget("api.github.com", "/", "/graphql")
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

// TestBudgetAnonymousQueryCost checks that queries without a name each
// reserve what they cost themselves, not what another such query did.
func TestBudgetAnonymousQueryCost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Hour)
		a := answers{"graphql": make(chan answer, 1)}
		c := newAnswered(t, a)
		const costly, cheap = "{ search { rateLimit { cost } } }", "query { viewer { login } }"
		a["graphql"] <- answer{header: quotaHeader(resourceGraphQL, 5000, 4990, reset), body: `{"data": {"rateLimit": {"cost": 10}}}`}
		if err := c.Query(t.Context(), costly, nil, nil); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- c.Query(context.Background(), cheap, nil, nil) }()
		synctest.Wait()
		if got := est(t, c, resourceGraphQL); got != 4989 {
			t.Errorf("est with another anonymous query in flight = %d, want 4990-1", got)
		}
		a["graphql"] <- answer{header: quotaHeader(resourceGraphQL, 5000, 4989, reset), body: `{"data": {}}`}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

// TestBudgetMutationCost checks that a mutation costs 1 of the primary
// limit, whatever a query of the same name cost.
func TestBudgetMutationCost(t *testing.T) {
	b := newBudget("api.github.com", "/", "/graphql")
	b.learnCost("Star", 9)
	ctx := withCall(t.Context(), &call{op: "Star"})
	r := b.reserve(httptest.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", http.NoBody))
	if r.resource != resourceGraphQL || r.cost != 1 {
		t.Errorf("reservation = %+v, want 1 of graphql", r)
	}
}

// spent is a refusal because resource is spent until reset, dated by
// GitHub's clock, which is ahead of the local one by skew.
func spent(resource string, reset time.Time, skew time.Duration) answer {
	h := quotaHeader(resource, 5000, 0, reset)
	h["Date"] = time.Now().Add(skew).UTC().Format(http.TimeFormat)
	h["X-GitHub-Request-Id"] = "ABCD:1234"
	return answer{status: http.StatusForbidden, header: h, body: `{"message": "API rate limit exceeded"}`}
}

// limitedUntil returns when the rate limit that err is lifts.
func limitedUntil(t *testing.T, err error) time.Time {
	t.Helper()
	rl, ok := errors.AsType[*core.RateLimitError](err)
	if !ok {
		t.Fatalf("error = %v, want a rate limit", err)
	}
	return rl.Reset
}

// TestBudgetSkew answers with a Date 5s ahead of the local clock, and
// checks that the reset is placed 5s earlier in local time.
func TestBudgetSkew(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const skew = 5 * time.Second
		reset := time.Now().Add(skew + 10*time.Minute) // in GitHub's clock
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)

		a["x"] <- spent(resourceCore, reset, skew)
		_, err := c.Get(t.Context(), "x", Conditional{}, nil)

		want := reset.Add(-skew).Add(minGuard)
		if got := limitedUntil(t, err); !got.Equal(want) {
			t.Errorf("Reset = %v, want %v, the local reset and a guard", got, want)
		}
		if st, _ := c.budget.status(resourceCore); !st.release.Equal(want) {
			t.Errorf("release = %v, want %v", st.release, want)
		}
	})
}

// TestBudgetSkewOnlyGitHub checks that the Date of an answer that isn't
// GitHub's, such as a proxy's, doesn't move the skew.
func TestBudgetSkewOnlyGitHub(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(10 * time.Minute)
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)

		ans := spent(resourceCore, reset, time.Hour)
		delete(ans.header, "X-GitHub-Request-Id")
		a["x"] <- ans
		_, err := c.Get(t.Context(), "x", Conditional{}, nil)
		if got, want := limitedUntil(t, err), reset.Add(minGuard); !got.Equal(want) {
			t.Errorf("Reset = %v, want %v, as if the clocks agreed", got, want)
		}
	})
}

// TestBudgetSkewMedian checks that one answer held up on its way doesn't
// move the skew.
func TestBudgetSkewMedian(t *testing.T) {
	var s clockSkew
	for _, d := range []time.Duration{2, 2, -30, 2, 3} {
		s.add(d * time.Second)
	}
	if got := s.offset(); got != 2*time.Second {
		t.Errorf("offset = %v, want 2s", got)
	}
	for _, tt := range []struct {
		offsets []time.Duration
		want    time.Duration
	}{
		{[]time.Duration{3, 1}, 1},
		{[]time.Duration{4, 1, 3, 2}, 2},
	} {
		var s clockSkew
		for _, d := range tt.offsets {
			s.add(d * time.Second)
		}
		if got := s.offset(); got != tt.want*time.Second {
			t.Errorf("offset of %v = %v, want the lower middle, %vs", tt.offsets, got, int64(tt.want))
		}
	}
	for range skewSamples {
		s.add(-time.Second)
	}
	if got := s.offset(); got != -time.Second {
		t.Errorf("offset after the old ones went = %v, want -1s", got)
	}
}

// TestBudgetGuard answers requests sent after the release with the old
// window still spent, and checks that the guard doubles each time, up to
// maxGuard, and that a new window starts with the shortest again.
func TestBudgetGuard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Minute)
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		get := func(ans answer) error {
			a["x"] <- ans
			_, err := c.Get(t.Context(), "x", Conditional{}, nil)
			return err
		}

		release := limitedUntil(t, get(spent(resourceCore, reset, 0)))
		if want := reset.Add(minGuard); !release.Equal(want) {
			t.Fatalf("first release = %v, want %v", release, want)
		}
		// Sent before the release, a refusal changes nothing.
		if got := limitedUntil(t, get(spent(resourceCore, reset, 0))); !got.Equal(release) {
			t.Errorf("release after a refusal before it = %v, want %v", got, release)
		}
		for _, guard := range []time.Duration{2, 4, 8, 8} {
			time.Sleep(time.Until(release))
			release = limitedUntil(t, get(spent(resourceCore, reset, 0)))
			if want := time.Now().Add(guard * time.Second); !release.Equal(want) {
				t.Errorf("release = %v, want %v, a guard of %ds after the refusal", release, want, guard)
			}
		}

		time.Sleep(time.Until(release))
		next := reset.Add(time.Hour)
		if err := get(answer{header: quotaHeader(resourceCore, 5000, 4999, next)}); err != nil {
			t.Fatal(err)
		}
		if st, _ := c.budget.status(resourceCore); !st.release.IsZero() {
			t.Errorf("release in a new window = %v, want none", st.release)
		}
		if got := limitedUntil(t, get(spent(resourceCore, next, 0))); !got.Equal(next.Add(minGuard)) {
			t.Errorf("release in a new window = %v, want %v", got, next.Add(minGuard))
		}
	})
}

// TestBudgetFarReset checks that a reset too far away to be real doesn't
// spend the resource until then.
func TestBudgetFarReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		a["x"] <- spent(resourceCore, time.Now().Add(48*time.Hour), 0)
		_, err := c.Get(t.Context(), "x", Conditional{}, nil)
		if got, want := limitedUntil(t, err), time.Now().Add(secondaryBackoff); !got.Equal(want) {
			t.Errorf("Reset = %v, want %v, as if it didn't say", got, want)
		}
		if st, _ := c.budget.status(resourceCore); !st.release.IsZero() || st.reported.Remaining != 0 {
			t.Errorf("status = %+v, want none left and no release", st)
		}

		// A real reset replaces it, and a far one doesn't replace a real one.
		reset := time.Now().Add(time.Hour)
		for _, ans := range []answer{
			{header: quotaHeader(resourceCore, 5000, 4000, reset)},
			{header: quotaHeader(resourceCore, 5000, 3000, time.Now().Add(72*time.Hour))},
		} {
			a["x"] <- ans
			if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
				t.Fatal(err)
			}
			if got := c.RateLimit(resourceCore); got.Remaining != 4000 || !got.Reset.Equal(reset) {
				t.Errorf("core = %+v, want 4000 left until %v", got, reset)
			}
		}
	})
}

// TestBudgetGuardByReservation answers a request reserved before the
// release after it, with the old window still spent, and checks that the
// guard stays: the request may have gone out before the release.
func TestBudgetGuardByReservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Minute)
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		a["x"] <- spent(resourceCore, reset, 0)
		_, err := c.Get(t.Context(), "x", Conditional{}, nil)
		release := limitedUntil(t, err)

		done := getAsync(c, "x")
		time.Sleep(time.Until(release) + time.Second)
		a["x"] <- spent(resourceCore, reset, 0)
		if got := limitedUntil(t, <-done); !got.Equal(release) {
			t.Errorf("release = %v, want %v, the guard unchanged", got, release)
		}
		if g := c.budget.quotas[resourceCore].guard; g != minGuard {
			t.Errorf("guard = %v, want %v", g, minGuard)
		}
	})
}

// TestBudgetGraphQLRefusedWithQuotaLeft checks that a query refused as
// RATE_LIMITED with quota left spends GraphQL until its reset only when
// the quota is what refused it: GitHub says so, or the query costs more
// than is left and GitHub doesn't say the limit is a secondary one.
// Otherwise the limit is a secondary one, which lifts when Retry-After
// says, or after a minute.
func TestBudgetGraphQLRefusedWithQuotaLeft(t *testing.T) {
	const spentMsg, secondaryMsg, otherMsg = "API rate limit exceeded", "You have exceeded a secondary rate limit.", "Rate limited."
	tests := []struct {
		name       string
		msg        string
		cost       int
		retryAfter time.Duration
		primary    bool
	}{
		{name: "quota spent", msg: spentMsg, primary: true},
		{name: "costs more than is left", msg: otherMsg, cost: 7, primary: true},
		{name: "secondary, costs more than is left", msg: secondaryMsg, cost: 7},
		{name: "secondary", msg: secondaryMsg},
		{name: "secondary, costs less than is left", msg: secondaryMsg, cost: 2},
		{name: "secondary with Retry-After", msg: secondaryMsg, retryAfter: 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reset := time.Now().Add(time.Minute)
				a := answers{"graphql": make(chan answer, 1)}
				c := newAnswered(t, a)
				if tt.cost > 0 {
					c.budget.learnCost("Big", tt.cost)
				}
				h := quotaHeader(resourceGraphQL, 5000, 3, reset)
				want, release := time.Now().Add(secondaryBackoff), time.Time{}
				if tt.retryAfter > 0 {
					h["Retry-After"] = strconv.Itoa(int(tt.retryAfter.Seconds()))
					want = time.Now().Add(tt.retryAfter)
				}
				a["graphql"] <- answer{
					header: h,
					body:   `{"data": null, "errors": [{"type": "RATE_LIMITED", "message": "` + tt.msg + `"}]}`,
				}
				err := c.Query(t.Context(), "query Big { viewer { login } }", nil, nil)
				if tt.primary {
					want, release = reset.Add(minGuard), reset.Add(minGuard)
				}
				if got := limitedUntil(t, err); !got.Equal(want) {
					t.Errorf("Reset = %v, want %v", got, want)
				}
				if st, _ := c.budget.status(resourceGraphQL); !st.release.Equal(release) {
					t.Errorf("release = %v, want %v", st.release, release)
				}
			})
		})
	}
}

// TestBudgetContact checks when the budget says GitHub last answered and a
// request last failed: any answer of GitHub's counts, whatever its status,
// but not a captive portal's page, and a canceled request isn't a failure.
func TestBudgetContact(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		github := map[string]string{"X-GitHub-Request-Id": "ABCD:1234"}
		check := func(step string, answered, failed time.Time) {
			t.Helper()
			gotAnswered, gotFailed := c.budget.contact()
			if !gotAnswered.Equal(answered) || !gotFailed.Equal(failed) {
				t.Errorf("%s: contact = %v, %v; want %v, %v", step, gotAnswered, gotFailed, answered, failed)
			}
		}
		get := func(path string) {
			_, _ = c.Get(t.Context(), path, Conditional{}, nil)
		}
		check("before any request", time.Time{}, time.Time{})

		a["x"] <- answer{header: github}
		get("x")
		start := time.Now()
		check("after an answer", start, time.Time{})

		time.Sleep(time.Second)
		a["x"] <- answer{header: map[string]string{"Content-Type": "text/html"}, body: "<html>Log in to the Wi-Fi</html>"}
		get("x")
		check("after a captive portal's page", start, time.Time{})

		a["x"] <- answer{status: http.StatusForbidden, header: github}
		get("x")
		refused := time.Now()
		check("after a refusal", refused, time.Time{})

		time.Sleep(time.Second)
		get("unanswered")
		failed := time.Now()
		check("after a failure", refused, failed)

		time.Sleep(time.Second)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = c.Get(ctx, "x", Conditional{}, nil)
		}()
		synctest.Wait()
		cancel()
		<-done
		check("after a cancellation", refused, failed)
	})
}
