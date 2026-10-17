package conv

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// KindEvent is a system line: one event of the timeline, or a run of the
// same actor's events of one family, merged. An event has no ID of GitHub's,
// so a host that acts on a selected item, to reply, edit or delete, must
// ignore this kind.
const KindEvent = "event"

// isEvent reports whether k is a kind of event that is drawn as a system
// line. Comments and reviews have rows of their own, and the pushes are not
// drawn.
func isEvent(k core.TimelineKind) bool {
	g, ok := groupOf(k)
	return ok && g != GroupComments && g != GroupReviews && g != GroupPushes
}

// eventKey names an event by its kind, its time, who did it and what it was
// done to.
func eventKey(it *core.TimelineItem) string {
	return strings.Join([]string{
		string(it.Kind), strconv.FormatInt(it.At.UnixNano(), 10), it.Actor.Login,
		it.Label.Name, it.Subject.User.Login, it.Subject.Team, it.From, it.To, it.Ref,
		strconv.Itoa(it.Source.Number),
	}, "|")
}

// eventID returns the ID of the line that starts with it. Two events with one
// key, which GitHub can send, tell apart by their order; seen counts the keys
// of the build.
func eventID(it *core.TimelineItem, seen map[string]int) string {
	k := eventKey(it)
	seen[k]++
	if n := seen[k]; n > 1 {
		return "e:" + k + "#" + strconv.Itoa(n)
	}
	return "e:" + k
}

// eventNode returns the node of the line that starts with it.
func eventNode(it *core.TimelineItem) *node {
	return &node{kind: KindEvent, item: it, events: []*core.TimelineItem{it}}
}

// clause is what an event says: a verb, and what it was done to.
type clause struct{ verb, obj string }

func (c clause) String() string {
	if c.obj == "" {
		return c.verb
	}
	return c.verb + " " + c.obj
}

// event draws a system line: the icon and what the actor did, centred, with
// the age at the right.
func (b *builder) event(n *node) foldtree.Row {
	var newest time.Time
	for _, it := range n.events {
		newest = latest(newest, it.At)
	}
	line := b.eventIcon(n.item.Kind) + " " + b.st.who(n.item.Actor, false) + " " + b.eventText(n.events)

	age := b.ageOf(newest)
	rw := ansi.StringWidth(age)
	w := b.width - 1
	room := w - rw - 2
	tw := ansi.StringWidth(line)
	pad := 0
	if tw > room {
		line = ansi.Truncate(line, max(room, 0), b.st.Glyphs.Cut)
	} else {
		pad = max(0, min((w-tw)/2, room-tw))
	}
	r := foldtree.Row{Lines: []string{strings.Repeat(" ", pad) + line}}
	if age != "" {
		r.Right = b.st.dim.Render(age)
	}
	return r
}

// eventIcon returns the dim icon of a kind of event.
func (b *builder) eventIcon(k core.TimelineKind) string {
	var g ui.Glyph
	switch k {
	case core.TimelineLabeled, core.TimelineUnlabeled:
		g = ui.GlyphLabelEvent
	case core.TimelineReviewRequested, core.TimelineReviewRequestRemoved, core.TimelineReviewDismissed:
		g = ui.GlyphReviewEvent
	case core.TimelineBaseChanged, core.TimelineHeadDeleted:
		g = ui.GlyphBranchEvent
	default:
		return b.st.dim.Render(b.st.Glyphs.Dot)
	}
	return b.st.dim.Render(b.st.icons.Glyph(g))
}

// eventText says what the events did: the clauses of one verb are told
// together, and the verbs follow one another in the order they first appear.
func (b *builder) eventText(events []*core.TimelineItem) string {
	var verbs []string
	objs := make(map[string][]string)
	for _, it := range events {
		c := b.clause(it)
		if _, ok := objs[c.verb]; !ok {
			verbs = append(verbs, c.verb)
		}
		objs[c.verb] = append(objs[c.verb], c.obj)
	}
	labels := familyOf(events[0].Kind) == familyLabels
	parts := make([]string, len(verbs))
	for i, v := range verbs {
		parts[i] = clause{v, b.list(objs[v], labels)}.String()
	}
	return strings.Join(parts, b.say(" and "))
}

// list joins the objects of one verb: chips of labels one space apart, the
// rest with commas and a last "and".
func (b *builder) list(objs []string, chips bool) string {
	if chips {
		return strings.Join(objs, " ")
	}
	var s strings.Builder
	for i, o := range objs {
		switch {
		case i == 0:
		case i == len(objs)-1:
			s.WriteString(b.say(" and "))
		default:
			s.WriteString(b.say(", "))
		}
		s.WriteString(o)
	}
	return s.String()
}

