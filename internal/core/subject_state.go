package core

// SubjectRef names the subject of a notification: a pull request, an issue or
// a discussion, by the repository and the number it has there. A notification
// says what its subject is about but not what state it is in, which a
// SubjectState does.
type SubjectRef struct {
	Repo   RepoRef
	Number int
	// Type is SubjectPullRequest, SubjectIssue or SubjectDiscussion.
	Type SubjectType
}

// SubjectState is the state of a subject, as read from GitHub.
type SubjectState struct {
	// State is open or closed, and merged for a pull request that was. A
	// discussion is open or closed.
	State State
	// Reason says why an issue was closed. It is empty for the others.
	Reason StateReason
	// Draft is set for a pull request that is a draft.
	Draft bool
	// Answered is set for a discussion that has an answer.
	Answered bool
}
