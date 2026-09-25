package issues

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

var (
	readCaps   = core.RepoCaps{Known: true, Permission: core.PermissionRead, Issues: true}
	triageCaps = core.RepoCaps{Known: true, Permission: core.PermissionTriage, Issues: true}
	writeCaps  = core.RepoCaps{Known: true, Permission: core.PermissionWrite, Issues: true}
)

// offered returns what the enabled keys of km do, as help shows them.
func offered(km help.KeyMap) []string {
	var out []string
	for _, b := range km.ShortHelp() {
		if b.Enabled() {
			out = append(out, b.Help().Desc)
		}
	}
	return out
}

func info(text string) tea.Msg { return ui.NotifyMsg{Level: toast.Info, Text: text} }

// me is the viewer, who opened the second sample issue.
func me(context.Context) (string, error) { return "HUBOT", nil }

func TestGatedChanges(t *testing.T) {
	archived := writeCaps
	archived.Archived = true
	tests := []struct {
		name string
		caps core.RepoCaps
		// lock locks the first issue.
		lock bool
		// keys lead to the issue, and the last one asks a change.
		keys []string
		// hidden are left out of help before the last key, and why is the
		// toast it shows instead of sending the change, or empty when it
		// is sent as want.
		hidden []string
		why    string
		want   []string
	}{
		{
			name: "read can't close another's issue", caps: readCaps, keys: []string{"x"}, hidden: []string{"close"},
			why: "You can't close #1000 in eggzec/gh-tui (read access).",
		},
		{name: "read closes their own", caps: readCaps, keys: []string{"down", "x"}, want: []string{"close 999"}},
		{name: "triage closes another's", caps: triageCaps, keys: []string{"x"}, want: []string{"close 1000"}},
		{
			name: "read can't label", caps: readCaps, keys: []string{"enter", "l"}, hidden: []string{"labels"},
			why: "Labeling needs triage access to eggzec/gh-tui.",
		},
		{
			name: "read can't comment on a locked issue", caps: readCaps, lock: true, keys: []string{"enter", "c"}, hidden: []string{"comment"},
			why: "#1000 is locked as resolved · only collaborators can comment.",
		},
		{
			name: "an archived repository takes no comment", caps: archived, keys: []string{"enter", "c"}, hidden: []string{"comment", "labels", "close"},
			why: "eggzec/gh-tui is archived, so it's read-only.",
		},
		{
			name: "read can't reopen another's in the modal", caps: readCaps, keys: []string{"]", "enter", "X"}, hidden: []string{"reopen"},
			why: "You can't reopen #996 in eggzec/gh-tui (read access).",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(8))
			if tt.lock {
				svc.issues[0].Locked, svc.issues[0].LockReason = true, "resolved"
			}
			h := started(t, svc, 120, 30, WithViewer(me))
			run(t, h, h.Update(ui.CapsMsg{Repo: testRepo, Caps: tt.caps}))
			last := len(tt.keys) - 1
			press(t, h, tt.keys[:last]...)
			km := h.Help()
			if m := h.modal(); m != nil {
				km = m.Help()
			}
			for _, desc := range tt.hidden {
				if slices.Contains(offered(km), desc) {
					t.Errorf("help offers %q: %v", desc, offered(km))
				}
			}
			msgs := press(t, h, tt.keys[last])
			if got := svc.changeCalls(); !slices.Equal(got, tt.want) {
				t.Errorf("changes = %v, want %v", got, tt.want)
			}
			if tt.why != "" && !slices.Contains(msgs, info(tt.why)) {
				t.Errorf("messages %v, want the toast %q", msgs, tt.why)
			}
			if m := h.modal(); m != nil && tt.why != "" && m.composing != composeNone {
				t.Error("the prompt opened although the change is refused")
			}
		})
	}
}

func TestCapsArrivingLaterGateTheIssues(t *testing.T) {
	svc := newFakeService(sampleIssues(8))
	h := started(t, svc, 120, 30, WithViewer(me))
	press(t, h, "enter")
	m := h.modal()
	if got := offered(m.Help()); !slices.Contains(got, "labels") || !slices.Contains(got, "close") {
		t.Errorf("help before the caps = %v, want labels and close offered", got)
	}
	run(t, h, h.Update(ui.CapsMsg{Repo: testRepo, Caps: readCaps}))
	if got := offered(m.Help()); slices.Contains(got, "labels") || slices.Contains(got, "close") {
		t.Errorf("help after read caps = %v, want neither labels nor close", got)
	}
	press(t, h, "esc")
	if got := offered(h.Help()); slices.Contains(got, "close") {
		t.Errorf("list help after read caps = %v, want no close", got)
	}
}

