package ui

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// BulkLimit is the most changes of one bulk change that are in flight at
// once, so that marking many rows doesn't open that many requests, which
// GitHub's secondary rate limits would refuse.
const BulkLimit = 4

// BulkItem is one change of a bulk change.
type BulkItem struct {
	// Key identifies the row in its list, which keeps it marked if the
	// change fails.
	Key string
	// Name names it in the toast, such as "#12".
	Name string
	// What names the change for the log and for the error, such as
	// "close #12".
	What string
	// Start shows the change in the cache at once and returns the op that
	// sends it. It is called only once the user said yes.
	Start func() Op
}

// BulkPlan is what a change on the marked rows of a list does: the rows it
// applies to, and the marked rows it leaves out and why, which the question
// to the user says, so that "Close 3 pull requests?" never hides that a
// fourth marked one isn't closed.
type BulkPlan struct {
	// Verb is the change as an imperative, such as "Close", Noun what it
	// changes, such as "pull request", and Tail what follows the count of
	// them, such as " as read" ("Mark 3 notifications as read"). Past is
	// the verb in the past tense, such as "Closed".
	Verb, Past, Noun, Tail string
	// Items are the changes, in the order of the list.
	Items []BulkItem
	// List identifies the list whose rows were marked, such as the ID of
	// its feed, so that a section unmarks rows only in that list.
	List int

	// skips counts the loaded rows left out by the reason, in the order
	// each reason was first given. unloaded counts the marked rows that are
	// not among the loaded ones.
	skips    []bulkSkip
	unloaded int
	// refusal is why the viewer may not make the change, as the first
	// refused row says it.
	refusal string
}

type bulkSkip struct {
	reason string
	n      int
}

// NewBulkPlan returns the plan of a change on marked rows, of which
// unloaded aren't among the loaded rows.
func NewBulkPlan(verb, past, noun, tail string, unloaded int) BulkPlan {
	return BulkPlan{Verb: verb, Past: past, Noun: noun, Tail: tail, unloaded: unloaded}
}

// Skip leaves a loaded row out, for reason, such as "already closed".
func (p *BulkPlan) Skip(reason string) {
	for i := range p.skips {
		if p.skips[i].reason == reason {
			p.skips[i].n++
			return
		}
	}
	p.skips = append(p.skips, bulkSkip{reason, 1})
}

// SkipRefused leaves a loaded row out because the viewer may not change
// it, for the reason why says, which tells the user if the change applies
// to no row.
func (p *BulkPlan) SkipRefused(why string) {
	p.Skip("not allowed")
	if p.refusal == "" {
		p.refusal = why
	}
}

// Marked returns how many rows are marked: those the change applies to,
// those it leaves out, and those not loaded.
func (p BulkPlan) Marked() int {
	n := len(p.Items) + p.unloaded
	for _, s := range p.skips {
		n += s.n
	}
	return n
}

// Empty reports whether the change applies to no row.
func (p BulkPlan) Empty() bool { return len(p.Items) == 0 }

// count returns n with the noun, such as "3 pull requests".
func (p BulkPlan) count(n int) string {
	s := strconv.Itoa(n) + " " + p.Noun
	if n != 1 {
		s += "s"
	}
	return s
}

// left says what the change leaves out, such as "1 already closed, 2 not
// loaded", or "" when it leaves out nothing.
func (p BulkPlan) left() string {
	var parts []string
	for _, s := range p.skips {
		parts = append(parts, strconv.Itoa(s.n)+" "+s.reason)
	}
	if p.unloaded > 0 {
		parts = append(parts, strconv.Itoa(p.unloaded)+" not loaded")
	}
	return strings.Join(parts, ", ")
}

// Question asks to make the change on its rows, such as "Close 3 pull
// requests?", or, when it leaves marked rows out, "Close 2 of 3 marked (1
// already closed)?". The question is about the marked rows, so it names no
// repository, which the screen shows.
func (p BulkPlan) Question() string {
	if left := p.left(); left != "" {
		return fmt.Sprintf("%s %d of %d marked%s (%s)?", p.Verb, len(p.Items), p.Marked(), p.Tail, left)
	}
	return p.Verb + " " + p.count(len(p.Items)) + p.Tail + "?"
}

// Nothing says why the change applies to no marked row, for a toast.
func (p BulkPlan) Nothing() string {
	if p.refusal != "" {
		return p.refusal
	}
	return "No marked " + p.Noun + " can be " + strings.ToLower(p.Past) + p.Tail + ": " + p.left() + "."
}

// Same reports whether q is the plan p: the same question about the same
// rows.
func (p BulkPlan) Same(q BulkPlan) bool {
	keys := func(it BulkItem) string { return it.Key }
	return p.Verb == q.Verb && p.Question() == q.Question() &&
		slices.Equal(mapSlice(p.Items, keys), mapSlice(q.Items, keys))
}

func mapSlice[T, U any](s []T, f func(T) U) []U {
	out := make([]U, len(s))
	for i, v := range s {
		out[i] = f(v)
	}
	return out
}

// Confirm returns the question of p. By the time the user says yes, the
// list may have changed behind it, such as a refresh that dropped a row, so
// now plans again, and run makes the change, with the plan it returns,
// only if that is still the plan asked; otherwise nothing is sent, and the
// user is told so. ok is unset when now has no plan any more.
func (p BulkPlan) Confirm(now func() (BulkPlan, bool), run func(BulkPlan) tea.Cmd) Confirm {
	return Confirm{Question: p.Question(), Run: func() tea.Cmd {
		if again, ok := now(); ok && again.Same(p) {
			return run(again)
		}
		return Notify(toast.Info, Meanwhile("The marked "+p.Noun+"s"))
	}}
}

