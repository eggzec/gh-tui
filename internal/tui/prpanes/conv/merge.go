package conv

import (
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// mergeWindow is how far apart in time the events of one line may be, from
// the first and from the latest of them.
const mergeWindow = 10 * time.Minute

// family is a set of kinds of events that one line can tell together.
type family int

const (
	// familyNone is a kind that stands alone.
	familyNone family = iota
	familyLabels
	familyAssignees
	familyRequests
	familyMilestones
)

// familyOf returns the family of a kind of event.
func familyOf(k core.TimelineKind) family {
	switch k {
	case core.TimelineLabeled, core.TimelineUnlabeled:
		return familyLabels
	case core.TimelineAssigned, core.TimelineUnassigned:
		return familyAssignees
	case core.TimelineReviewRequested, core.TimelineReviewRequestRemoved:
		return familyRequests
	case core.TimelineMilestoned, core.TimelineDemilestoned:
		return familyMilestones
	default:
		return familyNone
	}
}

// merger gathers the events of one actor and family that are close in time
// into one line. Feed it the visible items in the order of the timeline, and
// tell it, with split, of each one that is not an event.
type merger struct{ open *node }

// event adds it to the open line, or starts a line with it, which is fresh.
func (m *merger) event(it *core.TimelineItem) (n *node, fresh bool) {
	if m.open != nil && joins(m.open, it) {
		m.open.events = append(m.open.events, it)
		return m.open, false
	}
	m.open = eventNode(it)
	return m.open, true
}

// split closes the open line.
func (m *merger) split() { m.open = nil }

// joins reports whether it belongs to the line n: the same actor does the
// same sort of thing, within the window of both the first and the latest
// event of the line.
func joins(n *node, it *core.TimelineItem) bool {
	f := familyOf(it.Kind)
	if f == familyNone || f != familyOf(n.item.Kind) || it.Actor.Login != n.item.Actor.Login {
		return false
	}
	last := n.events[len(n.events)-1]
	return within(n.item.At, it.At) && within(last.At, it.At)
}

// within reports whether two times are no further apart than the window.
func within(a, b time.Time) bool { return b.Sub(a).Abs() <= mergeWindow }
