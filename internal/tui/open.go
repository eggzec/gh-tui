package tui

import (
	"net/url"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// openCommand opens what arg names on GitHub, in the browser: a
// repository, an issue or pull request, or a link to any page of a
// repository on the user's host. Without arg it opens what is selected,
// as the open key does.
func (m *Model) openCommand(arg string) tea.Cmd {
	if arg == "" {
		return m.press(config.ActionOpen)
	}
	t, err := core.ParseTarget(arg, m.host)
	if err != nil {
		return m.badTarget(err)
	}
	if t.HasOwner() {
		return m.toast.Push(toast.Error, cantOpen(t.String(), "pages of users and organizations aren't supported yet", m.icons.Ellipsis, m.fitsToast))
	}
	if isLink(arg) {
		// A link names more than its target, such as a file, and
		// ParseTarget made sure it is on the user's host.
		return ui.Open(webLink(m.host, arg))
	}
	if !t.HasRepo() {
		// As goto, a number alone is one of the repository on view.
		if m.screen != repoScreen || m.repo == (core.RepoRef{}) {
			return m.toast.Push(toast.Error, "Open a repository first, or use open owner/name"+t.String()+".")
		}
		t.Repo = m.repo
	}
	return ui.Open(m.targetURL(t))
}

// targetURL returns the page of t on the user's host. A number whose kind
// isn't known opens as an issue, whose page GitHub redirects to the pull
// request's.
func (m *Model) targetURL(t core.Target) string {
	path := t.Repo.String()
	if !t.HasNumber() {
		return ui.WebURL(m.host, path)
	}
	kind := t.Kind
	if !kind.Known() && m.kinds != nil {
		kind, _ = m.kinds.CachedKind(t.Repo, t.Number)
	}
	page := "/issues/"
	if kind == core.KindPull {
		page = "/pull/"
	}
	return ui.WebURL(m.host, path+page+strconv.Itoa(t.Number))
}

// webLink returns s, a link to a page on host that core.ParseTarget read,
// on host as ui.WebURL writes it: with its scheme, which s may have left
// out or given as http, and its port, which s may have spelled otherwise.
func webLink(host, s string) string {
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		// ParseTarget parsed it already.
		return s
	}
	base, err := url.Parse(ui.WebURL(host, ""))
	if err != nil {
		return u.String()
	}
	u.Scheme, u.Host, u.User = base.Scheme, base.Host, nil
	return u.String()
}
