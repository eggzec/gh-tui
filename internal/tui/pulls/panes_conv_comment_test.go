package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestConvCommentHintAndReload(t *testing.T) {
	t.Parallel()
	svc := convService()
	_, m := convActions(t, svc, "me", "", nil)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "c comment") {
		t.Fatalf("the footer lacks the comment hint:\n%s", v)
	}
	// The service forgets the timeline once the comment went through, so the
	// conversation must read its pages again rather than from the cache.
	if cmd := m.Update(convRecacheMsg{id: m.id, reload: true}); cmd == nil {
		t.Fatal("a posted comment does not make the conversation read again")
	}
}
