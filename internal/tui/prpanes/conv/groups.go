package conv

import (
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// Group is a kind of event that the filter shows or hides.
type Group int

// The groups, in the order the host lists them.
const (
	// GroupComments is the comments of the conversation.
	GroupComments Group = iota
	// GroupReviews is the reviews, with their threads.
	GroupReviews
	// GroupPushes is the commits pushed and the force pushes.
	GroupPushes
	// GroupLabels is the labels added and removed.
	GroupLabels
	// GroupPeople is assignments, review requests and dismissed reviews.
	GroupPeople
	// GroupState is the changes of state: ready for review, draft, merged,
	// closed, reopened, locked and unlocked.
	GroupState
	// GroupBranch is the renames, the changes of base and the deleted head.
	GroupBranch
	// GroupReferences is the mentions by other issues and pull requests.
	GroupReferences
	// GroupMilestones is the milestones added and removed.
	GroupMilestones
	// NumGroups is how many groups there are.
	NumGroups
)

// groupNames name the groups as the config does: the table of the names is
// config.ConversationGroups, and a test keeps the two the same.
var groupNames = [NumGroups]string{
	GroupComments:   "comments",
	GroupReviews:    "reviews",
	GroupPushes:     "pushes",
	GroupLabels:     "labels",
	GroupPeople:     "people",
	GroupState:      "state",
	GroupBranch:     "branch",
	GroupReferences: "references",
	GroupMilestones: "milestones",
}

// String returns the name of g, as the config says it.
func (g Group) String() string {
	if g < 0 || g >= NumGroups {
		return "group " + strconv.Itoa(int(g))
	}
	return groupNames[g]
}

// kindGroups is the group of each kind of event of the timeline.
var kindGroups = map[core.TimelineKind]Group{
	core.TimelineComment:              GroupComments,
	core.TimelineReview:               GroupReviews,
	core.TimelinePushed:               GroupPushes,
	core.TimelineForcePushed:          GroupPushes,
	core.TimelineLabeled:              GroupLabels,
	core.TimelineUnlabeled:            GroupLabels,
	core.TimelineAssigned:             GroupPeople,
	core.TimelineUnassigned:           GroupPeople,
	core.TimelineReviewRequested:      GroupPeople,
	core.TimelineReviewRequestRemoved: GroupPeople,
	core.TimelineReviewDismissed:      GroupPeople,
	core.TimelineReadyForReview:       GroupState,
	core.TimelineConvertedToDraft:     GroupState,
	core.TimelineMerged:               GroupState,
	core.TimelineClosed:               GroupState,
	core.TimelineReopened:             GroupState,
	core.TimelineLocked:               GroupState,
	core.TimelineUnlocked:             GroupState,
	core.TimelineRenamed:              GroupBranch,
	core.TimelineBaseChanged:          GroupBranch,
	core.TimelineHeadDeleted:          GroupBranch,
	core.TimelineCrossReferenced:      GroupReferences,
	core.TimelineMilestoned:           GroupMilestones,
	core.TimelineDemilestoned:         GroupMilestones,
}

// groupOf returns the group of a kind of event, and false for a kind that has
// none, which no filter hides.
func groupOf(k core.TimelineKind) (Group, bool) {
	g, ok := kindGroups[k]
	return g, ok
}

// Choice is a group as the host offers it to the user: its name, how many
// top-level items of it the conversation holds, and whether it is shown.
type Choice struct {
	// Group is the group this choice stands for.
	Group Group
	// Name is the name of the group, as the config says it.
	Name string
	// Count is how many top-level items of the group the conversation holds.
	Count int
	// On is whether the filter shows the group.
	On bool
}

// Choices returns every group, those with no item too, in order.
func (m Model) Choices() []Choice {
	c := countOf(m.dataOf())
	out := make([]Choice, NumGroups)
	for g := range NumGroups {
		out[g] = Choice{Group: g, Name: groupNames[g], Count: c[g], On: m.shown[g]}
	}
	return out
}

// Shown returns how many top-level items the filter shows, and how many the
// conversation holds, as far as it has been read.
func (m Model) Shown() (n, total int) {
	c := countOf(m.dataOf())
	for g := range NumGroups {
		total += c[g]
		if m.shown[g] {
			n += c[g]
		}
	}
	return n, total
}

// FilterName names what the filter shows, for the info line: "" when every
// group is shown, the name of the group when it is one, the names of two
// joined by ", ", "nothing" when none is, and the number of groups when it is
// more.
func (m Model) FilterName() string {
	var on []string
	for g := range NumGroups {
		if m.shown[g] {
			on = append(on, groupNames[g])
		}
	}
	switch len(on) {
	case int(NumGroups):
		return ""
	case 0:
		return "nothing"
	case 1, 2:
		return strings.Join(on, ", ")
	}
	return strconv.Itoa(len(on)) + " kinds"
}

// countOf counts the top-level items of d that have a row, by group: the
// comments and the submitted reviews, and the review that holds the drafts
// of the viewer, once. A line of merged events counts once, under the group
// of its first event.
func countOf(d Data) (c [NumGroups]int) {
	attached := make(map[string]bool)
	var lines merger
	for i := range d.Items {
		it := &d.Items[i]
		if !drawn(it) {
			continue
		}
		if it.Kind == core.TimelineReview {
			attached[it.ID] = true
		}
		if isEvent(it.Kind) {
			if _, fresh := lines.event(it); !fresh {
				continue
			}
		} else {
			lines.split()
		}
		if g, ok := groupOf(it.Kind); ok {
			c[g]++
		}
	}
	for i := range d.Threads {
		if t := &d.Threads[i]; len(t.Comments) > 0 && t.Pending && !attached[t.ReviewID] {
			c[GroupReviews]++
			break
		}
	}
	return c
}

// Configure returns the option that sets which groups are shown when the
// conversation opens from the settings of c.
func Configure(c config.Config) Option {
	var on [NumGroups]bool
	for _, name := range c.UI.Conversation.Filter {
		for g, n := range groupNames {
			if n == name {
				on[g] = true
			}
		}
	}
	return WithShown(on)
}

// WithShown sets which groups the conversation shows when it is made. Every
// group is shown by default.
func WithShown(on [NumGroups]bool) Option { return func(m *Model) { m.shown = on } }
