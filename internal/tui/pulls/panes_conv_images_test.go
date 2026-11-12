package pulls

import (
	"strconv"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// A Conversation with no height (another pane is zoomed) keeps the picture
// rows it had, whichever way its options are asked for again.
func TestConvOptionsKeepPicturesWithoutHeight(t *testing.T) {
	t.Parallel()
	_, m := convModal(t, convService())
	m.pics = uitest.Avatars(&uitest.ImageHost{}, true)
	m.View() // sizes the Conversation: convContent sets its height
	if m.conv.h <= 2 {
		t.Fatalf("the Conversation has height %d, want a real one", m.conv.h)
	}
	m.convOptions()
	rows := m.conv.picRows
	if rows <= 1 { // PictureRows(0) is 1, so 1 can't tell kept from recomputed
		t.Fatalf("picRows = %d at height %d, want more than 1", rows, m.conv.h)
	}
	m.conv.h = 0
	m.convOptions()
	if m.conv.picRows != rows {
		t.Errorf("picRows = %d at no height, want %d kept", m.conv.picRows, rows)
	}
}

// A comment of the Conversation draws its images when the app's images setting
// is on, and shows its alt text as it did before when it is off.
func TestConvCommentImages(t *testing.T) {
	t.Parallel()
	for _, on := range []bool{true, false} {
		t.Run(strconv.FormatBool(on), func(t *testing.T) {
			t.Parallel()
			svc := convService()
			svc.timeline.pages = map[string]core.Page[core.TimelineItem]{"": {Items: []core.TimelineItem{
				{Kind: core.TimelineComment, ID: "C1", Actor: core.User{Login: "ann"}, At: convAt, Body: withImage},
			}}}
			src := &uitest.ImageHost{}
			_, m := convModal(t, svc)
			m.pics = uitest.Avatars(src, on)
			m.Update(ui.ImagesMsg{})
			m.View()
			uitest.LoadAvatars(t, m.pics)
			m.Update(ui.ImagesMsg{})
			view := m.View()
			n := uitest.Placeholders(t, view)
			if on && n == 0 {
				t.Errorf("with images on, the comment draws no image:\n%s", view)
			}
			if !on && (n != 0 || !strings.Contains(view, "the logo")) {
				t.Errorf("with images off, the comment draws images (%d) or lacks its alt text:\n%s", n, view)
			}
		})
	}
}
