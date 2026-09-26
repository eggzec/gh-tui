package dashboard

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// KeyMap holds the keys of the dashboard.
type KeyMap struct {
	// Next and Prev move the focus through the panes, and Panes focus the
	// pane with that number.
	Next  key.Binding
	Prev  key.Binding
	Panes [numPanes]key.Binding
	// Zoom shows the focused pane alone, or every pane again, and Back
	// shows them again too.
	Zoom key.Binding
	Back key.Binding
	// Select opens what is under the cursor: a repository, a pull request
	// or issue, or what a notification is about.
	Select key.Binding
	// Open opens what is under the cursor in the browser.
	Open    key.Binding
	Refresh key.Binding
	// Filter names the key that opens the filter of the repositories,
	// which the app handles, and ClearFilter clears it.
	Filter      key.Binding
	ClearFilter key.Binding
	// NextOwner and PrevOwner switch the repositories between the viewer's
	// own and those of each organization, and the work between its lists.
	NextOwner key.Binding
	PrevOwner key.Binding
	// Here opens the repository of the current directory.
	Here key.Binding
	// Checks opens the pull request of the work under the cursor on its
	// checks.
	Checks key.Binding
	// Notifications names the key that shows every notification, which
	// the app handles.
	Notifications key.Binding
	// Up, Down, Left and Right move through the cards, the work and the
	// notifications.
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding

	// feed is the navigation of the repositories, without the keys above.
	feed feed.KeyMap
	// jump stands for Panes in the help.
	jump key.Binding
}

func newKeyMap(keys map[string][]string) KeyMap {
	k := KeyMap{
		Next:          ui.Binding(keys, config.ActionNextTab, "next pane"),
		Prev:          ui.Binding(keys, config.ActionPrevTab, "previous pane"),
		Zoom:          ui.Binding(keys, config.ActionZoom, "zoom"),
		Back:          ui.Binding(keys, config.ActionBack, "unzoom"),
		Select:        ui.Binding(keys, config.ActionSelect, "open"),
		Open:          ui.Binding(keys, config.ActionOpen, "browser"),
		Refresh:       ui.Binding(keys, config.ActionRefresh, "refresh"),
		Filter:        ui.Binding(keys, config.ActionFilter, "filter"),
		ClearFilter:   ui.Binding(keys, config.ActionClearFilter, "clear filters"),
		NextOwner:     ui.Binding(keys, config.ActionNextOwner, "next owner"),
		PrevOwner:     ui.Binding(keys, config.ActionPrevOwner, "previous owner"),
		Here:          ui.Binding(keys, config.ActionCurrentRepo, "this repo"),
		Checks:        ui.Binding(keys, config.ActionChecks, "checks"),
		Notifications: ui.Binding(keys, config.ActionNotifications, "all notifications"),
		Up:            key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:          key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:          key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:         key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
	}
	actions := [numPanes]string{config.ActionPane1, config.ActionPane2, config.ActionPane3, config.ActionPane4, config.ActionPane5}
	labels := make([]string, 0, numPanes)
	for i, a := range actions {
		k.Panes[i] = ui.Binding(keys, a, paneTitles[i])
		if k.Panes[i].Enabled() {
			labels = append(labels, k.Panes[i].Help().Key)
		}
	}
	k.jump = key.NewBinding(key.WithDisabled())
	if len(labels) > 0 {
		k.jump = key.NewBinding(key.WithKeys(labels...), key.WithHelp(labels[0]+"-"+labels[len(labels)-1], "focus pane"))
	}

	own := []key.Binding{k.Select, k.Open, k.Refresh, k.Filter, k.ClearFilter, k.NextOwner, k.PrevOwner, k.Here, k.Next, k.Prev, k.Zoom, k.Back}
	f := feed.DefaultKeyMap()
	f.Up = free(f.Up, own)
	f.Down = free(f.Down, own)
	f.PageUp = free(f.PageUp, own)
	f.PageDown = free(f.PageDown, own)
	f.Home = free(f.Home, own)
	f.End = free(f.End, own)
	f.Retry = key.NewBinding(key.WithKeys(k.Refresh.Keys()...), key.WithHelp(k.Refresh.Help().Key, "retry"), key.WithDisabled())
	k.feed = f
	return k
}

