package history

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/graph"
)

// rowDateLayout is how a row's date reads when dates are relative: the
// row has an age field for that.
const rowDateLayout = "2006-01-02"

// shortSHA is how many characters of a SHA name a commit in short.
const shortSHA = 7

// format turns commits into the text the graph and the header show, from
// the configured fields.
type format struct {
	row, detail []string
	date        ui.Dates
	email       bool
}

func newFormat(h config.History, dates ui.Dates) format {
	return format{row: slices.Clone(h.Row), detail: slices.Clone(h.Detail), date: dates, email: h.ShowEmail}
}

func short(sha string) string {
	return sha[:min(len(sha), shortSHA)]
}

// graphCommit returns the row of c in the graph at time now.
func (f format) graphCommit(c core.Commit, now time.Time) graph.Commit {
	g := graph.Commit{ID: c.SHA, Parents: c.Parents, Value: c}
	var detail, right []string
	for _, field := range f.row {
		switch field {
		case config.FieldShortSHA:
			g.Short = short(c.SHA)
		case config.FieldSubject:
			g.Title = ui.OneLine(c.Subject)
		case config.FieldAuthor:
			detail = appendNonEmpty(detail, handle(c.Author))
		case config.FieldCommitter:
			detail = appendNonEmpty(detail, handle(c.Committer))
		case config.FieldVerified:
			detail = appendNonEmpty(detail, verifiedMark(c.Verification))
		case config.FieldTrailers:
			if n := coauthors(c.Trailers); n > 0 {
				detail = append(detail, "+"+strconv.Itoa(n))
			}
		case config.FieldAge:
			if !c.Author.Date.IsZero() {
				right = append(right, ui.Ago(c.Author.Date, now))
			}
		case config.FieldDate:
			if !c.Author.Date.IsZero() {
				right = append(right, f.rowDate(c.Author.Date))
			}
		}
	}
	g.Detail = strings.Join(detail, " · ")
	g.Right = strings.Join(right, " ")
	return g
}

func appendNonEmpty(s []string, v string) []string {
	if v == "" {
		return s
	}
	return append(s, v)
}

// handle names who signed in a row: the GitHub login, or else the name.
func handle(s core.Signature) string {
	if s.Login != "" {
		return s.Login
	}
	return ui.OneLine(s.Name)
}

// verifiedMark marks a verified signature, and one GitHub couldn't verify.
func verifiedMark(v core.Verification) string {
	switch {
	case v.Verified:
		return "✓"
	case v.Signed:
		return "✗"
	}
	return ""
}

// coauthors counts the Co-authored-by trailers.
func coauthors(ts []core.Trailer) int {
	n := 0
	for _, t := range ts {
		if strings.EqualFold(t.Key, "Co-authored-by") {
			n++
		}
	}
	return n
}

// rowDate is the date of a row, in the configured layout.
func (f format) rowDate(t time.Time) string {
	return f.date.Date(t, rowDateLayout)
}

// dates is when a commit was authored in the header, and when it was
// committed if that was later, as after a rebase. Relative dates add the
// date to an age, unless there are two.
func (f format) dates(authored, committed, now time.Time) string {
	if committed.Sub(authored) > time.Minute {
		return f.date.Prose(authored, now) + ", committed " + f.date.Prose(committed, now)
	}
	if f.date.Relative() {
		return f.date.Prose(authored, now) + " · " + f.date.Date(authored, ui.AbsoluteLayout)
	}
	return f.date.Prose(authored, now)
}

// person is who signed in the header: the name, the email if configured,
// and the login.
func (f format) person(s core.Signature) string {
	parts := make([]string, 0, 3)
	parts = appendNonEmpty(parts, ui.OneLine(s.Name))
	if f.email && s.Email != "" {
		parts = append(parts, "<"+ui.OneLine(s.Email)+">")
	}
	if s.Login != "" && !strings.EqualFold(s.Login, s.Name) {
		parts = append(parts, "@"+s.Login)
	}
	return strings.Join(parts, " ")
}

// samePerson reports whether a and b are one person, by name and email.
func samePerson(a, b core.Signature) bool {
	return a.Name == b.Name && strings.EqualFold(a.Email, b.Email)
}

// trailerValue is the value of a trailer, without the email that a
// Co-authored-by or Signed-off-by carries unless emails are shown.
func (f format) trailerValue(v string) string {
	v = ui.OneLine(v)
	if f.email {
		return v
	}
	if i := strings.LastIndexByte(v, '<'); i > 0 && strings.HasSuffix(v, ">") {
		return strings.TrimSpace(v[:i])
	}
	return v
}

// verification says in words whether the signature is verified.
func verification(v core.Verification) (text string, ok, signed bool) {
	switch {
	case v.Verified:
		return "✓ verified", true, true
	case v.Signed:
		reason := strings.ReplaceAll(v.Reason, "_", " ")
		if reason == "" {
			reason = "not verified"
		}
		return "✗ " + reason, false, true
	}
	return "unsigned", false, false
}
