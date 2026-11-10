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

// The account in the status bar links to the user's profile, or to the
// host's page before the login is known.
func TestStatusAccountLink(t *testing.T) {
	t.Parallel()
	for _, host := range uitest.Hosts {
		m, _ := newTestApp(t)
		m.host = host
		m.login = ""
		if got := uitest.Links(t, strings.Join(m.linkItem().Forms, "\n")); len(got) == 0 || got[0] != ui.WebURL(host, "") {
			t.Errorf("%s: no login: links to %q", host, got)
		}
		m.login = "octo-cat"
		if got := uitest.Links(t, strings.Join(m.linkItem().Forms, "\n")); len(got) == 0 || got[0] != ui.WebURL(host, "octo-cat") {
			t.Errorf("%s: links to %q, want the profile", host, got)
		}
	}
}

// The title of a modal with a page links to it, on the screen as the
// terminal gets it, however narrow the frame.
func TestModalTitleLinks(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	for _, host := range uitest.Hosts {
		m := newHeaderApp(t, WithHost(host))
		for _, width := range []int{120, 40, 10} {
			m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			want := ui.WebURL(host, testRepo.String())
			links := uitest.Links(t, m.header)
			if len(links) == 0 || links[0] != want {
				t.Errorf("%s at %d: header links to %q, want %q first", host, width, links, want)
			}
			// The count of notifications links to the inbox while it fits.
			if width == 120 && !slices.Contains(links, ui.WebURL(host, "notifications")) {
				t.Errorf("%s at %d: header links to %q, not to the inbox", host, width, links)
			}
		}
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
		run(m, m.showScreen(dashScreen, 0))
		if links := uitest.Links(t, m.header); !slices.Equal(links, []string{ui.WebURL(host, "notifications")}) {
			t.Errorf("%s: the dashboard's header links to %q, want the inbox only", host, links)
		}
	}
}

// The titles of the pull requests and the issues link to their pages on the
// repository, the others to nothing.
func TestPaneTitleLinks(t *testing.T) {
	t.Parallel()
	for _, host := range uitest.Hosts {
		m, _ := newTestApp(t, WithHost(host))
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
		want := map[string]string{
			"Pull requests": ui.WebURL(host, testRepo.String()+"/pulls"),
			"Issues":        ui.WebURL(host, testRepo.String()+"/issues"),
		}
		for _, p := range m.panes {
			links := uitest.Links(t, p.top)
			if w, ok := want[p.section.Title()]; ok {
				if len(links) != 1 || links[0] != w {
					t.Errorf("%s: %s links to %q, want %q", host, p.label, links, w)
				}
			} else if len(links) != 0 {
				t.Errorf("%s: %s links to %q", host, p.label, links)
			}
		}
	}
}
