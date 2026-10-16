package pulls

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
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

// offered returns what the enabled keys of layers do.
func offered(layers []keyhelp.Layer) []string { return uitest.Enabled(layers) }

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
		{name: "merge", caps: readCaps, keys: []string{"M"}, why: "You can't merge in eggzec/gh-tui (read access)."},
		{name: "close", caps: readCaps, keys: []string{"X"}, why: "You can't close #142 in eggzec/gh-tui (read access)."},
		{name: "reopen", caps: readCaps, keys: []string{"]", "O"}, why: "You can't reopen #93 in eggzec/gh-tui (read access)."},
		{name: "draft", caps: readCaps, keys: []string{"W"}, why: "You can't change #142 in eggzec/gh-tui (read access)."},
		{name: "archived", caps: archived, keys: []string{"X"}, why: "eggzec/gh-tui is archived, so it's read-only."},
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
			km := h.KeyLayers()
			if m := h.modal(); m != nil {
				km = m.KeyLayers()
			}
			for _, desc := range offered(km) {
				if slices.Contains([]string{"merge", "close PR", "reopen PR", "convert to draft"}, desc) {
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
	if got := offered(h.KeyLayers()); !slices.Contains(got, "merge") {
		t.Errorf("help before the caps = %v, want merge offered", got)
	}
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: readCaps}))
	if got := offered(h.KeyLayers()); slices.Contains(got, "merge") {
		t.Errorf("help after read caps = %v, want no merge", got)
	}
	// The caps of another repository change nothing.
	drain(t, h, h.Update(ui.CapsMsg{Repo: core.RepoRef{Owner: "o", Name: "r"}, Caps: writeCaps}))
	if got := offered(h.KeyLayers()); slices.Contains(got, "merge") {
		t.Errorf("help after another's caps = %v, want no merge", got)
	}
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))
	if got := offered(h.KeyLayers()); !slices.Contains(got, "merge") {
		t.Errorf("help after write caps = %v, want merge", got)
	}
	press(t, h, "M")
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
	if got := offered(h.KeyLayers()); !slices.Contains(got, "close PR") || slices.Contains(got, "merge") {
		t.Errorf("help = %v, want close but no merge", got)
	}
	for _, k := range []string{"W", "y", "X", "y"} {
		press(t, h, k)
	}
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
			h := started(t, svc, 160, 20, WithMergeMethod(core.MergeSquash))
			drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: tt.caps}))
			if got := offered(h.KeyLayers()); !slices.Contains(got, tt.label) {
				t.Errorf("help = %v, want %q", got, tt.label)
			}
			press(t, h, "M")
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

func TestTokenGatesChanges(t *testing.T) {
	private := writeCaps
	private.Private = true
	why := "Merging needs the repo scope · :auth to grant it"
	for _, keys := range [][]string{{"M"}} {
		t.Run(strings.Join(keys, " "), func(t *testing.T) {
			tok := &uitest.Checker{A: uitest.Classic("public_repo")}
			v := ui.NewVoice(config.Default().Keys, "")
			v.Token = uitest.Token(tok)
			svc := newFakeService()
			h := started(t, svc, 120, 20, WithVoice(v))
			drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: private}))
			last := len(keys) - 1
			for _, k := range keys[:last] {
				press(t, h, k)
			}
			layers := func() []keyhelp.Layer {
				if m := h.modal(); m != nil {
					return m.KeyLayers()
				}
				return h.KeyLayers()
			}
			if got := offered(layers()); slices.Contains(got, "merge") {
				t.Errorf("help = %v, want no merge", got)
			}
			msgs := press(t, h, keys[last])
			if got := svc.changes(); len(got) != 0 || question(h) != "" || !slices.Contains(msgs, info(why)) {
				t.Errorf("merge sent %v and showed %v, want the toast %q only", got, msgs, why)
			}
			// Once the token has repo, the key works again.
			tok.A = uitest.Classic("repo")
			if got := offered(layers()); !slices.Contains(got, "merge") {
				t.Errorf("help after repo = %v, want merge", got)
			}
		})
	}
}
