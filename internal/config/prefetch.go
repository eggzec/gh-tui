package config

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// PrefetchLayers configures reading ahead: what the app reads before it is
// opened, so that it opens at once. It has three layers: these global
// knobs, a page's, and a kind of item's on that page. Each knob of a kind
// is its own if it sets it, else its page's, else the global one
// ([PrefetchLayers.Resolve]).
type PrefetchLayers struct {
	// Enabled says whether anything is read ahead.
	Enabled bool `yaml:"enabled"`
	// Window is the rows around the cursor read besides the row under it.
	Window Window `yaml:"window"`
	// Rest is how long the cursor stays on a row before anything is read
	// for it.
	Rest time.Duration `yaml:"rest"`
	// Parallel is the most reads ahead in flight at once, on top of the
	// user's own. The connection is shared by every page, so it is set
	// here, not per page or kind.
	Parallel int `yaml:"parallel"`

	Pulls         PullsKinds         `yaml:"pulls"`
	Issues        IssuesKinds        `yaml:"issues"`
	Notifications NotificationsKinds `yaml:"notifications"`
	Dashboard     DashboardKinds     `yaml:"dashboard"`
	Search        SearchKinds        `yaml:"search"`
	Files         FilesKinds         `yaml:"files"`
	Finder        FinderKinds        `yaml:"finder"`
	History       HistoryKinds       `yaml:"history"`
	Actions       ActionsKinds       `yaml:"actions"`
}

// Window is the rows read around the cursor: Before above it and After
// below it, besides the row under it. 0/0 reads the row under the cursor
// only.
type Window struct {
	Before int `yaml:"before"`
	After  int `yaml:"after"`
}

// Span is the window of a page or a kind, each side of which it may leave
// to the layer above.
type Span struct {
	Before *int `yaml:"before,omitempty" inherit:"prefetch"`
	After  *int `yaml:"after,omitempty" inherit:"prefetch"`
}

// Layer is the knobs of a kind of item, each of which it may leave to its
// page, and the page to the global knobs.
type Layer struct {
	Enabled *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window  Span           `yaml:"window,omitempty"`
	Rest    *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
}

// PreviewLayer is the knobs of a kind that reads the content of files,
// and the largest file it reads around the cursor.
type PreviewLayer struct {
	Enabled *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window  Span           `yaml:"window,omitempty"`
	Rest    *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	// MaxSize is the largest file read around the cursor, at most
	// files.preview.max_size. The file under the cursor, which a pane
	// shows or is likely opened next, is read up to that.
	MaxSize Size `yaml:"max_size"`
}

// isKind reports whether t is the type of a kind of item's settings.
func isKind(t reflect.Type) bool {
	return t == reflect.TypeFor[Layer]() || t == reflect.TypeFor[PreviewLayer]()
}

// The pages and their kinds of items. A page's own knobs are fields of its
// own rather than an embedded Layer, so that the path of each setting is
// the path of its field.

// PullsKinds is the pull request list's.
type PullsKinds struct {
	Enabled *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window  Span           `yaml:"window,omitempty"`
	Rest    *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	// Details is the pull request: title, body, state, labels, checks.
	Details Layer `yaml:"details,omitempty"`
	// Comments is the first page of its conversation.
	Comments Layer `yaml:"comments,omitempty"`
	// Checks is the full list of its Checks tab.
	Checks Layer `yaml:"checks,omitempty"`
	// OtherTabs is the first page of the state tabs not shown yet.
	OtherTabs Layer `yaml:"other_tabs,omitempty"`
}

// IssuesKinds is the issue list's.
type IssuesKinds struct {
	Enabled   *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window    Span           `yaml:"window,omitempty"`
	Rest      *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	Details   Layer          `yaml:"details,omitempty"`
	Comments  Layer          `yaml:"comments,omitempty"`
	OtherTabs Layer          `yaml:"other_tabs,omitempty"`
}

// NotificationsKinds is the notifications screen's: what a thread is
// about, and its first comments.
type NotificationsKinds struct {
	Enabled  *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window   Span           `yaml:"window,omitempty"`
	Rest     *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	Details  Layer          `yaml:"details,omitempty"`
	Comments Layer          `yaml:"comments,omitempty"`
}

