package files

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// markdownFile reports whether the file at name is markdown, which the
// previews show rendered: a .md or .markdown file, or a README without
// an extension, as GitHub renders them.
func markdownFile(name string) bool {
	base := strings.ToLower(path.Base(name))
	switch path.Ext(base) {
	case ".md", ".markdown":
		return true
	case "":
		return base == "readme"
	}
	return false
}

// rawHint is what the note that ends a markdown file too long to render
// in full offers.
const rawHint = "raw on shows all of it"

// markdownView renders the markdown files a pager shows, in the style of
// the theme, which it renders again with once that changes. The file
// preview and the finder's preview each have one.
//
// Where images draws them, the images that stand alone on their lines
// show as pictures, as in comments: an image of the repository, by a
// relative address, read as its blob, as an image file of the preview
// is, so a private repository's needs no signed address; and an image on
// the web as comments fetch it. A file has no HTML from GitHub, so an
// image of a host whose images load only through GitHub's proxy stays
// text.
type markdownView struct {
	theme ui.Theme
	icons ui.Icons
	// md renders the files, and is nil until the first one is.
	md *markdown.Renderer

	// images draws the pictures, at most as tall as height, the rows of
	// the pager above its status line, allows.
	images *ui.Images
	height int
	// svc reads the directories that find the image files of repo at
	// ref, under ctx. file is the path of the file rendered, which the
	// relative addresses in it start from.
	ctx  context.Context
	svc  Service
	repo core.RepoRef
	ref  string
	file string
	// found holds the image files of the repository the files name, by
	// their paths, as far as they were looked up, pending the paths still
	// to look up, and looking how many are being looked up.
	found   map[string]found
	pending []string
	looking int
	// gen counts the documents, or the times one was shown again: a
	// look-up of an older one is ignored, since what it found may never
	// have arrived, as while the finder was hidden.
	gen int
	// loading holds the images on their way, by what names them, as far
	// as the renders saw; drew reports whether the last render asked for
	// any picture, and on whether it drew pictures at all.
	loading  map[string]bool
	drew, on bool
}

// maxBusy is how many image files a document looks up at once, and how
// many of its images may be on their way at once: the rest show as text
// until those arrived, so a file of many images doesn't ask for them all
// as it opens.
const maxBusy = 4

// found is how far the look-up of an image file of the repository got.
type found struct {
	entry core.TreeEntry
	state foundState
}

type foundState int

const (
	foundLooking foundState = iota
	foundFile
	// foundNone is a path that names no file at the commit.
	foundNone
	// foundFailed is a path whose look-up failed otherwise, such as while
	// GitHub couldn't be reached, which is looked up again once it can.
	foundFailed
)

// imageEntryMsg carries the entry of the image file at path, which the
// markdown v renders names, looked up in its generation gen.
type imageEntryMsg struct {
	v     *markdownView
	gen   int
	path  string
	entry core.TreeEntry
	err   error
}

// setTheme renders in the style of t from now.
func (v *markdownView) setTheme(t ui.Theme, ic ui.Icons) {
	v.theme, v.icons = t, ic
	if v.md != nil {
		v.md.SetStyle(t.Thread(ic).Markdown)
	}
}

// setFiles finds the image files the markdown names in repo at ref, the
// base it is shown at, through svc under ctx, and draws them with
// images.
func (v *markdownView) setFiles(ctx context.Context, svc Service, repo core.RepoRef, ref string, images *ui.Images) {
	v.ctx, v.svc, v.repo, v.ref, v.images = ctx, svc, repo, ref, images
}

// setHeight bounds the pictures by height rows, those of the pager above
// its status line.
func (v *markdownView) setHeight(height int) {
	v.height = height
}

// render returns what renders src, the content of the markdown file at
// name, at a width, for a pager, as comments render.
func (v *markdownView) render(name, src string) pager.Render {
	return func(width int) string {
		if v.md == nil {
			v.md = markdown.New(v.theme.Thread(v.icons).Markdown)
			v.md.SetHint(rawHint)
			v.md.SetRelativePictures(true)
		}
		if name != v.file {
			v.reset()
		}
		v.file = name
		v.drew = false
		pics := v.pictures()
		v.on = pics != nil
		v.md.SetPictures(pics)
		return v.md.Render(src, width)
	}
}

// pictures returns what draws the images of the markdown, or nil while
// none are drawn, so it renders as it does without images.
func (v *markdownView) pictures() markdown.Pictures {
	if v.images.PictureRows(v.height) <= 0 {
		return nil
	}
	return v.picture
}

