package pulls

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	refssvc "github.com/eggzec/gh-tui/internal/service/refs"
)

// fakeRefs serves the references and the first page of mentions it holds.
type fakeRefs struct {
	refs     core.References
	mentions []core.Reference
	next     string

	// refQueries and mentionQueries record every query, cached reads too.
	refQueries     []refssvc.Query
	mentionQueries []refssvc.MentionsQuery
}

func (f *fakeRefs) CachedReferences(q refssvc.Query) (core.References, bool) {
	f.refQueries = append(f.refQueries, q)
	return core.References{}, false
}

func (f *fakeRefs) References(_ context.Context, q refssvc.Query) (core.References, error) {
	f.refQueries = append(f.refQueries, q)
	return f.refs, nil
}

func (f *fakeRefs) CachedMentions(q refssvc.MentionsQuery) (core.Page[core.Reference], bool) {
	f.mentionQueries = append(f.mentionQueries, q)
	return core.Page[core.Reference]{}, false
}

func (f *fakeRefs) Mentions(_ context.Context, q refssvc.MentionsQuery) (core.Page[core.Reference], error) {
	f.mentionQueries = append(f.mentionQueries, q)
	return core.Page[core.Reference]{Items: f.mentions, Next: f.next}, nil
}

func (f *fakeRefs) Invalidate(core.RepoRef, int) {}
