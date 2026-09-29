package github

import (
	"io"
	"math/rand/v2"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// answerQuota answers with the quota of resource, from GitHub.
func answerQuota(resource string, limit, remaining int, reset time.Time) answer {
	h := quotaHeader(resource, limit, remaining, reset)
	h["X-GitHub-Request-Id"] = "ABCD:1234"
	return answer{header: h}
}

// coreQuota returns the core quota of s.
func coreQuota(t *testing.T, s core.RateStatus) core.Quota {
	t.Helper()
	i := slices.IndexFunc(s.Quotas, func(q core.Quota) bool { return q.Resource == resourceCore })
	if i < 0 {
		t.Fatalf("no core quota in %+v", s.Quotas)
	}
	return s.Quotas[i]
}

// TestRateStatusOrder checks that the resources gh-tui uses most come
// first, and the others after them by name.
func TestRateStatusOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		if got := c.RateStatus().Quotas; len(got) != 0 {
			t.Fatalf("Quotas before any answer = %+v, want none", got)
		}
		reset := time.Now().Add(time.Hour)
		for _, r := range []string{"source_import", resourceCodeSearch, resourceSearch, "audit_log", resourceCore, resourceGraphQL} {
			a["x"] <- answerQuota(r, 100, 50, reset)
			if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
				t.Fatalf("Get: %v", err)
			}
		}
		quotas := c.RateStatus().Quotas
		got := make([]string, 0, len(quotas))
		for _, q := range quotas {
			got = append(got, q.Resource)
		}
		want := []string{resourceCore, resourceGraphQL, resourceSearch, resourceCodeSearch, "audit_log", "source_import"}
		if !slices.Equal(got, want) {
			t.Errorf("resources = %v, want %v", got, want)
		}
	})
}

// TestRateStatusRemaining checks that Remaining is what GitHub said, with
// the requests in flight not taken off, so that one GitHub doesn't count,
// such as a 304, doesn't show as the quota going down and then up; that
// it never goes up within a window, whatever order the answers come in;
// and that it is the full limit once the reset has passed, until GitHub
// reports the new window, whatever is left in it.
func TestRateStatusRemaining(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 3)}
		c := newAnswered(t, a)
		reset := time.Now().Add(time.Hour)
		a["x"] <- answerQuota(resourceCore, 5000, 10, reset)
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Fatalf("Get: %v", err)
		}
		shown := func() int {
			t.Helper()
			return coreQuota(t, c.RateStatus()).Remaining
		}

		first, second, third := getAsync(c, "x"), getAsync(c, "x"), getAsync(c, "x")
		if got := shown(); got != 10 {
			t.Errorf("with three in flight: Remaining = %d, want 10, what GitHub said", got)
		}
		// GitHub counted the third first, and the others' answers come
		// late, with more left.
		for i, left := range []int{7, 9, 8} {
			a["x"] <- answerQuota(resourceCore, 5000, left, reset)
			synctest.Wait()
			if got := shown(); got != 7 {
				t.Errorf("after %d answers: Remaining = %d, want 7", i+1, got)
			}
		}
		for _, done := range []<-chan error{first, second, third} {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
		// A 304 isn't counted, and gives back what it took.
		done := getAsync(c, "x")
		a["x"] <- answer{status: http.StatusNotModified, header: answerQuota(resourceCore, 5000, 7, reset).header}
		<-done
		if got := shown(); got != 7 {
			t.Errorf("after a 304: Remaining = %d, want 7", got)
		}

		// Past the reset the window has refilled, before GitHub says so.
		time.Sleep(time.Until(reset))
		next := getAsync(c, "x")
		if got := coreQuota(t, c.RateStatus()); got.Remaining != 5000 || !got.Reset.Equal(reset) {
			t.Errorf("past the reset, one in flight: Remaining, Reset = %d, %v; want 5000, %v", got.Remaining, got.Reset, reset)
		}
		a["x"] <- answerQuota(resourceCore, 5000, 4990, reset.Add(time.Hour))
		<-next
		if got := coreQuota(t, c.RateStatus()); got.Remaining != 4990 || !got.Reset.Equal(reset.Add(time.Hour)) {
			t.Errorf("once GitHub reports the new window: Remaining, Reset = %d, %v; want 4990, the new reset", got.Remaining, got.Reset)
		}
	})
}