func TestIssuesTurnedOff(t *testing.T) {
	off := core.RepoCaps{Known: true, Permission: core.PermissionAdmin}
	t.Run("known before the list loads", func(t *testing.T) {
		svc := newFakeService(sampleIssues(8))
		repos := &fakeRepos{caps: map[core.RepoRef]core.RepoCaps{testRepo: off}, cached: map[core.RepoRef]bool{testRepo: true}}
		h := newSection(t, svc, 80, 12, WithRepos(repos), WithPrefetch(3, 0), WithFilterPrefetch())
		run(t, h, h.Update(ui.RepoMsg{Repo: testRepo}))
		run(t, h, h.Init())
		press(t, h, "r", "]", "down", "enter")
		run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
		if got := svc.listCalls(); len(got) != 0 {
			t.Errorf("lists = %v, want none", got)
		}
		if got := svc.getCalls(); len(got) != 0 || h.modal() != nil {
			t.Errorf("gets = %v, modal %v; want neither", got, h.modal())
		}
		if v := h.View(); !strings.Contains(v, "Issues are turned off for eggzec/gh-tui") {
			t.Errorf("view:\n%s", v)
		}
		if _, ok := h.Filter(); ok || len(offered(h.Help())) != 0 {
			t.Errorf("the filter or help keys are offered: %v", offered(h.Help()))
		}
	})
	t.Run("learned after the list loaded", func(t *testing.T) {
		svc := newFakeService(sampleIssues(8))
		h := started(t, svc, 80, 12, WithPrefetch(3, 0), WithFilterPrefetch())
		lists, gets := len(svc.listCalls()), len(svc.getCalls())
		run(t, h, h.Update(ui.CapsMsg{Repo: testRepo, Caps: off}))
		press(t, h, "r", "down")
		run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
		if l, g := len(svc.listCalls()), len(svc.getCalls()); l != lists || g != gets {
			t.Errorf("reads went from %d lists and %d gets to %d and %d, want no more", lists, gets, l, g)
		}
		if v := h.View(); !strings.Contains(v, "Issues are turned off") {
			t.Errorf("view:\n%s", v)
		}
		// Turned on again, they load.
		run(t, h, h.Update(ui.CapsMsg{Repo: testRepo, Caps: writeCaps}))
		if l := len(svc.listCalls()); l == lists {
			t.Error("the list didn't load once the issues were on")
		}
	})
}

// fakeRepos serves the caps of repositories, and counts its reads.
type fakeRepos struct {
	mu     sync.Mutex
	caps   map[core.RepoRef]core.RepoCaps
	cached map[core.RepoRef]bool
	gets   []core.RepoRef
}

func (f *fakeRepos) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cached[ref] {
		return core.Repo{}, false
	}
	return core.Repo{Ref: ref, Caps: f.caps[ref]}, true
}

func (f *fakeRepos) Get(_ context.Context, ref core.RepoRef) (core.Repo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, ref)
	c, ok := f.caps[ref]
	if !ok {
		return core.Repo{}, errors.New("not found")
	}
	f.cached[ref] = true
	return core.Repo{Ref: ref, Caps: c}, nil
}

func TestModalOfAnotherRepoReadsItsCaps(t *testing.T) {
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	repos := &fakeRepos{caps: map[core.RepoRef]core.RepoCaps{other: readCaps}, cached: map[core.RepoRef]bool{}}
	svc := newFakeService(sampleIssues(8))
	h := started(t, svc, 120, 30, WithRepos(repos), WithViewer(me))
	run(t, h, h.Update(ui.OpenIssueMsg{Repo: other, Number: 1000}))
	m := h.modal()
	if m == nil || m.caps != readCaps || !slices.Equal(repos.gets, []core.RepoRef{other}) {
		t.Fatalf("modal %v after reads %v, want the caps of %v read", m, repos.gets, other)
	}
	if got := offered(m.Help()); slices.Contains(got, "labels") {
		t.Errorf("help = %v, want no labels in %v", got, other)
	}
}
