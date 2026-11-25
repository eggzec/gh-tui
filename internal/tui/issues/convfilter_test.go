package issues

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The filter key opens the checklist of the Conversation pane only while that
// pane has the focus, and applying it shows in the pane's title.
func TestConvFilter(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(12)), 30)
	press(t, h, "1")
	press(t, h, "f")
	if m.over != nil {
		t.Fatal("f opened the filter over another pane")
	}

	press(t, h, "2")
	if got := ansi.Strip(m.View()); !strings.Contains(got, "filter: all") {
		t.Errorf("the Conversation pane says nothing of its filter:\n%s", got)
	}
	press(t, h, "f")
	if m.over == nil {
		t.Fatal("f did not open the filter over the Conversation pane")
	}
	press(t, h, "esc")
	if m.over != nil {
		t.Fatal("esc left the filter open")
	}

	// Unticking the first kind and applying narrows the conversation.
	press(t, h, "f", "space", "enter")
	if m.over != nil {
		t.Fatal("enter left the filter open")
	}
	if got := m.conv.FilterName(); got == "" {
		t.Error("applying the filter left every kind on")
	}
	if got := ansi.Strip(m.View()); strings.Contains(got, "filter: all") {
		t.Errorf("the title still says all:\n%s", got)
	}
}