// TestRateStatusNeverRises sends many requests at once, answered in any
// order, some with 304s, some failing, and checks that no two snapshots
// taken one after the other show more left within the same window.
func TestRateStatusNeverRises(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Hour)
		var mu sync.Mutex
		used, sent := 0, 0
		c := newNotified(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			sent++
			n := sent
			status := http.StatusOK
			switch n % 7 {
			case 0:
				mu.Unlock()
				return nil, io.ErrUnexpectedEOF
			case 3:
				// GitHub doesn't count a 304.
				status = http.StatusNotModified
			default:
				used++
			}
			left := 5000 - used
			mu.Unlock()
			// Answers are held up for a while, so they come back in
			// another order than GitHub counted them.
			time.Sleep(time.Duration(rand.IntN(50)) * time.Millisecond)
			h := make(http.Header)
			for k, v := range quotaHeader(resourceCore, 5000, left, reset) {
				h.Set(k, v)
			}
			return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
		}))
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				for range 20 {
					_, _ = c.Get(t.Context(), "x", Conditional{}, nil)
				}
			})
		}
		last := -1
		watching := make(chan struct{})
		go func() {
			defer close(watching)
			for range 400 {
				s := c.RateStatus()
				if i := slices.IndexFunc(s.Quotas, func(q core.Quota) bool { return q.Resource == resourceCore }); i >= 0 {
					if got := s.Quotas[i].Remaining; last >= 0 && got > last {
						t.Errorf("Remaining went up from %d to %d within a window", last, got)
					} else {
						last = got
					}
				}
				time.Sleep(time.Millisecond)
			}
		}()
		wg.Wait()
		<-watching
	})
}

// TestRateStatusLocalTimes answers from a GitHub whose clock is 5s ahead,
// with core spent, and checks that the reset and the release are in local
// time, and that the resource shows as limited only until its release.
func TestRateStatusLocalTimes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const skew = 5 * time.Second
		reset := time.Now().Add(skew + 10*time.Minute) // in GitHub's clock
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		a["x"] <- spent(resourceCore, reset, skew)
		_, _ = c.Get(t.Context(), "x", Conditional{}, nil)

		s := c.RateStatus()
		q := coreQuota(t, s)
		local := reset.Add(-skew)
		if !q.Reset.Equal(local) {
			t.Errorf("Reset = %v, want %v, in local time", q.Reset, local)
		}
		if want := local.Add(minGuard); !q.LimitedUntil.Equal(want) {
			t.Errorf("LimitedUntil = %v, want %v, the release", q.LimitedUntil, want)
		}
		if q.Remaining != 0 || q.Held != 0 || !q.SeenAt.Equal(time.Now()) || !s.At.Equal(time.Now()) {
			t.Errorf("quota = %+v at %v, want none left or held, seen now", q, s.At)
		}
		if !s.SecondaryUntil.IsZero() {
			t.Errorf("SecondaryUntil = %v, want zero", s.SecondaryUntil)
		}

		time.Sleep(time.Until(q.LimitedUntil))
		if q := coreQuota(t, c.RateStatus()); !q.LimitedUntil.IsZero() {
			t.Errorf("LimitedUntil after the release = %v, want zero", q.LimitedUntil)
		}
	})
}

