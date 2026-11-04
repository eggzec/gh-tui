package pulls

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/prpanes"
)

func TestConvReactTicksAndSends(t *testing.T) {
	t.Parallel()
	svc := convService()
	h, m := convActions(t, svc, "ann", "C1", nil)
	press(t, h, "R")
	if _, ok := m.top().(*prpanes.TickOverlay[string]); !ok {
		t.Fatalf("R opens %T, want the reactions checklist", m.top())
	}
	press(t, h, "space")
	if got := svc.changes(); len(got) != 0 {
		t.Fatalf("a change was made before applying: %v", got)
	}
	msgs := press(t, h, "enter")
	if got, want := svc.changes(), []string{"react C1 THUMBS_UP true"}; !slices.Equal(got, want) {
		t.Errorf("changes %v, want %v", got, want)
	}
	if got := toasts(msgs); !slices.Equal(got, []string{"reaction added"}) {
		t.Errorf("toasts %v, want reaction added", got)
	}
}

func TestConvReactNeedsAComment(t *testing.T) {
	t.Parallel()
	svc := convService()
	h, m := convActions(t, svc, "ann", "R1", nil)
	msgs := press(t, h, "R")
	if got := toasts(msgs); !slices.Equal(got, []string{"select a comment to react to"}) {
		t.Errorf("toasts %v, want the hint", got)
	}
	if len(m.overlays) != 0 {
		t.Errorf("an overlay opened")
	}
}
