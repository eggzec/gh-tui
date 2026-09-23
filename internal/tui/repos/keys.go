package repos

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// KeyMap holds the keys of the section. It implements help.KeyMap.
type KeyMap struct {
	Select  key.Binding
	Star    key.Binding
	Open    key.Binding
	Refresh key.Binding
	// Feed moves through the list.
	Feed feed.KeyMap
}

func newKeyMap(keys map[string][]string) KeyMap {
	fk := feed.DefaultKeyMap()
	// Refresh reloads failed pages too, so the list's retry hint names the
	// refresh keys. The section handles them before the list sees them.
	fk.Retry = ui.Binding(keys, config.ActionRefresh, "retry")
	return KeyMap{
		Select:  ui.Binding(keys, config.ActionSelect, "pull requests"),
		Star:    ui.Binding(keys, config.ActionStar, "star"),
		Open:    ui.Binding(keys, config.ActionOpen, "open"),
		Refresh: ui.Binding(keys, config.ActionRefresh, "refresh"),
		Feed:    fk,
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Feed.Up, k.Feed.Down, k.Select, k.Star, k.Open, k.Refresh}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Feed.Up, k.Feed.Down, k.Feed.PageUp, k.Feed.PageDown},
		{k.Feed.Home, k.Feed.End},
		{k.Select, k.Star, k.Open, k.Refresh},
	}
}
