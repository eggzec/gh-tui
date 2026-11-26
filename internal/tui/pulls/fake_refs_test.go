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
}

func (f *fakeRefs) CachedReferences(refssvc.Query) (core.References, bool) {
	return core.References{}, false
}

func (f *fakeRefs) References(context.Context, refssvc.Query) (core.References, error) {
	return f.refs, nil
}

func (f *fakeRefs) CachedMentions(refssvc.MentionsQuery) (core.Page[core.Reference], bool) {
	return core.Page[core.Reference]{}, false
}

func (f *fakeRefs) Mentions(context.Context, refssvc.MentionsQuery) (core.Page[core.Reference], error) {
	return core.Page[core.Reference]{Items: f.mentions, Next: f.next}, nil
}

func (f *fakeRefs) Invalidate(core.RepoRef, int) {}
