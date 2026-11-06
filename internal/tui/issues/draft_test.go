package issues

import (
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
)

// memDrafts is a draft store in memory.
type memDrafts struct {
	mu sync.Mutex
	m  map[string]string
}

func (d *memDrafts) Load(key string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.m[key], nil
}

func (d *memDrafts) Save(key, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.m == nil {
		d.m = map[string]string{}
	}
	d.m[key] = text
	return nil
}

func (d *memDrafts) Delete(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.m, key)
	return nil
}

var _ composer.Drafts = (*memDrafts)(nil)

func TestHasDraft(t *testing.T) {
	tests := []struct {
		name  string
		open  bool
		value string
		want  bool
	}{
		{"no composer", false, "", false},
		{"an empty comment", true, "  ", false},
		{"a typed comment", true, "On it.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, m := opened(t, newFakeService(sampleIssues(12)), 30)
			if tt.open {
				press(t, h, "c")
				m.composerOpen().Comp.SetValue(tt.value)
			}
			if got := m.HasDraft(); got != tt.want {
				t.Errorf("HasDraft() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveKeepsTheDraft(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 30)
	store := &memDrafts{}
	m.drafts = store
	press(t, h, "c")
	m.composerOpen().Comp.SetValue("Half written.")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if len(store.m) != 1 {
		t.Fatalf("saved %v, want one draft", store.m)
	}
	for _, v := range store.m {
		if v != "Half written." {
			t.Errorf("saved %q", v)
		}
	}
}

func TestEditedCommentKeepsNoDraft(t *testing.T) {
	_, h, m, _ := ownModal(t)
	store := &memDrafts{}
	m.drafts = store
	press(t, h, "E")
	if m.composerOpen() == nil {
		t.Fatal("E opened no composer")
	}
	typeText(t, h, " Changed.")
	press(t, h, "esc")
	if m.composerOpen() != nil {
		t.Fatal("esc left the composer open")
	}
	if len(store.m) != 0 {
		t.Errorf("a cancelled edit saved %v, want no draft", store.m)
	}
}

func TestDraftIsKeptPerIssue(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 30)
	store := &memDrafts{}
	m.drafts = store
	press(t, h, "c")
	typeText(t, h, "Half")
	press(t, h, "esc")
	if m.composerOpen() != nil {
		t.Fatal("esc left the composer open")
	}
	if len(store.m) != 1 {
		t.Fatalf("saved %v, want one draft", store.m)
	}
	press(t, h, "c")
	if got := m.composerOpen().Comp.Value(); got != "Half" {
		t.Errorf("the reopened composer holds %q, want the draft %q", got, "Half")
	}

	// Another issue starts blank.
	press(t, h, "esc", "esc")
	if h.modal() != nil {
		t.Fatal("the modal stayed open")
	}
	press(t, h, "down", "enter")
	other := h.modal()
	if other == nil || other.number == m.number {
		t.Fatalf("opened %v, want another issue than #%d", other, m.number)
	}
	other.drafts = store
	press(t, h, "c")
	if other.composerOpen() == nil {
		t.Fatal("c opened no composer")
	}
	if got := other.composerOpen().Comp.Value(); got != "" {
		t.Errorf("another issue's composer holds %q, want it blank", got)
	}
}

func TestPostedCommentRemovesTheDraft(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 30)
	store := &memDrafts{}
	m.drafts = store
	press(t, h, "c")
	typeText(t, h, "Ready to go.")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if len(store.m) != 1 {
		t.Fatalf("saved %v, want one draft", store.m)
	}
	press(t, h, "ctrl+s", "y")
	if len(store.m) != 0 {
		t.Errorf("a posted comment left the draft %v", store.m)
	}
}

func TestCancelledBlankCommentKeepsNoDraft(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 30)
	store := &memDrafts{}
	m.drafts = store
	press(t, h, "c")
	if m.composerOpen() == nil {
		t.Fatal("c opened no composer")
	}
	press(t, h, "esc")
	if len(store.m) != 0 {
		t.Errorf("a cancelled blank comment saved %v", store.m)
	}
}
