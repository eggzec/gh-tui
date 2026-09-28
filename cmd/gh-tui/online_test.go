package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
)

// flakyTransport answers as GitHub does, with graphqlTransport's page,
// or, while down, fails to connect, and counts the requests that reach
// it.
type flakyTransport struct {
	mu   sync.Mutex
	down bool
	sent int
	up   graphqlTransport
}

func (f *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent++
	if f.down {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: network is unreachable")}
	}
	resp, err := f.up.RoundTrip(req)
	if err == nil {
		// Only GitHub's answers count, which carry its request id.
		resp.Header.Set("X-GitHub-Request-Id", "0400:1:2:3:4")
	}
	return resp, err
}

func (f *flakyTransport) set(down bool) {
	f.mu.Lock()
	f.down = down
	f.mu.Unlock()
}

func (f *flakyTransport) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent
}

// TestCacheHitIsNotOnline checks that the connection the app shows, and
// wakes what failed on, comes back only with an answer from GitHub: a
// read the cache serves while GitHub can't be reached leaves it offline.
func TestCacheHitIsNotOnline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := new(flakyTransport)
		client, err := github.New(github.WithBaseURL("https://gh.test/"), github.WithToken("t"), github.WithHTTPClient(&http.Client{Transport: rt}))
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		offline := func() bool {
			s := client.RateStatus()
			return s.Failed.After(s.Answered)
		}
		svc := pullsvc.New(client)
		cached := pullsvc.ListQuery{Repo: core.RepoRef{Owner: "cli", Name: "cli"}}
		other := pullsvc.ListQuery{Repo: core.RepoRef{Owner: "cli", Name: "go-gh"}}
		ctx := context.Background()

		if _, err := svc.List(ctx, cached); err != nil {
			t.Fatal(err)
		}
		rt.set(true)
		if _, err := svc.List(ctx, other); !errors.Is(err, core.ErrOffline) {
			t.Fatalf("List while down = %v, want %v", err, core.ErrOffline)
		}
		if !offline() {
			t.Fatal("not offline after a request got no answer")
		}

		sent := rt.count()
		if _, err := svc.List(ctx, cached); err != nil {
			t.Fatal(err)
		}
		if rt.count() != sent {
			t.Fatal("the cached list was read from GitHub")
		}
		if !offline() {
			t.Error("a read the cache served counts as GitHub answering")
		}

		time.Sleep(time.Second)
		rt.set(false)
		if _, err := svc.List(ctx, other); err != nil {
			t.Fatal(err)
		}
		if offline() {
			t.Error("still offline after GitHub answered")
		}
	})
}
