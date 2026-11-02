package pulls

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// The state and title go to the top edge of the frame once they are known; the
// number alone is there before.
func TestTrimTitle(t *testing.T) {
	s := started(t, newFakeService(), 120, 30)
	press(t, s, "enter")
	m := s.modal()
	m.loaded = false
	if got := m.TrimTitle(); got.Lead != "" || got.Detail != "" || got.Name != m.Title() {
		t.Errorf("before it loads: %+v, want the number only", got)
	}
	m.loaded = true
	m.detail.Title = "Fix it"
	leads := map[string]bool{}
	for _, c := range []struct {
		state core.State
		draft bool
	}{{core.StateOpen, false}, {core.StateOpen, true}, {core.StateMerged, false}, {core.StateClosed, false}} {
		m.detail.State, m.detail.Draft = c.state, c.draft
		got := m.TrimTitle()
		if got.Detail != "Fix it" || got.Name != m.Title() || ansi.Strip(got.Lead) == "" {
			t.Errorf("%v draft %v: %+v", c.state, c.draft, got)
		}
		leads[got.Lead] = true
	}
	if len(leads) != 4 {
		t.Errorf("the four states share glyphs: %d distinct", len(leads))
	}
	m.other = true
	if got := m.TrimTitle().Name; !strings.HasPrefix(got, m.repo.String()) || !strings.HasSuffix(got, "#"+strconv.Itoa(m.number)) {
		t.Errorf("another repository's name %q lacks the repository", got)
	}
}

// With no labels the header takes no row, and the panes get it; with labels it
// is one line that shows as many as fit and the number of the others.
func TestHeaderRows(t *testing.T) {
	s := started(t, newFakeService(), 80, 30)
	press(t, s, "enter")
	m := s.modal()
	m.SetSize(80, 30)
	m.detail.Labels = nil
	rows := func() int { return len(strings.Split(m.View(), "\n")) }
	if m.headerRows() != 0 {
		t.Fatalf("no labels: %d header rows", m.headerRows())
	}
	free := m.areaHeight()
	for i := range 30 {
		m.detail.Labels = append(m.detail.Labels, core.Label{Name: "label-" + strconv.Itoa(i)})
	}
	if m.headerRows() != 1 || m.areaHeight() != free-1 {
		t.Errorf("labels: %d header rows, area %d, want 1 and %d", m.headerRows(), m.areaHeight(), free-1)
	}
	if rows() != 30 {
		t.Errorf("the view is %d rows, want 30", rows())
	}
	h := ansi.Strip(m.header())
	if ansi.StringWidth(h) != 80 || !strings.Contains(h, "+") || !strings.Contains(h, "label-0") {
		t.Errorf("header %q should hold the first labels and a +N", h)
	}
	m.detail.Labels = nil
	if rows() != 30 {
		t.Errorf("no labels: the view is %d rows, want 30", rows())
	}
}
