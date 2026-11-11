package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestReposCountTotal(t *testing.T) {
	t.Parallel()
	svc := newFake()
	svc.totals = map[string]int{"github": 1234}
	s := newSection(t, svc, nil, 140, 38)
	press(t, s, "]")
	if got := s.repos.current().label; got != "github" {
		t.Fatalf("after ] the tab is %q, want github", got)
	}
	got := s.reposCount()
	if len(got) == 0 || got[0] != "1.2k[5]" {
		t.Fatalf("reposCount() = %q, want 1.2k[5] first", got)
	}
}

func TestInboxCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		threads []core.Notification
		want    []string
	}{
		{"unread threads", inboxThreads(), []string{"3 unread"}},
		{"one unread", []core.Notification{thread("9", "o/r", "title", true, time.Minute)}, []string{"1 unread"}},
		{"all read", []core.Notification{thread("9", "o/r", "title", false, time.Minute)}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := newSection(t, newFake(), &fakeInbox{threads: c.threads}, 140, 38)
			if got := s.inboxCount(); !slices.Equal(got, c.want) {
				t.Fatalf("inboxCount() = %q, want %q", got, c.want)
			}
		})
	}
}
