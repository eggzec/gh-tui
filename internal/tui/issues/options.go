package issues

import (
	"context"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Option configures a Section.
type Option func(*Section)

// WithNow sets the clock that ages are measured against. Tests pin it.
func WithNow(now func() time.Time) Option {
	return func(s *Section) {
		if now != nil {
			s.now = now
		}
	}
}

// WithPrefetch reads ahead as p says for prefetch.issues, so that what the
// user opens next opens at once:
//   - details and comments: the issue and its first comments, for the rows
//     in the window around the cursor, each time it rests, and at once
//     when a list loads. Each costs a request; what is cached is skipped.
//   - other_tabs: the first page of each state not shown, once the user
//     switched tabs in a repository, with the next or previous tab key,
//     and the list shown loaded, so that switching further shows them at
//     once. It reads them once per repository and session. Each costs a
//     request; pages cached fresh are skipped, and so is a list the user
//     filtered.
//
// The default reads nothing ahead.
func WithPrefetch(p config.PrefetchLayers) Option {
	return func(s *Section) { s.prefetch = &p }
}

// WithSlots bounds the reads ahead of the section with those of every page
// and modal that shares s, so that together they keep to
// prefetch.parallel. Without it, each kind it reads has slots of its own.
func WithSlots(s *ui.Slots) Option {
	return func(x *Section) { x.slots = s }
}

// WithIcons sets the glyphs of the states of issues. Without it, the icons
// are the config's default.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// WithDates sets how dates read, as ui.date_format says. The default is
// as ages.
func WithDates(d ui.Dates) Option {
	return func(s *Section) { s.dates = d }
}

// WithAvatars draws the avatars of the authors of comments with a.
// Without it, or where the terminal shows no images, the comments show
// none and take no room for them.
func WithAvatars(a *ui.Images) Option {
	return func(s *Section) { s.avatars = a }
}

// WithRepos reads what the viewer may do in the repository of a modal
// from r, when it isn't the selected one, whose caps the app sends in a
// ui.CapsMsg. Until they are known, every change is offered, and GitHub
// refuses what it doesn't allow.
func WithRepos(r ui.Repos) Option {
	return func(s *Section) { s.repos = r }
}

// Viewer returns the login of the signed-in user. It may do I/O.
type Viewer func(ctx context.Context) (string, error)

// WithViewer sets how the section learns who the user is, once it starts,
// so that they may close and reopen their own issues in a repository they
// can only read. Without it, or until it answers, those changes are
// offered on every issue.
func WithViewer(v Viewer) Option {
	return func(s *Section) { s.readViewer = v }
}

// WithVoice sets how the section words what went wrong, with the keys a
// hint names and the log it points to. By default the hints name the
// configured keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(s *Section) { s.voice = v }
}
