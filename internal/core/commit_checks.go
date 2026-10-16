package core

// CheckApp is the GitHub App that owns a check suite, and so reports its
// check runs.
type CheckApp struct {
	Name string
	Slug string
	// LogoURL is the App's logo, empty if it has none.
	LogoURL string
}

// CommitCheck is a check run of a commit, with what the panes of checks
// need beyond a Check: the App that reported it, the workflow run of a job
// of GitHub Actions, and its annotations when they were asked for.
type CommitCheck struct {
	Check
	// SuiteID is the check suite the run belongs to.
	SuiteID int64
	// App is the App of the suite, nil for a run of none (or when the server
	// doesn't say).
	App *CheckApp
	// RunNumber, Event and RunURL describe the workflow run of a job of
	// GitHub Actions, and are zero for any other check run.
	RunNumber int
	Event     string
	RunURL    string
	// Notes are the annotations read for the run, of the Annotations it
	// has: fewer when a page of them was all that was read.
	Notes []Annotation
}

// InActions reports whether c is a job of GitHub Actions.
func (c CommitCheck) InActions() bool {
	return c.RunID != 0
}

// CommitChecks is everything CI reported on one commit, of any kind: the
// check runs of GitHub Actions and of other Apps, and the commit statuses.
type CommitChecks struct {
	SHA   string
	State ChecksState
	Runs  []CommitCheck
	// Statuses are the commit statuses.
	Statuses []StatusContext
	// Total counts the runs and statuses GitHub lists, Truncated is set when
	// fewer were read.
	Total     int
	Truncated bool
	// Notes reports whether the annotations of the runs were asked for.
	Notes bool
}

// Pending reports whether any check run or status of c hasn't finished.
func (c CommitChecks) Pending() bool {
	for i := range c.Runs {
		if c.Runs[i].Outcome() == CheckPending {
			return true
		}
	}
	for i := range c.Statuses {
		if c.Statuses[i].Outcome() == CheckPending {
			return true
		}
	}
	return false
}
