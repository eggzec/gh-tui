package config

import (
	"fmt"
	"slices"
)

// Global action names, used as keys of [Config.Keys].
const (
	ActionQuit    = "quit"
	ActionHelp    = "help"
	ActionRefresh = "refresh"
	ActionSearch  = "search"
	ActionNextTab = "next_tab"
	ActionPrevTab = "prev_tab"
	ActionOpen    = "open_in_browser"
)

// Actions of the sections. A key may serve different actions in different
// sections, such as "m" for merge in pull requests and mark read in
// notifications.
const (
	ActionSelect      = "select"
	ActionBack        = "back"
	ActionFilter      = "filter"
	ActionMerge       = "merge"
	ActionClose       = "close"
	ActionReopen      = "reopen"
	ActionToggleDraft = "toggle_draft"
	ActionMarkRead    = "mark_read"
	ActionMarkDone    = "mark_done"
	ActionMarkAllRead = "mark_all_read"
	ActionStar        = "star"
)

func defaultKeys() map[string][]string {
	return map[string][]string{
		ActionQuit:    {"q", "ctrl+c"},
		ActionHelp:    {"?"},
		ActionRefresh: {"r", "ctrl+r"},
		ActionSearch:  {"/"},
		ActionNextTab: {"tab", "]"},
		ActionPrevTab: {"shift+tab", "["},
		ActionOpen:    {"o"},

		ActionSelect:      {"enter"},
		ActionBack:        {"esc"},
		ActionFilter:      {"f"},
		ActionMerge:       {"m"},
		ActionClose:       {"x"},
		ActionReopen:      {"X"},
		ActionToggleDraft: {"D"},
		ActionMarkRead:    {"m"},
		ActionMarkDone:    {"d"},
		ActionMarkAllRead: {"M"},
		ActionStar:        {"s"},
	}
}

// validateKeys rejects unknown actions so that a typo in the config file
// doesn't silently leave the default binding in place.
func validateKeys(action string, keys []string) error {
	if _, ok := defaultKeys()[action]; !ok {
		return fmt.Errorf("keys.%s: unknown action", action)
	}
	if len(keys) == 0 {
		return fmt.Errorf("keys.%s: needs at least one key", action)
	}
	if slices.Contains(keys, "") {
		return fmt.Errorf("keys.%s: empty key", action)
	}
	return nil
}
