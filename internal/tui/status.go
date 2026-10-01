package tui

import (
	"cmp"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/statusbar"
)

// WithLogin sets the login of the account the app acts as, which the
// status bar shows before the host. Without it the bar shows the host
// alone.
func WithLogin(login string) Option {
	return func(m *Model) { m.login = login }
}

// The ranks of what the status bar shows, in the order it gives way as
// the terminal narrows: the hints but the help key's, the account, the
// rate limits, the help key, and the connection, which lasts longest.
const (
	rankHint = iota
	rankAccount
	rankRates
	rankHelp
	rankLink
)

// link is the state of the connection to GitHub.
type link int

const (
	// linkUnknown is before GitHub answered or a request failed.
	linkUnknown link = iota
	linkOnline
	// linkOffline is while the last request that ended got no answer.
	linkOffline
	// linkLimited is while a rate limit holds the requests of the
	// quotas the app lives on, or all of them.
	linkLimited
	// linkRejected is while GitHub rejects the token.
	linkRejected
	// linkFailing is while GitHub answers with server errors, and reads
	// are served what earlier ones kept.
	linkFailing
)

// barStyles are the styles of the status bar, built once per theme.
type barStyles struct {
	key, desc    lipgloss.Style
	label, value lipgloss.Style
	// low marks a quota nearly spent.
	low                lipgloss.Style
	online, warn, fail lipgloss.Style
}

func newBarStyles(t ui.Theme) barStyles {
	return barStyles{
		key: t.Muted, desc: t.Subtle,
		label: t.Subtle, value: t.Muted,
		low:    t.Warning,
		online: t.Success, warn: t.Warning, fail: t.Error,
	}
}

// bar is the footer when nothing takes its place: the status bar.
func (m *Model) bar() string {
	m.refreshBar()
	return m.status.View()
}

// refreshBar brings the hints up to date before they are drawn. Finding
// what each key reaches and rendering the hints costs more than listing
// the keys, so that is done again only when the keys changed.
func (m *Model) refreshBar() {
	layers := m.keyLayers()
	if sameLayers(layers, m.layers) {
		return
	}
	m.layers = cloneLayers(layers)
	m.drawHints()
}

// drawHints renders the hints of the layers kept, and lays the bar out
// with them.
func (m *Model) drawHints() {
	// The help key, as the first layer holds it now, leads, and the way
	// out of a zoom follows, where a narrow bar still shows them.
	var help key.Binding
	if short := m.layers[0].Short; len(short) > 0 {
		help = short[0]
	}
	h := ui.Hints{Layers: m.layers, Lead: []key.Binding{help, m.keys.state(m).Back}}
	short := h.ShortHelp()
	left := make([]statusbar.Item, 0, len(short))
	for i, b := range short {
		rank := rankHint
		if i == 0 && sameBinding(b, help) {
			rank = rankHelp
		}
		form := m.bst.key.Render(b.Help().Key) + " " + m.bst.desc.Render(b.Help().Desc)
		left = append(left, statusbar.Item{Forms: []string{form}, Rank: rank})
	}
	m.hints = left
	m.status.SetItems(m.hints, m.stats)
}

// drawStatus renders what the right of the bar tells: the rate limits,
// the connection and the account.
func (m *Model) drawStatus() {
	m.stats = m.stats[:0]
	if it := m.ratesItem(); len(it.Forms) > 0 {
		m.stats = append(m.stats, it)
	}
	if it := m.linkItem(); len(it.Forms) > 0 {
		m.stats = append(m.stats, it)
	}
	m.stats = append(m.stats, statusbar.Item{Forms: []string{m.bst.label.Render(m.account())}, Rank: rankAccount})
	m.status.SetItems(m.hints, m.stats)
}

// account names the account the app acts as: the login before the host,
// or the host alone while the login isn't known.
func (m *Model) account() string {
	account := cmp.Or(m.host, "github.com")
	if m.login != "" {
		account = m.login + "@" + account
	}
	return account
}

