package refs

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	refssvc "github.com/eggzec/gh-tui/internal/service/refs"
)

// Service is what the step needs of the references service.
type Service interface {
	// CachedReferences returns the links of an item from memory, without
	// a request.
	CachedReferences(q refssvc.Query) (core.References, bool)
	References(ctx context.Context, q refssvc.Query) (core.References, error)
	// CachedMentions returns a page of the items that mention an item
	// from memory, without a request.
	CachedMentions(q refssvc.MentionsQuery) (core.Page[core.Reference], bool)
	Mentions(ctx context.Context, q refssvc.MentionsQuery) (core.Page[core.Reference], error)
	// Invalidate marks everything cached of an item stale, so that the
	// reads after it ask GitHub.
	Invalidate(repo core.RepoRef, number int)
}
