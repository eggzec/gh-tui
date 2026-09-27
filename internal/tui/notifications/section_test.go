package notifications

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

func TestUpdate(t *testing.T) {
	urls := map[string]string{}
	for _, n := range inbox() {
		urls[n.ID] = n.Subject.WebURL
	}
	url := func(id string) string { return urls[id] }
	tests := []struct {
		name    string
		threads []core.Notification
		keys    []string
		// Filled from the section and the fake after the keys.
		wantOpen    []string
		wantDone    []string
		wantReads   []string
		wantDones   []string
		wantAllRead int
		wantRows    []string
		wantAll     bool
	}{
		{
			name:      "select opens and marks read",
			keys:      []string{"enter"},
			wantOpen:  []string{"pull charmbracelet/bubbletea#1"},
			wantDone:  []string{"mark read"},
			wantReads: []string{"1"},
			// Read threads stay in the list until it is refetched from the
			// server; the fake drops them from the unread filter at once.
			wantRows: []string{"2", "3", "6"},
		},
		{
			name:     "select on a read thread only opens",
			keys:     []string{showAll, "down", "down", "down", "enter"},
			wantOpen: []string{url("4")},
			wantAll:  true,
			wantRows: []string{"1", "2", "3", "4", "5", "6", "7"},
		},
		{
			name:     "open doesn't mark read",
			keys:     []string{"down", "o"},
			wantOpen: []string{url("2")},
			wantRows: []string{"1", "2", "3", "6"},
		},
		{
			name:      "mark read",
			keys:      []string{"down", "m", "y"},
			wantDone:  []string{"mark read"},
			wantReads: []string{"2"},
			wantRows:  []string{"1", "3", "6"},
		},
		{
			name:     "mark read skips a read thread",
			keys:     []string{showAll, "down", "down", "down", "m"},
			wantAll:  true,
			wantRows: []string{"1", "2", "3", "4", "5", "6", "7"},
		},
		{
			name:      "mark done removes the thread",
			keys:      []string{showAll, "end", "d", "y"},
			wantDone:  []string{"mark done"},
			wantDones: []string{"7"},
			wantAll:   true,
			wantRows:  []string{"1", "2", "3", "4", "5", "6"},
		},
		{
			name:        "mark all read",
			keys:        []string{"M", "y"},
			wantDone:    []string{"mark all read"},
			wantAllRead: 1,
			wantRows:    nil,
		},
		{
			name:     "F goes back to unread",
			keys:     []string{showAll, "F"},
			wantRows: []string{"1", "2", "3", "6"},
		},
		{
			name:    "no selection does nothing",
			threads: []core.Notification{},
			keys:    []string{"enter", "o", "m", "d", "M"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threads := tt.threads
			if threads == nil {
				threads = inbox()
			}
			svc := newFake(threads...)
			s := newSection(t, svc, 80, 12)
			app := press(t, s, tt.keys...)

			var open, done []string
			for _, msg := range app {
				if o, ok := opened(msg); ok {
					open = append(open, o)
				}
				if msg, ok := msg.(ui.DoneMsg); ok {
					if msg.Err != nil {
						t.Errorf("DoneMsg for %s has error %v", msg.What, msg.Err)
					}
					done = append(done, msg.What)
				}
			}
			check := func(what string, got, want []string) {
				t.Helper()
				if !slices.Equal(got, want) {
					t.Errorf("%s = %q, want %q", what, got, want)
				}
			}
			check("opened", open, tt.wantOpen)
			check("done", done, tt.wantDone)
			check("marked read", svc.reads, tt.wantReads)
			check("marked done", svc.dones, tt.wantDones)
			check("rows", rows(s), tt.wantRows)
			if svc.allRead != tt.wantAllRead {
				t.Errorf("mark all read called %d times, want %d", svc.allRead, tt.wantAllRead)
			}
			if s.All() != tt.wantAll || svc.lastList().Filter.All != tt.wantAll {
				t.Errorf("All() = %v, last list %+v; want all %v", s.All(), svc.lastList(), tt.wantAll)
			}
		})
	}
}

