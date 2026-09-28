package github

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// step is how a scripted transport answers one attempt, after delay: with
// err, by hanging until the attempt is done, or with a response.
type step struct {
	delay  time.Duration
	err    error
	hang   bool
	status int
	header http.Header
	body   string
}

var (
	errDial  = &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	errReset = &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	errCert  = &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}
	// errProxyDial is how the transport says that it couldn't dial the
	// proxy of HTTPS_PROXY.
	errProxyDial = &net.OpError{Op: "proxyconnect", Net: "tcp", Err: errDial}

	ok          = step{status: http.StatusOK, body: `{}`}
	dialFails   = step{err: errDial}
	resets      = step{err: errReset}
	hangs       = step{hang: true}
	badGateway  = step{status: http.StatusBadGateway, body: `{"message":"Server Error"}`}
	unavailable = step{status: http.StatusServiceUnavailable}
	gwTimeout   = step{status: http.StatusGatewayTimeout}
	serverError = step{status: http.StatusInternalServerError}
	notFound    = step{status: http.StatusNotFound, body: `{"message":"Not Found"}`}
	// wentWrong is how GitHub answers a query that failed on its side.
	wentWrong  = step{status: http.StatusOK, body: `{"data":null,"errors":[{"message":"Something went wrong while executing your query."}]}`}
	missing    = step{status: http.StatusOK, body: `{"data":null,"errors":[{"type":"NOT_FOUND","message":"Could not resolve"}]}`}
	partial    = step{status: http.StatusOK, body: `{"data":{"x":1},"errors":[{"message":"Something went wrong"}]}`}
	answered   = step{status: http.StatusOK, body: `{"data":{"x":1}}`}
	secondary3 = step{status: http.StatusForbidden, header: http.Header{"Retry-After": {"3"}},
		body: `{"message":"You have exceeded a secondary rate limit."}`}
	secondary30 = step{status: http.StatusForbidden, header: http.Header{"Retry-After": {"30"}},
		body: `{"message":"You have exceeded a secondary rate limit."}`}
	secondary10 = step{status: http.StatusForbidden, header: http.Header{"Retry-After": {"10"}}}
	// huge went wrong too, but is too large to look at.
	huge     = step{status: http.StatusOK, body: `{"data":null,"errors":[{"message":"` + strings.Repeat("x", maxPeek) + `"}]}`}
	tooMany3 = step{status: http.StatusTooManyRequests, header: http.Header{"Retry-After": {"3"}}}
	primary  = step{status: http.StatusForbidden, header: http.Header{
		"Retry-After": {"3"}, "X-Ratelimit-Limit": {"5000"}, "X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {"1790000000"},
	}}
)

// slow returns st after 4s, which leaves time to send the request again.
func slow(st step) step {
	st.delay = 4 * time.Second
	return st
}

// script is a transport that answers the attempts at requests with its
// steps in turn, the last one again once they run out, or with any of them
// if random is set, and keeps when each attempt came and the body it sent.
type script struct {
	steps  []step
	random bool

	mu     sync.Mutex
	times  []time.Time
	bodies []string
}

func (s *script) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}
	s.mu.Lock()
	n := len(s.times)
	s.times = append(s.times, time.Now())
	s.bodies = append(s.bodies, string(body))
	st := s.steps[min(n, len(s.steps)-1)]
	if s.random {
		st = s.steps[rand.IntN(len(s.steps))]
	}
	s.mu.Unlock()
	time.Sleep(st.delay)
	switch {
	case st.err != nil:
		return nil, st.err
	case st.hang:
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	h := st.header.Clone()
	if h == nil {
		h = http.Header{}
	}
	return &http.Response{StatusCode: st.status, Header: h, Body: io.NopCloser(strings.NewReader(st.body)), Request: req}, nil
}

// attempts returns how many attempts came, and the time between each
// attempt and the next.
func (s *script) attempts() (int, []time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var gaps []time.Duration
	for i := 1; i < len(s.times); i++ {
		gaps = append(gaps, s.times[i].Sub(s.times[i-1]))
	}
	return len(s.times), gaps
}

