package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// History configures the History modal: the branches of the repository, the
// graph of one of them, and the commit under the graph's cursor.
type History struct {
	// Row is what each row of the graph shows, in this order within its
	// place: the short SHA first, then the subject, then the rest after
	// it, and the age or date at the right edge. Choose from the
	// HistoryRowFields.
	Row []string `yaml:"row"`
	// Detail is what the header of the commit pane shows, in this order.
	// Choose from the HistoryDetailFields. When it lists both the author
	// and the committer and they are the same person, with the same name
	// and email, they share one line, where the author is listed; list
	// only one of them to always show just that one.
	Detail []string `yaml:"detail"`
	// DateFormat is how dates read: relative ("3d ago"), absolute
	// ("2006-01-02 15:04 MST"), or a Go time layout such as
	// "2006-01-02 15:04".
	DateFormat string `yaml:"date_format"`
	// ShowEmail shows the git email next to each name, and in trailers
	// such as Co-authored-by.
	ShowEmail bool            `yaml:"show_email"`
	Prefetch  HistoryPrefetch `yaml:"prefetch"`
}

// HistoryPrefetch configures reading commits before the cursor lands on
// them, so the commit pane keeps up as the cursor moves. A commit costs one
// request the first time, and none after: what a commit changed never
// changes, so it is kept on disk.
type HistoryPrefetch struct {
	// Around is how many commits on each side of the cursor are read once
	// the cursor rests. Zero reads only the commit under the cursor.
	Around int `yaml:"around"`
	// HoverDelay is how long the cursor rests on a commit, or a branch,
	// before what it needs is read, so that moving through the graph
	// doesn't read every commit passed.
	HoverDelay time.Duration `yaml:"hover_delay"`
}

// Fields of a graph row, for [History.Row].
const (
	FieldShortSHA = "short_sha"
	FieldSubject  = "subject"
	// FieldAuthor and FieldCommitter show the GitHub login, or the name
	// without one.
	FieldAuthor    = "author"
	FieldCommitter = "committer"
	// FieldAge is how long ago the commit was authored, such as "3d".
	FieldAge = "age"
	// FieldDate is the date it was authored, in DateFormat, or as
	// 2006-01-02 when that is relative.
	FieldDate = "date"
	// FieldVerified marks a verified signature with ✓, and a signature
	// GitHub couldn't verify with ✗.
	FieldVerified = "verified"
	// FieldTrailers counts the co-authors in a row, and lists every
	// trailer in the commit pane.
	FieldTrailers = "trailers"
)

// Fields of the commit pane's header, for [History.Detail], besides
// FieldAuthor, FieldCommitter, FieldDate and FieldTrailers.
const (
	FieldSHA = "sha"
	// FieldVerification says whether the signature is verified, and why
	// not: ✓ verified, unsigned, or ✗ with GitHub's reason.
	FieldVerification = "verification"
	FieldParents      = "parents"
	// FieldBody is the message after the subject, without the trailers.
	FieldBody = "body"
	// FieldStats counts the files and lines the commit changed.
	FieldStats = "stats"
)

// HistoryRowFields and HistoryDetailFields are the fields that History.Row
// and History.Detail may list.
var (
	HistoryRowFields = []string{
		FieldShortSHA, FieldSubject, FieldAuthor, FieldCommitter, FieldAge, FieldDate, FieldVerified, FieldTrailers,
	}
	HistoryDetailFields = []string{
		FieldSHA, FieldAuthor, FieldCommitter, FieldDate, FieldVerification, FieldParents, FieldTrailers, FieldBody, FieldStats,
	}
)

// Date formats of [History.DateFormat] besides a Go layout.
const (
	DateRelative = "relative"
	DateAbsolute = "absolute"
)

// maxAround bounds History.Prefetch.Around: each commit may be a large
// diff, and the cursor rarely jumps further before it rests.
const maxAround = 10

func defaultHistory() History {
	return History{
		Row:        []string{FieldShortSHA, FieldSubject, FieldAuthor, FieldAge},
		Detail:     []string{FieldSHA, FieldAuthor, FieldCommitter, FieldDate, FieldVerification, FieldParents, FieldTrailers, FieldBody, FieldStats},
		DateFormat: DateRelative,
		Prefetch:   HistoryPrefetch{Around: 3, HoverDelay: 150 * time.Millisecond},
	}
}

func (h History) validate() error {
	var errs []error
	if len(h.Row) == 0 {
		errs = append(errs, errors.New("history.row: needs at least one field"))
	}
	errs = append(errs, validateFields("history.row", h.Row, HistoryRowFields), validateFields("history.detail", h.Detail, HistoryDetailFields))
	if !validDateFormat(h.DateFormat) {
		errs = append(errs, fmt.Errorf(`history.date_format: must be relative, absolute or a Go time layout such as "2006-01-02 15:04", got %q`, h.DateFormat))
	}
	if p := h.Prefetch; p.Around < 0 || p.Around > maxAround {
		errs = append(errs, fmt.Errorf("history.prefetch.around: must be between 0 and %d, got %d", maxAround, p.Around))
	}
	if h.Prefetch.HoverDelay < 0 {
		errs = append(errs, fmt.Errorf("history.prefetch.hover_delay: must not be negative, got %v", h.Prefetch.HoverDelay))
	}
	return errors.Join(errs...)
}

// validateFields rejects fields that aren't known, so that a typo doesn't
// silently leave a field out, and fields listed twice.
func validateFields(name string, fields, known []string) error {
	var errs []error
	for i, f := range fields {
		switch {
		case !slices.Contains(known, f):
			errs = append(errs, fmt.Errorf("%s[%d]: unknown field %q, want one of %s", name, i, f, strings.Join(known, ", ")))
		case slices.Index(fields, f) < i:
			errs = append(errs, fmt.Errorf("%s[%d]: %q is listed twice", name, i, f))
		}
	}
	return errors.Join(errs...)
}

// validDateFormat reports whether f is a format name, or a layout with at
// least one element of the reference time: any other text formats to
// itself, whatever the time. The time must not be the reference time, which
// formats every layout to itself.
func validDateFormat(f string) bool {
	if f == DateRelative || f == DateAbsolute {
		return true
	}
	t := time.Date(2001, time.March, 4, 7, 8, 9, 0, time.UTC)
	return strings.TrimSpace(f) != "" && t.Format(f) != f
}
