package conv

import (
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// withCI puts the line with the state of the checks after each push whose last
// commit has a known roll-up other than none. It runs after the sort, so that
// a line stays next to its push.
func withCI(out []*node, o Options) []*node {
	var with []*node
	for i, n := range out {
		if n.kind != KindPush {
			if with != nil {
				with = append(with, n)
			}
			continue
		}
		last := n.children[len(n.children)-1].item
		state, ok := o.checks[strings.ToLower(last.Commit.OID)]
		if !ok || state == core.CheckNone {
			if with != nil {
				with = append(with, n)
			}
			continue
		}
		if with == nil {
			with = append(make([]*node, 0, len(out)+1), out[:i]...)
		}
		with = append(with, n, &node{kind: KindCI, id: "ci:" + last.Commit.OID, item: last})
	}
	if with == nil {
		return out
	}
	return with
}

// ci draws the line with the state of the checks of the last commit of a
// push, centred like the other system lines and with no age.
func (b *builder) ci(n *node) treeview.Node {
	oid := n.item.Commit.OID
	state := b.o.checks[strings.ToLower(oid)]
	what := "checks passed on"
	switch state {
	case core.CheckFailing:
		what = "checks failed on"
	case core.CheckPending:
		what = "checks pending on"
	case core.CheckPassing, core.CheckNone:
	}
	line := b.checkGlyph(state) + " " + b.say(what) + " " + b.st.bold.Render(shortOID(oid))
	r := b.system(line, "", true)
	r.Action = true
	return r
}