// scriptedClient returns a client whose requests s answers, with the
// whole chain of transports, the default timeout per attempt, and the
// backoff without its jitter, and the gate's stagger at its least.
func scriptedClient(t *testing.T, s http.RoundTripper) *Client {
	t.Helper()
	c, err := New(WithBaseURL("https://gh.test/"), WithToken("t"),
		WithHTTPClient(&http.Client{Timeout: defaultTimeout, Transport: s}))
	if err != nil {
		t.Fatal(err)
	}
	c.http.Transport.(*retryTransport).jitter = func() float64 { return 0.5 }
	c.budget.gate.jitter = func() float64 { return 0 }
	return c
}

// Requests the tests send.
var (
	get = func(ctx context.Context, c *Client) error {
		var v any
		_, err := c.Get(ctx, "repos/o/r", Conditional{}, &v)
		return err
	}
	post = func(ctx context.Context, c *Client) error {
		_, err := c.Do(ctx, http.MethodPost, "repos/o/r/issues", map[string]string{"title": "t"}, nil)
		return err
	}
	query = func(ctx context.Context, c *Client) error {
		var v any
		return c.Query(ctx, "query Q { x }", nil, &v)
	}
	mutation = func(ctx context.Context, c *Client) error {
		var v any
		return c.Query(ctx, "mutation M { x }", nil, &v)
	}
	prefetch = func(ctx context.Context, c *Client) error {
		return get(obs.ForPrefetch(ctx), c)
	}
	background = func(ctx context.Context, c *Client) error {
		return query(obs.ForBackground(ctx), c)
	}
)

