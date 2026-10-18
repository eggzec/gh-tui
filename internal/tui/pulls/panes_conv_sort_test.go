package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestConvSortCycles(t *testing.T) {
	t.Parallel()
	h, m := convModal(t, convService())
	if got := m.conv.model.Order(); got != conv.OrderOldest {
		t.Fatalf("the conversation starts in order %v, want oldest first", got)
	}

	for _, want := range []struct {
		order conv.Order
		said  string
	}{
		{conv.OrderNewest, "newest first"},
		{conv.OrderActivity, "recent activity first"},
		{conv.OrderOldest, "oldest first"},
	} {
		msgs := press(t, h, "s")
		if got := m.conv.model.Order(); got != want.order {
			t.Errorf("s left order %v, want %v", got, want.order)
		}
		var said bool
		for _, msg := range msgs {
			if n, ok := msg.(ui.NotifyMsg); ok && n.Text == want.said {
				said = true
			}
		}
		if !said {
			t.Errorf("no toast %q after s; got %v", want.said, msgs)
		}
	}
}
