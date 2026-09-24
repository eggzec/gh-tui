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
	ActionOpen    = "open_in_browser"
	// ActionNextTab and ActionPrevTab cycle the focus through the panes of
	// the screen. They keep the names they had when the app had tabs.
	ActionNextTab = "next_tab"
	ActionPrevTab = "prev_tab"
	// ActionPane1 to ActionPane3 focus the pane with that number on the
	// repository screen: files, pull requests and issues.
	ActionPane1 = "pane_1"
	ActionPane2 = "pane_2"
	ActionPane3 = "pane_3"
	// ActionNotifications switches between the repository screen and the
	// notifications.
	ActionNotifications = "notifications"
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
	ActionComment     = "comment"
	ActionLabel       = "label"
	// Actions of the file tree.
	ActionExpand      = "expand"
	ActionCollapse    = "collapse"
	ActionExpandAll   = "expand_all"
	ActionCollapseAll = "collapse_all"
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

		ActionPane1:         {"1"},
		ActionPane2:         {"2"},
		ActionPane3:         {"3"},
		ActionNotifications: {"n"},

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
		ActionComment:     {"c"},
		ActionLabel:       {"l"},
		ActionExpand:      {"+"},
		ActionCollapse:    {"-"},
		ActionExpandAll:   {"*"},
		ActionCollapseAll: {"="},
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
