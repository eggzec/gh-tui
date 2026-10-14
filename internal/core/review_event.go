package core

// ReviewEvent is what a review says when it is submitted, as GitHub names it.
type ReviewEvent string

// The events of a submitted review.
const (
	ReviewEventComment        ReviewEvent = "COMMENT"
	ReviewEventApprove        ReviewEvent = "APPROVE"
	ReviewEventRequestChanges ReviewEvent = "REQUEST_CHANGES"
)

// Valid reports whether e is one of the events above.
func (e ReviewEvent) Valid() bool {
	switch e {
	case ReviewEventComment, ReviewEventApprove, ReviewEventRequestChanges:
		return true
	}
	return false
}