// rows returns the IDs of the threads of inbox on screen, in order, found
// by the start of their titles.
func rows(s *Section) []string {
	var ids []string
	for line := range strings.Lines(ansi.Strip(s.View())) {
		for _, n := range inbox() { //nolint:gocritic // Test data, copied for clarity.
			if strings.Contains(line, n.Subject.Title[:min(len(n.Subject.Title), 8)]) {
				ids = append(ids, n.ID)
			}
		}
	}
	return ids
}

func TestFailedChangeRollsBack(t *testing.T) {
	svc := newFake(inbox()...)
	svc.fail = errors.New("403 Forbidden")
	s := newSection(t, svc, 80, 12)

	press(t, s, "d")
	app := press(t, s, "y")
	if len(app) != 1 {
		t.Fatalf("messages = %v, want one DoneMsg", app)
	}
	if done, ok := app[0].(ui.DoneMsg); !ok || done.What != "mark done" || done.Err == nil {
		t.Fatalf("message = %#v, want a failed mark done", app[0])
	}
	// The DoneMsg reloaded the list from the rolled back cache.
	if got, want := rows(s), []string{"1", "2", "3", "6"}; !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func TestRefreshReloads(t *testing.T) {
	svc := newFake(inbox()...)
	s := newSection(t, svc, 80, 12)
	before := svc.listCount()
	press(t, s, "r")
	if got, want := svc.invalidations(), []int{before}; !slices.Equal(got, want) {
		t.Errorf("invalidated after %v lists, want once after %d", got, before)
	}
	if got := svc.listCount() - before; got != 1 {
		t.Errorf("refresh listed %d pages, want 1", got)
	}
}

func TestSync(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.Msg
		want int
	}{
		{"own key reloads", ui.SyncMsg{Key: SyncKey}, 1},
		{"other key is ignored", ui.SyncMsg{Key: "pulls"}, 0},
		{"failed poll is ignored", ui.SyncMsg{Key: SyncKey, Err: errors.New("offline")}, 0},
		{"a finished change reloads", ui.DoneMsg{From: ui.NotificationsTitle, What: "mark read"}, 1},
		{"another section's change doesn't", ui.DoneMsg{From: ui.PullsTitle, What: "merge #42"}, 0},
		{"a repository is ignored", ui.RepoMsg{Repo: core.RepoRef{Owner: "o", Name: "r"}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFake(inbox()...)
			s := newSection(t, svc, 80, 12)
			before := svc.listCount()
			run(t, s, s.Update(tt.msg))
			if got := svc.listCount() - before; got != tt.want {
				t.Errorf("listed %d pages, want %d", got, tt.want)
			}
		})
	}
}

func TestSyncBeforeInitWaits(t *testing.T) {
	svc := newFake(inbox()...)
	s := New(t.Context(), svc, config.Default().Keys)
	s.SetSize(80, 10)
	run(t, s, s.Update(ui.SyncMsg{Key: SyncKey}))
	if n := svc.listCount(); n != 0 {
		t.Errorf("listed %d pages before Init, want 0", n)
	}
}

func TestBadge(t *testing.T) {
	unread := func(ids ...string) []core.Notification {
		ns := make([]core.Notification, 0, len(ids))
		for _, id := range ids {
			ns = append(ns, thread(id, "o/r", core.SubjectIssue, "t", "mention", true, time.Hour))
		}
		return ns
	}
	read := thread("r", "o/r", core.SubjectIssue, "t", "mention", false, time.Hour)
	tests := []struct {
		name   string
		cached bool
		page   core.Page[core.Notification]
		want   string
	}{
		{"not cached", false, core.Page[core.Notification]{}, ""},
		{"nothing unread", true, core.Page[core.Notification]{Items: []core.Notification{read}}, ""},
		{"unread", true, core.Page[core.Notification]{Items: append(unread("1", "2", "3"), read)}, "3"},
		{"more pages", true, core.Page[core.Notification]{Items: unread("1", "2"), Next: "2"}, "2+"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFake()
			svc.cached, svc.isCached = tt.page, tt.cached
			s := New(t.Context(), svc, config.Default().Keys)
			if got := s.Badge(); got != tt.want {
				t.Errorf("Badge() = %q, want %q", got, tt.want)
			}
			if n := svc.listCount(); n != 0 {
				t.Errorf("Badge listed %d pages, want 0", n)
			}
		})
	}
}

