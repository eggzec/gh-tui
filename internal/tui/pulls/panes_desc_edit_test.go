package pulls

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// descPanes returns a modal that the viewer may change, with the Description
// pane focused over a loaded pull request that has title and body.
func descPanes(t *testing.T, title, body string) (*panesModal, *fakeService) {
	t.Helper()
	m, svc := reviewPanes(t)
	m.detail.Title, m.detail.Body = title, body
	m.focus = panes.Description
	m.View()
	return m, svc
}

func TestQuote(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, in, want string }{
		{"one line", "hello", "> hello\n\n"},
		{"blank line stays a quote", "a\n\nb", "> a\n>\n> b\n\n"},
		{"trailing newlines", "a\n\n", "> a\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := quote(tt.in); got != tt.want {
				t.Errorf("quote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDescriptionEditSaves(t *testing.T) {
	t.Parallel()
	m, svc := descPanes(t, "Old", "text")
	m.Update(keyMsg("e"))
	o, ok := m.top().(*descEditOverlay)
	if !ok {
		t.Fatalf("e opened %T, want the editor", m.top())
	}
	if o.titleF.Value() != "Old" || o.bodyF.Value() != "text" {
		t.Fatalf("the editor holds %q and %q", o.titleF.Value(), o.bodyF.Value())
	}
	typeText(m, "!")
	m.Update(keyMsg("tab"))
	typeText(m, "y")
	if len(svc.changes()) != 0 {
		t.Fatal("the change was sent before the save")
	}
	m.Update(keyMsg("ctrl+s"))
	got := svc.changes()
	if len(got) != 1 || !strings.HasPrefix(got[0], "edit title=Old! body=text") || !strings.Contains(got[0], "y") {
		t.Fatalf("changes = %q, want the edited title and body", got)
	}
	if m.top() != nil {
		t.Errorf("the editor stayed open: %T", m.top())
	}
}

func TestDescriptionEditCancels(t *testing.T) {
	t.Parallel()
	m, svc := descPanes(t, "Old", "text")
	m.Update(keyMsg("e"))
	typeText(m, "!")
	m.Update(keyMsg("esc"))
	if m.top() != nil || len(svc.changes()) != 0 || m.closed {
		t.Errorf("esc: open %v, changes %q, modal closed %v; want only the editor closed", m.top() != nil, svc.changes(), m.closed)
	}
}

func TestDescriptionEditNeedsATitle(t *testing.T) {
	t.Parallel()
	m, svc := descPanes(t, "A", "text")
	m.Update(keyMsg("e"))
	m.Update(keyMsg("backspace"))
	cmd := m.Update(keyMsg("ctrl+s"))
	if _, ok := find[ui.NotifyMsg](cmd); !ok || len(svc.changes()) != 0 || m.top() == nil {
		t.Errorf("an empty title: notice %v, changes %q, editor open %v", ok, svc.changes(), m.top() != nil)
	}
}

func TestDescriptionEditRefusedWithoutUpdateRight(t *testing.T) {
	t.Parallel()
	m, _ := descPanes(t, "Old", "text")
	m.detail.Caps = core.ItemCaps{Known: true}
	cmd := m.Update(keyMsg("e"))
	if m.top() != nil {
		t.Fatalf("e opened %T for a viewer who may not update", m.top())
	}
	if _, ok := find[ui.NotifyMsg](cmd); !ok {
		t.Error("e said nothing")
	}
}

func TestDescriptionQuoteComments(t *testing.T) {
	t.Parallel()
	m, svc := descPanes(t, "Old", "line one\n\nline two")
	m.Update(keyMsg("c"))
	o, ok := m.top().(*composerOverlay)
	if !ok {
		t.Fatalf("c opened %T, want the composer", m.top())
	}
	if want := "> line one\n>\n> line two\n\n"; o.comp.Value() != want {
		t.Fatalf("the composer starts with %q, want %q", o.comp.Value(), want)
	}
	typeText(m, "hi")
	submit(m)
	got := svc.changes()
	if len(got) != 1 || !strings.Contains(got[0], "> line two\n\nhi") {
		t.Fatalf("changes = %q, want the quote and the answer as a comment", got)
	}
	if m.top() != nil {
		t.Errorf("the composer stayed open: %T", m.top())
	}
}

func TestDescriptionQuoteOfNothing(t *testing.T) {
	t.Parallel()
	m, _ := descPanes(t, "Old", "")
	cmd := m.Update(keyMsg("c"))
	if m.top() != nil {
		t.Fatalf("c opened %T for an empty description", m.top())
	}
	if _, ok := find[ui.NotifyMsg](cmd); !ok {
		t.Error("c said nothing")
	}
}
