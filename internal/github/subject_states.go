package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// subjectSlots is how many subjects one query reads. Its text never changes,
// so the schema check holds it and the budget learns its cost.
const subjectSlots = 25

// SubjectSlots is how many subjects one request of SubjectStates reads, so
// the size of the batches that callers send it.
const SubjectSlots = subjectSlots

// subjectStatesQuery reads the state of subjectSlots subjects.
var subjectStatesQuery = subjectSlotsQuery(subjectSlots)

// subjectSlotsQuery builds the query that reads the state of n subjects. A
// slot that isn't used is skipped by its w variable. The number of a
// discussion never goes to issueOrPullRequest, nor that of a pull request or
// issue to discussion: the p and d variables say which one a slot asks. The
// state is aliased, since a selection of both types of a union under one
// name refuses two fields of different types.
func subjectSlotsQuery(n int) string {
	var vars, slots strings.Builder
	for i := range n {
		s := strconv.Itoa(i)
		if i > 0 {
			vars.WriteString(", ")
		}
		fmt.Fprintf(&vars, "$o%[1]s: String!, $n%[1]s: String!, $i%[1]s: Int!, $w%[1]s: Boolean!, $p%[1]s: Boolean!, $d%[1]s: Boolean!", s)
		fmt.Fprintf(&slots, "\n  s%[1]s: repository(owner: $o%[1]s, name: $n%[1]s) @include(if: $w%[1]s) {"+
			"\n    issueOrPullRequest(number: $i%[1]s) @include(if: $p%[1]s) { __typename ... on Issue { issueState: state stateReason } ... on PullRequest { pullState: state isDraft } }"+
			"\n    discussion(number: $i%[1]s) @include(if: $d%[1]s) { closed isAnswered }"+
			"\n  }", s)
	}
	return "query SubjectStates(" + vars.String() + ") {\n  " + rateLimitField + slots.String() + "\n}"
}

// subjectSlot is what is read of one subject.
type subjectSlot struct {
	Item *struct {
		Typename    string `json:"__typename"`
		IssueState  string `json:"issueState"`
		PullState   string `json:"pullState"`
		StateReason string `json:"stateReason"`
		IsDraft     bool   `json:"isDraft"`
	} `json:"issueOrPullRequest"`
	Discussion *struct {
		Closed     bool `json:"closed"`
		IsAnswered bool `json:"isAnswered"`
	} `json:"discussion"`
}

// state returns what the slot read, or false if it read nothing.
func (s subjectSlot) state() (core.SubjectState, bool) {
	switch {
	case s.Item != nil && s.Item.Typename == "PullRequest":
		return core.SubjectState{State: refStates[s.Item.PullState], Draft: s.Item.IsDraft}, true
	case s.Item != nil && s.Item.Typename == "Issue":
		return core.SubjectState{State: refStates[s.Item.IssueState], Reason: refReasons[s.Item.StateReason]}, true
	case s.Discussion != nil:
		st := core.SubjectState{State: core.StateOpen, Answered: s.Discussion.IsAnswered}
		if s.Discussion.Closed {
			st.State = core.StateClosed
		}
		return st, true
	}
	return core.SubjectState{}, false
}

// SubjectStates reads the state of the subjects, subjectSlots at a time, one
// request after another. A subject that is a pull request, an issue or a
// discussion with a number is read; the others, such as a discussion that
// GitHub gave no number, are left out. A subject that can't be read, since it
// is gone or the token has no access, is absent from the result. It fails
// only on an error that names no subject, such as a rate limit.
func (c *Client) SubjectStates(ctx context.Context, subjects []core.SubjectRef) (map[core.SubjectRef]core.SubjectState, error) {
	out := make(map[core.SubjectRef]core.SubjectState, len(subjects))
	seen := make(map[core.SubjectRef]bool, len(subjects))
	var todo []core.SubjectRef
	for _, s := range subjects {
		if seen[s] || !subjectReadable(s) {
			continue
		}
		seen[s] = true
		todo = append(todo, s)
	}
	for batch := range subjectBatches(todo) {
		if err := c.readSubjectSlots(ctx, batch, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// subjectReadable reports whether s is one that the query can read.
func subjectReadable(s core.SubjectRef) bool {
	if s.Number <= 0 || s.Repo.Owner == "" || s.Repo.Name == "" {
		return false
	}
	return s.Type == core.SubjectPullRequest || s.Type == core.SubjectIssue || s.Type == core.SubjectDiscussion
}

// subjectBatches splits subjects into runs of at most subjectSlots.
func subjectBatches(subjects []core.SubjectRef) func(yield func([]core.SubjectRef) bool) {
	return func(yield func([]core.SubjectRef) bool) {
		for len(subjects) > 0 {
			n := min(subjectSlots, len(subjects))
			if !yield(subjects[:n]) {
				return
			}
			subjects = subjects[n:]
		}
	}
}

// readSubjectSlots reads up to subjectSlots subjects in one query into out.
func (c *Client) readSubjectSlots(ctx context.Context, subjects []core.SubjectRef, out map[core.SubjectRef]core.SubjectState) error {
	vars := make(map[string]any, 6*subjectSlots)
	for i := range subjectSlots {
		s := strconv.Itoa(i)
		// An unused slot is skipped, but every variable is required.
		o, n, num, used, pull, disc := "-", "-", 1, false, false, false
		if i < len(subjects) {
			t := subjects[i]
			discussion := t.Type == core.SubjectDiscussion
			o, n, num, used, pull, disc = t.Repo.Owner, t.Repo.Name, t.Number, true, !discussion, discussion
		}
		vars["o"+s], vars["n"+s], vars["i"+s], vars["w"+s], vars["p"+s], vars["d"+s] = o, n, num, used, pull, disc
	}
	var data map[string]json.RawMessage
	err := c.Query(ctx, subjectStatesQuery, vars, &data)
	if err != nil && (len(data) == 0 || !subjectPartial(err)) {
		return fmt.Errorf("read subject states: %w", err)
	}
	for i, t := range subjects {
		raw := data["s"+strconv.Itoa(i)]
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var slot subjectSlot
		if err := json.Unmarshal(raw, &slot); err != nil {
			return fmt.Errorf("read subject states: decode %s#%d: %w", t.Repo, t.Number, err)
		}
		if st, ok := slot.state(); ok {
			out[t] = st
		}
	}
	return nil
}

// subjectPartial reports whether err only refuses some subjects: every error
// is FORBIDDEN or NOT_FOUND on the path of a slot, such as a subject that was
// deleted or is in a repository the token can't see. Such a slot is unknown,
// and the others are read.
func subjectPartial(err error) bool {
	gerr, ok := errors.AsType[*GraphQLError](err)
	if !ok || len(gerr.Errors) == 0 {
		return false
	}
	for _, item := range gerr.Errors {
		if item.Type != "FORBIDDEN" && item.Type != "NOT_FOUND" || !subjectSlotPath(item.Path) {
			return false
		}
	}
	return true
}

// subjectSlotPath reports whether path starts at a slot, such as "s7".
func subjectSlotPath(path []any) bool {
	if len(path) == 0 {
		return false
	}
	alias, _ := path[0].(string)
	n, err := strconv.Atoi(strings.TrimPrefix(alias, "s"))
	return err == nil && strings.HasPrefix(alias, "s") && n >= 0 && n < subjectSlots
}