func TestRetryPolicy(t *testing.T) {
	const (
		first  = 250 * time.Millisecond
		second = time.Second
	)
	tests := []struct {
		name  string
		send  func(context.Context, *Client) error
		steps []step
		// gaps are the times between attempts, so one fewer than the
		// attempts.
		gaps    []time.Duration
		wantErr bool
	}{
		{name: "get dial fails, then works", send: get, steps: []step{dialFails, ok}, gaps: []time.Duration{first}},
		{name: "get dial keeps failing", send: get, steps: []step{dialFails}, gaps: []time.Duration{first, second}, wantErr: true},
		{name: "get connection reset", send: get, steps: []step{resets, resets, ok}, gaps: []time.Duration{first, second}},
		{name: "get times out", send: get, steps: []step{hangs, ok}, wantErr: true},
		{name: "get slow 502", send: get, steps: []step{slow(badGateway), ok}, gaps: []time.Duration{4*time.Second + first}},
		{name: "get slow 502 twice", send: get, steps: []step{slow(badGateway)}, gaps: []time.Duration{4*time.Second + first}, wantErr: true},
		{name: "get 502 after 6s", send: get, steps: []step{{delay: 6 * time.Second, status: http.StatusBadGateway}, ok}, wantErr: true},
		{name: "get proxy dial fails", send: get, steps: []step{{err: errProxyDial}, ok}, gaps: []time.Duration{first}},
		{name: "get 502", send: get, steps: []step{badGateway, ok}, gaps: []time.Duration{first}},
		{name: "get 503", send: get, steps: []step{unavailable, unavailable, ok}, gaps: []time.Duration{first, second}},
		{name: "get 504 keeps failing", send: get, steps: []step{gwTimeout}, gaps: []time.Duration{first, second}, wantErr: true},
		{name: "get 500", send: get, steps: []step{serverError}, wantErr: true},
		{name: "get 404", send: get, steps: []step{notFound}, wantErr: true},
		{name: "get untrusted certificate", send: get, steps: []step{{err: errCert}}, wantErr: true},
		{name: "get secondary limit waits once", send: get, steps: []step{secondary3, ok}, gaps: []time.Duration{3*time.Second + minStagger}},
		{name: "get secondary limit twice", send: get, steps: []step{secondary3}, gaps: []time.Duration{3*time.Second + minStagger}, wantErr: true},
		{name: "get 429 waits once", send: get, steps: []step{tooMany3, ok}, gaps: []time.Duration{3*time.Second + minStagger}},
		{name: "get 503 after a secondary limit", send: get, steps: []step{secondary3, unavailable, ok}, gaps: []time.Duration{3*time.Second + minStagger, second}},
		{name: "get slow secondary limit", send: get, steps: []step{slow(secondary3), ok}, gaps: []time.Duration{4*time.Second + 3*time.Second + minStagger}},
		{name: "get secondary limit after 6s", send: get, steps: []step{{delay: 6 * time.Second, status: http.StatusForbidden, header: secondary3.header}, ok}, wantErr: true},
		{name: "get 503 after a long secondary limit", send: get, steps: []step{secondary10, unavailable, ok}, gaps: []time.Duration{10*time.Second + minStagger}, wantErr: true},
		{name: "get long secondary limit", send: get, steps: []step{secondary30}, wantErr: true},
		{name: "get primary limit", send: get, steps: []step{primary}, wantErr: true},
		{name: "query went wrong", send: query, steps: []step{wentWrong, answered}, gaps: []time.Duration{first}},
		{name: "query keeps going wrong", send: query, steps: []step{wentWrong}, gaps: []time.Duration{first, second}, wantErr: true},
		{name: "query 502", send: query, steps: []step{badGateway, answered}, gaps: []time.Duration{first}},
		{name: "query dial fails", send: query, steps: []step{dialFails, answered}, gaps: []time.Duration{first}},
		{name: "query not found", send: query, steps: []step{missing}, wantErr: true},
		{name: "query partial data", send: query, steps: []step{partial}, wantErr: true},
		{name: "query too large to peek", send: query, steps: []step{huge}, wantErr: true},
		{name: "query secondary limit", send: query, steps: []step{secondary3, answered}, gaps: []time.Duration{3*time.Second + minStagger}},
		{name: "mutation dial fails", send: mutation, steps: []step{dialFails, answered}, gaps: []time.Duration{first}},
		{name: "mutation dial keeps failing", send: mutation, steps: []step{dialFails}, gaps: []time.Duration{first, second}, wantErr: true},
		{name: "mutation proxy dial fails", send: mutation, steps: []step{{err: errProxyDial}, answered}, gaps: []time.Duration{first}},
		{name: "mutation proxy refuses", send: mutation, steps: []step{{err: &net.OpError{Op: "proxyconnect", Net: "tcp", Err: errReset}}}, wantErr: true},
		{name: "mutation went wrong", send: mutation, steps: []step{wentWrong}, wantErr: true},
		{name: "mutation 502", send: mutation, steps: []step{badGateway}, wantErr: true},
		{name: "mutation connection reset", send: mutation, steps: []step{resets}, wantErr: true},
		{name: "mutation times out", send: mutation, steps: []step{hangs}, wantErr: true},
		{name: "mutation secondary limit", send: mutation, steps: []step{secondary3}, wantErr: true},
		{name: "post dial fails", send: post, steps: []step{dialFails, ok}, gaps: []time.Duration{first}},
		{name: "post 503", send: post, steps: []step{unavailable}, wantErr: true},
		{name: "post times out", send: post, steps: []step{hangs}, wantErr: true},
		{name: "prefetch dial fails", send: prefetch, steps: []step{dialFails}, wantErr: true},
		{name: "prefetch 502", send: prefetch, steps: []step{badGateway}, wantErr: true},
		{name: "prefetch secondary limit", send: prefetch, steps: []step{secondary3}, wantErr: true},
		{name: "background dial fails", send: background, steps: []step{dialFails, answered}, wantErr: true},
		{name: "background went wrong", send: background, steps: []step{wentWrong, answered}, wantErr: true},
		{name: "background secondary limit", send: background, steps: []step{secondary3, answered}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := &script{steps: tt.steps}
				err := tt.send(t.Context(), scriptedClient(t, s))
				if (err != nil) != tt.wantErr {
					t.Errorf("err = %v, want an error %v", err, tt.wantErr)
				}
				n, gaps := s.attempts()
				if n != len(tt.gaps)+1 || !slices.Equal(gaps, tt.gaps) {
					t.Errorf("%d attempts %v apart, want %d %v apart", n, gaps, len(tt.gaps)+1, tt.gaps)
				}
			})
		})
	}
}

// What a request comes to after its retries is what its last attempt
// came to.
func TestRetryKeepsTheLastFailure(t *testing.T) {
	tests := []struct {
		name  string
		send  func(context.Context, *Client) error
		steps []step
		kind  core.ProblemKind
	}{
		{"dial", get, []step{dialFails}, core.Offline},
		{"timeout", get, []step{hangs}, core.Offline},
		{"503", get, []step{unavailable}, core.Unavailable},
		{"secondary limit", get, []step{secondary3}, core.RateLimited},
		{"query went wrong", query, []step{wentWrong}, core.Internal},
		{"mutation timeout", mutation, []step{hangs}, core.Offline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				err := tt.send(t.Context(), scriptedClient(t, &script{steps: tt.steps}))
				if p := core.Explain("load it", err); p.Kind != tt.kind {
					t.Errorf("Explain(%v) = %v, want %v", err, p.Kind, tt.kind)
				}
			})
		})
	}
}