// say returns a connecting word of a line, dim.
func (b *builder) say(s string) string { return b.st.dim.Render(s) }

// keyword returns a name or a title of a line, bold, cut to n cells.
func (b *builder) keyword(s string, n int) string {
	return b.st.bold.Render(termtext.Truncate(termtext.OneLine(s), n, b.st.Glyphs.Cut))
}

// subject returns the person or the team of an event: a team is its slug,
// bold, and a dim "team".
func (b *builder) subject(s core.TimelineSubject) string {
	if s.Team == "" {
		return b.st.who(s.User, false)
	}
	return b.keyword(teamSlug(s.Team), maxName) + b.say(" team")
}

// chip returns the chip of a label.
func (b *builder) chip(l core.Label) string {
	if b.st.chips == nil {
		return " " + termtext.OneLine(l.Name) + " "
	}
	text, _ := b.st.chips.Get(l)
	return text
}

// clause says what one event did. A kind that is not of a family says it all
// in the verb.
func (b *builder) clause(it *core.TimelineItem) clause {
	switch it.Kind {
	case core.TimelineLabeled:
		return clause{b.say("added"), b.chip(it.Label)}
	case core.TimelineUnlabeled:
		return clause{b.say("removed"), b.chip(it.Label)}
	case core.TimelineAssigned:
		return clause{b.say("assigned"), b.subject(it.Subject)}
	case core.TimelineUnassigned:
		return clause{b.say("unassigned"), b.subject(it.Subject)}
	case core.TimelineReviewRequested:
		return clause{b.say("requested review from"), b.subject(it.Subject)}
	case core.TimelineReviewRequestRemoved:
		return clause{b.say("removed the review request for"), b.subject(it.Subject)}
	case core.TimelineMilestoned:
		return clause{b.say("added this to"), b.keyword(it.Ref, maxName)}
	case core.TimelineDemilestoned:
		return clause{b.say("removed this from"), b.keyword(it.Ref, maxName)}
	default:
		return clause{verb: b.whole(it)}
	}
}

// whole says an event that has no family.
func (b *builder) whole(it *core.TimelineItem) string {
	switch it.Kind {
	case core.TimelineReviewDismissed:
		return b.say("dismissed a review")
	case core.TimelineReadyForReview:
		return b.say("marked this ready for review")
	case core.TimelineConvertedToDraft:
		return b.say("converted this to a draft")
	case core.TimelineClosed:
		return b.say("closed this")
	case core.TimelineReopened:
		return b.say("reopened this")
	case core.TimelineMerged:
		s := b.say("merged")
		if sha := shortSHA(it.Commit); sha != "" {
			s += " " + b.keyword(sha, maxName)
		}
		if it.Ref != "" {
			s += b.say(" into ") + b.keyword(it.Ref, maxName)
		}
		return s
	case core.TimelineLocked:
		s := b.say("locked the conversation")
		if it.Ref != "" {
			s += b.say(" as ") + b.keyword(it.Ref, maxName)
		}
		return s
	case core.TimelineUnlocked:
		return b.say("unlocked the conversation")
	case core.TimelineRenamed:
		return b.say("changed the title from ") + b.keyword(it.From, 2*maxName) +
			b.say(" to ") + b.keyword(it.To, 2*maxName)
	case core.TimelineBaseChanged:
		return b.say("changed the base branch from ") + b.keyword(it.From, maxName) +
			b.say(" to ") + b.keyword(it.To, maxName)
	case core.TimelineHeadDeleted:
		return b.say("deleted the branch ") + b.keyword(it.Ref, maxName)
	case core.TimelineCrossReferenced:
		s := b.say("referenced this in ") + b.keyword("#"+strconv.Itoa(it.Source.Number), maxName)
		if title := termtext.Truncate(termtext.OneLine(it.Source.Title), 2*maxName, b.st.Glyphs.Cut); title != "" {
			s += " " + b.say(title)
		}
		return s
	default:
		return b.say(strings.ReplaceAll(string(it.Kind), "_", " "))
	}
}

// shortSHA returns the abbreviated SHA of a commit, from its full one when
// that is all there is.
func shortSHA(c core.TimelineCommit) string {
	if c.Short != "" {
		return c.Short
	}
	return c.OID[:min(len(c.OID), 7)]
}