// readRates reads the rate limits again, if the app has what tells them,
// and redraws the status. The connection is offline since the request
// that failed first after GitHub last answered. Once GitHub answers again,
// or the last rate limit lifts, the command wakes what failed meanwhile;
// only the client's rate status does that, never a read the caches
// served.
func (m *Model) readRates() tea.Cmd {
	if m.rates == nil {
		return nil
	}
	was, wasRejected, wasMended := m.offSince, !m.rate.Rejected.IsZero(), m.rate.Mended
	wasLimited := limitedUntil(m.rate).After(m.rate.At)
	m.rate = m.rates.RateStatus()
	if offline := m.rate.Failed.After(m.rate.Answered); !offline {
		m.offSince = time.Time{}
	} else if m.offSince.IsZero() {
		m.offSince = m.rate.Failed
	}
	mended := m.rate.Mended.After(wasMended)
	lifted := wasLimited && !limitedUntil(m.rate).After(m.rate.At)
	m.drawStatus()
	m.logLink(was, wasRejected, mended)
	// GitHub answering again after an outage, or a resource that failed
	// with server errors answering well again, mends what failed
	// meanwhile. Failing that only went quiet, because other resources
	// answered well, doesn't: the polls of the resource would fail
	// again, and waking them would undo their backoff. Coming back from
	// an outage wakes them whether or not some resource still fails. The
	// last rate limit lifting wakes them too, unless GitHub fails, so
	// that a section that reads again on a wake what it was served kept
	// meanwhile, such as the files' listing, does.
	lifted = lifted && m.rate.Failing.IsZero()
	if (!was.IsZero() || mended || lifted) && m.offSince.IsZero() {
		return m.cameOnline()
	}
	return nil
}

// logLink logs a change of the connection, once: going offline, and
// since when; online again, and for how long it was offline, from was,
// when it went offline; GitHub rejecting the token, unless it was
// already; and GitHub failing with server errors, once per start, and
// mended, if it was.
func (m *Model) logLink(was time.Time, wasRejected, mended bool) {
	host := cmp.Or(m.host, "github.com")
	switch {
	case was.IsZero() && !m.offSince.IsZero():
		slog.InfoContext(m.ctx, "connection", "span", "tui", "state", "offline", "host", host, "since", m.offSince)
	case !was.IsZero() && m.offSince.IsZero():
		slog.InfoContext(m.ctx, "connection", "span", "tui", "state", "online", "host", host,
			"offline_s", m.rate.Answered.Sub(was).Round(time.Second).Seconds())
	}
	if !wasRejected && !m.rate.Rejected.IsZero() {
		slog.InfoContext(m.ctx, "connection", "span", "tui", "state", "rejected", "host", host)
	}
	if failing := m.rate.Failing; !failing.IsZero() && !failing.Equal(m.failSince) {
		m.failSince = failing
		slog.InfoContext(m.ctx, "connection", "span", "tui", "state", "failing", "host", host, "since", failing)
	}
	if mended && !m.failSince.IsZero() {
		slog.InfoContext(m.ctx, "connection", "span", "tui", "state", "mended", "host", host,
			"failing_s", m.rate.Mended.Sub(m.failSince).Round(time.Second).Seconds())
		m.failSince = time.Time{}
	}
}

// linkNow returns the state of the connection, and the time that goes
// with it: since when it is offline or failing, or until when it is
// limited. One state shows at a time, the first that holds of offline,
// token rejected, GitHub failing, rate limited and online: no answer at
// all says more than a rejected token, which fails every request, and
// that more than server errors, which serve what was kept. Failing hides
// a rate limit, whose reset the rates still show.
func (m *Model) linkNow() (link, time.Time) {
	r := m.rate
	switch {
	case !m.offSince.IsZero():
		return linkOffline, m.offSince
	case !r.Rejected.IsZero():
		return linkRejected, time.Time{}
	case !r.Failing.IsZero():
		return linkFailing, r.Failing
	}
	switch until := limitedUntil(r); {
	case until.After(r.At):
		return linkLimited, until
	case !r.Answered.IsZero():
		return linkOnline, time.Time{}
	}
	return linkUnknown, time.Time{}
}

// limitedUntil returns when the last rate limit of r that holds the
// requests lifts: a secondary limit, or a quota the bar shows that is
// spent. It is in the past, or zero, if none holds them at r.At.
func limitedUntil(r core.RateStatus) time.Time {
	until := r.SecondaryUntil
	for _, q := range shownQuotas(r) {
		if q.LimitedUntil.After(until) {
			until = q.LimitedUntil
		}
	}
	return until
}