// TestRateStatusContact checks that the status says when GitHub last
// answered and when a request last failed.
func TestRateStatusContact(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		if s := c.RateStatus(); !s.Answered.IsZero() || !s.Failed.IsZero() {
			t.Errorf("before any request: Answered, Failed = %v, %v; want zero", s.Answered, s.Failed)
		}
		a["x"] <- answer{status: http.StatusNotFound, header: map[string]string{"X-GitHub-Request-Id": "ABCD:1234"}}
		_, _ = c.Get(t.Context(), "x", Conditional{}, nil)
		answered := time.Now()

		time.Sleep(time.Second)
		_, _ = c.Get(t.Context(), "unanswered", Conditional{}, nil)
		s := c.RateStatus()
		if !s.Answered.Equal(answered) || !s.Failed.Equal(time.Now()) {
			t.Errorf("Answered, Failed = %v, %v; want %v, %v", s.Answered, s.Failed, answered, time.Now())
		}
	})
}

// TestRateStatusRejected checks that the status says GitHub rejected the
// token while its last answer did, and not once another is accepted.
func TestRateStatusRejected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		github := map[string]string{"X-GitHub-Request-Id": "ABCD:1234"}
		get := func(status int) {
			a["x"] <- answer{status: status, header: github}
			_, _ = c.Get(t.Context(), "x", Conditional{}, nil)
		}
		get(http.StatusUnauthorized)
		if s := c.RateStatus(); !s.Rejected.Equal(time.Now()) {
			t.Errorf("after a 401: Rejected = %v, want %v", s.Rejected, time.Now())
		}
		time.Sleep(time.Second)
		get(http.StatusNotFound)
		if s := c.RateStatus(); !s.Rejected.IsZero() {
			t.Errorf("after a 404: Rejected = %v, want zero", s.Rejected)
		}
	})
}

// failingClient is a client whose REST path x and GraphQL endpoint are
// answered as a test says, with what it was told of the rate limits.
type failingClient struct {
	*notified
	a answers
}

// githubAnswer is the header of an answer GitHub sent.
var githubAnswer = map[string]string{"X-GitHub-Request-Id": "ABCD:1234"}

func newFailingClient(t *testing.T) *failingClient {
	t.Helper()
	a := answers{"x": make(chan answer, 1), "graphql": make(chan answer, 1), "search/issues": make(chan answer, 1)}
	return &failingClient{notified: newNotified(t, a), a: a}
}

// get has a REST read answered with status and header, and query a
// GraphQL one. Background reads aren't sent again, so one answer ends
// each.
func (f *failingClient) get(t *testing.T, status int, header map[string]string) {
	t.Helper()
	f.a["x"] <- answer{status: status, header: header}
	_, _ = f.Get(obs.ForBackground(t.Context()), "x", Conditional{}, nil)
}

func (f *failingClient) query(t *testing.T, status int) {
	t.Helper()
	f.a["graphql"] <- answer{status: status, header: githubAnswer, body: `{"data":{}}`}
	_ = f.Query(obs.ForBackground(t.Context()), "query Q { x }", nil, nil)
}

// toldFailing returns since when the status said GitHub fails, the last time
// f was told of the rate limits.
func (f *failingClient) toldFailing() time.Time {
	_, s := f.last()
	return s.Failing
}

