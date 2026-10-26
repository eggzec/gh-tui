package modals

// Priority ranks a modal. The levels are defined here; which modal gets which
// level is the root's business.
type Priority int

// The levels. A higher level opens over a lower one, an equal one replaces
// the open one, and a lower one is refused. A negative level opens over any
// modal and is never replaced by, nor joined to, another.
const (
	// PriorityAnywhere is for the search and the notifications.
	PriorityAnywhere Priority = -10
	// PriorityList is for the file finder.
	PriorityList Priority = 10
	// PriorityContent is for a pull request, an issue, a release, the
	// history and the actions.
	PriorityContent Priority = 20
	// PriorityDetail is for a file preview, a job log and the text pager.
	PriorityDetail Priority = 25
	// PriorityShape is for the filter, sort and group modals.
	PriorityShape Priority = 30
	// PriorityForm is for the merge dialog.
	PriorityForm Priority = 40
	// PriorityConfirm is for a confirmation.
	PriorityConfirm Priority = 50
	// PriorityPrompt is for the token prompt.
	PriorityPrompt Priority = 60
)
