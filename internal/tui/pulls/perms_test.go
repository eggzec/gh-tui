package pulls

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

var (
	readCaps  = core.RepoCaps{Known: true, Permission: core.PermissionRead, MergeCommit: true, Squash: true, Rebase: true}
	writeCaps = core.RepoCaps{Known: true, Permission: core.PermissionWrite, MergeCommit: true, Squash: true, Rebase: true}
)

// asStranger makes every pull request of svc one that GitHub says the
// viewer may not change, as in a repository they can only read.
func asStranger(svc *fakeService) {
	for i := range svc.pulls {
		svc.pulls[i].Caps = core.ItemCaps{Known: true}
	}
}

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

func TestReadAccessHidesChanges(t *testing.T) {
	archived := writeCaps
	archived.Archived = true
	tests := []struct {
		name string
		caps core.RepoCaps
		// keys lead to the pull request, and the last one asks a change.
		keys []string
		why  string
	}{
		{name: "merge", caps: readCaps, keys: []string{"m"}, why: "You can't merge in eggzec/gh-tui (read access)."},
		{name: "close", caps: readCaps, keys: []string{"x"}, why: "You can't close #142 in eggzec/gh-tui (read access)."},
		{name: "reopen", caps: readCaps, keys: []string{"]", "X"}, why: "You can't reopen #93 in eggzec/gh-tui (read access)."},
		{name: "draft", caps: readCaps, keys: []string{"D"}, why: "You can't change #142 in eggzec/gh-tui (read access)."},
		{name: "merge in the modal", caps: readCaps, keys: []string{"enter", "m"}, why: "You can't merge in eggzec/gh-tui (read access)."},
		{name: "archived", caps: archived, keys: []string{"x"}, why: "eggzec/gh-tui is archived, so it's read-only."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			asStranger(svc)
			h := started(t, svc, 120, 20)
			drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: tt.caps}))
			last := len(tt.keys) - 1
			for _, k := range tt.keys[:last] {
				press(t, h, k)
			}
			km := h.Help()
			if m := h.modal(); m != nil {
				km = m.Help()
			}
			for _, desc := range offered(km) {
				if slices.Contains([]string{"merge", "close", "reopen", "convert to draft"}, desc) {
					t.Errorf("help offers %q: %v", desc, offered(km))
				}
			}
			msgs := press(t, h, tt.keys[last])
			if got := question(h); got != "" {
				t.Errorf("asks %q of a change the viewer can't make", got)
			}
			if got := svc.changes(); len(got) != 0 {
				t.Errorf("changes = %v, want none sent", got)
			}
			if !slices.Contains(msgs, info(tt.why)) {
				t.Errorf("messages %v, want the toast %q", msgs, tt.why)
			}
		})
	}
}

func TestCapsArrivingLaterGateTheList(t *testing.T) {
	svc := newFakeService()
	asStranger(svc)
	h := started(t, svc, 120, 20)
	// Until the caps are known, GitHub decides; GitHub's own no still
	// counts, as it does here for close.
	if got := offered(h.Help()); !slices.Contains(got, "merge") {
		t.Errorf("help before the caps = %v, want merge offered", got)
	}
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: readCaps}))
	if got := offered(h.Help()); slices.Contains(got, "merge") {
		t.Errorf("help after read caps = %v, want no merge", got)
	}
	// The caps of another repository change nothing.
	drain(t, h, h.Update(ui.CapsMsg{Repo: core.RepoRef{Owner: "o", Name: "r"}, Caps: writeCaps}))
	if got := offered(h.Help()); slices.Contains(got, "merge") {
		t.Errorf("help after another's caps = %v, want no merge", got)
	}
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))
	if got := offered(h.Help()); !slices.Contains(got, "merge") {
		t.Errorf("help after write caps = %v, want merge", got)
	}
	press(t, h, "m")
	press(t, h, "y")
	if got := svc.changes(); !slices.Equal(got, []string{"merge squash 142"}) {
		t.Errorf("changes = %v, want the merge", got)
	}
}

func TestAuthorChangesTheirOwn(t *testing.T) {
	svc := newFakeService()
	asStranger(svc)
	svc.pulls[0].Caps = core.ItemCaps{Known: true, Update: true, Close: true, Authored: true}
	h := started(t, svc, 120, 20)
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: readCaps}))
	if got := offered(h.Help()); !slices.Contains(got, "close") || slices.Contains(got, "merge") {
		t.Errorf("help = %v, want close but no merge", got)
	}
	press(t, h, "D")
	press(t, h, "x")
	press(t, h, "y")
	if got := svc.changes(); !slices.Equal(got, []string{"draft 142", "close 142"}) {
		t.Errorf("changes = %v, want the draft and the close", got)
	}
}

func TestMergeUsesAnAllowedMethod(t *testing.T) {
	rebaseOnly := core.RepoCaps{Known: true, Permission: core.PermissionAdmin, Rebase: true, DefaultMerge: core.MergeRebase}
	tests := []struct {
		name  string
		caps  core.RepoCaps
		want  string
		label string
		ask   string
	}{
		{
			name: "the configured method when allowed", caps: writeCaps, want: "merge squash 142", label: "merge",
			ask: "Squash-merge #142 into main?",
		},
		{
			name: "the allowed one otherwise", caps: rebaseOnly, want: "merge rebase 142", label: "merge (rebase)",
			ask: "Rebase-merge #142 into main?",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			h := started(t, svc, 160, 20)
			drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: tt.caps}))
			if got := offered(h.Help()); !slices.Contains(got, tt.label) {
				t.Errorf("help = %v, want %q", got, tt.label)
			}
			press(t, h, "m")
			if got := question(h); got != tt.ask {
				t.Errorf("asks %q, want %q", got, tt.ask)
			}
			press(t, h, "y")
			if got := svc.changes(); !slices.Equal(got, []string{tt.want}) {
				t.Errorf("changes = %v, want %q", got, tt.want)
			}
		})
	}
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
	svc := newFakeService()
	asStranger(svc)
	h := started(t, svc, 120, 20, WithRepos(repos))
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))

	drain(t, h, h.Update(ui.OpenPullMsg{Repo: other, Number: 142}))
	m := h.modal()
	if m == nil {
		t.Fatal("OpenPullMsg opened no modal")
	}
	if m.caps != readCaps || !slices.Equal(repos.gets, []core.RepoRef{other}) {
		t.Fatalf("modal %v with caps %+v after reads %v, want those of %v read", m, m.caps, repos.gets, other)
	}
	if got := offered(m.Help()); slices.Contains(got, "merge") {
		t.Errorf("help = %v, want no merge in %v", got, other)
	}
	msgs := press(t, h, "m")
	if got := svc.changes(); len(got) != 0 || question(h) != "" || !slices.Contains(msgs, info("You can't merge in charmbracelet/bubbletea (read access).")) {
		t.Errorf("merge sent %v and showed %v, want a toast only", got, msgs)
	}

	// Opened again, the caps are cached, so they aren't read again.
	press(t, h, "esc")
	drain(t, h, h.Update(ui.OpenPullMsg{Repo: other, Number: 135}))
	if m := h.modal(); m.caps != readCaps || len(repos.gets) != 1 {
		t.Errorf("caps %+v after reads %v, want the cached ones", m.caps, repos.gets)
	}
	// The selected repository's come from the section.
	press(t, h, "esc")
	drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 142}))
	if m := h.modal(); m.caps != writeCaps || len(repos.gets) != 1 {
		t.Errorf("caps %+v after reads %v, want the section's", m.caps, repos.gets)
	}
}
