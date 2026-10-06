package config

import (
	"fmt"
	"slices"
	"sync"

	"github.com/eggzec/gh-tui/internal/keyname"
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
	// ActionFindFile opens the file finder of the repository screen, which
	// finds a file by some letters of its path.
	ActionFindFile = "find_file"
	// ActionCommand opens the command line at the bottom of the screen,
	// where commands such as goto are typed.
	ActionCommand = "command"
)

// Actions of the sections. A key may serve different actions in different
// sections, such as "m" for merge in pull requests and mark read in
// notifications.
const (
	ActionSelect = "select"
	ActionBack   = "back"
	// ActionFilter opens the filter modal of the focused list where it
	// has one, such as the notifications or the repositories of the
	// dashboard, on its Filters tab, and ActionSort opens it on its Sort
	// tab, in a list that can be sorted. ActionClearFilter puts the
	// filters of a list back to its defaults.
	ActionFilter      = "filter"
	ActionSort        = "sort"
	ActionClearFilter = "clear_filter"
	ActionMerge       = "merge"
	ActionClose       = "close"
	ActionReopen      = "reopen"
	ActionToggleDraft = "toggle_draft"
	ActionMarkRead    = "mark_read"
	ActionMarkDone    = "mark_done"
	ActionMarkAllRead = "mark_all_read"
	// ActionStar will star the repository, or unstar it. It is reserved,
	// with its key, until starring is wired in the tui, and does nothing
	// yet.
	ActionStar    = "star"
	ActionComment = "comment"
	ActionLabel   = "label"
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
	// ActionOwner shows the page of the person or organization behind what
	// is selected: the author of a pull request or issue, the owner of a
	// repository or of what is in it, such as a file or a notification,
	// or an organization of the dashboard's Repositories pane.
	ActionOwner = "owner"
	// ActionNextFilter and ActionPrevFilter switch between the tabs of a
	// list, such as All, Failing, Running and Mine of the Actions modal,
	// the states of the pull requests and the issues, and the lists of
	// the page of a user or an organization.
	ActionNextFilter = "next_filter"
	ActionPrevFilter = "prev_filter"
	// ActionPaneLeft and ActionPaneRight move the focus to the pane on the
	// left or right, in a modal with panes such as Actions.
	ActionPaneLeft  = "pane_left"
	ActionPaneRight = "pane_right"
	// ActionZoom shows the focused pane of the dashboard, the repository
	// screen, or a modal such as Actions, alone, or all of them again.
	ActionZoom = "zoom"
	// Actions of the Actions modal: re-run the failed jobs of a run, all
	// of them, or the job under the cursor, and cancel a run.
	ActionRerunFailed = "rerun_failed"
	ActionRerun       = "rerun"
	ActionRerunJob    = "rerun_job"
	ActionCancelRun   = "cancel_run"
	// ActionAnnotations moves the focus between the annotations of a
	// failed job and its log.
	ActionAnnotations = "annotations"
	// ActionChecks shows the checks of a pull request, in its modal.
	ActionChecks = "checks"
)

// actions are the names of the actions: those that default.yaml gives
// keys.
var actions = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{}
	for action := range Default().Keys {
		out[action] = true
	}
	return out
})

// validateKeys rejects unknown actions and keys, so that a typo in the
// config file doesn't silently leave the default binding in place, or bind
// a key no press can match. No keys, [], unbinds the action.
func validateKeys(action string, keys []string) error {
	if !actions()[action] {
		return fmt.Errorf("keys.%s: unknown action", action)
	}
	if slices.Contains(keys, "") {
		return fmt.Errorf("keys.%s: empty key", action)
	}
	for _, k := range keys {
		if !keyname.Valid(k) {
			return fmt.Errorf("keys.%s: unknown key %q, want a name such as r, R, ctrl+r, shift+tab, enter or space", action, k)
		}
	}
	return nil
}
