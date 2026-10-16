package pulls

import (
	"context"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// opCall is one call of a comment operation of the fake: the method and the
// arguments it was given, in order.
type opCall struct {
	method string
	args   []string
}

// commentOpsFake is the part of fakeAPI that answers the edits, replies and
// deletes of comments. It records each call, and answers what the test sets.
type commentOpsFake struct {
	opsMu    sync.Mutex
	opsCalls []opCall
	// opsErr fails the method it names, by its name.
	opsErr map[string]error
	// edited is what the edits answer, by method, and pullEdit what
	// UpdatePullRequestText does.
	edited   map[string]core.Edited
	pullEdit core.PullRequest
	pullAt   time.Time
	// reply is what ReplyToReviewThread answers, and replyErrs the errors it
	// returns first, one per call.
	reply     core.ThreadComment
	replyErrs []error
}

func (f *commentOpsFake) record(method string, args ...string) error {
	f.opsMu.Lock()
	defer f.opsMu.Unlock()
	f.opsCalls = append(f.opsCalls, opCall{method, args})
	return f.opsErr[method]
}

// opCalls returns the calls so far.
func (f *commentOpsFake) callsOf() []opCall {
	f.opsMu.Lock()
	defer f.opsMu.Unlock()
	return append([]opCall(nil), f.opsCalls...)
}

func (f *commentOpsFake) UpdatePullRequestText(_ context.Context, pullID, title, body string) (core.PullRequest, time.Time, error) {
	if err := f.record("UpdatePullRequestText", pullID, title, body); err != nil {
		return core.PullRequest{}, time.Time{}, err
	}
	pr := f.pullEdit
	pr.Title, pr.Body = title, body
	return pr, f.pullAt, nil
}

func (f *commentOpsFake) ReplyToReviewThread(_ context.Context, threadID, reviewID, body string) (core.ThreadComment, error) {
	err := f.record("ReplyToReviewThread", threadID, reviewID, body)
	f.opsMu.Lock()
	defer f.opsMu.Unlock()
	if err == nil && len(f.replyErrs) > 0 {
		err, f.replyErrs = f.replyErrs[0], f.replyErrs[1:]
	}
	if err != nil {
		return core.ThreadComment{}, err
	}
	return f.reply, nil
}

func (f *commentOpsFake) edit(method, id, body string) (core.Edited, error) {
	if err := f.record(method, id, body); err != nil {
		return core.Edited{}, err
	}
	f.opsMu.Lock()
	defer f.opsMu.Unlock()
	if e, ok := f.edited[method]; ok {
		return e, nil
	}
	return core.Edited{ID: id, Body: body}, nil
}

func (f *commentOpsFake) EditIssueComment(_ context.Context, id, body string) (core.Edited, error) {
	return f.edit("EditIssueComment", id, body)
}

func (f *commentOpsFake) EditReviewComment(_ context.Context, id, body string) (core.Edited, error) {
	return f.edit("EditReviewComment", id, body)
}

func (f *commentOpsFake) EditReview(_ context.Context, id, body string) (core.Edited, error) {
	return f.edit("EditReview", id, body)
}

func (f *commentOpsFake) DeleteIssueComment(_ context.Context, id string) error {
	return f.record("DeleteIssueComment", id)
}

func (f *commentOpsFake) DeleteReviewComment(_ context.Context, id string) error {
	return f.record("DeleteReviewComment", id)
}
