package files

import "github.com/eggzec/gh-tui/internal/core"

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
