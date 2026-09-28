package core

// What the operations of the app need of a token, by GitHub's docs. Reads
// need nothing: without repo, GitHub hides what is private rather than
// refusing it, with a 404 or by leaving it out of a list.
var (
	// NeedNotifications is reading notifications and marking them: only
	// a classic token may, with notifications or repo.
	NeedNotifications = Need{Scopes: []string{"notifications", "repo"}, Classic: true}
	// NeedRuns is re-running and cancelling workflow runs. GitHub's docs
	// ask for repo, even in a public repository, and not for workflow.
	NeedRuns = Need{Scopes: []string{"repo"}}
	// NeedWorkflow is merging a pull request that changes a workflow. The
	// files aren't known when merging, so it can only hint.
	NeedWorkflow = Need{Scopes: []string{"workflow"}}
)

// NeedWrite is what changing an issue, pull request, label, comment or
// star in a repository with caps c needs: public_repo is enough in a
// public one, and a private one needs repo. When c is unknown, a token
// with either may try, but repo is the one to ask for, since the
// repository may be private.
func NeedWrite(c RepoCaps) Need {
	switch {
	case !c.Known:
		return Need{Scopes: []string{"repo", "public_repo"}}
	case c.Private:
		return Need{Scopes: []string{"repo"}}
	default:
		return Need{Scopes: []string{"public_repo", "repo"}}
	}
}
