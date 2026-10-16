package pulls

import (
	"context"
	"strings"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
)

// labelsFake is the labelsAPI of a test. A method whose function is unset
// answers with nothing. It records each call as "method args", in order.
type labelsFake struct {
	labelsMu sync.Mutex
	// addLabels, removeLabel, addAssignees and removeAssignees back the four
	// methods.
	addLabels       func(names []string) ([]core.Label, error)
	removeLabel     func(name string) ([]core.Label, error)
	addAssignees    func(logins []string) (core.Issue, error)
	removeAssignees func(logins []string) (core.Issue, error)
	// sent is what was called.
	sent []string
}

func (f *labelsFake) record(method string, args ...string) {
	f.labelsMu.Lock()
	defer f.labelsMu.Unlock()
	f.sent = append(f.sent, method+" "+strings.Join(args, ","))
}

// labelCalls returns what was called so far.
func (f *labelsFake) labelCalls() []string {
	f.labelsMu.Lock()
	defer f.labelsMu.Unlock()
	return append([]string(nil), f.sent...)
}

func (f *labelsFake) AddIssueLabels(_ context.Context, _ core.RepoRef, _ int, names []string) ([]core.Label, error) {
	f.record("AddIssueLabels", names...)
	if f.addLabels == nil {
		return nil, nil
	}
	return f.addLabels(names)
}

func (f *labelsFake) RemoveIssueLabel(_ context.Context, _ core.RepoRef, _ int, name string) ([]core.Label, error) {
	f.record("RemoveIssueLabel", name)
	if f.removeLabel == nil {
		return nil, nil
	}
	return f.removeLabel(name)
}

func (f *labelsFake) AddIssueAssignees(_ context.Context, _ core.RepoRef, _ int, logins []string) (core.Issue, error) {
	f.record("AddIssueAssignees", logins...)
	if f.addAssignees == nil {
		return core.Issue{}, nil
	}
	return f.addAssignees(logins)
}

func (f *labelsFake) RemoveIssueAssignees(_ context.Context, _ core.RepoRef, _ int, logins []string) (core.Issue, error) {
	f.record("RemoveIssueAssignees", logins...)
	if f.removeAssignees == nil {
		return core.Issue{}, nil
	}
	return f.removeAssignees(logins)
}