// TestRateStatusFailing checks that the status says GitHub fails once its
// answers, or a proxy's on the way to it, kept being server errors for
// failingAfter, since the first of them, which more don't move, and no
// longer once GitHub answered otherwise, with no error, for failingClear.
// Each change is told of with no answer to bring it.
func TestRateStatusFailing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFailingClient(t)
		f.get(t, http.StatusOK, githubAnswer)
		if s := f.RateStatus(); !s.Failing.IsZero() {
			t.Errorf("after a 200: Failing = %v, want zero", s.Failing)
		}
		time.Sleep(time.Second)
		f.get(t, http.StatusServiceUnavailable, githubAnswer)
		began := time.Now()
		if s := f.RateStatus(); !s.Failing.IsZero() {
			t.Errorf("right after a 503: Failing = %v, want zero until it lasts", s.Failing)
		}
		time.Sleep(failingAfter)
		if got := f.toldFailing(); !got.Equal(began) {
			t.Errorf("%v after a 503 with no answer since: told Failing = %v, want %v", failingAfter, got, began)
		}
		// A proxy's 502 fails too, and doesn't move when it began.
		time.Sleep(time.Second)
		f.get(t, http.StatusBadGateway, nil)
		if s := f.RateStatus(); !s.Failing.Equal(began) {
			t.Errorf("after a proxy's 502: Failing = %v, want %v", s.Failing, began)
		}
		last := time.Now()
		// An answer without GitHub's request id, such as a portal's page,
		// doesn't count toward ending it.
		time.Sleep(time.Second)
		f.get(t, http.StatusOK, nil)
		// GitHub answering otherwise, a 404 too, ends it only once no
		// error came for failingClear.
		time.Sleep(time.Second)
		f.get(t, http.StatusNotFound, githubAnswer)
		answered := time.Now()
		if s := f.RateStatus(); !s.Failing.Equal(began) {
			t.Errorf("right after GitHub's 404: Failing = %v, want %v until it lasts", s.Failing, began)
		}
		time.Sleep(time.Until(last.Add(failingClear)) - time.Second)
		if s := f.RateStatus(); !s.Failing.Equal(began) {
			t.Errorf("a second before failingClear: Failing = %v, want %v", s.Failing, began)
		}
		time.Sleep(time.Second)
		if _, s := f.last(); !s.Failing.IsZero() || !s.Mended.Equal(answered) {
			t.Errorf("failingClear after the last error: told Failing = %v and Mended = %v, want zero and %v, when GitHub answered", s.Failing, s.Mended, answered)
		}
	})
}

// TestRateStatusFailingBlip checks that a server error that a retry
// mends at once, or a few within failingAfter, never say GitHub fails.
func TestRateStatusFailingBlip(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFailingClient(t)
		f.get(t, http.StatusOK, githubAnswer)
		for range 3 {
			f.get(t, http.StatusBadGateway, nil)
			time.Sleep(time.Second)
			f.get(t, http.StatusOK, githubAnswer)
			time.Sleep(time.Second)
		}
		for range 60 {
			if s := f.RateStatus(); !s.Failing.IsZero() || !s.Mended.IsZero() {
				t.Fatalf("after blips: Failing = %v and Mended = %v, want both zero", s.Failing, s.Mended)
			}
			time.Sleep(time.Second)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, s := range f.told {
			if !s.Failing.IsZero() {
				t.Errorf("told Failing = %v after blips, want it never told", s.Failing)
			}
		}
	})
}

// TestRateStatusFailingNotAskedAgain checks that a run of server errors
// of a resource no request asks for again, such as a search the user
// doesn't run again, goes quiet failingClear after its last error once
// GitHub answered another resource well, without mending; that the next
// error of the resource takes it up again, since when it began; and that
// it mends once the resource answers well.
func TestRateStatusFailingNotAskedAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFailingClient(t)
		search := func(status int, header map[string]string) {
			f.a["search/issues"] <- answer{status: status, header: header}
			_, _ = f.Get(obs.ForBackground(t.Context()), "search/issues", Conditional{}, nil)
		}
		began := time.Now()
		for range 3 {
			search(http.StatusBadGateway, nil)
			time.Sleep(time.Second)
		}
		last := began.Add(2 * time.Second)
		time.Sleep(failingAfter)
		if got := f.toldFailing(); !got.Equal(began) {
			t.Fatalf("searches failing: told Failing = %v, want %v", got, began)
		}
		f.get(t, http.StatusOK, githubAnswer)
		if s := f.RateStatus(); !s.Failing.Equal(began) {
			t.Errorf("right after REST answered: Failing = %v, want %v until failingClear", s.Failing, began)
		}
		time.Sleep(time.Until(last.Add(failingClear)))
		if _, s := f.last(); !s.Failing.IsZero() || !s.Mended.IsZero() {
			t.Errorf("failingClear after the last error: told Failing = %v and Mended = %v, want both zero", s.Failing, s.Mended)
		}

		// Within failingResume of its last error, the next error takes it
		// up again: it shows by the same rules as a new streak, since when
		// it began.
		time.Sleep(failingResume - failingClear - time.Minute)
		search(http.StatusBadGateway, nil)
		if s := f.RateStatus(); !s.Failing.IsZero() {
			t.Errorf("right after a search failed again: Failing = %v, want zero until it lasts", s.Failing)
		}
		time.Sleep(failingAfter)
		if got := f.toldFailing(); !got.Equal(began) {
			t.Errorf("%v after a search failed again: told Failing = %v, want %v", failingAfter, got, began)
		}
		last = time.Now().Add(-failingAfter)
		search(http.StatusOK, githubAnswer)
		answered := time.Now()
		time.Sleep(time.Until(last.Add(failingClear)))
		if _, s := f.last(); !s.Failing.IsZero() || !s.Mended.Equal(answered) {
			t.Errorf("failingClear after the search failed again: told Failing = %v and Mended = %v, want zero and %v, when it answered", s.Failing, s.Mended, answered)
		}
		f.budget.mu.Lock()
		defer f.budget.mu.Unlock()
		if n := len(f.budget.failing); n != 0 {
			t.Errorf("%d runs of errors still kept, want none", n)
		}
	})
}