// linkItem is the connection as the bar tells it, or with no forms while
// it is unknown. Its shorter form is the dot alone.
func (m *Model) linkItem() statusbar.Item {
	state, at := m.linkNow()
	var dot lipgloss.Style
	var text string
	switch state {
	case linkUnknown:
		return statusbar.Item{}
	case linkOnline:
		dot, text = m.bst.online, "online"
	case linkOffline:
		dot, text = m.bst.warn, "offline since "+ui.Clock(at, m.rate.At)
	case linkLimited:
		dot, text = m.bst.warn, "rate limited until "+ui.Clock(at, m.rate.At)
	case linkRejected:
		dot, text = m.bst.fail, "token rejected"
	case linkFailing:
		dot, text = m.bst.warn, "GitHub failing since "+ui.Clock(at, m.rate.At)
	}
	mark := dot.Render("●")
	return statusbar.Item{Forms: []string{mark + " " + m.bst.value.Render(text), mark}, Rank: rankLink}
}

// ratesItem is the rate limits as the bar tells them: what is left of
// core and graphql, the soonest reset and the requests held, and shorter,
// the tightest of the quotas in percent. It has no forms while GitHub
// reported none.
func (m *Model) ratesItem() statusbar.Item {
	quotas := shownQuotas(m.rate)
	if len(quotas) == 0 {
		return statusbar.Item{}
	}
	st := &m.bst
	sep := m.st.edge.Render(" · ")
	var parts []string
	var reset time.Time
	held := 0
	for _, q := range quotas {
		v := st.value
		if share(q) < core.LowQuotaShare {
			v = st.low
		}
		parts = append(parts, st.label.Render(quotaName(q.Resource)+" ")+v.Render(grouped(q.Remaining)+"/"+grouped(q.Limit)))
		if q.Reset.After(m.rate.At) && (reset.IsZero() || q.Reset.Before(reset)) {
			reset = q.Reset
		}
	}
	for _, q := range m.rate.Quotas {
		held += q.Held
	}
	if !reset.IsZero() {
		parts = append(parts, st.label.Render("resets ")+st.value.Render(ui.Clock(reset, m.rate.At)))
	}
	if held > 0 {
		parts = append(parts, st.warn.Render(strconv.Itoa(held)+" held"))
	}
	tight := slices.MinFunc(quotas, func(a, b core.Quota) int { return cmp.Compare(share(a), share(b)) })
	v := st.value
	if share(tight) < core.LowQuotaShare {
		v = st.low
	}
	pct := strconv.Itoa(share(tight)) + "%"
	if share(tight) == 0 && tight.Remaining > 0 {
		// What is left isn't all spent.
		pct = "<1%"
	}
	compact := st.label.Render(quotaName(tight.Resource)+" ") + v.Render(pct)
	return statusbar.Item{Forms: []string{strings.Join(parts, sep), compact}, Rank: rankRates}
}

// shownQuotas returns the quotas the bar shows, those the app lives on:
// core and graphql, of those GitHub reported with a limit.
func shownQuotas(r core.RateStatus) []core.Quota {
	var out []core.Quota
	for _, q := range r.Quotas {
		if (q.Resource == "core" || q.Resource == "graphql") && q.Limit > 0 {
			out = append(out, q)
		}
	}
	return out
}

// quotaName is the short name the bar gives a resource.
func quotaName(resource string) string {
	if resource == "graphql" {
		return "gql"
	}
	return resource
}

// share returns the whole percent of q's limit left.
func share(q core.Quota) int { return q.Remaining * 100 / q.Limit }

// grouped returns n with its thousands apart, as "4 812".
func grouped(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 && s[i-1] != '-' {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sameBinding reports whether a and b are the same binding, in the same
// state.
func sameBinding(a, b key.Binding) bool {
	return a.Enabled() == b.Enabled() && a.Help() == b.Help() && slices.Equal(a.Keys(), b.Keys())
}

// sameLayers reports whether a and b hold the same bindings, in the same
// state, so that the hints they give are the same.
func sameLayers(a, b []keyhelp.Layer) bool {
	return slices.EqualFunc(a, b, func(x, y keyhelp.Layer) bool {
		return x.Source == y.Source && x.Typing == y.Typing &&
			slices.EqualFunc(x.Bindings, y.Bindings, sameBinding) && slices.EqualFunc(x.Short, y.Short, sameBinding)
	})
}

// cloneLayers returns a copy of layers that what made them can't change,
// as a section may keep its bindings and change them later.
func cloneLayers(layers []keyhelp.Layer) []keyhelp.Layer {
	out := slices.Clone(layers)
	for i := range out {
		out[i].Bindings = slices.Clone(out[i].Bindings)
		out[i].Short = slices.Clone(out[i].Short)
	}
	return out
}
