package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

func TestMergePreviewFollowsTheCursor(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, func(d *core.PullRequestDetail) {
		behindEdit(d)
		d.Draft = true
	})
	b := panes.Box{Pane: panes.Merge, Rect: panes.Rect{W: 60, H: 12}}
	if got := m.hover[panes.Merge-1]; got.id != "behind" || got.title != "Behind main" {
		t.Fatalf("hovered %+v before any key, want the behind row", got)
	}
	if body := ansi.Strip(m.mergePreview(b)); !strings.Contains(body, "updates the branch with main") {
		t.Errorf("the preview of the behind row:\n%s", body)
	}

	m.Update(keyMsg("j"))
	if got := m.hover[panes.Merge-1]; got.id != "draft" || got.title != "Draft" {
		t.Fatalf("hovered %+v after j, want the draft row", got)
	}
	if body := ansi.Strip(m.mergePreview(b)); !strings.Contains(body, "enter marks it ready for review") {
		t.Errorf("the preview of the draft row:\n%s", body)
	}
}