// lapseSearch has searches fail for failingAfter while REST answers
// well, until the streak lapses.
func (f *failingClient) lapseSearch(t *testing.T) {
	t.Helper()
	began := time.Now()
	f.search(t, http.StatusBadGateway, nil)
	time.Sleep(failingAfter)
	if got := f.toldFailing(); !got.Equal(began) {
		t.Fatalf("searches failing: told Failing = %v, want %v", got, began)
	}
	f.get(t, http.StatusOK, githubAnswer)
	time.Sleep(failingClear)
	if s := f.RateStatus(); !s.Failing.IsZero() {
		t.Fatalf("failingClear after the search failed: Failing = %v, want zero", s.Failing)
	}
}

func (f *failingClient) search(t *testing.T, status int, header map[string]string) {
	t.Helper()
	f.a["search/issues"] <- answer{status: status, header: header}
	_, _ = f.Get(obs.ForBackground(t.Context()), "search/issues", Conditional{}, nil)
}

// TestRateStatusFailingResumedBlip checks that a blip that takes up a
// streak that lapsed, mended at once, neither shows nor mends anything.
func TestRateStatusFailingResumedBlip(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFailingClient(t)
		f.lapseSearch(t)
		_, before := f.last()
		time.Sleep(10 * time.Minute)
		f.search(t, http.StatusBadGateway, nil)
		time.Sleep(time.Second)
		f.search(t, http.StatusOK, githubAnswer)
		for range 60 {
			if s := f.RateStatus(); !s.Failing.IsZero() || !s.Mended.Equal(before.Mended) {
				t.Fatalf("after a blip: Failing = %v and Mended = %v, want zero and %v", s.Failing, s.Mended, before.Mended)
			}
			time.Sleep(time.Second)
		}
		f.budget.mu.Lock()
		defer f.budget.mu.Unlock()
		if n := len(f.budget.failing); n != 0 {
			t.Errorf("%d runs of errors still kept, want none", n)
		}
	})
}

// TestRateStatusFailingResumeBound checks that an error more than
// failingResume after the last of a streak that lapsed starts a streak
// of its own.
func TestRateStatusFailingResumeBound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFailingClient(t)
		f.lapseSearch(t)
		time.Sleep(failingResume)
		again := time.Now()
		f.search(t, http.StatusBadGateway, nil)
		time.Sleep(failingAfter)
		if got := f.toldFailing(); !got.Equal(again) {
			t.Errorf("a search failing %v later: told Failing = %v, want %v", failingResume, got, again)
		}
	})
}