func TestKeysFromConfig(t *testing.T) {
	keys := config.Default().Keys
	keys[config.ActionMarkDone] = []string{"x"}
	s := New(t.Context(), newFake(), keys)
	k := s.Help().(KeyMap)
	if !key.Matches(keyPress("x"), k.MarkDone) || key.Matches(keyPress("d"), k.MarkDone) {
		t.Error("mark done should follow the config")
	}
	// The list's page down gives "f" up to the filter.
	if slices.Contains(k.feed.PageDown.Keys(), "f") {
		t.Errorf("page down keys = %q, want no f", k.feed.PageDown.Keys())
	}
	short := k.ShortHelp()
	for _, b := range []key.Binding{k.feed.Up, k.feed.Down, k.Select, k.Filter} {
		if !slices.ContainsFunc(short, func(h key.Binding) bool { return slices.Equal(h.Keys(), b.Keys()) }) {
			t.Errorf("short help lacks %q", b.Keys())
		}
	}
	if len(k.FullHelp()) < 3 {
		t.Errorf("full help = %d columns, want the list's and the section's", len(k.FullHelp()))
	}
}

func TestViewFits(t *testing.T) {
	for _, w := range []int{30, 50, 64, 80, 100, 120, 200} {
		s := newSection(t, newFake(inbox()...), w, 10)
		press(t, s, showAll)
		for i, line := range strings.Split(s.View(), "\n") {
			if got := ansi.StringWidth(line); got != w {
				t.Errorf("width %d: line %d is %d cells: %q", w, i, got, ansi.Strip(line))
			}
		}
	}
}

func TestLayoutDropsColumns(t *testing.T) {
	tests := []struct {
		width      int
		repo, tag  bool
		withReason bool
	}{
		{118, true, true, true},
		{78, true, true, true},
		{60, true, true, false},
		{44, true, false, false},
		{36, false, false, false},
		{20, false, false, false},
	}
	for _, tt := range tests {
		l := newLayout(tt.width)
		if (l.repo > 0) != tt.repo || (l.tag > 0) != tt.tag || (l.reason > 0) != tt.withReason {
			t.Errorf("newLayout(%d) = %+v, want repo %v, tag %v, reason %v", tt.width, l, tt.repo, tt.tag, tt.withReason)
		}
	}
}

func TestRefreshRetriesAFailedPage(t *testing.T) {
	svc := newFake(inbox()...)
	svc.listErr = errors.New("502 Bad Gateway")
	s := newSection(t, svc, 80, 12)
	if v := ansi.Strip(s.View()); !strings.Contains(v, "r to retry") {
		t.Errorf("error row doesn't offer refresh as retry:\n%s", v)
	}
	svc.mu.Lock()
	svc.listErr = nil
	svc.mu.Unlock()
	press(t, s, "r")
	if got := len(svc.invalidations()); got != 1 {
		t.Errorf("retry invalidated %d times, want 1", got)
	}
	if got, want := rows(s), []string{"1", "2", "3", "6"}; !slices.Equal(got, want) {
		t.Errorf("rows after retry = %q, want %q", got, want)
	}
}

func TestRunNotificationOpensTheActions(t *testing.T) {
	n := thread("9", "charmbracelet/bubbletea", core.SubjectCheckSuite, "ci workflow run failed for feat/x branch", "ci_activity", true, time.Minute)
	s := newSection(t, newFake(n), 120, 20)
	want := ui.OpenActionsMsg{Repo: n.Repo, Filter: core.RunFilter{Branch: "feat/x", Status: "failure"}}
	if app := press(t, s, "o"); !slices.Contains(app, tea.Msg(ui.OpenMsg{URL: n.Subject.WebURL})) {
		t.Errorf("o sent %v, want the browser", app)
	}
	if app := press(t, s, "enter"); !slices.Contains(app, tea.Msg(want)) {
		t.Errorf("enter sent %v, want %v", app, want)
	}
}