// A read that times out every time takes its timeout once, not once per
// attempt.
func TestRetryTimesOutOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &script{steps: []step{hangs}}
		start := time.Now()
		err := get(t.Context(), scriptedClient(t, s))
		if d := time.Since(start); d != defaultTimeout {
			t.Errorf("the read failed after %v, want %v", d, defaultTimeout)
		}
		if !errors.Is(err, core.ErrOffline) {
			t.Errorf("err = %v, want core.ErrOffline", err)
		}
	})
}

// A query whose answer is too large to look at passes through whole.
func TestRetryPassesLargeBodies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		big := strings.Repeat("x", maxPeek)
		s := &script{steps: []step{{status: http.StatusOK, body: `{"data":{"x":"` + big + `"}}`}}}
		var v struct {
			X string `json:"x"`
		}
		if err := scriptedClient(t, s).Query(t.Context(), "query Q { x }", nil, &v); err != nil {
			t.Fatal(err)
		}
		if v.X != big {
			t.Errorf("decoded %d bytes of x, want %d", len(v.X), len(big))
		}
	})
}

// The wait between attempts is the backoff give or take a fifth.
func TestRetryJitter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for range 50 {
			s := &script{steps: []step{dialFails}}
			c := scriptedClient(t, s)
			c.http.Transport.(*retryTransport).jitter = rand.Float64
			_ = get(t.Context(), c)
			_, gaps := s.attempts()
			for i, gap := range gaps {
				lo, hi := retryBackoff[i]*8/10, retryBackoff[i]*12/10
				if gap < lo || gap > hi {
					t.Errorf("wait %d is %v, want within [%v, %v]", i+1, gap, lo, hi)
				}
			}
		}
	})
}

// A request whose caller gives up during a backoff ends then.
func TestRetryCancelDuringBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &script{steps: []step{dialFails}}
		c := scriptedClient(t, s)
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(100*time.Millisecond, cancel)
		start := time.Now()
		err := get(ctx, c)
		if d := time.Since(start); d != 100*time.Millisecond {
			t.Errorf("the request ended after %v, want 100ms", d)
		}
		if !errors.Is(err, context.Canceled) || errors.Is(err, core.ErrOffline) {
			t.Errorf("err = %v, want context.Canceled without core.ErrOffline", err)
		}
		if n, _ := s.attempts(); n != 1 {
			t.Errorf("%d attempts, want 1", n)
		}
	})
}