// TestRateStatusFailingPartial checks that GraphQL failing at each of
// its polls a minute apart, while REST answers well in between, tells
// the same start each time it shows, never mends, and mends once GraphQL
// answers well.
func TestRateStatusFailingPartial(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFailingClient(t)
		began := time.Now()
		for i := range 600 {
			if i%60 == 0 {
				f.query(t, http.StatusBadGateway)
			}
			f.get(t, http.StatusOK, githubAnswer)
			time.Sleep(time.Second)
			if s := f.RateStatus(); !s.Failing.IsZero() && !s.Failing.Equal(began) || !s.Mended.IsZero() {
				t.Fatalf("%v into the outage: Failing = %v and Mended = %v, want %v or zero, and zero", time.Since(began), s.Failing, s.Mended, began)
			}
		}
		f.mu.Lock()
		shown := 0
		for _, s := range f.told {
			if !s.Failing.IsZero() {
				shown++
			}
		}
		f.mu.Unlock()
		if shown == 0 {
			t.Error("never told GitHub fails while GraphQL failed")
		}
		f.query(t, http.StatusOK)
		time.Sleep(failingClear)
		if _, s := f.last(); !s.Failing.IsZero() || s.Mended.IsZero() {
			t.Errorf("failingClear after GraphQL answered well: told Failing = %v and Mended = %v, want zero and set", s.Failing, s.Mended)
		}
	})
}

// TestRateStatusRejectedNotByGitHub checks that a 401 without GitHub's
// request id, such as one from a proxy on the way, doesn't say GitHub
// rejected the token.
func TestRateStatusRejectedNotByGitHub(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		a["x"] <- answer{status: http.StatusUnauthorized}
		_, _ = c.Get(t.Context(), "x", Conditional{}, nil)
		if s := c.RateStatus(); !s.Rejected.IsZero() {
			t.Errorf("after a 401 without a request id: Rejected = %v, want zero", s.Rejected)
		}
	})
}

// notified is a client that keeps the status it reads each time it is
// told the rate limits changed, as the app does.
type notified struct {
	*Client
	mu   sync.Mutex
	told []core.RateStatus
}

func newNotified(t *testing.T, base http.RoundTripper) *notified {
	t.Helper()
	n := &notified{}
	c, err := New(WithBaseURL("https://api.github.com/"), WithToken("t"), WithHTTPClient(&http.Client{Transport: base}),
		WithRateNotify(func() {
			// Read under no lock of the client's, or this would deadlock.
			s := n.RateStatus()
			n.mu.Lock()
			n.told = append(n.told, s)
			n.mu.Unlock()
		}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	n.Client = retryAtOnce(c)
	return n
}

// last returns how many times n was told, and the status it read last.
func (n *notified) last() (int, core.RateStatus) {
	synctest.Wait()
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.told) == 0 {
		return 0, core.RateStatus{}
	}
	return len(n.told), n.told[len(n.told)-1]
}

// TestRateNotifyThrottle answers many requests within a second, and
// checks that they are told of once at once and once a second later,
// with the last state.
func TestRateNotifyThrottle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newNotified(t, a)
		reset := time.Now().Add(time.Hour)
		for i := range 50 {
			a["x"] <- answerQuota(resourceCore, 100, 99-i, reset)
			if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
				t.Fatalf("Get: %v", err)
			}
		}
		if n, s := c.last(); n != 1 || coreQuota(t, s).Remaining != 99 {
			t.Fatalf("told %d times, last of %+v; want once, of the first answer", n, s.Quotas)
		}
		time.Sleep(notifyEvery)
		if n, s := c.last(); n != 2 || coreQuota(t, s).Remaining != 50 {
			t.Fatalf("a second later: told %d times, last of %+v; want twice, of the last answer", n, s.Quotas)
		}
		time.Sleep(time.Minute)
		if n, _ := c.last(); n != 2 {
			t.Errorf("with no change since: told %d times, want 2", n)
		}
	})
}

