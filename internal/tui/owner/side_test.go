package owner

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// The README and the calendar, each focused in a narrow page, and the
// page of a user and of an organization that has them beside the list,
// in every icon set.
func TestSideView(t *testing.T) {
	tests := []struct {
		name          string
		login         string
		width, height int
		keys          []string
	}{
		{"user readme", "octocat", 80, 24, []string{"3"}},
		{"user readme scrolled", "octocat", 80, 16, []string{"3", "d"}},
		{"user calendar", "octocat", 80, 24, []string{"4"}},
		{"org readme", "github", 80, 24, []string{"3"}},
		{"user wide", "octocat", 120, 40, nil},
		{"org wide", "github", 120, 40, []string{"3"}},
	}
	for _, tt := range tests {
		for _, icons := range []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII} {
			t.Run(tt.name+" "+icons, func(t *testing.T) {
				s := newSection(t, newFake(), tt.login, tt.width, tt.height, WithIcons(ui.NewIcons(icons)))
				press(t, s, tt.keys...)
				golden.RequireEqual(t, s.View())
			})
		}
	}
}

func TestSideViewStates(t *testing.T) {
	t.Run("no readme", func(t *testing.T) {
		svc := newFake()
		delete(svc.readmes, "octocat")
		s := newSection(t, svc, "octocat", 80, 24)
		press(t, s, "3")
		golden.RequireEqual(t, s.View())
	})
	t.Run("org without readme", func(t *testing.T) {
		svc := newFake()
		delete(svc.readmes, "github")
		s := newSection(t, svc, "github", 80, 24)
		press(t, s, "3")
		golden.RequireEqual(t, s.View())
	})
	t.Run("readme failed", func(t *testing.T) {
		svc := newFake()
		svc.sideFake.fail["readme"] = core.ErrRateLimited
		s := newSection(t, svc, "octocat", 80, 24)
		press(t, s, "3")
		golden.RequireEqual(t, s.View())
	})
	t.Run("calendar failed", func(t *testing.T) {
		svc := newFake()
		svc.sideFake.fail["contributions"] = errors.New("github: decode: unexpected EOF")
		s := newSection(t, svc, "octocat", 80, 24)
		press(t, s, "4")
		golden.RequireEqual(t, s.View())
	})
}

// The README is read once its pane is on view, with what the header says
// of the account, and the calendar once its pane is.
func TestSideReadsOnView(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 80, 24)
	if slices.ContainsFunc(svc.sideFake.calls, func(c string) bool { return !strings.HasPrefix(c, "followers") }) {
		t.Fatalf("a narrow page on the list read %q", svc.sideFake.calls)
	}
	press(t, s, "3")
	if want := []string{"readme octocat", "sponsors octocat", "sponsoring octocat"}; !slices.Equal(svc.sideFake.calls, want) {
		t.Errorf("focusing the README read %q, want %q", svc.sideFake.calls, want)
	}
	svc.sideFake.calls = nil
	press(t, s, "4", "3")
	if want := []string{"contributions octocat"}; !slices.Equal(svc.sideFake.calls, want) {
		t.Errorf("focusing the calendar and the README again read %q, want %q", svc.sideFake.calls, want)
	}
}

// An organization has no calendar: its key and the cycle of the panes
// pass it by, and it reads its follower count for the profile.
func TestOrgHasNoCalendar(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "github", 120, 40)
	press(t, s, "4")
	if s.page.focus != listPane {
		t.Errorf("4 focused pane %d of an organization", s.page.focus)
	}
	press(t, s, "tab", "tab")
	if s.page.focus != pinnedPane {
		t.Errorf("tab twice from the list focused pane %d, want the pins past the README", s.page.focus)
	}
	if slices.Contains(svc.sideFake.calls, "contributions github") || !slices.Contains(svc.sideFake.calls, "followers github") {
		t.Errorf("an organization read %q", svc.sideFake.calls)
	}
	if !strings.Contains(ansi.Strip(s.View()), "41k followers") {
		t.Error("the profile doesn't count the organization's followers")
	}
}