// Do shows every change of p in the cache at once, and returns the command
// that sends them, at most [BulkLimit] at a time, for the section titled
// from. Each is rolled back if it fails, and the command reports one
// [BulkDoneMsg] when all are done.
func (p BulkPlan) Do(ctx context.Context, from string) tea.Cmd {
	ops := make([]Op, len(p.Items))
	for i, it := range p.Items {
		ops[i] = it.Start()
	}
	return func() tea.Msg {
		ctx := obs.WithTrace(ctx, "bulk")
		start := time.Now()
		slog.InfoContext(ctx, "bulk sent", "span", "op", "section", from, "what", p.Verb, "count", len(ops))
		errs := make([]error, len(ops))
		var wg sync.WaitGroup
		slots := make(chan struct{}, BulkLimit)
		for i, it := range p.Items {
			slots <- struct{}{}
			wg.Go(func() {
				defer func() { <-slots }()
				errs[i] = sendOp(ctx, from, ops[i], it.What)
			})
		}
		wg.Wait()
		msg := BulkDoneMsg{From: from, Plan: p}
		for i, err := range errs {
			if err != nil {
				msg.Failed = append(msg.Failed, BulkFailure{Item: p.Items[i], Err: err})
			}
		}
		slog.InfoContext(ctx, "bulk done", "span", "op", "section", from, "what", p.Verb,
			"count", len(ops), "failed", len(msg.Failed), slog.Float64("duration_ms", obs.Millis(time.Since(start))))
		return msg
	}
}

// BulkFailure is a change of a bulk change that was rolled back.
type BulkFailure struct {
	Item BulkItem
	Err  error
}

// BulkDoneMsg reports that every change of a bulk change finished. The app
// says how it went in one toast, then passes the message on so the section
// that sent it can re-render from the cache and unmark the rows that
// changed.
type BulkDoneMsg struct {
	// From is the title of the section that sent the changes.
	From string
	// Plan is what was sent, as asked.
	Plan BulkPlan
	// Failed are the changes the server refused, which were rolled back.
	Failed []BulkFailure
}

// FailedKeys returns the keys of the rows whose change failed.
func (m BulkDoneMsg) FailedKeys() []string {
	return mapSlice(m.Failed, func(f BulkFailure) string { return f.Item.Key })
}

// Toast returns the toast that tells how the bulk change went: a success
// that says how many rows changed, or an error that names those that
// didn't and why, as much as fits reports a toast of that level shows
// whole. It is "" when nothing is worth saying, such as for changes that
// were canceled.
func (m BulkDoneMsg) Toast(v Voice, fits func(toast.Level, string) bool) (lvl toast.Level, msg string) {
	p, n := m.Plan, len(m.Plan.Items)
	if len(m.Failed) == 0 {
		return toast.Success, p.Past + " " + p.count(n) + p.Tail + "."
	}
	head := "Couldn't " + strings.ToLower(p.Verb) + " " + p.count(n) + p.Tail
	if len(m.Failed) < n {
		head = fmt.Sprintf("Couldn't %s %d of %d %ss%s", strings.ToLower(p.Verb), len(m.Failed), n, p.Noun, p.Tail)
	}
	names := make([]string, len(m.Failed))
	for i, f := range m.Failed {
		names[i] = f.Item.Name
	}
	// Each failure says its cause as SayToast does: from the fullest
	// sentence to the shortest, which keeps the call to action, such as
	// the command that grants the token what it lacks. Text that names
	// something, such as GitHub's own reason, keeps its capitals.
	causes := make([][]string, len(m.Failed))
	levels, silent := 1, true
	for i, f := range m.Failed {
		p := core.Explain(f.Item.What, f.Err)
		text, _, named := say(p, v)
		if text == "" {
			continue
		}
		silent = false
		if !named {
			text = lowerFirst(text)
		}
		for _, c := range toastCauses(p, v, text) {
			causes[i] = append(causes[i], strings.TrimSuffix(c, "."))
		}
		levels = max(levels, len(causes[i]))
	}
	if silent {
		// Only what the user needn't hear of, such as a cancellation.
		return toast.Error, ""
	}
	// Rows that failed for one reason share it.
	why := func(level int) string {
		var groups []string
		byReason := map[string][]string{}
		for i, f := range m.Failed {
			reason := ""
			if c := causes[i]; len(c) > 0 {
				reason = c[min(level, len(c)-1)]
			}
			if _, seen := byReason[reason]; !seen {
				groups = append(groups, reason)
			}
			byReason[reason] = append(byReason[reason], f.Item.Name)
		}
		parts := make([]string, len(groups))
		for i, r := range groups {
			parts[i] = strings.Join(byReason[r], ", ")
			if r != "" {
				parts[i] += " (" + r + ")"
			}
		}
		return strings.Join(parts, "; ")
	}
	var texts []string
	for level := range levels {
		texts = append(texts, head+": "+why(level)+".")
	}
	texts = append(texts, head+": "+strings.Join(names, ", ")+".", head+".")
	for _, text := range texts {
		if fits(toast.Error, text) {
			return toast.Error, text
		}
	}
	return toast.Error, head + "."
}