// TestRateNotifyChanges goes through the changes that are told of, and
// some that aren't, a while apart so that none waits for the throttle.
func TestRateNotifyChanges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newNotified(t, a)
		reset := time.Now().Add(30 * time.Minute)
		told := 0
		step := func(name string, change bool, ans *answer, path string) {
			t.Helper()
			time.Sleep(time.Minute)
			if ans != nil {
				a["x"] <- *ans
			}
			if path != "" {
				_, _ = c.Get(t.Context(), path, Conditional{}, nil)
			}
			if change {
				told++
			}
			if n, _ := c.last(); n != told {
				t.Errorf("%s: told %d times, want %d", name, n, told)
				told = n
			}
		}
		ans := func(a answer) *answer { return &a }

		step("first quota", true, ans(answerQuota(resourceCore, 5000, 4990, reset)), "x")
		step("less left, same percent", false, ans(answerQuota(resourceCore, 5000, 4960, reset)), "x")
		step("less left, a percent less", true, ans(answerQuota(resourceCore, 5000, 4949, reset)), "x")
		step("another resource", true, ans(answerQuota(resourceSearch, 30, 30, reset)), "x")
		// Near enough to be real.
		reset = reset.Add(30 * time.Minute)
		step("a new window", true, ans(answerQuota(resourceCore, 5000, 4949, reset)), "x")
		step("spent", true, ans(spent(resourceCore, reset, 0)), "x")
		// The window refills at its reset, and the resource is released
		// a guard later, each with no answer.
		spentQuota := coreQuota(t, c.RateStatus())
		time.Sleep(time.Until(spentQuota.Reset))
		told++
		if n, s := c.last(); n != told || coreQuota(t, s).Remaining != 5000 || coreQuota(t, s).LimitedUntil.IsZero() {
			t.Errorf("at the reset: told %d times, last of %+v; want %d, the full limit, still limited", n, s.Quotas, told)
			told = n
		}
		time.Sleep(time.Until(spentQuota.LimitedUntil))
		told++
		if n, s := c.last(); n != told || !coreQuota(t, s).LimitedUntil.IsZero() {
			t.Errorf("at the release: told %d times, last of %+v; want %d, not limited", n, s.Quotas, told)
			told = n
		}
		// Until an answer shows the new window, the gate holds each
		// request for a probe of the limits, which is told of too.
		reset = reset.Add(time.Hour)
		step("held for a probe", true, ans(answerQuota(resourceCore, 5000, 5000, reset)), "x")
		time.Sleep(notifyEvery)
		told++
		if n, s := c.last(); n != told || coreQuota(t, s).Held != 0 || coreQuota(t, s).Remaining != 5000 {
			t.Errorf("let go in the new window: told %d times, last of %+v; want %d, none held", n, s.Quotas, told)
			told = n
		}
		step("offline", true, nil, "unanswered")
		step("still offline", false, nil, "unanswered")
		step("online", true, ans(answerQuota(resourceCore, 5000, 4999, reset)), "x")
		step("still online", false, ans(answerQuota(resourceCore, 5000, 4998, reset)), "x")
	})
}

// TestRateNotifyClose checks that a change waiting for the throttle isn't
// told of once the client is closed, nor a release.
func TestRateNotifyClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newNotified(t, a)
		reset := time.Now().Add(time.Hour)
		a["x"] <- answerQuota(resourceCore, 100, 50, reset)
		_, _ = c.Get(t.Context(), "x", Conditional{}, nil)
		a["x"] <- spent(resourceCore, reset, 0)
		_, _ = c.Get(t.Context(), "x", Conditional{}, nil)
		c.Close()
		time.Sleep(2 * time.Hour)
		if n, _ := c.last(); n != 1 {
			t.Errorf("told %d times, want only before Close", n)
		}
		a["x"] <- answerQuota(resourceCore, 100, 100, reset.Add(time.Hour))
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Errorf("Get after Close: %v", err)
		}
		if n, _ := c.last(); n != 1 {
			t.Errorf("after a request: told %d times, want 1", n)
		}
	})
}