// Each attempt sends the whole body again.
func TestRetryReplaysBody(t *testing.T) {
	tests := []struct {
		name  string
		send  func(context.Context, *Client) error
		steps []step
		want  string
	}{
		{"post", post, []step{dialFails, dialFails, ok}, `{"title":"t"}`},
		{"query", query, []step{badGateway, wentWrong, answered}, `{"query":"query Q { x }"}`},
		{"mutation", mutation, []step{dialFails, answered}, `{"query":"mutation M { x }"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := &script{steps: tt.steps}
				if err := tt.send(t.Context(), scriptedClient(t, s)); err != nil {
					t.Fatal(err)
				}
				if len(s.bodies) != len(tt.steps) {
					t.Fatalf("%d attempts, want %d", len(s.bodies), len(tt.steps))
				}
				for i, b := range s.bodies {
					if b != tt.want {
						t.Errorf("attempt %d sent %q, want %q", i+1, b, tt.want)
					}
				}
			})
		})
	}
}

// A body that can't be read again is sent once.
func TestRetryNeedsAReplayableBody(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &script{steps: []step{dialFails, ok}}
		c := scriptedClient(t, s)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, "https://gh.test/x", io.NopCloser(strings.NewReader("once")))
		if err != nil {
			t.Fatal(err)
		}
		if resp, err := c.http.Do(req); err == nil {
			_ = resp.Body.Close()
			t.Fatal("Do succeeded, want the dial's error")
		}
		if n, _ := s.attempts(); n != 1 {
			t.Errorf("%d attempts, want 1", n)
		}
	})
}

// Every attempt is logged with a request_id of its own, and each decision
// to send a request again is logged and counted.
func TestRetryLogs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, stats := captureLog(t, slog.LevelDebug)
		s := &script{steps: []step{unavailable, dialFails, ok}}
		if err := get(t.Context(), scriptedClient(t, s)); err != nil {
			t.Fatal(err)
		}
		recs := httpRecords(t, buf)
		ids := make(map[any]bool)
		for _, r := range recs {
			ids[r["request_id"]] = true
		}
		if len(recs) != 3 || len(ids) != 3 {
			t.Errorf("got %d records with %d request ids, want 3 of each:\n%s", len(recs), len(ids), buf)
		}
		var retries []string
		for line := range strings.Lines(buf.String()) {
			if strings.Contains(line, `"msg":"http retry"`) {
				retries = append(retries, line)
			}
		}
		if len(retries) != 2 ||
			!strings.Contains(retries[0], `"level":"INFO"`) ||
			!strings.Contains(retries[0], `"attempt":1,"reason":"unavailable","status":503,"delay_ms":250`) ||
			!strings.Contains(retries[1], `"attempt":2,"reason":"dial","err":`) ||
			!strings.Contains(retries[1], `"delay_ms":1000`) {
			t.Errorf("retry records = %q, want one for each of the first two attempts", retries)
		}
		if got := stats.Summary().Retries; got["unavailable"] != 1 || got["dial"] != 1 || len(got) != 2 {
			t.Errorf("retries = %v, want 1 unavailable and 1 dial", got)
		}
		for i, r := range recs {
			want := any(float64(i + 1))
			if i == 0 {
				want = nil
			}
			if r["attempt"] != want {
				t.Errorf("attempt %d logs attempt %v, want %v", i+1, r["attempt"], want)
			}
		}
	})
}

// records returns the records in buf with the message msg.
func records(t *testing.T, buf *bytes.Buffer, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", line, err)
		}
		if m["msg"] == msg {
			out = append(out, m)
		}
	}
	return out
}

// A request that fails every attempt logs that it gave up, at warn level,
// and why.
func TestRetryLogsGivingUp(t *testing.T) {
	tests := []struct {
		name     string
		steps    []step
		attempt  float64
		reason   string
		stop     string
		giveUps  int
		statusOK bool
	}{
		{name: "attempts", steps: []step{unavailable}, attempt: 3, reason: "unavailable", stop: stopAttempts, giveUps: 1, statusOK: true},
		{name: "time", steps: []step{{delay: 6 * time.Second, status: http.StatusBadGateway}}, attempt: 1, reason: "unavailable", stop: stopTime, giveUps: 1, statusOK: true},
		{name: "no retry", steps: []step{notFound}},
		{name: "success", steps: []step{unavailable, ok}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				buf, _ := captureLog(t, slog.LevelInfo)
				_ = get(t.Context(), scriptedClient(t, &script{steps: tt.steps}))
				recs := records(t, buf, "http retry gave up")
				if len(recs) != tt.giveUps {
					t.Fatalf("%d give-up records, want %d:\n%s", len(recs), tt.giveUps, buf)
				}
				if tt.giveUps == 0 {
					return
				}
				r := recs[0]
				if r["level"] != "WARN" || r["attempt"] != tt.attempt || r["reason"] != tt.reason || r["stop"] != tt.stop ||
					r["method"] != "GET" || r["elapsed_ms"] == nil || (r["status"] != nil) != tt.statusOK {
					t.Errorf("give-up record = %v", r)
				}
			})
		})
	}
}

// An outage that fails every request logs a retry and giving up once per
// reason at info level and above, and says how many it held back.
func TestRetryLogsThrottled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, _ := captureLog(t, slog.LevelInfo)
		c := scriptedClient(t, &script{steps: []step{dialFails}})
		for range 20 {
			_ = get(t.Context(), c)
		}
		if n := len(records(t, buf, "http retry")); n != 1 {
			t.Errorf("%d retry records for 40 retries, want 1", n)
		}
		if n := len(records(t, buf, "http retry gave up")); n != 1 {
			t.Errorf("%d give-up records for 20 requests, want 1", n)
		}
		time.Sleep(retryLogEvery)
		buf.Reset()
		_ = get(t.Context(), c)
		retries := records(t, buf, "http retry")
		if len(retries) != 1 || retries[0]["suppressed"] != float64(39) {
			t.Errorf("retry records after a minute = %v, want 1 that held back 39", retries)
		}
		if gave := records(t, buf, "http retry gave up"); len(gave) != 1 || gave[0]["suppressed"] != float64(19) {
			t.Errorf("give-up records after a minute = %v, want 1 that held back 19", gave)
		}
	})
}

// An attempt sent again is logged at debug level, and the last attempt at
// the level of its failure, so that a request logs one record at info
// level or above however often it was sent.
func TestRetryLogsLastAttemptOnly(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
		level string
	}{
		{name: "unavailable", steps: []step{unavailable}, level: "ERROR"},
		{name: "dial", steps: []step{dialFails}, level: "ERROR"},
		{name: "secondary limit", steps: []step{secondary3, notFound}, level: "WARN"},
		{name: "recovers", steps: []step{badGateway, resets, ok}, level: "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				buf, _ := captureLog(t, slog.LevelDebug)
				_ = get(t.Context(), scriptedClient(t, &script{steps: tt.steps}))
				recs := httpRecords(t, buf)
				if len(recs) < 2 {
					t.Fatalf("%d http records, want one per attempt:\n%s", len(recs), buf)
				}
				last := len(recs) - 1
				for i, r := range recs[:last] {
					if r["level"] != "DEBUG" {
						t.Errorf("attempt %d, sent again, logged at %v, want DEBUG", i+1, r["level"])
					}
				}
				if recs[last]["level"] != tt.level {
					t.Errorf("the last attempt logged at %v, want %s", recs[last]["level"], tt.level)
				}
			})
		})
	}
}

// An outage that fails every request, those sent again and the polls,
// which never are, logs one record at info level or above for each
// request, and one retry and one giving up for each reason, however many
// requests fail at once.
func TestRetryLogsOutageVolume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, _ := captureLog(t, slog.LevelInfo)
		c := scriptedClient(t, &script{steps: []step{dialFails, unavailable, resets}, random: true})
		const reads, polls = 30, 30
		var wg sync.WaitGroup
		for range reads {
			wg.Go(func() { _ = get(t.Context(), c) })
		}
		for range polls {
			wg.Go(func() { _ = get(obs.ForBackground(t.Context()), c) })
		}
		wg.Wait()
		counts := make(map[string]int)
		for line := range strings.Lines(buf.String()) {
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("line %q is not JSON: %v", line, err)
			}
			counts[m["msg"].(string)]++
		}
		if n := counts["http"]; n != reads+polls {
			t.Errorf("%d http records at info level or above for %d requests, want one each", n, reads+polls)
		}
		// Three reasons at most, each logged once a minute.
		if n := counts["http retry"]; n > 3 {
			t.Errorf("%d retry records, want one per reason", n)
		}
		if n := counts["http retry gave up"]; n > 3 {
			t.Errorf("%d give-up records, want one per reason", n)
		}
		delete(counts, "http")
		delete(counts, "http retry")
		delete(counts, "http retry gave up")
		if len(counts) > 0 {
			t.Errorf("other records during the outage: %v", counts)
		}
	})
}

// A request sent again after a redirect logs its attempts sent again at
// debug level and its last attempt once, with all that a record says of
// a request: where it was redirected from, the attempt, that it was
// conditional, the page links and media type, and its query without what
// the user typed. The deprecation of its route is logged once.
func TestRetryLogsLastRecordWhole(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, _ := captureLog(t, slog.LevelDebug)
		moved := step{status: http.StatusMovedPermanently, header: http.Header{
			"Location": {"https://gh.test/repos/o/r2/issues?q=secret+plans&per_page=30"},
		}}
		last := step{status: http.StatusOK, body: `[]`, header: http.Header{
			"Link":                {`<https://gh.test/repos/o/r2/issues?page=2>; rel="next"`},
			"X-Github-Media-Type": {"github.v3; format=json"},
			"Deprecation":         {"true"},
			"X-Github-Request-Id": {"ABCD:1"},
		}}
		c := scriptedClient(t, &script{steps: []step{moved, unavailable, last}})
		var v any
		if _, err := c.Get(t.Context(), "repos/o/r/issues", Conditional{ETag: `"v1"`}, &v); err != nil {
			t.Fatal(err)
		}
		recs := httpRecords(t, buf)
		if len(recs) != 3 {
			t.Fatalf("%d http records, want the redirect, the attempt sent again and the last:\n%s", len(recs), buf)
		}
		if recs[1]["level"] != "DEBUG" || recs[1]["status"] != 503.0 {
			t.Errorf("attempt sent again = %v, want the 503 at DEBUG", recs[1])
		}
		r := recs[2]
		for k, want := range map[string]any{
			"level": "INFO", "status": 200.0, "attempt": 2.0, "conditional": true,
			"redirected_from": "301 /repos/{owner}/{repo}/issues", "has_next": true,
			"media_type": "github.v3; format=json", "query": "q=…12&per_page=30", "gh_request_id": "ABCD:1",
		} {
			if r[k] != want {
				t.Errorf("last record %s = %v, want %v", k, r[k], want)
			}
		}
		if n := len(records(t, buf, "api deprecated")); n != 1 {
			t.Errorf("%d deprecation records, want 1", n)
		}
		if strings.Contains(buf.String(), "secret") {
			t.Errorf("the log holds the search:\n%s", buf)
		}
	})
}