// DashboardKinds is the dashboard's, by its panes.
type DashboardKinds struct {
	Enabled      *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window       Span           `yaml:"window,omitempty"`
	Rest         *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	WaitingOnYou Layer          `yaml:"waiting_on_you,omitempty"`
	Inbox        Layer          `yaml:"inbox,omitempty"`
	Repositories Layer          `yaml:"repositories,omitempty"`
	Pinned       Layer          `yaml:"pinned,omitempty"`
}

// SearchKinds is the search results': pull request and issue hits,
// and the first page of the other kinds of results.
type SearchKinds struct {
	Enabled    *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window     Span           `yaml:"window,omitempty"`
	Rest       *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	Details    Layer          `yaml:"details,omitempty"`
	Comments   Layer          `yaml:"comments,omitempty"`
	OtherKinds Layer          `yaml:"other_kinds,omitempty"`
}

// FilesKinds is the files tree's: the listings of folders, and the
// content of files.
type FilesKinds struct {
	Enabled *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window  Span           `yaml:"window,omitempty"`
	Rest    *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	Tree    Layer          `yaml:"tree,omitempty"`
	Preview PreviewLayer   `yaml:"preview,omitempty"`
}

// FinderKinds is the file finder's: the content of the result.
type FinderKinds struct {
	Enabled *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window  Span           `yaml:"window,omitempty"`
	Rest    *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	Preview PreviewLayer   `yaml:"preview,omitempty"`
}

// HistoryKinds is the history's: what commits changed, and the graphs
// of branches.
type HistoryKinds struct {
	Enabled  *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window   Span           `yaml:"window,omitempty"`
	Rest     *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	Commits  Layer          `yaml:"commits,omitempty"`
	Branches Layer          `yaml:"branches,omitempty"`
}

// ActionsKinds is the Actions modal's: the jobs of runs, and the logs
// of failed jobs.
type ActionsKinds struct {
	Enabled *bool          `yaml:"enabled,omitempty" inherit:"prefetch"`
	Window  Span           `yaml:"window,omitempty"`
	Rest    *time.Duration `yaml:"rest,omitempty" inherit:"prefetch"`
	Jobs    Layer          `yaml:"jobs,omitempty"`
	Logs    Layer          `yaml:"logs,omitempty"`
}

// Bounds of the knobs.
const (
	// maxWindow is the most rows a window reads on either side.
	maxWindow = 30
	// maxRest is the longest the cursor may need to rest.
	maxRest = 2 * time.Second
	// maxParallel is the most reads ahead in flight at once.
	maxParallel = 10
)

// windowBounds are the most rows on either side of the pages and kinds
// whose windows go further or less far than maxWindow, by path below
// prefetch: a page's bound holds for its kinds too.
var windowBounds = map[string]int{
	// A file's content is kept on disk by its SHA, and the rows are short.
	"files.preview": 64,
	// Each commit costs a request, once.
	"history": 10,
}

// windowless are the kinds that read a page at a time, not around the
// cursor, and say so of a window set on them.
var windowless = map[string]string{
	"pulls.other_tabs":   "other tabs are read a page at a time, not around the cursor",
	"issues.other_tabs":  "other tabs are read a page at a time, not around the cursor",
	"search.other_kinds": "other kinds of results are read a page at a time, not around the cursor",
}

// PageKind names a kind of item on a page, such as pulls and details.
type PageKind struct {
	Page, Kind string
}

func (pk PageKind) String() string { return pk.Page + "." + pk.Kind }

// Kinds returns every page and kind of item, in the order of the fields.
func (PrefetchLayers) Kinds() []PageKind {
	var out []PageKind
	for page := range reflect.TypeFor[PrefetchLayers]().Fields() {
		if !isPage(page.Type) {
			continue
		}
		for kind := range page.Type.Fields() {
			if isKind(kind.Type) {
				out = append(out, PageKind{yamlName(page), yamlName(kind)})
			}
		}
	}
	return out
}

// isPage reports whether t is the type of a page's settings.
func isPage(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && strings.HasSuffix(t.Name(), "Kinds")
}

// Resolved is what a kind of item on a page reads ahead, with the layer
// each knob came from: the path of its setting, such as prefetch.window
// or prefetch.pulls.details.window.
type Resolved struct {
	Enabled bool
	Window  Window
	Rest    time.Duration
	From    struct{ Enabled, Before, After, Rest string }
}

