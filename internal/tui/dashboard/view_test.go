package dashboard

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
		icons         string
		// filter is applied after the keys, as the filter modal would.
		filter string
	}{
		{"140 columns", 140, 38, nil, "", ""},
		{"140 columns filter", 140, 38, nil, "", "is:private language:go sort:stars-desc"},
		{"140 columns calendar", 140, 38, []string{"4", "left"}, "", ""},
		{"140 columns work tab", 140, 38, []string{"3", "]"}, "", ""},
		{"100 columns", 100, 30, []string{"3", "[", "down"}, "", ""},
		{"120 columns zoomed", 120, 34, []string{"z"}, "", ""},
		{"120 columns zoomed work", 120, 34, []string{"z", "3"}, "", ""},
		{"80 columns", 80, 22, nil, "", ""},
		{"80 columns pinned", 80, 22, []string{"1", "right"}, "", ""},
		{"80 columns work", 80, 22, []string{"3", "down"}, "", ""},
		{"80 columns calendar", 80, 22, []string{"4"}, "", ""},
		{"80 columns notifications", 80, 22, []string{"5"}, "", ""},
		{"80 columns filter", 80, 22, []string{"]"}, "", "r2"},
		{"190 columns unicode", 190, 50, nil, config.IconsUnicode, ""},
		{"80 columns ascii", 80, 24, nil, config.IconsASCII, ""},
		{"80 columns work unicode", 80, 24, []string{"3"}, config.IconsUnicode, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.icons != "" {
				opts = append(opts, WithIcons(ui.NewIcons(tt.icons)))
			}
			s := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, tt.width, tt.height, opts...)
			press(t, s, tt.keys...)
			if tt.filter != "" {
				run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: tt.filter}))
			}
			golden.RequireEqual(t, s.View())
		})
	}
}

func TestViewStates(t *testing.T) {
	t.Run("loading", func(t *testing.T) {
		s := New(t.Context(), newFake(), config.Default().Keys, WithNow(func() time.Time { return now }))
		s.SetSize(140, 38)
		s.Focus()
		golden.RequireEqual(t, s.View())
	})
	t.Run("empty", func(t *testing.T) {
		svc := newFake()
		svc.header.Pinned, svc.header.Orgs = nil, nil
		svc.work = core.Work{}
		svc.contrib = core.Contributions{}
		svc.repos = map[string][]core.Repo{}
		s := newSection(t, svc, &fakeInbox{}, 140, 38, WithHere(core.RepoRef{}, nil))
		golden.RequireEqual(t, s.View())
	})
	// Every read fails, and each pane says why, in both themes: the
	// profile and the work, then the contributions.
	for _, tt := range []struct {
		pane string
		dark bool
	}{{"3", false}, {"3", true}, {"4", false}, {"4", true}} {
		t.Run(fmt.Sprintf("failed %s dark=%t", tt.pane, tt.dark), func(t *testing.T) {
			svc := newFake()
			err := errors.New("github: decode: unexpected EOF")
			for _, what := range []string{"header", "work", "contributions"} {
				svc.fail[what] = err
			}
			s := newSection(t, svc, &fakeInbox{err: err}, 80, 24, WithIcons(ui.NewIcons(config.IconsUnicode)),
				WithVoice(logVoice(t)))
			p, perr := config.Default().Palette(tt.dark)
			if perr != nil {
				t.Fatal(perr)
			}
			s.SetTheme(ui.NewTheme(p, tt.dark))
			press(t, s, tt.pane)
			golden.RequireEqual(t, s.View())
		})
	}
}

func TestLayout(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		days          int
		// cal is the outer width of the calendar, and weeks how many of
		// its weeks show.
		cal, weeks int
	}{
		// A year fits beside the notifications on a wide screen.
		{"190 year", 190, 50, 0, 4 + 4 + 53*2 - 1, 53},
		// Narrower, the calendar gives up its oldest weeks so that the
		// notifications keep minInboxW.
		{"140 year", 140, 40, 0, 140 - minInboxW, (140 - minInboxW - calendarPad - 4 + 1) / 2},
		{"100 year", 100, 30, 0, 100 - minInboxW, (100 - minInboxW - calendarPad - 4 + 1) / 2},
		// 90 days take the width of their total at any size.
		{"190 90 days", 190, 50, 90, calendarPad + len("297 contributions in the last 90 days"), 14},
		{"100 90 days", 100, 30, 90, calendarPad + len("297 contributions in the last 90 days"), 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, tt.width, tt.height, WithContributions(tt.days))
			b := s.boxes
			if b[calendarPane].w != tt.cal || b[calendarPane].w+b[inboxPane].w != tt.width {
				t.Errorf("the bottom row is %d + %d wide, want a calendar of %d and the notifications the rest of %d",
					b[calendarPane].w, b[inboxPane].w, tt.cal, tt.width)
			}
			if b[inboxPane].w < minInboxW {
				t.Errorf("the notifications are %d wide, narrower than %d", b[inboxPane].w, minInboxW)
			}
			if b[calendarPane].h != calendarHeight || b[inboxPane].h != calendarHeight {
				t.Errorf("the bottom row is %d and %d high, want the %d of the calendar", b[calendarPane].h, b[inboxPane].h, calendarHeight)
			}
			if b[workPane].h != b[reposPane].h || b[workPane].w+b[reposPane].w != tt.width {
				t.Errorf("the work is %dx%d beside repositories of %dx%d, want it to fill the middle row",
					b[workPane].w, b[workPane].h, b[reposPane].w, b[reposPane].h)
			}
			if got := s.profileHeight() + b[pinnedPane].h + b[reposPane].h + b[calendarPane].h; got != tt.height {
				t.Errorf("the rows take %d lines of %d", got, tt.height)
			}
			if s.cal.Range() != tt.days {
				t.Errorf("the calendar shows %d days, want %d", s.cal.Range(), tt.days)
			}
			// Each week is a column of the weekday rows of the calendar.
			lines := strings.Split(screen(s), "\n")
			bottom := lines[len(lines)-calendarHeight:]
			cols := map[int]bool{}
			for _, l := range bottom[3:10] {
				for i, r := range []rune(ansi.Truncate(l, tt.cal, "")) {
					if r == '■' {
						cols[i] = true
					}
				}
			}
			if weeks := len(cols); weeks != tt.weeks {
				t.Errorf("%d weeks show, want %d:\n%s", weeks, tt.weeks, strings.Join(bottom, "\n"))
			}
		})
	}
}

// TestViewFits checks that every line of the dashboard is exactly as wide
// as it and holds no tab, whichever icons draw it. It measures with
// ansi.StringWidth, the renderer's own measure, so it catches padding and
// truncation that disagree with it; it can't tell whether a terminal draws
// a glyph of the private use area in one cell or two.
func TestViewFits(t *testing.T) {
	sizes := []struct{ width, height int }{{120, 36}, {80, 24}}
	for _, icons := range []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII} {
		for _, sz := range sizes {
			t.Run(fmt.Sprintf("%s/%dx%d", icons, sz.width, sz.height), func(t *testing.T) {
				s := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, sz.width, sz.height, WithIcons(ui.NewIcons(icons)))
				for _, pane := range []string{"1", "2", "3", "4", "5"} {
					press(t, s, pane)
					for i, l := range strings.Split(s.View(), "\n") {
						if w := ansi.StringWidth(l); w != sz.width || strings.ContainsAny(l, "\t\r") {
							t.Errorf("pane %s: line %d is %d wide, want %d: %q", pane, i, w, sz.width, ansi.Strip(l))
							break
						}
					}
				}
			})
		}
	}
}
