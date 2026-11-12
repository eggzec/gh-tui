package pulls

import (
	"strconv"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

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
