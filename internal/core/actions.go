package core

import (
	"errors"
	"time"
)

// RunStatus is where a workflow run, a job or a step is in its life, as
// GitHub reports it in lower case.
type RunStatus string

// Run statuses. RunCancelling is gh-tui's own: GitHub accepts a cancel at
// once but cancels later, and the run shows it in between.
const (
	RunQueued     RunStatus = "queued"
	RunInProgress RunStatus = "in_progress"
	RunCompleted  RunStatus = "completed"
	RunWaiting    RunStatus = "waiting"
	RunRequested  RunStatus = "requested"
	RunPending    RunStatus = "pending"
	RunCancelling RunStatus = "cancelling"
)

// Conclusion is how a completed run, job or step ended. It is empty until
// then.
type Conclusion string

// Conclusions.
const (
	ConclusionNone           Conclusion = ""
	ConclusionSuccess        Conclusion = "success"
	ConclusionFailure        Conclusion = "failure"
	ConclusionCancelled      Conclusion = "cancelled"
	ConclusionSkipped        Conclusion = "skipped"
	ConclusionTimedOut       Conclusion = "timed_out"
	ConclusionActionRequired Conclusion = "action_required"
	ConclusionNeutral        Conclusion = "neutral"
	ConclusionStartupFailure Conclusion = "startup_failure"
	ConclusionStale          Conclusion = "stale"
)

// Failed reports whether c is an ending that needs a look: a failure, a
// timeout or a run that couldn't start.
func (c Conclusion) Failed() bool {
	switch c {
	case ConclusionFailure, ConclusionTimedOut, ConclusionStartupFailure:
		return true
	default:
		return false
	}
}

// Run is one attempt of a GitHub Actions workflow run: the latest one, as
// the runs list reports it. A re-run starts the next attempt of the same
// run.
type Run struct {
	ID      int64
	Attempt int
	// Name is the name of the workflow, and DisplayTitle what the run is
	// about, such as the title of the commit or of the pull request.
	Name         string
	DisplayTitle string
	Number       int
	Event        string
	Branch       string
	HeadSHA      string
	Status       RunStatus
	Conclusion   Conclusion
	// Actor is the login of who triggered the run.
	Actor        string
	WorkflowID   int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	RunStartedAt time.Time
	URL          string
	// PullRequests are the numbers of the pull requests of the repository
	// whose head the run ran on. GitHub leaves out those from forks.
	PullRequests []int
}

// Done reports whether the run's latest attempt has completed.
func (r Run) Done() bool {
	return r.Status == RunCompleted
}

// RunFilter narrows a list of workflow runs. The zero value lists every
// run. Status takes a RunStatus or a Conclusion, such as "failure".
type RunFilter struct {
	Branch     string
	Event      string
	Status     string
	Actor      string
	WorkflowID int64
	HeadSHA    string
}

// Workflow is a workflow file of a repository.
type Workflow struct {
	ID    int64
	Name  string
	Path  string
	State string
	URL   string
}

// Job is one job of an attempt of a workflow run. Its ID is also the ID
// of the check run that reports it.
type Job struct {
	ID           int64
	RunID        int64
	Attempt      int
	Name         string
	WorkflowName string
	Status       RunStatus
	Conclusion   Conclusion
	StartedAt    time.Time
	CompletedAt  time.Time
	Steps        []Step
	URL          string
	RunnerName   string
	Labels       []string
}

// Done reports whether the job has completed, after which its log can be
// read and never changes.
func (j Job) Done() bool {
	return j.Status == RunCompleted
}

// Step is a step of a job. Number counts from 1 but may skip numbers, as
// GitHub numbers the post steps apart.
type Step struct {
	Number      int
	Name        string
	Status      RunStatus
	Conclusion  Conclusion
	StartedAt   time.Time
	CompletedAt time.Time
}

// Annotation is a note that a check run left on a line range of a file,
// such as a compiler error.
type Annotation struct {
	Path        string
	StartLine   int
	EndLine     int
	StartColumn int
	EndColumn   int
	Level       AnnotationLevel
	Title       string
	Message     string
	RawDetails  string
}

// AnnotationLevel is how severe an annotation is.
type AnnotationLevel string

// Annotation levels.
const (
	AnnotationNotice  AnnotationLevel = "notice"
	AnnotationWarning AnnotationLevel = "warning"
	AnnotationFailure AnnotationLevel = "failure"
)

// StatusContext is a commit status, which services outside GitHub Actions
// report, such as a CI of their own. State is GitHub's in lower case:
// error, expected, failure, pending or success.
type StatusContext struct {
	Context     string
	State       string
	Description string
	TargetURL   string
	CreatedAt   time.Time
	// Required reports whether a pull request needs it to pass before it
	// can merge. Checks read by commit leave it false.
	Required bool
}

// Check is a check run on a commit, with what the checks of a pull request
// show of it. A check run reports a job of GitHub Actions with the IDs of
// its job and run, and the name of its workflow; other apps, such as a
// coverage service, report under their own names. Summary and Text are
// markdown.
type Check struct {
	ID          int64
	Name        string
	Status      RunStatus
	Conclusion  Conclusion
	DetailsURL  string
	JobID       int64
	RunID       int64
	Workflow    string
	Title       string
	Summary     string
	Text        string
	Annotations int
	StartedAt   time.Time
	CompletedAt time.Time
	// Required reports whether the pull request needs the check to pass
	// before it can merge. Checks read by commit leave it false.
	Required bool
}

// Checks is what CI reported on a commit: its check runs and its commit
// statuses, and their summary.
type Checks struct {
	SHA      string
	State    ChecksState
	Runs     []Check
	Statuses []StatusContext
	// Total counts the checks and statuses, of which only the first
	// hundred are read, in which case Truncated is set.
	Total     int
	Truncated bool
}

// ErrLogPending is returned for the log of a job that hasn't completed:
// GitHub publishes a job's log only when the job ends.
var ErrLogPending = errors.New("log not available until the job completes")

// ErrLogExpired is returned for the log of a job that GitHub no longer
// keeps, once the repository's retention period has passed.
var ErrLogExpired = errors.New("log expired")

// RefusedError reports that GitHub refused an action, such as re-running
// a run that is too old, with GitHub's reason. Action says what was
// refused, such as "run can't be re-run". It unwraps to the error of the
// response.
type RefusedError struct {
	Action string
	Reason string
	Err    error
}

func (e *RefusedError) Error() string {
	if e.Reason == "" {
		return e.Action
	}
	return e.Action + ": " + e.Reason
}

// Unwrap returns the error of the response.
func (e *RefusedError) Unwrap() error {
	return e.Err
}
