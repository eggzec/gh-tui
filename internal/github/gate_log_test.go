package github

import (
	"log/slog"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

// A burst of reads that a spent quota fails at once logs one record, at
// info level, which says why and until when, and the next one, once the
// throttle lets it through, how many it held back.
func TestGateLogsRefused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, _ := captureLog(t, slog.LevelInfo)
		h := &hub{limit: 5, window: time.Minute}
		c := newHubClient(t, h, 0)
		reset := time.Now().Add(30 * time.Minute)
		spendCore(t, c, h, reset)

		for i := range 50 {
			_, _ = c.Get(t.Context(), "repos/o/r/"+strconv.Itoa(i), Conditional{}, nil)
		}
		recs := records(t, buf, "rate limit refused")
		if len(recs) != 1 {
			t.Fatalf("%d refused records for a burst of 50, want 1:\n%s", len(recs), buf)
		}
		r := recs[0]
		if r["level"] != "INFO" || r["resource"] != resourceCore || r["class"] != "foreground" ||
			r["reason"] != "foreground" || r["until"] != reset.Add(minGuard).Format(time.RFC3339Nano) || r["suppressed"] != nil {
			t.Errorf("refused record = %v", r)
		}
		// Nothing else, such as a header, rides along.
		for k := range r {
			if !slices.Contains([]string{"time", "level", "msg", "session_id", "span", "resource", "class", "reason", "until"}, k) {
				t.Errorf("refused record has %q", k)
			}
		}

		time.Sleep(gateLogEvery)
		buf.Reset()
		_, _ = c.Get(t.Context(), "repos/o/r", Conditional{}, nil)
		if recs := records(t, buf, "rate limit refused"); len(recs) != 1 || recs[0]["suppressed"] != float64(49) {
			t.Errorf("refused records a minute later = %v, want 1 that held back 49", recs)
		}
	})
}

// A read ahead that a limit fails logs at debug level only.
func TestGateLogsRefusedPrefetchAtDebug(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelInfo, slog.LevelDebug} {
		t.Run(level.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				buf, _ := captureLog(t, level)
				h := &hub{limit: 5, window: time.Minute}
				c := newHubClient(t, h, 0)
				spendCore(t, c, h, time.Now().Add(time.Second))
				for range 10 {
					_, _ = c.Get(obs.ForPrefetch(t.Context()), "repos/o/r", Conditional{}, nil)
				}
				recs := records(t, buf, "rate limit refused")
				want := 0
				if level == slog.LevelDebug {
					want = 1
				}
				if len(recs) != want {
					t.Fatalf("%d refused records at %v, want %d:\n%s", len(recs), level, want, buf)
				}
				if want == 1 && (recs[0]["level"] != "DEBUG" || recs[0]["class"] != "prefetch" || recs[0]["reason"] != "prefetch") {
					t.Errorf("refused record = %v", recs[0])
				}
			})
		})
	}
}

// A request held and then failed logs its hold, at debug level, and its
// failure, with why.
func TestGateLogsHeldThenRefused(t *testing.T) {
	watchdog(t)
	synctest.Test(t, func(t *testing.T) {
		buf, _ := captureLog(t, slog.LevelDebug)
		h := &hub{limit: 5, window: time.Minute}
		c := newHubClient(t, h, 0)
		spendCore(t, c, h, time.Now().Add(55*time.Minute))
		dones := make([]<-chan error, 0, 10)
		for range 10 {
			dones = append(dones, goAsync(func() error {
				_, err := c.Get(obs.ForBackground(t.Context()), "repos/o/r", Conditional{}, nil)
				return err
			}))
		}
		time.Sleep(30 * time.Minute)
		h.secondary(strconv.Itoa(int((45 * time.Minute).Seconds())))
		_, _ = c.Get(t.Context(), "search/issues?q=x", Conditional{}, nil)
		for _, done := range dones {
			<-done
		}
		holds := records(t, buf, "rate limit hold")
		if len(holds) != 1 || holds[0]["class"] != "background" || holds[0]["resource"] != resourceCore {
			t.Errorf("hold records = %v, want 1 for the 10 held", holds)
		}
		var capped []map[string]any
		for _, r := range records(t, buf, "rate limit refused") {
			if r["reason"] == "cap" {
				capped = append(capped, r)
			}
		}
		if len(capped) != 1 || capped[0]["level"] != "INFO" || capped[0]["class"] != "background" || capped[0]["until"] == nil {
			t.Errorf("refused records for the cap = %v, want 1 at info level", capped)
		}
	})
}