// GitHub Sponsors is github.com's only, so elsewhere the README ends
// without them, and none are read.
func TestSponsorsOnGitHubOnly(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 80, 40, WithHost("ghe.example.com"))
	press(t, s, "3")
	if slices.ContainsFunc(svc.sideFake.calls, func(c string) bool { return strings.HasPrefix(c, "sponsor") }) {
		t.Errorf("an Enterprise Server read %q", svc.sideFake.calls)
	}
	if v := ansi.Strip(s.View()); strings.Contains(v, "Sponsors") {
		t.Errorf("an Enterprise Server shows sponsors:\n%s", v)
	}

	s = newSection(t, newFake(), "octocat", 80, 40)
	press(t, s, "3")
	v := ansi.Strip(s.View())
	for _, want := range []string{"Sponsors @mona, @hubot, @mona_corp", "Sponsoring @charmbracelet and more"} {
		if !strings.Contains(v, want) {
			t.Errorf("the README doesn't end with %q:\n%s", want, v)
		}
	}
}

// Sponsors that fail to load leave the README as it is.
func TestSponsorsFailed(t *testing.T) {
	svc := newFake()
	svc.sideFake.fail["sponsors"] = core.ErrUnsupported
	s := newSection(t, svc, "octocat", 80, 40)
	press(t, s, "3")
	if v := ansi.Strip(s.View()); strings.Contains(v, "Sponsor") || !strings.Contains(v, "Hi, I'm Mona") {
		t.Errorf("failed sponsors show:\n%s", v)
	}
}

