package refs

import (
	"cmp"
	"slices"

	"github.com/eggzec/gh-tui/internal/core"
)

// maxResolve is how many of the items that the texts name are read to
// say what they are; the others are counted, not read, so a long thread
// of links costs a bounded number of requests.
const maxResolve = 100

// plan is what the sources say before the items the texts name are read.
type plan struct {
	// closing are the items GitHub links: what it says the item closes or
	// is closed by, the links of the sidebar and the closers of an issue.
	closing []core.Reference
	// pending are the items the texts name that closing lacks, by first
	// appearance, with where each was written. They are only targets.
	pending []core.Reference
}

// merge reads the sources of the item self, a pull request if pull is
// set, into closing and pending. Its texts are searched for links to
// host, and links whose repository is not named are of self's. Every item
// is one entry, whatever number of places named it, and the item itself
// is left out.
func merge(src core.RefSources, self core.Target, pull bool, host string) plan {
	var p plan
	index := map[string]int{}
	add := func(list *[]core.Reference, index map[string]int, ref core.Reference, origin core.RefOrigin) {
		k := core.RefKey(ref.Target)
		i, ok := index[k]
		if !ok {
			i = len(*list)
			index[k] = i
			ref.Origins = nil
			*list = append(*list, ref)
		}
		(*list)[i].Origins = append((*list)[i].Origins, origin)
	}
	for i := range src.Closing {
		add(&p.closing, index, src.Closing[i], refClosing(pull))
	}
	// A link made in the sidebar is cancelled by one that undoes it, and
	// made again by another.
	type link struct {
		ref  core.Reference
		live bool
	}
	var links []link
	at := map[string]int{}
	for i := range src.Linked {
		l := &src.Linked[i]
		k := core.RefKey(l.Ref.Target)
		i, ok := at[k]
		switch {
		case l.Disconnected && ok:
			links[i].live = false
			delete(at, k)
		case !l.Disconnected && !ok:
			at[k] = len(links)
			links = append(links, link{l.Ref, true})
		}
	}
	for i := range links {
		if links[i].live {
			add(&p.closing, index, links[i].ref, refClosing(pull))
		}
	}
	for i := range src.Closers {
		add(&p.closing, index, src.Closers[i], core.RefOrigin{Group: core.RefClosing, Where: "closed by"})
	}

	pendingAt := map[string]int{}
	for _, t := range src.Texts {
		for _, target := range core.ScanRefs(t.Text, host, self.Repo) {
			if target.Same(self) {
				continue
			}
			if _, ok := index[core.RefKey(target)]; ok {
				add(&p.closing, index, core.Reference{Target: target}, t.Origin)
				continue
			}
			add(&p.pending, pendingAt, core.Reference{Target: target}, t.Origin)
		}
	}
	for i := range p.closing {
		p.closing[i].Origins = refOrigins(p.closing[i].Origins)
	}
	slices.SortStableFunc(p.closing, refClosingOrder)
	return p
}

// refClosing is the origin of what GitHub says the item closes, or is
// closed by.
func refClosing(pull bool) core.RefOrigin {
	if pull {
		return core.RefOrigin{Group: core.RefClosing, Where: "closes"}
	}
	return core.RefOrigin{Group: core.RefClosing, Where: "closed by"}
}

// refClosingOrder puts the open items first, then the others, each by
// repository and number.
func refClosingOrder(a, b core.Reference) int {
	if c := cmp.Compare(refOpen(b), refOpen(a)); c != 0 {
		return c
	}
	if c := cmp.Compare(core.RefKey(core.Target{Repo: a.Target.Repo}), core.RefKey(core.Target{Repo: b.Target.Repo})); c != 0 {
		return c
	}
	return cmp.Compare(a.Target.Number, b.Target.Number)
}

// refOpen is 1 for an item that is open, else 0.
func refOpen(r core.Reference) int {
	if r.State == core.StateOpen {
		return 1
	}
	return 0
}

// refOrigins puts origins in order, the strongest group first and each
// group as found, and leaves out an origin that was found twice. What
// closes an item is one origin however often it is said, so one found
// twice takes the author of the one that has it.
func refOrigins(origins []core.RefOrigin) []core.RefOrigin {
	sorted := slices.Clone(origins)
	slices.SortStableFunc(sorted, func(a, b core.RefOrigin) int { return cmp.Compare(a.Group, b.Group) })
	var out []core.RefOrigin
	for _, o := range sorted {
		i := slices.IndexFunc(out, func(x core.RefOrigin) bool {
			return x == o || o.Group == core.RefClosing && x.Group == o.Group && x.Where == o.Where
		})
		switch {
		case i < 0:
			out = append(out, o)
		case out[i].By == "":
			out[i].By, out[i].Bot = o.By, o.Bot
		}
	}
	return out
}
