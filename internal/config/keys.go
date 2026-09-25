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
	// ActionPane1 to ActionPane5 focus the pane with that number on the
	// screen: files, pull requests and issues on the repository screen,
	// and pinned, repositories, work, contributions and notifications on
	// the dashboard.
	ActionPane1 = "pane_1"
	ActionPane2 = "pane_2"
	ActionPane3 = "pane_3"
	ActionPane4 = "pane_4"
	ActionPane5 = "pane_5"
	// ActionNotifications switches between the screen on view and the
	// notifications.
	ActionNotifications = "notifications"
	// ActionDashboard shows the dashboard, from any screen.
	ActionDashboard = "dashboard"
	// ActionHistory opens the History modal of the repository screen.
	ActionHistory = "history"
	// ActionResetBase shows the files at the head of the default branch
	// again, after ActionUseAsBase chose another base.
	ActionResetBase = "reset_base"
	// ActionActions opens the Actions modal of the repository screen.
	ActionActions = "actions"
)

// Actions of the sections. A key may serve different actions in different
// sections, such as "m" for merge in pull requests and mark read in
// notifications.
const (
	ActionSelect = "select"
	ActionBack   = "back"
	// ActionFilter opens the filter modal of the focused list where it
	// has one; the notifications filter with it, and the dashboard finds
	// a repository.
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
	// ActionUseAsBase shows the files at the branch or commit under the
	// cursor of the History modal.
	ActionUseAsBase = "use_as_base"
	// ActionNextOwner and ActionPrevOwner switch the repositories of the
	// dashboard between the viewer's own and those of each organization.
	ActionNextOwner = "next_owner"
	ActionPrevOwner = "prev_owner"
	// ActionCurrentRepo opens the repository of the current directory from
	// the dashboard.
	ActionCurrentRepo = "current_repo"
	// ActionGoToRepo shows the repository of the search result under the
	// cursor, where enter previews the result over the search.
	ActionGoToRepo = "go_to_repo"
	// ActionNextFilter and ActionPrevFilter switch between the tabs of a
	// list, such as All, Failing, Running and Mine of the Actions modal.
	ActionNextFilter = "next_filter"
	ActionPrevFilter = "prev_filter"
	// ActionPaneLeft and ActionPaneRight move the focus to the pane on the
	// left or right, in a modal with panes such as Actions.
	ActionPaneLeft  = "pane_left"
	ActionPaneRight = "pane_right"
	// ActionZoom shows the focused pane of a modal alone, or all of them
	// again.
	ActionZoom = "zoom"
	// Actions of the Actions modal: re-run the failed jobs of a run, all
	// of them, or the job under the cursor, and cancel a run.
	ActionRerunFailed = "rerun_failed"
	ActionRerun       = "rerun"
	ActionRerunJob    = "rerun_job"
	ActionCancelRun   = "cancel_run"
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
		ActionPane4:         {"4"},
		ActionPane5:         {"5"},
		ActionNotifications: {"n"},
		// 0 sits before the pane keys, as the dashboard comes before the
		// repository, and no section binds it.
		ActionDashboard: {"0"},
		ActionHistory:   {"B"},
		ActionResetBase: {"H"},
		ActionActions:   {"a"},

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
		ActionUseAsBase:   {"space"},
		ActionNextOwner:   {"]", "right"},
		ActionPrevOwner:   {"[", "left"},
		ActionCurrentRepo: {"."},
		ActionGoToRepo:    {"ctrl+o"},
		ActionNextFilter:  {"]"},
		ActionPrevFilter:  {"["},
		ActionPaneLeft:    {"h"},
		ActionPaneRight:   {"l"},
		ActionZoom:        {"z"},
		ActionRerunFailed: {"ctrl+r"},
		ActionRerun:       {"R"},
		ActionRerunJob:    {"J"},
		ActionCancelRun:   {"x"},
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
