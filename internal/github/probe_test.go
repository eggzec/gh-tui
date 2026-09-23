package github

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// probes are the change probes and the paths of the lists they watch.
var probes = map[string]struct {
	path  string
	probe func(c *Client, ctx context.Context, repo core.RepoRef, cond Conditional) (Response, error)
}{
	"pulls":  {"/repos/octo-org/hello/pulls", (*Client).ProbePullRequests},
	"issues": {"/repos/octo-org/hello/issues", (*Client).ProbeIssues},
}

func TestProbe(t *testing.T) {
	for name, p := range probes {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				checkIssueRequest(t, r, http.MethodGet, p.path, map[string]string{
					"state": "all", "sort": "updated", "direction": "desc", "per_page": "1",
				})
				w.Header().Set("X-Poll-Interval", "60")
				if r.Header.Get("If-None-Match") == `W/"e1"` {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("ETag", `W/"e1"`)
				_, _ = w.Write([]byte(`[{"number":7,"updated_at":"2026-09-20T17:42:10Z"}]`))
			}))

			res, err := p.probe(c, t.Context(), issueRepo, Conditional{})
			if err != nil {
				t.Fatalf("first probe: %v", err)
			}
			if res.NotModified || res.ETag != `W/"e1"` || res.PollInterval != time.Minute {
				t.Errorf("first probe = %+v, want a 200 with ETag W/\"e1\" and a 1m interval", res)
			}

			res, err = p.probe(c, t.Context(), issueRepo, Conditional{ETag: res.ETag})
			if err != nil {
				t.Fatalf("second probe: %v", err)
			}
			if !res.NotModified || res.PollInterval != time.Minute {
				t.Errorf("second probe = %+v, want a 304 with a 1m interval", res)
			}
		})
	}
}

func TestProbeErrors(t *testing.T) {
	for name, p := range probes {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			}))
			if _, err := p.probe(c, t.Context(), issueRepo, Conditional{}); !errors.Is(err, core.ErrNotFound) {
				t.Errorf("error = %v, want %v", err, core.ErrNotFound)
			}
		})
	}
}
