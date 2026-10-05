package files

import (
	"path"
	"strings"

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
type markdownView struct {
	theme ui.Theme
	icons ui.Icons
	// md renders the files, and is nil until the first one is.
	md *markdown.Renderer
}

// setTheme renders in the style of t from now.
func (v *markdownView) setTheme(t ui.Theme, ic ui.Icons) {
	v.theme, v.icons = t, ic
	if v.md != nil {
		v.md.SetStyle(t.Thread(ic).Markdown)
	}
}

// render returns what renders src, the content of a markdown file, at a
// width, for a pager, as comments render.
func (v *markdownView) render(src string) pager.Render {
	return func(width int) string {
		if v.md == nil {
			v.md = markdown.New(v.theme.Thread(v.icons).Markdown)
			v.md.SetHint(rawHint)
		}
		return v.md.Render(src, width)
	}
}