// free drops the keys of b that the dashboard binds itself, such as "f",
// which the list uses for page down and the dashboard for the filter.
func free(b key.Binding, taken []key.Binding) key.Binding {
	keys := slices.DeleteFunc(slices.Clone(b.Keys()), func(k string) bool {
		return slices.ContainsFunc(taken, func(t key.Binding) bool {
			return t.Enabled() && slices.Contains(t.Keys(), k)
		})
	})
	if len(keys) == len(b.Keys()) {
		return b
	}
	if len(keys) == 0 {
		return key.NewBinding(key.WithDisabled())
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(keys, "/"), b.Help().Desc))
}

// pane returns the pane that msg focuses, or -1.
func (k KeyMap) pane(msg tea.KeyPressMsg) paneID {
	for i, b := range k.Panes {
		if key.Matches(msg, b) {
			return paneID(i)
		}
	}
	return -1
}

// helpKeys lists the keys of the focused pane, then those of the
// dashboard.
type helpKeys struct {
	k     KeyMap
	pane  paneID
	repos *repoTabs
	cal   calendar.KeyMap
	here  bool
	// wide is set while the dashboard fits every pane, so zooming shows,
	// and zoom while the focused pane is zoomed.
	wide, zoom bool
	// markRead is set when opening a notification marks it read.
	markRead bool
}

func (h helpKeys) paneKeys() []key.Binding {
	k := h.k
	switch h.pane {
	case pinnedPane:
		return []key.Binding{k.Left, k.Right, k.Select, k.Open}
	case reposPane:
		clr := k.ClearFilter
		clr.SetEnabled(clr.Enabled() && h.repos.filter().active())
		return []key.Binding{h.repos.feedKeys().Up, h.repos.feedKeys().Down, k.Select, k.Filter, clr, k.NextOwner, k.Open, h.repos.feedKeys().Retry}
	case workPane:
		next := k.NextOwner
		next.SetHelp(next.Help().Key, "next list")
		return []key.Binding{k.Up, k.Down, k.Select, k.Checks, k.Open, next}
	case inboxPane:
		sel, open := k.Select, k.Open
		sel.SetHelp(sel.Help().Key, "open")
		if h.markRead {
			sel.SetHelp(sel.Help().Key, "open & read")
		}
		return []key.Binding{k.Up, k.Down, sel, open}
	default:
		return h.cal.ShortHelp()
	}
}

func (h helpKeys) own() []key.Binding {
	here := h.k.Here
	if !h.here {
		here.SetEnabled(false)
	}
	zoom, back := h.k.Zoom, h.k.Back
	zoom.SetEnabled(zoom.Enabled() && h.wide)
	back.SetEnabled(back.Enabled() && h.zoom)
	return []key.Binding{h.k.Next, h.k.jump, zoom, back, h.k.Refresh, here}
}

// ShortHelp returns the bindings for the short help view.
func (h helpKeys) ShortHelp() []key.Binding {
	return append(h.paneKeys(), h.own()...)
}

// FullHelp returns the bindings for the full help view.
func (h helpKeys) FullHelp() [][]key.Binding {
	groups := [][]key.Binding{h.paneKeys()}
	switch h.pane {
	case reposPane:
		f := h.repos.feedKeys()
		groups = append(groups, []key.Binding{f.PageUp, f.PageDown, f.Home, f.End, h.k.PrevOwner})
	case workPane:
		prev := h.k.PrevOwner
		prev.SetHelp(prev.Help().Key, "previous list")
		groups = append(groups, []key.Binding{prev})
	case calendarPane:
		groups = h.cal.FullHelp()
	default:
	}
	return append(groups, append(h.own(), h.k.Prev))
}
