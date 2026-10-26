package owner

import (
	"strings"
	"testing"
)

// The group modal names the repositories it groups, not the profile page.
func TestGroupModalTitle(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 120, 40)
	title := s.groupModal().Title()
	if !strings.HasSuffix(title, groupTitle) || strings.Contains(title, s.Title()) {
		t.Errorf("group modal title = %q, want it to end in %q", title, groupTitle)
	}
}