func TestReadmeAddresses(t *testing.T) {
	user := core.Readme{Source: core.RepoRef{Owner: "octocat", Name: "octocat"}}
	org := core.Readme{Source: core.RepoRef{Owner: "github", Name: ".github"}}
	tests := []struct {
		name, host string
		readme     core.Readme
		kind       core.OwnerKind
		addr       string
		image      bool
		want       string
	}{
		{"link", "", user, core.OwnerUser, "notes/a b.md#top", false, "https://github.com/octocat/octocat/blob/HEAD/notes/a%20b.md#top"},
		{"root link", "", user, core.OwnerUser, "/docs?plain=1", false, "https://github.com/octocat/octocat/blob/HEAD/docs?plain=1"},
		{"image", "", user, core.OwnerUser, "./img/logo.png", true, "https://raw.githubusercontent.com/octocat/octocat/HEAD/img/logo.png"},
		{"org image", "", org, core.OwnerOrg, "banner.png", true, "https://raw.githubusercontent.com/github/.github/HEAD/profile/banner.png"},
		{"org root image", "", org, core.OwnerOrg, "/banner.png", true, "https://raw.githubusercontent.com/github/.github/HEAD/banner.png"},
		{"enterprise image", "ghe.example.com", user, core.OwnerUser, "logo.png", true, "https://ghe.example.com/raw/octocat/octocat/HEAD/logo.png"},
		{"enterprise link", "ghe.example.com", user, core.OwnerUser, "a.md", false, "https://ghe.example.com/octocat/octocat/blob/HEAD/a.md"},
		{"web", "", user, core.OwnerUser, "https://example.com/a.png", true, "https://example.com/a.png"},
		{"anchor", "", user, core.OwnerUser, "#usage", false, "#usage"},
		{"mail", "", user, core.OwnerUser, "mailto:mona@example.com", false, "mailto:mona@example.com"},
		{"outside", "", user, core.OwnerUser, "../../other", false, "../../other"},
		{"escaped slash", "", user, core.OwnerUser, "docs/a%2Fb.md", false, "https://github.com/octocat/octocat/blob/HEAD/docs/a%2Fb.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(t.Context(), newFake(), config.Default().Keys, WithHost(tt.host))
			if got := s.resolver(tt.readme, tt.kind)(tt.addr, tt.image); got != tt.want {
				t.Errorf("%q = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// The README pane scrolls with the pager's keys, which the help shows,
// and o opens the README on GitHub, as copy copies it.
func TestReadmeKeys(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 80, 16)
	press(t, s, "3")
	if b, src, ok := uitest.Winner(s.KeyLayers(), "d"); !ok || src != "readme" || b.Help().Desc != "½ page down" {
		t.Errorf("d reaches %v %q %q, want the README's half page down", ok, src, b.Help().Desc)
	}
	if _, _, ok := uitest.Winner(s.KeyLayers(), "/"); ok {
		t.Error("/ reaches the README, which doesn't search")
	}
	before := s.page.side.pager.View()
	press(t, s, "d")
	if s.page.side.pager.View() == before {
		t.Error("d didn't scroll the README")
	}
	msg, ok := has[ui.OpenMsg](press(t, s, "o"))
	if !ok || msg.URL != "https://github.com/octocat/octocat/blob/HEAD/README.md" {
		t.Errorf("o sent %v, want the README", msg)
	}
	if sel, ok := s.Selected(); !ok || sel.URL != "https://github.com/octocat/octocat/blob/HEAD/README.md" || sel.Path != "README.md" {
		t.Errorf("selected %+v, want the README", sel)
	}
	press(t, s, "4")
	if _, src, ok := uitest.Winner(s.KeyLayers(), "left"); !ok || src != "calendar" {
		t.Errorf("left reaches %v %q, want the calendar", ok, src)
	}
}

// The pages keep their place in the README, and a refresh reads it again.
func TestReadmeRefresh(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 120, 40)
	svc.sideFake.calls = nil
	press(t, s, "r")
	for _, want := range []string{"readme octocat", "contributions octocat"} {
		if !slices.Contains(svc.sideFake.calls, want) {
			t.Errorf("refresh read %q, want %q among them", svc.sideFake.calls, want)
		}
	}
}

// Coming back to the page reads again what of the side went stale.
func TestSideRevisit(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 120, 40)
	svc.sideFake.calls = nil
	run(t, s, s.Revisit())
	if len(svc.sideFake.calls) != 0 {
		t.Errorf("a fresh page read %q", svc.sideFake.calls)
	}
	svc.sideFake.read = map[string]bool{}
	run(t, s, s.Revisit())
	for _, want := range []string{"readme octocat", "contributions octocat", "sponsors octocat"} {
		if !slices.Contains(svc.sideFake.calls, want) {
			t.Errorf("a stale page read %q, want %q among them", svc.sideFake.calls, want)
		}
	}
}

// The calendar follows dashboard.calendar_glyph and
// dashboard.contributions, as the dashboard's does.
func TestCalendarSettings(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 80, 24, WithCalendar("#", 30))
	press(t, s, "4")
	if v := ansi.Strip(s.View()); !strings.Contains(v, "contributions in the last 30 days") || !strings.Contains(v, "# # #") {
		t.Errorf("the calendar doesn't follow its options:\n%s", v)
	}
	c := config.Default()
	c.Dashboard.CalendarGlyph = "▪"
	c.Dashboard.Contributions = config.ContributionsYear
	s.Update(ui.SettingsMsg{Config: c})
	if v := ansi.Strip(s.View()); !strings.Contains(v, "in the last year") || !strings.Contains(v, "▪") {
		t.Errorf("the calendar doesn't follow the settings:\n%s", v)
	}
}

// Where the terminal shows images, the README draws those alone on their
// lines, its own by their raw files, and leaves SVG images as links.
func TestReadmeImages(t *testing.T) {
	svc := newFake()
	svc.readmes["octocat"] = core.Readme{
		Markdown: "# Hi\n\n![logo](img/logo.png)\n\n![card](https://example.com/card.svg)\n",
		Source:   core.RepoRef{Owner: "octocat", Name: "octocat"},
	}
	host := &uitest.ImageHost{}
	images := uitest.Avatars(host, true)
	s := newSection(t, svc, "octocat", 80, 24, WithAvatars(images))
	press(t, s, "3")
	if _, changed := uitest.LoadAvatars(t, images); changed {
		run(t, s, s.Update(ui.ImagesMsg{}))
	}
	asked := slices.DeleteFunc(host.Asked(), func(a string) bool { return strings.Contains(a, "avatars") })
	if want := []string{"https://raw.githubusercontent.com/octocat/octocat/HEAD/img/logo.png"}; !slices.Equal(asked, want) {
		t.Errorf("the README asked for %q, want %q", asked, want)
	}
	v := s.View()
	if uitest.Placeholders(t, v) == 0 {
		t.Errorf("the README draws no image:\n%s", ansi.Strip(v))
	}
	press(t, s, "G")
	if v := ansi.Strip(s.View()); !strings.Contains(v, "card.svg") {
		t.Errorf("the SVG image isn't a link:\n%s", v)
	}
}

// A refresh while a read of the side is on its way drops its reply, and
// the read no longer shows as loading.
func TestRefreshDropsSideReads(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 80, 24)
	p := s.page
	// The calendar's pane is off view, so the refresh doesn't read it.
	cmd := s.readContribs(p, false)
	press(t, s, "r")
	run(t, s, cmd)
	if p.side.contribs.loading || p.side.contribs.ok {
		t.Errorf("the calendar read before the refresh is loading %v, kept %v", p.side.contribs.loading, p.side.contribs.ok)
	}
	if s.updating() {
		t.Error("the page still shows as updating")
	}
}

