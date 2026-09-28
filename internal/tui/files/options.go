package files

import (
	"time"

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

// WithPrefetch reads the top-level files of at most maxSize bytes as soon
// as a repository is listed, so that their preview opens at once. Each
// costs a request. Files that are likely binary are skipped. The default,
// 0, reads nothing ahead.
func WithPrefetch(maxSize int64) Option {
	return func(s *Section) { s.prefetchMax = max(maxSize, 0) }
}

// WithHoverPrefetch reads the file under the cursor once the cursor has
// rested on it for delay, if it has at most maxSize bytes, so that its
// preview opens at once. Each costs a request. The default, a maxSize of 0,
// reads nothing ahead.
func WithHoverPrefetch(delay time.Duration, maxSize int64) Option {
	return func(s *Section) {
		s.hover.delay, s.hover.max = max(delay, 0), max(maxSize, 0)
	}
}

// WithFinderPreview sets whether the finder shows the content of the
// selected file beside the paths, where the width leaves room for it. It
// does by default; the toggle key shows or hides it either way.
func WithFinderPreview(show bool) Option {
	return func(s *Section) { s.findPreview = show }
}

// WithIcons sets the glyphs drawn before the names of files and
// directories. The default is the Nerd Font set.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// WithOffline shares off with other sections, so that the user is told once
// for all of them that GitHub can't be reached. By default the section has
// its own.
func WithOffline(off *ui.Offline) Option {
	return func(s *Section) {
		if off != nil {
			s.offline = off
		}
	}
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
