package github

import (
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// TestLimitSlotsComeBack sends many requests at once, of every kind the
// client sends and ending every way they can, and checks that each gave
// its slots back.
func TestLimitSlotsComeBack(t *testing.T) {
	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Duration(rand.IntN(3)) * time.Millisecond)
		_, _ = w.Write([]byte(`{"a":1}`))
	})
	mux.HandleFunc("/nm", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"x"`)
		w.WriteHeader(http.StatusNotModified)
	})
	mux.HandleFunc("/err", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})
	mux.HandleFunc("/bad", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	})
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srvURL+"/ok", http.StatusFound)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"a":`))
		fl.Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
		_, _ = w.Write([]byte(`1}`))
	})
	mux.HandleFunc("/repos/o/r/actions/jobs/1/logs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srvURL+"/blob", http.StatusFound)
	})
	mux.HandleFunc("/blob", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("line\n", 2000)))
	})
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if rand.IntN(2) == 0 {
			_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"nope","type":"NOT_FOUND"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"rateLimit":{"cost":3,"limit":5000,"remaining":4000,"resetAt":"2030-01-01T00:00:00Z"},"x":1}}`))
	})
	c := newTestClient(t, mux)
	srvURL = strings.TrimSuffix(c.restURL.String(), "/")
	lt := c.http.Transport.(*limitTransport)

	var wg sync.WaitGroup
	for i := range 200 {
		wg.Go(func() {
			ctx := t.Context()
			var cancel context.CancelFunc = func() {}
			if i%5 == 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(rand.IntN(30))*time.Millisecond)
			}
			if i%7 == 0 {
				ctx = obs.ForPrefetch(ctx)
			}
			defer cancel()
			var v any
			switch i % 9 {
			case 0:
				_, _ = c.Get(ctx, "ok", Conditional{}, &v)
			case 1:
				_, _ = c.Get(ctx, "nm", Conditional{ETag: `"x"`}, &v)
			case 2:
				_, _ = c.Get(ctx, "err", Conditional{}, &v)
			case 3:
				_, _ = c.Get(ctx, "bad", Conditional{}, &v)
			case 4:
				_, _ = c.Get(ctx, "redir", Conditional{}, &v)
			case 5:
				_, _ = c.Get(ctx, "slow", Conditional{}, &v)
			case 6:
				_, _, _ = c.JobLog(ctx, core.RepoRef{Owner: "o", Name: "r"}, 1, 1000)
			case 7:
				_, _, _ = c.JobLog(ctx, core.RepoRef{Owner: "o", Name: "r"}, 1, 1<<20)
			case 8:
				_ = c.Query(ctx, "query{x}", nil, &v)
			}
		})
	}
	wg.Wait()
	if n, m := len(lt.slots), len(lt.background); n != 0 || m != 0 {
		t.Fatalf("%d slots and %d background slots still taken after all calls, want none", n, m)
	}
}
