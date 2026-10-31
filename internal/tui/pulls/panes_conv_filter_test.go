package pulls

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
)

// convChoice returns the choice of group g in the filter of m.
func convChoice(t *testing.T, m *panesModal, g conv.Group) conv.Choice {
	t.Helper()
	for _, c := range m.conv.model.Choices() {
		if c.Group == g {
			return c
		}
	}
	t.Fatalf("no choice for %v", g)
	return conv.Choice{}
}

func TestConvFilterToggleApply(t *testing.T) {
	t.Parallel()
	h, m := convModal(t, convService())
	n, total := m.conv.model.Shown()
	if convChoice(t, m, conv.GroupReviews).Count == 0 {
		t.Fatal("the conversation has no reviews to hide")
	}

	press(t, h, "f")
	if m.top() == nil {
		t.Fatal("f didn't open the filter")
	}
	press(t, h, "down")
	press(t, h, "space")
	if !convChoice(t, m, conv.GroupReviews).On {
		t.Fatal("space applied the filter before enter")
	}
	press(t, h, "enter")

	if m.top() != nil {
		t.Error("enter left the filter open")
	}
	if convChoice(t, m, conv.GroupReviews).On {
		t.Error("Reviews is still shown")
	}
	after, _ := m.conv.model.Shown()
	if after >= n {
		t.Errorf("%d items shown after hiding Reviews, was %d", after, n)
	}
	want := strconv.Itoa(after) + " of " + strconv.Itoa(total) + " shown"
	if info := m.convInfo(); len(info) == 0 || !strings.HasPrefix(info[0], want) || strings.Contains(info[0], "filter: all") {
		t.Errorf("info is %q, want %q and a filter that isn't all", info, want)
	}
}

func TestConvFilterCancel(t *testing.T) {
	t.Parallel()
	h, m := convModal(t, convService())
	before := m.conv.model.Choices()

	press(t, h, "f")
	press(t, h, "down")
	press(t, h, "space")
	press(t, h, "esc")

	if m.top() != nil {
		t.Error("esc left the filter open")
	}
	after := m.conv.model.Choices()
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("esc changed %v to %v", before[i], after[i])
		}
	}
}

// The back key reaches the checklist, which closes without applying.
func TestConvFilterBack(t *testing.T) {
	t.Parallel()
	h, m := convModal(t, convService())
	before := m.conv.model.Choices()

	press(t, h, "f")
	press(t, h, "down")
	press(t, h, "space")
	press(t, h, "backspace")

	if m.top() != nil {
		t.Error("back left the filter open")
	}
	after := m.conv.model.Choices()
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("back changed %v to %v", before[i], after[i])
		}
	}
}

func TestConvFilterEnterAppliesTheTicks(t *testing.T) {
	t.Parallel()
	h, m := convModal(t, convService())

	press(t, h, "f")
	press(t, h, "down")
	press(t, h, "space")
	press(t, h, "enter")

	if m.top() != nil {
		t.Error("enter left the filter open")
	}
	if convChoice(t, m, conv.GroupReviews).On {
		t.Error("enter kept the kind that was unticked")
	}
	if !convChoice(t, m, conv.GroupComments).On {
		t.Error("enter unticked a kind that wasn't toggled")
	}
}

func TestConvFilterEnterWithNothingTickedChangesNothing(t *testing.T) {
	t.Parallel()
	h, m := convModal(t, convService())
	before := m.conv.model.Choices()

	press(t, h, "f")
	for range conv.NumGroups {
		press(t, h, "space")
		press(t, h, "down")
	}
	press(t, h, "enter")

	if m.top() != nil {
		t.Error("enter left the filter open")
	}
	after := m.conv.model.Choices()
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("enter changed %v to %v", before[i], after[i])
		}
	}
}

func TestConvFilterOverlay(t *testing.T) {
	t.Parallel()
	for _, w := range []int{80, 120} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()
			h := started(t, convService(), w, 30)
			press(t, h, "enter")
			press(t, h, "f")
			golden.RequireEqual(t, modalScreen(t, h))
		})
	}
}