// marks returns the marks sent to svc, such as "read 2".
func marks(svc *fakeService) []string {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	out := make([]string, 0, len(svc.reads)+len(svc.dones)+svc.allRead)
	for _, id := range svc.reads {
		out = append(out, "read "+id)
	}
	for _, id := range svc.dones {
		out = append(out, "done "+id)
	}
	for range svc.allRead {
		out = append(out, "all read")
	}
	return out
}

func TestMarksAsk(t *testing.T) {
	tests := []struct {
		name string
		// keys lead to the thread, and the last one asks the mark.
		keys     []string
		question string
		want     string
	}{
		{"mark read", []string{"down", "m"}, "Mark eggzec/gh-tui#2 as read?", "read 2"},
		{"mark done", []string{"down", "d"}, "Mark eggzec/gh-tui#2 as done?", "done 2"},
		{"mark done without a number", []string{showAll, "end", "d"},
			`Mark "Moderate severity vulnerability in golang.org/x/net" in eggzec/gh-tui as done?`, "done 7"},
		{"mark all read", []string{"M"}, "Mark all notifications as read?", "all read"},
	}
	for _, tt := range tests {
		for _, answer := range [][]string{{"y"}, {"n"}, {"esc"}, {"y", "y"}} {
			t.Run(tt.name+"/"+strings.Join(answer, " "), func(t *testing.T) {
				svc := newFake(inbox()...)
				s := newSection(t, svc, 80, 12)
				before := rows(s)
				if tt.keys[0] == showAll {
					before = nil
				}
				msgs := press(t, s, tt.keys...)
				if got := question(s); got != tt.question {
					t.Fatalf("asks %q, want %q", got, tt.question)
				}
				if got := marks(svc); len(got) != 0 || slices.ContainsFunc(msgs, isDone) {
					t.Fatalf("sent %v before the answer", got)
				}
				if before != nil && !slices.Equal(rows(s), before) {
					t.Errorf("rows = %q before the answer, want them unchanged %q", rows(s), before)
				}
				// Other keys, even the marks, do nothing while it asks.
				press(t, s, "enter", "m", "d", "M", "q")
				if got := question(s); got != tt.question || len(marks(svc)) != 0 {
					t.Fatalf("after other keys asks %q with %v sent", got, marks(svc))
				}
				// The answers arrive before what the first starts runs,
				// as a repeated key does.
				m := asking[s]
				var cmds []tea.Cmd
				for _, k := range answer {
					cmds = append(cmds, m.Update(keyPress(k)))
				}
				for _, c := range cmds {
					msgs = append(msgs, run(t, s, c)...)
				}
				if question(s) != "" {
					t.Fatalf("%v left the question open", answer)
				}
				var want []string
				if answer[0] == "y" {
					want = []string{tt.want}
				}
				if got := marks(svc); !slices.Equal(got, want) {
					t.Errorf("sent %v, want %v", got, want)
				}
				if done := slices.ContainsFunc(msgs, isDone); done != (want != nil) {
					t.Errorf("messages %v, want a DoneMsg %v", msgs, want != nil)
				}
			})
		}
	}
}

// Mark all read marks the threads until the newest the list shows, so a
// thread GitHub has that the list doesn't yet stays unread.
func TestMarkAllReadUntilTheNewestSeen(t *testing.T) {
	svc := newFake(inbox()...)
	s := newSection(t, svc, 80, 12)
	// Thread 8 arrives on GitHub, but no poll has shown it yet.
	svc.mu.Lock()
	svc.threads = append(svc.threads, thread("8", "eggzec/gh-tui", core.SubjectIssue, "Unseen", "mention", true, time.Second))
	svc.mu.Unlock()
	press(t, s, "M", "y")
	// The newest the list shows is thread 6, 30 seconds old.
	svc.mu.Lock()
	until := svc.until
	svc.mu.Unlock()
	if want := now.Add(-30 * time.Second); !until.Equal(want) {
		t.Errorf("marked until %v, want %v, the newest thread shown", until, want)
	}
	press(t, s, "r")
	if got := rows(s); len(got) != 0 {
		t.Errorf("rows = %q, want none of the inbox's", got)
	}
	if v := ansi.Strip(s.View()); !strings.Contains(v, "Unseen") {
		t.Errorf("the unseen thread was marked read too:\n%s", v)
	}
}