// What is off view isn't read again when it went stale, or once GitHub
// answers again.
func TestSideRereadsOnView(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 80, 24)
	press(t, s, "3", "4", "2")
	svc.sideFake.read = map[string]bool{}
	svc.sideFake.calls = nil
	run(t, s, s.Revisit())
	run(t, s, s.Update(ui.OnlineMsg{}))
	if len(svc.sideFake.calls) != 0 {
		t.Errorf("the panes off view read %q", svc.sideFake.calls)
	}
	press(t, s, "3")
	run(t, s, s.Revisit())
	if !slices.Contains(svc.sideFake.calls, "readme octocat") || slices.Contains(svc.sideFake.calls, "contributions octocat") {
		t.Errorf("the README on view read %q, want the README alone", svc.sideFake.calls)
	}
}

// A README the revalidator found changed shows, and so does the README
// only members see, once the viewer joins.
func TestReadmeSyncs(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "github", 120, 40)
	svc.readmes["github"] = core.Readme{Markdown: "Changed in the background.", Source: core.RepoRef{Owner: "github", Name: ".github"}}
	run(t, s, s.Update(ui.SyncMsg{Key: owners.SyncKey("other")}))
	if strings.Contains(ansi.Strip(s.View()), "Changed in the background") {
		t.Fatal("a change of another account showed")
	}
	run(t, s, s.Update(ui.SyncMsg{Key: owners.SyncKey("GitHub")}))
	if !strings.Contains(ansi.Strip(s.View()), "Changed in the background") {
		t.Errorf("the README didn't change:\n%s", ansi.Strip(s.View()))
	}

	o := org()
	o.Viewer.Member = false
	svc.owners["github"] = o
	press(t, s, "r")
	svc.sideFake.calls = nil
	o.Viewer.Member = true
	svc.owners["github"] = o
	press(t, s, "r")
	if !slices.Contains(svc.sideFake.calls, "readme github") {
		t.Errorf("a change of membership read %q, want the README", svc.sideFake.calls)
	}
	if !s.page.side.member {
		t.Error("the README wasn't read as a member's")
	}
}

// Images that arrive render the README again only if it has images.
func TestReadmeRedrawsForItsImages(t *testing.T) {
	host := &uitest.ImageHost{}
	images := uitest.Avatars(host, true)
	s := newSection(t, newFake(), "github", 80, 24, WithAvatars(images))
	press(t, s, "3")
	if !s.readmeStale(s.page) {
		t.Error("a README with an image doesn't render again for images")
	}
	svc := newFake()
	svc.readmes["github"] = core.Readme{Markdown: "No images.", Source: core.RepoRef{Owner: "github", Name: ".github"}}
	s = newSection(t, svc, "github", 80, 24, WithAvatars(images))
	press(t, s, "3")
	if s.readmeStale(s.page) {
		t.Error("a README without images renders again for images")
	}
}
