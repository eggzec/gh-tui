package issues

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// A body taller than its pane scrolls with the keys of the tree, so no text
// is out of reach.
func TestDescriptionScrolls(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	var body []string
	for i := range 60 {
		body = append(body, fmt.Sprintf("paragraph %d", i))
	}
	for i := range svc.issues {
		svc.issues[i].Body = strings.Join(body, "\n\n")
	}
	h := started(t, svc, 80, 24)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "1")
	if v := ansi.Strip(m.View()); strings.Contains(v, "paragraph 59") {
		t.Fatalf("the last paragraph shows before any scrolling:\n%s", v)
	}
	for range 150 {
		press(t, h, "down")
	}
	v := ansi.Strip(m.View())
	assertFits(t, m.View(), 80, 24)
	if !strings.Contains(v, "paragraph 59") {
		t.Errorf("the end of the body is out of reach:\n%s", v)
	}
}

// People who comment appear in Involved once the timeline has been read,
// however late it arrives.
func TestInvolvedListsCommenters(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, core.Comment{
		Author: core.User{Login: "latecomer"}, Body: "me too",
	})
	h := started(t, svc, 80, 24)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "3")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "latecomer") {
		t.Errorf("Involved lacks the commenter:\n%s", v)
	}
}
