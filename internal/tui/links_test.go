package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// linkedModal is a modal about something with a page on the web.
type linkedModal struct {
	fakeModal
	link string
}

func (l *linkedModal) Link() string { return l.link }

// The title of a modal with a page links to it, on the screen as the
// terminal gets it, however narrow the frame.
func TestModalTitleLinks(t *testing.T) {
	for _, host := range uitest.Hosts {
		for _, width := range []int{120, 80, 30, 12} {
			m, _ := newTestApp(t)
			m.Update(tea.WindowSizeMsg{Width: width, Height: 12})
			mod := &linkedModal{link: ui.WebURL(host, "o/r/pull/7")}
			mod.title = "#7"
			run(m, ui.OpenModal(mod))
			top, _, _ := strings.Cut(m.frame(mod), "\n")
			if links := uitest.Links(t, top); len(links) != 1 || links[0] != mod.link {
				t.Errorf("%s at %d: title links to %q, want %q once", host, width, links, mod.link)
			}
			if links := uitest.Links(t, m.View().Content); slices.Index(links, mod.link) < 0 {
				t.Errorf("%s at %d: screen links to %q, not to %q", host, width, links, mod.link)
			}
			// A title can't add a link of its own.
			mod.title = uitest.HostileTitle
			top, _, _ = strings.Cut(m.frame(mod), "\n")
			if links := uitest.Links(t, top); len(links) != 1 || links[0] != mod.link || strings.Contains(top, "evil.test") {
				t.Errorf("%s at %d: a hostile title links to %q: %q", host, width, links, top)
			}
		}
	}
}

// The header links the repository to its page on the user's host, and
// the other screens' titles to nothing.
func TestHeaderLinks(t *testing.T) {
	for _, host := range uitest.Hosts {
		m := newHeaderApp(t, WithHost(host))
		for _, width := range []int{120, 40, 10} {
			m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			want := ui.WebURL(host, testRepo.String())
			if links := uitest.Links(t, m.header); len(links) != 1 || links[0] != want {
				t.Errorf("%s at %d: header links to %q, want %q once", host, width, links, want)
			}
		}
		run(m, m.key(press("I")))
		if links := uitest.Links(t, m.header); len(links) != 0 {
			t.Errorf("%s: the notifications' header links to %q", host, links)
		}
	}
}