// Resolve returns the knobs of kind on page: for each, the kind's own if
// it sets it, else the page's, else the global one. A window wider than
// the kind's bound is cut to it, and a kind with no window has 0/0.
func (p PrefetchLayers) Resolve(page, kind string) (Resolved, error) {
	pv, kv, err := p.layers(page, kind)
	if err != nil {
		return Resolved{}, err
	}
	var r Resolved
	const global = "prefetch"
	pagePath, kindPath := global+"."+page, global+"."+page+"."+kind
	r.Enabled, r.From.Enabled = p.Enabled, global+".enabled"
	r.Window, r.From.Before, r.From.After = p.Window, global+".window.before", global+".window.after"
	r.Rest, r.From.Rest = p.Rest, global+".rest"
	for _, l := range []struct {
		path string
		v    reflect.Value
	}{{pagePath, pv}, {kindPath, kv}} {
		if b := l.v.FieldByName("Enabled").Interface().(*bool); b != nil {
			r.Enabled, r.From.Enabled = *b, l.path+".enabled"
		}
		w := l.v.FieldByName("Window").Interface().(Span)
		if w.Before != nil {
			r.Window.Before, r.From.Before = *w.Before, l.path+".window.before"
		}
		if w.After != nil {
			r.Window.After, r.From.After = *w.After, l.path+".window.after"
		}
		if d := l.v.FieldByName("Rest").Interface().(*time.Duration); d != nil {
			r.Rest, r.From.Rest = *d, l.path+".rest"
		}
	}
	if _, ok := windowless[page+"."+kind]; ok {
		r.Window, r.From.Before, r.From.After = Window{}, "", ""
	}
	limit := windowBound(page, kind)
	r.Window.Before, r.Window.After = min(r.Window.Before, limit), min(r.Window.After, limit)
	return r, nil
}

// layers returns the settings of page and of kind on it.
func (p PrefetchLayers) layers(page, kind string) (pv, kv reflect.Value, err error) {
	v := reflect.ValueOf(p)
	pf, ok := fieldByName(v.Type(), page)
	if !ok || !isPage(pf.Type) {
		return pv, kv, fmt.Errorf("%w %q", ErrUnknownKey, "prefetch."+page)
	}
	pv = v.FieldByIndex(pf.Index)
	kf, ok := fieldByName(pf.Type, kind)
	if !ok || !isKind(kf.Type) {
		return pv, kv, fmt.Errorf("%w %q", ErrUnknownKey, "prefetch."+page+"."+kind)
	}
	return pv, pv.FieldByIndex(kf.Index), nil
}

// windowBound returns the most rows on either side that kind on page may
// read: that of the kind, else of its page, else maxWindow.
func windowBound(page, kind string) int {
	if n, ok := windowBounds[page+"."+kind]; ok {
		return n
	}
	if n, ok := windowBounds[page]; ok {
		return n
	}
	return maxWindow
}

// validate checks the bounds of every knob each layer sets, that no kind
// without a window sets one, and that no kind reads files ahead larger
// than previewMax, the largest the preview reads.
func (p PrefetchLayers) validate(previewMax Size) error {
	var errs []error
	errs = append(errs, checkWindow("prefetch.window.before", p.Window.Before, maxWindow),
		checkWindow("prefetch.window.after", p.Window.After, maxWindow),
		checkRest("prefetch.rest", p.Rest))
	if p.Parallel < 1 || p.Parallel > maxParallel {
		errs = append(errs, fmt.Errorf("prefetch.parallel: must be between 1 and %d, got %d", maxParallel, p.Parallel))
	}
	v := reflect.ValueOf(p)
	for pf := range v.Type().Fields() {
		if !isPage(pf.Type) {
			continue
		}
		page := yamlName(pf)
		pv := v.FieldByIndex(pf.Index)
		errs = append(errs, validateLayer("prefetch."+page, pv, windowBound(page, "")))
		for kf := range pf.Type.Fields() {
			if !isKind(kf.Type) {
				continue
			}
			kind := yamlName(kf)
			path := "prefetch." + page + "." + kind
			kv := pv.FieldByIndex(kf.Index)
			if why, ok := windowless[page+"."+kind]; ok {
				if w := kv.FieldByName("Window").Interface().(Span); w.Before != nil || w.After != nil {
					errs = append(errs, fmt.Errorf("%s.window: %s", path, why))
				}
			}
			errs = append(errs, validateLayer(path, kv, windowBound(page, kind)))
			if l, ok := reflect.TypeAssert[PreviewLayer](kv); ok && (l.MaxSize < 0 || l.MaxSize > previewMax) {
				errs = append(errs, fmt.Errorf("%s.max_size: must be between 0B and files.preview.max_size (%v), got %v", path, previewMax, l.MaxSize))
			}
		}
	}
	return errors.Join(errs...)
}

