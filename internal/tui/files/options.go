package files

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Option configures a Section in [New].
type Option func(*Section)

// WithRepo sets the repository the section starts with, so that Init loads
// its files without waiting for a ui.RepoMsg.
func WithRepo(repo core.RepoRef) Option {
	return func(s *Section) { s.repo = repo }
}

// WithHost sets the web host of the user's GitHub, with its port if it has
// one, whose pages the section opens. It defaults to github.com.
func WithHost(host string) Option {
	return func(s *Section) { s.host = host }
}

// WithPrefetch reads ahead as p, the prefetch settings, says for the
// files tree and the finder, so that the preview of a file opens at once.
// Each file costs a request, and files that are likely binary are
// skipped. The file under a cursor is read up to previewMax, the largest
// the preview reads. Without it, the tree reads nothing ahead, and the
// finder reads only the file under its cursor, which it shows, once the
// cursor rests for the default rest.
func WithPrefetch(p config.PrefetchLayers, previewMax config.Size) Option {
	return func(s *Section) { s.prefetch = newPrefetch(p, previewMax) }
}

// WithFinderPreview sets whether the finder shows the content of the
// selected file beside the paths, where the width leaves room for it.
// Without it, it does as the config's default says; the toggle key shows or
// hides it either way.
func WithFinderPreview(show bool) Option {
	return func(s *Section) { s.findPreview = show }
}

// WithIcons sets the glyphs drawn before the names of files and
// directories. Without it, the icons are the config's default.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// WithVoice sets how the section words what went wrong, with the keys a
// hint names and the log it points to. By default the hints name the
// configured keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(s *Section) { s.voice = v }
}

// WithEditor sets the command of the editor that the preview opens a file
// in, before $VISUAL and $EDITOR, as pager.WithEditor takes it.
func WithEditor(cmd string) Option {
	return func(s *Section) { s.editor = cmd }
}