// TestRateStatusConcurrent sends many requests at once while reading the
// status, for the race detector, and checks what is told once they end.
func TestRateStatusConcurrent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const workers, each = 64, 20
		reset := time.Now().Add(time.Hour)
		var sent atomic.Int64
		c := newNotified(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			n := sent.Add(1)
			resource := resourceCore
			if strings.HasPrefix(req.URL.Path, "/search/") {
				resource = resourceSearch
			}
			h := make(http.Header)
			for k, v := range quotaHeader(resource, 5000, 5000-int(n), reset) {
				h.Set(k, v)
			}
			h.Set("X-GitHub-Request-Id", strconv.FormatInt(n, 10))
			return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
		}))
		var wg sync.WaitGroup
		for i := range workers {
			wg.Go(func() {
				path := "x"
				if i%2 == 0 {
					path = "search/issues"
				}
				for range each {
					_, _ = c.Get(t.Context(), path, Conditional{}, nil)
					s := c.RateStatus()
					if !slices.IsSortedFunc(s.Quotas, func(x, y core.Quota) int {
						return resourceRank(x.Resource) - resourceRank(y.Resource)
					}) {
						t.Errorf("Quotas out of order: %+v", s.Quotas)
					}
				}
			})
		}
		wg.Wait()
		if n, _ := c.last(); n > 2 {
			t.Errorf("told %d times at one instant, want at most twice", n)
		}
		time.Sleep(notifyEvery)
		_, s := c.last()
		if len(s.Quotas) != 2 {
			t.Fatalf("Quotas = %+v, want core and search", s.Quotas)
		}
		// Each resource's lowest answer wins, whatever the order they came
		// in; what is left is below the last sent's by up to its count.
		for _, q := range s.Quotas {
			if q.Held != 0 || q.Remaining > 5000-workers*each/2 || q.Remaining < 5000-workers*each {
				t.Errorf("%s: Remaining %d, Held %d; want at most %d, none held", q.Resource, q.Remaining, q.Held, 5000-workers*each/2)
			}
		}
		if want := c.RateStatus(); !slices.Equal(want.Quotas, s.Quotas) {
			t.Errorf("last told %+v, want the final %+v", s.Quotas, want.Quotas)
		}
	})
}

// TestRateStatusHeld checks that the requests held for a limit show in
// the status, and that holding one and letting it go are told of.
func TestRateStatusHeld(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5, window: time.Minute}
		c := newNotified(t, h)
		c.budget.gate.jitter = func() float64 { return 0 }
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c.Client, h, reset)
		time.Sleep(2 * time.Second)
		told, _ := c.last()

		done := goAsync(func() error {
			_, err := c.Get(obs.ForBackground(t.Context()), "repos/o/r", Conditional{}, nil)
			return err
		})
		n, s := c.last()
		if n != told+1 || coreQuota(t, s).Held != 1 || coreQuota(t, c.RateStatus()).Held != 1 {
			t.Errorf("once held: told %d times, last of %+v; want %d, 1 held", n, s.Quotas, told+1)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		time.Sleep(notifyEvery)
		if n, s := c.last(); n <= told+1 || coreQuota(t, s).Held != 0 {
			t.Errorf("once let go: told %d times, last of %+v; want more than %d, none held", n, s.Quotas, told+1)
		}
	})
}

// TestRateStatusSecondary checks that a secondary limit shows in the
// status until it lifts, and that both are told of.
func TestRateStatusSecondary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5000, window: time.Hour}
		c := newNotified(t, h)
		h.secondary("30")
		lifts := time.Now().Add(30 * time.Second)
		_, _ = c.Get(t.Context(), "user", Conditional{}, nil)
		time.Sleep(notifyEvery)
		n, s := c.last()
		if !s.SecondaryUntil.Equal(lifts) || !c.RateStatus().SecondaryUntil.Equal(lifts) {
			t.Errorf("SecondaryUntil = %v, want %v", s.SecondaryUntil, lifts)
		}
		time.Sleep(time.Until(lifts))
		if m, s := c.last(); m != n+1 || !s.SecondaryUntil.IsZero() {
			t.Errorf("once it lifted: told %d times, SecondaryUntil %v; want %d, none", m, s.SecondaryUntil, n+1)
		}
	})
}