// validateLayer checks the knobs that the layer at path, v, sets.
func validateLayer(path string, v reflect.Value, limit int) error {
	var errs []error
	w := v.FieldByName("Window").Interface().(Span)
	if w.Before != nil {
		errs = append(errs, checkWindow(path+".window.before", *w.Before, limit))
	}
	if w.After != nil {
		errs = append(errs, checkWindow(path+".window.after", *w.After, limit))
	}
	if d := v.FieldByName("Rest").Interface().(*time.Duration); d != nil {
		errs = append(errs, checkRest(path+".rest", *d))
	}
	return errors.Join(errs...)
}

func checkWindow(key string, n, limit int) error {
	if n < 0 || n > limit {
		return fmt.Errorf("%s: must be between 0 and %d, got %d", key, limit, n)
	}
	return nil
}

func checkRest(key string, d time.Duration) error {
	if d < 0 || d > maxRest {
		return fmt.Errorf("%s: must be between 0 and %v, got %v", key, maxRest, d)
	}
	return nil
}

// Table returns what each kind of item on each page reads ahead,
// as [PrefetchLayers.Resolve] resolves it, as lines of text: one for each kind,
// each knob followed by the setting it came from, for :config to show.
func (p PrefetchLayers) Table() []string {
	kinds := p.Kinds()
	width := 0
	for _, pk := range kinds {
		width = max(width, len(pk.String()))
	}
	out := make([]string, 0, len(kinds))
	for _, pk := range kinds {
		r, err := p.Resolve(pk.Page, pk.Kind)
		if err != nil {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%-*s  enabled %-5s (%s)", width, pk.String(), strconv.FormatBool(r.Enabled), r.From.Enabled)
		if r.From.Before != "" {
			fmt.Fprintf(&b, "  window %d (%s) / %d (%s)", r.Window.Before, r.From.Before, r.Window.After, r.From.After)
		}
		fmt.Fprintf(&b, "  rest %s (%s)", formatDuration(r.Rest), r.From.Rest)
		out = append(out, b.String())
	}
	return out
}

// inherited returns the value that the setting key, a knob that its layer
// leaves to the one above, takes, and the setting it takes it from, such
// as "4 (from prefetch.window.after)".
func (p PrefetchLayers) inherited(key string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(key, "prefetch."), ".")
	knob := parts[len(parts)-1]
	var page, kind string
	switch {
	case len(parts) >= 2 && parts[len(parts)-2] == "window":
		parts = parts[:len(parts)-2]
	default:
		parts = parts[:len(parts)-1]
	}
	switch len(parts) {
	case 1:
		page = parts[0]
	case 2:
		page, kind = parts[0], parts[1]
	default:
		return "", false
	}
	if kind == "" {
		// A page's knob comes from the global one, and its window is cut
		// to the page's bound, as Resolve cuts it.
		limit := windowBound(page, "")
		switch knob {
		case "enabled":
			return fmt.Sprintf("%t (from prefetch.enabled)", p.Enabled), true
		case "before":
			return fmt.Sprintf("%d (from prefetch.window.before)", min(p.Window.Before, limit)), true
		case "after":
			return fmt.Sprintf("%d (from prefetch.window.after)", min(p.Window.After, limit)), true
		case "rest":
			return formatDuration(p.Rest) + " (from prefetch.rest)", true
		}
		return "", false
	}
	r, err := p.Resolve(page, kind)
	if err != nil {
		return "", false
	}
	if why, ok := windowless[page+"."+kind]; ok && (knob == "before" || knob == "after") {
		return "none: " + why, true
	}
	switch knob {
	case "enabled":
		return fmt.Sprintf("%t (from %s)", r.Enabled, r.From.Enabled), true
	case "before":
		return fmt.Sprintf("%d (from %s)", r.Window.Before, r.From.Before), true
	case "after":
		return fmt.Sprintf("%d (from %s)", r.Window.After, r.From.After), true
	case "rest":
		return fmt.Sprintf("%s (from %s)", formatDuration(r.Rest), r.From.Rest), true
	}
	return "", false
}
