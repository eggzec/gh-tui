package owner

import (
	"net/url"
	"path"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// maxSponsors is how many sponsors, and how many sponsored, the end of the
// README names; the rest are counted as more.
const maxSponsors = 12

// readmeKeys are the keys of the pager that the README pane passes on: it
// scrolls, and leaves searching, filtering and the rest of a pager's keys
// to the full pagers, since the page's keys and the app's take most of
// them here.
type readmeKeys struct {
	pager.KeyMap
}

// newReadmeKeys returns the pager's keys that scroll the README.
func newReadmeKeys() readmeKeys {
	k := pager.DefaultKeyMap()
	for _, b := range []*key.Binding{
		&k.Count, &k.Percent, &k.Left, &k.Right, &k.Option, &k.Search, &k.Confirm, &k.Cancel,
		&k.Filter, &k.Next, &k.Prev, &k.Edit, &k.Close,
	} {
		*b = key.NewBinding(key.WithDisabled())
	}
	return readmeKeys{k}
}

// ShortHelp implements help.KeyMap.
func (k readmeKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.PageDown}
}

// FullHelp implements help.KeyMap: the keys that scroll.
func (k readmeKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End}}
}

// readmePager returns the pager of the README of p, made at its first use.
func (s *Section) readmePager(p *page) *pager.Model {
	if p.side.pager == nil {
		pg := pager.New(pager.WithKeyMap(s.sc.keys.KeyMap), pager.WithStyles(s.theme.Pager(s.icons)), pager.WithLineNumbers(false))
		p.side.pager = &pg
		if p == s.page {
			s.resizeSide()
			s.focusSide()
		}
	}
	return p.side.pager
}

// setReadme shows the README of p, ended with its sponsors, once read. A
// source that didn't change is left as it is, so the place in it stays.
func (s *Section) setReadme(p *page) {
	r := p.side.readme
	if !r.ok {
		return
	}
	src := s.readmeSource(p)
	if src == p.side.shown && p.side.pager != nil {
		return
	}
	p.side.shown = src
	s.readmePager(p).SetRendered(s.readmeTitle(p, false), src, s.readmeRender(p, src))
}

// readmeRender renders src, the markdown of the README of p, at a width,
// drawing its images where the terminal shows them.
func (s *Section) readmeRender(p *page, src string) pager.Render {
	return func(width int) string {
		if s.sc.md == nil {
			s.sc.md = markdown.New(s.theme.Thread(s.icons).Markdown)
		}
		p.side.drew = false
		pics := s.readmePictures(p)
		p.side.pics = pics != nil
		s.sc.md.SetPictures(pics)
		return s.sc.md.Render(src, width)
	}
}

// readmePictures returns what draws the images of the README of p alone
// on their lines, at most as tall as the images allow in the pane, or nil
// while none are drawn. An SVG image stays a link, as images of formats
// the app can't draw do once they fail.
//
// The images load as any image of markdown does, without credentials and
// only from GitHub's own image hosts; a README has no rendered HTML to
// say where GitHub's proxy serves the rest. So an image on another host,
// and an image of a private README, such as an organization's that only
// members see, whose raw file needs a token, stay links; and so do the
// relative images of an Enterprise Server in private mode.
func (s *Section) readmePictures(p *page) markdown.Pictures {
	_, h := s.inside(readmePane)
	pics := s.avatars.Pictures(s.avatars.PictureRows(h), nil)
	if pics == nil {
		return nil
	}
	return func(addr string, width int) []string {
		p.side.drew = true
		if u, err := url.Parse(addr); err != nil || strings.EqualFold(path.Ext(u.Path), ".svg") {
			return nil
		}
		return pics(addr, width)
	}
}

// readmeStale reports whether the images that changed may change the
// render of the README of p: it asked for pictures, or pictures began or
// stopped being drawn. A README without images isn't rendered again for
// every image of the app that arrives.
func (s *Section) readmeStale(p *page) bool {
	_, h := s.inside(readmePane)
	return p.side.drew || (s.avatars.PictureRows(h) > 0) != p.side.pics
}

// readmeSource is the markdown the README pane of p shows: the README
// with its relative addresses made absolute, or what says there is none,
// and the sponsors after it.
func (s *Section) readmeSource(p *page) string {
	r := p.side.readme.value
	login := ownerui.CleanLine(p.header.value.Profile.Login)
	var b strings.Builder
	switch {
	case r.Source != (core.RepoRef{}):
		b.WriteString(markdown.Resolve(r.Markdown, s.resolver(r.Readme, p.header.value.Kind)))
	case p.header.value.Kind == core.OwnerOrg:
		b.WriteString(escape(login) + " has no profile README. A README.md in the profile folder of its .github repository shows one here.")
	default:
		b.WriteString(escape(login) + " has no profile README. A repository " + escape(login+"/"+login) + " with a README.md shows one here.")
	}
	sd := &p.side
	for _, list := range []struct {
		title string
		read  sideRead[core.Page[core.Person]]
	}{{"Sponsors", sd.sponsors}, {"Sponsoring", sd.sponsoring}} {
		if line := sponsorsLine(list.read.value); list.read.ok && line != "" {
			b.WriteString("\n\n**" + list.title + "** " + line)
		}
	}
	return b.String()
}

