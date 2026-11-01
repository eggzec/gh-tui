package notifications

import (
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Rows draws threads as the notifications list does, for the panes that
// show a few of them. Create one with NewRows.
type Rows struct {
	styles styles
	icons  ui.Icons
}

// NewRows returns rows painted in theme t, with icons ic.
func NewRows(t ui.Theme, ic ui.Icons) Rows {
	return Rows{styles: newStyles(t, ic), icons: ic}
}

// Row draws n in width cells, with its age as dates tell it at now. sub is
// the state of its subject, if known, and look is ui.Look of whether the row
// is selected and its pane focused.
func (r *Rows) Row(n core.Notification, sub core.SubjectState, known bool, look, width int, dates ui.Dates, now time.Time, links termtext.Links) string {
	return drawRow(&r.styles, r.icons, dates, links, now, n, sub, known, look, width)
}