func isDone(m tea.Msg) bool { _, ok := m.(ui.DoneMsg); return ok }

func TestMarksAskAgain(t *testing.T) {
	// read marks thread id read on GitHub, and the list reads it again.
	read := func(id string) func(t *testing.T, s *Section, svc *fakeService) {
		return func(t *testing.T, s *Section, svc *fakeService) {
			t.Helper()
			svc.mu.Lock()
			for i := range svc.threads {
				if svc.threads[i].ID == id {
					svc.threads[i].Unread = false
				}
			}
			svc.mu.Unlock()
			run(t, s, s.Update(ui.SyncMsg{Key: SyncKey}))
		}
	}
	tests := []struct {
		name   string
		keys   []string
		meddle func(t *testing.T, s *Section, svc *fakeService)
		want   string
	}{
		{
			name: "read elsewhere", keys: []string{"down", "m"},
			meddle: read("2"), want: "eggzec/gh-tui#2 changed meanwhile, so nothing was sent.",
		},
		{
			// The unread list drops #2, so the cursor is on another thread.
			name: "the cursor's thread went away", keys: []string{"down", "d"},
			meddle: read("2"), want: "eggzec/gh-tui#2 changed meanwhile, so nothing was sent.",
		},
		{
			name: "the cursor moved", keys: []string{"down", "d"},
			meddle: func(t *testing.T, s *Section, _ *fakeService) {
				t.Helper()
				// As a reload that reorders the list would, behind the
				// question.
				run(t, s, s.Update(keyPress("down")))
			},
			want: "eggzec/gh-tui#2 changed meanwhile, so nothing was sent.",
		},
		{
			name: "a thread was read before all", keys: []string{"M"},
			meddle: read("3"), want: "The inbox changed meanwhile, so nothing was sent.",
		},
		{
			name: "a thread arrived before all", keys: []string{"M"},
			meddle: func(t *testing.T, s *Section, svc *fakeService) {
				t.Helper()
				svc.mu.Lock()
				svc.threads = append(svc.threads, thread("8", "eggzec/gh-tui", core.SubjectIssue, "New", "mention", true, time.Second))
				svc.mu.Unlock()
				run(t, s, s.Update(ui.SyncMsg{Key: SyncKey}))
			},
			want: "The inbox changed meanwhile, so nothing was sent.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFake(inbox()...)
			s := newSection(t, svc, 80, 12)
			press(t, s, tt.keys...)
			if question(s) == "" {
				t.Fatal("asked nothing")
			}
			tt.meddle(t, s, svc)
			msgs := press(t, s, "y")
			want := ui.NotifyMsg{Level: toast.Info, Text: tt.want}
			if got := marks(svc); len(got) != 0 || !slices.Contains(msgs, tea.Msg(want)) {
				t.Errorf("sent %v and showed %v, want only %q", got, msgs, tt.want)
			}
		})
	}
}

// The list says what went wrong the way the user should read it, without
// the error's chain, request or status code.
func TestErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"offline", fmt.Errorf("list notifications: github: GET /repos/o/r/notifications: %w", core.ErrOffline), "✗ Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("list notifications: github: 403 Forbidden: %w", core.ErrForbidden), "✗ You don't have access to this · o to open on GitHub"},
		{"internal", errors.New("list notifications: github: decode: unexpected EOF"), "✗ Something went wrong · r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFake(inbox()...)
			svc.listErr = tt.err
			s := newSection(t, svc, 100, 12)
			if v := ansi.Strip(s.View()); !strings.Contains(v, tt.want) || strings.Contains(v, "github") || strings.Contains(v, "403") {
				t.Errorf("screen = %q, want %q", v, tt.want)
			}
		})
	}
}