// sponsorsLine names the accounts of page, at most maxSponsors of them, or
// is "" when it has none.
func sponsorsLine(page core.Page[core.Person]) string {
	if len(page.Items) == 0 {
		return ""
	}
	names := make([]string, 0, min(len(page.Items), maxSponsors))
	for _, p := range page.Items[:min(len(page.Items), maxSponsors)] {
		if l := ownerui.CleanLine(p.Login); l != "" {
			names = append(names, "@"+escape(l))
		}
	}
	line := strings.Join(names, ", ")
	if len(page.Items) > maxSponsors || page.Next != "" {
		line += " and more"
	}
	return line
}

// escape keeps the markdown of s from styling it, as the underscores of
// some logins would.
func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "_", `\_`, "*", `\*`, "[", `\[`, "]", `\]`, "<", `\<`).Replace(s)
}

// resolver makes the relative addresses of README r absolute, against the
// repository it is in, as GitHub shows them: an image's on the host of
// raw files, and a link's to the file on GitHub. An organization's README
// is in the profile folder of its repository. An address that leaves the
// repository stays as it is.
func (s *Section) resolver(r core.Readme, kind core.OwnerKind) func(addr string, image bool) string {
	dir := ""
	if kind == core.OwnerOrg {
		dir = "profile"
	}
	host := s.host
	return func(addr string, image bool) string {
		u, err := url.Parse(strings.TrimSpace(addr))
		if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
			return addr
		}
		// The escaped path keeps what was escaped, such as %2F, as it was.
		p := u.EscapedPath()
		if strings.HasPrefix(p, "/") {
			p = path.Clean(p)[1:]
		} else {
			p = path.Join(dir, p)
		}
		if p == "" || p == "." || p == ".." || strings.HasPrefix(p, "../") {
			return addr
		}
		repo := escapePath(r.Source.Owner + "/" + r.Source.Name)
		var out string
		switch {
		case !image:
			out = ui.WebURL(host, repo+"/blob/HEAD/"+p)
		case host == "" || host == core.DefaultHost:
			out = "https://raw.githubusercontent.com/" + repo + "/HEAD/" + p
		default:
			out = ui.WebURL(host, "raw/"+repo+"/HEAD/"+p)
		}
		if u.RawQuery != "" {
			out += "?" + u.RawQuery
		}
		if u.Fragment != "" {
			out += "#" + u.EscapedFragment()
		}
		return out
	}
}

// escapePath escapes each segment of p for an address.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// readmeTitle titles the README pane of p: the path of the README once
// read, or README, which is its short title too.
func (s *Section) readmeTitle(p *page, short bool) string {
	r := p.side.readme
	if short || !r.ok || r.value.Source == (core.RepoRef{}) {
		return "README"
	}
	if p.header.value.Kind == core.OwnerOrg {
		return ownerui.CleanLine(r.value.Source.Name) + "/profile/README.md"
	}
	return ownerui.CleanLine(r.value.Source.Name) + "/README.md"
}

// readmeBody renders the README pane of the page on view in w by h cells.
func (s *Section) readmeBody(w, h int) []string {
	p, st := s.page, &s.st.shared
	r := p.side.readme
	switch {
	case !r.ok && r.err != nil:
		return ownerui.Indent(s.failure("load the README of "+p.login, r.err, w-1))
	case !p.header.ok && p.header.err != nil:
		// The README is read once the header says whose it is.
		return ownerui.Indent(s.failure("load the README of "+p.login, p.header.err, w-1))
	case !r.ok || p.side.pager == nil:
		return []string{" " + st.Muted.Render("Loading the README"+s.icons.Ellipsis)}
	}
	pg := p.side.pager
	if pg.Height() != h+1 {
		s.resizeSide()
	}
	// The last line is the pager's status line, which the frame's title
	// stands for.
	lines := strings.Split(pg.View(), "\n")
	return ownerui.Indent(lines[:len(lines)-1])
}

// pressReadme passes msg to the pager of the README while its pane has
// the focus.
func (s *Section) pressReadme(msg tea.KeyPressMsg) tea.Cmd {
	pg := s.page.side.pager
	if pg == nil {
		return nil
	}
	var cmd tea.Cmd
	*pg, cmd = pg.Update(msg)
	return cmd
}

// readmeSelection is the README of the page on view, for the copy
// command, or the profile while it has none.
func (s *Section) readmeSelection() (ui.Selection, bool) {
	p := s.page
	r := p.side.readme
	if !r.ok || r.value.Source == (core.RepoRef{}) {
		return s.profileSelection()
	}
	file := "README.md"
	if p.header.value.Kind == core.OwnerOrg {
		file = "profile/README.md"
	}
	ref := r.value.Source
	return ui.Selection{
		What: "README", Repo: ref, Path: file,
		URL: ui.WebURL(s.host, escapePath(ref.Owner+"/"+ref.Name)+"/blob/HEAD/"+escapePath(file)),
	}, true
}

// profileSelection is the account of the page on view, for the copy
// command.
func (s *Section) profileSelection() (ui.Selection, bool) {
	p := s.page
	if p == nil || !p.header.ok {
		return ui.Selection{}, false
	}
	return ui.Selection{What: "profile", URL: s.pageURL(p)}, true
}

// pageURL is the page of the account of p on GitHub.
func (s *Section) pageURL(p *page) string {
	if u := p.header.value.Profile.URL; u != "" {
		return u
	}
	return ui.WebURL(s.host, url.PathEscape(p.login))
}