// picture returns the lines that draw the image at addr in at most width
// cells, or nil while it shows as text: before it arrived, or its file
// was found, or when it won't load.
func (v *markdownView) picture(addr string, width int) []string {
	rows := v.images.PictureRows(v.height)
	if rows <= 0 {
		return nil
	}
	v.drew = true
	p, rel := repoPath(v.file, addr)
	if !rel {
		return v.fit(ui.ImageSource{URL: addr}, width, rows)
	}
	if p == "" || !imageFile(p) {
		return nil
	}
	f, ok := v.found[p]
	if !ok {
		if e, cached := cachedEntry(v.svc, v.repo, v.ref, p); cached {
			f = found{entry: e, state: foundFile}
		} else {
			v.pending = append(v.pending, p)
		}
		v.remember(p, f)
	}
	if f.state != foundFile {
		return nil
	}
	return v.fit(ui.ImageSource{Repo: v.repo, SHA: f.entry.SHA, Size: f.entry.Size}, width, rows)
}

// fit returns the lines of the image of src fitted to width by rows
// cells, once it arrived, and asks for it unless maxBusy others are on
// their way already.
func (v *markdownView) fit(src ui.ImageSource, width, rows int) []string {
	k := src.URL
	if src.SHA != "" {
		k = "blob " + src.SHA
	}
	if !v.loading[k] && len(v.loading) >= maxBusy {
		return nil
	}
	lines, st := v.images.Fit(src, ui.ImageSize{Cols: width, Rows: rows})
	switch {
	case st == ui.ImageLoading && v.loading == nil:
		v.loading = map[string]bool{k: true}
	case st == ui.ImageLoading:
		v.loading[k] = true
	default:
		delete(v.loading, k)
	}
	return lines
}

// online looks up again the image files whose look-ups failed while
// GitHub couldn't be reached, as they are next drawn, and reports
// whether there were any, which the pager renders again to ask.
func (v *markdownView) online() bool {
	again := false
	for p, f := range v.found {
		if f.state == foundFailed {
			delete(v.found, p)
			again = true
		}
	}
	return again
}

// stale reports whether the images that changed may change the render:
// it asked for pictures, or pictures began or stopped being drawn. A
// file without images, which may be too long to keep rendered, isn't
// rendered again for every image of the app that arrives.
func (v *markdownView) stale() bool {
	return v.drew || (v.images.PictureRows(v.height) > 0) != v.on
}

// reset starts a generation: the look-ups on their way are forgotten, and
// the paths they were for are looked up again when next drawn, as are
// the images on their way, which another file left or which arrived while
// the view was hidden.
func (v *markdownView) reset() {
	v.gen++
	v.looking, v.pending, v.loading = 0, nil, nil
	for p, f := range v.found {
		if f.state == foundLooking {
			delete(v.found, p)
		}
	}
}

// remember keeps how far the look-up of the image file at p got.
func (v *markdownView) remember(p string, f found) {
	if v.found == nil {
		v.found = make(map[string]found)
	}
	v.found[p] = f
}

// lookUp returns the command that finds the image files the renders since
// the last call named and no cached directory held, each in its own read,
// up to maxBusy at once; the rest wait for those to end.
func (v *markdownView) lookUp() tea.Cmd {
	n := min(len(v.pending), maxBusy-v.looking)
	if n <= 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, n)
	for _, p := range v.pending[:n] {
		ctx, svc, repo, ref, gen := v.ctx, v.svc, v.repo, v.ref, v.gen
		cmds = append(cmds, func() tea.Msg {
			ctx, end := obs.Begin(ctx, "open.file.image")
			e, err := findEntry(ctx, svc, repo, ref, p)
			end(err, "span", "tui", "repo", repo.String(), "depth", strings.Count(p, "/")+1)
			return imageEntryMsg{v: v, gen: gen, path: p, entry: e, err: err}
		})
	}
	v.pending = v.pending[n:]
	v.looking += n
	return tea.Batch(cmds...)
}

// take keeps the entry msg found, and reports whether it was one of v's,
// of the generation shown.
// A path that failed for another reason than naming no file, such as
// GitHub being out of reach, is looked up again only once GitHub answers
// again (online).
func (v *markdownView) take(msg imageEntryMsg) bool {
	if msg.v != v || msg.gen != v.gen {
		return false
	}
	v.looking = max(v.looking-1, 0)
	switch {
	case msg.err == nil:
		v.remember(msg.path, found{entry: msg.entry, state: foundFile})
	case errors.Is(msg.err, errNoFile):
		v.remember(msg.path, found{state: foundNone})
	default:
		v.remember(msg.path, found{state: foundFailed})
	}
	return true
}

// repoPath returns the path in the repository of the image at addr,
// relative to the file at name as GitHub resolves it, or from the root
// when it starts with a slash, and whether addr is relative at all. The
// path is empty for one that leaves the repository or names nothing.
func repoPath(name, addr string) (p string, rel bool) {
	u, err := url.Parse(strings.TrimSpace(addr))
	if err != nil {
		return "", true
	}
	if u.Scheme != "" || u.Host != "" {
		return "", false
	}
	p = u.Path
	if p == "" {
		return "", true
	}
	if strings.HasPrefix(p, "/") {
		p = path.Clean(p)[1:]
	} else {
		p = path.Join(path.Dir(name), p)
	}
	if p == "" || p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", true
	}
	return p, true
}
