package tui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/modals"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The fakes of the tests rank as the real modals they stand for do: a content
// modal by default, a detail where the fake is a pager.
func init() {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*fakeModal](),
		reflect.TypeFor[*backModal](),
		reflect.TypeFor[*disabledBackModal](),
		reflect.TypeFor[*leavingModal](),
		reflect.TypeFor[*tabbedModal](),
		reflect.TypeFor[*overlayingModal](),
		reflect.TypeFor[*hidingModal](),
		reflect.TypeFor[*settlingModal](),
		reflect.TypeFor[*ctxModal](),
		reflect.TypeFor[*linkedModal](),
		reflect.TypeFor[*pullsModal](),
		reflect.TypeFor[*screenModal](),
		reflect.TypeFor[*savingModal](),
	} {
		modalPriorities[typ] = modals.PriorityContent
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*pagerModal](),
		reflect.TypeFor[*sourceModal](),
	} {
		modalPriorities[typ] = modals.PriorityDetail
	}
}

// previewModal is a backModal that ranks as a detail over a content modal,
// so closing it returns to what is under it.
type previewModal struct{ backModal }

func init() { modalPriorities[reflect.TypeFor[*previewModal]()] = modals.PriorityDetail }

// Update and Act close the preview itself, not the backModal inside it, which
// the manager does not know.
func (p *previewModal) Update(msg tea.Msg) tea.Cmd {
	p.msgs = append(p.msgs, msg)
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyEscape {
		return ui.CloseModal(p)
	}
	return nil
}

func (p *previewModal) Act(action string) (tea.Cmd, bool) {
	if action == ui.ActQuit {
		return ui.CloseModal(p), true
	}
	return nil, false
}

// TestEveryRealModalIsRanked checks that the table ranks the modals the app
// opens, and that a type it lacks is refused rather than opened.
func TestEveryRealModalIsRanked(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	if _, ok := m.priorityOf(&authModal{}); !ok {
		t.Error("the token prompt has no rank")
	}
	if _, ok := m.priorityOf(&textModal{}); !ok {
		t.Error("the text modal has no rank")
	}
	if _, ok := m.priorityOf(struct{ *fakeModal }{}); ok {
		t.Error("an unlisted type has a rank")
	}
}
