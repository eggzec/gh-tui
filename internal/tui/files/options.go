package files

import (
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Option configures a Section in [New].
type Option func(*Section)

// WithRepo sets the repository the section starts with, so that Init loads
// its files without waiting for a ui.RepoMsg.
func WithRepo(repo core.RepoRef) Option {
	return func(s *Section) { s.repo = repo }
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