// The records of retries never hold the token.
func TestRetryLogsNoToken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, _ := captureLog(t, slog.LevelDebug)
		const token = "ghp_retrysecret0123456789"
		c, err := New(WithBaseURL("https://gh.test/"), WithToken(token),
			WithHTTPClient(&http.Client{Timeout: defaultTimeout, Transport: &script{steps: []step{unavailable}}}))
		if err != nil {
			t.Fatal(err)
		}
		_ = get(t.Context(), c)
		if len(records(t, buf, "http retry gave up")) != 1 {
			t.Fatalf("no give-up record:\n%s", buf)
		}
		if strings.Contains(buf.String(), token) {
			t.Errorf("the log holds the token:\n%s", buf)
		}
	})
}

// No attempt holds a slot while the request waits to send the next.
func TestRetryWaitsWithoutASlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &script{steps: []step{unavailable, ok}}
		c := scriptedClient(t, s)
		lt := limiter(c)
		done := make(chan error)
		go func() { done <- get(t.Context(), c) }()
		synctest.Wait()
		if n, _ := s.attempts(); n != 1 {
			t.Fatalf("%d attempts before the backoff, want 1", n)
		}
		if n := len(lt.slots); n != 0 {
			t.Errorf("%d slots taken during the backoff, want none", n)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

// limiter returns the limit transport of c.
func limiter(c *Client) *limitTransport {
	return c.http.Transport.(*retryTransport).base.(*rateTransport).base.(*timeoutTransport).base.(*limitTransport)
}

// TestRetrySlotsComeBack sends many requests at once, of every kind, that
// fail and are sent again in every way they can, some canceled on the
// way, and checks that each gave its slots back.
func TestRetrySlotsComeBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := scriptedClient(t, &script{random: true, steps: []step{ok, answered, dialFails, resets, hangs,
			badGateway, unavailable, serverError, wentWrong, missing, secondary3, secondary30, primary}})
		c.http.Transport.(*retryTransport).jitter = rand.Float64
		sends := []func(context.Context, *Client) error{get, post, query, mutation, prefetch, background}
		var wg sync.WaitGroup
		for i := range 300 {
			wg.Go(func() {
				ctx := t.Context()
				if i%3 == 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, time.Duration(rand.IntN(3000))*time.Millisecond)
					defer cancel()
				}
				_ = sends[i%len(sends)](ctx, c)
			})
		}
		wg.Wait()
		lt := limiter(c)
		if n, m := len(lt.slots), len(lt.background); n != 0 || m != 0 {
			t.Fatalf("%d slots and %d background slots still taken after all calls, want none", n, m)
		}
	})
}

func TestReadOnly(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{listPullsQuery, true},
		{getPullQuery, true},
		{viewerWorkQuery, true},
		{multiSearchQuery, true},
		{"{ viewer { login } }", true},
		{"  query\n  Named { x }", true},
		{"query($a: Int) { x }", true},
		{"query{x}", true},
		{closePullMutation, false},
		{mergePullMutation, false},
		{"mutation { x }", false},
		{"queryX { x }", false},
		{"fragment f on User { login }\nquery { viewer { ...f } }", false},
		{"# a comment\nquery { x }", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := readOnly(tt.query); got != tt.want {
			t.Errorf("readOnly(%.30q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}
