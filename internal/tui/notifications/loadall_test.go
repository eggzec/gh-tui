package notifications

import (
	"strconv"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func manyThreads(n int) []core.Notification {
	threads := make([]core.Notification, 0, n)
	for i := range n {
		threads = append(threads, thread(strconv.Itoa(i+1), "a/r", core.SubjectIssue, "t", "mention", true, time.Duration(i+1)*time.Minute))
	}
	return threads
}

func TestLoadsEveryPage(t *testing.T) {
	t.Parallel()
	svc := newFake(manyThreads(7)...)
	svc.size = 2
	s := newSection(t, svc, 80, 12)
	if got := len(loaded(s)); got != 7 {
		t.Fatalf("loaded %d threads, want 7", got)
	}
	if got := svc.listCount(); got != 4 {
		t.Errorf("%d reads, want 4", got)
	}
}

func TestStopsAtTheCap(t *testing.T) {
	t.Parallel()
	svc := newFake(manyThreads(maxPages+5)...)
	svc.size = 1
	s := newSection(t, svc, 80, 12)
	if got := len(loaded(s)); got != maxPages {
		t.Fatalf("loaded %d threads, want %d", got, maxPages)
	}
}
