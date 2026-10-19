package orderpick

import (
	"slices"
	"testing"
)

func TestTickMovesToTheRightAndTheCursorStaysOnTheNextItem(t *testing.T) {
	m := open(t)
	m, _ = press(t, m, letter('j'), space)
	if got, want := values(m), []any{"updated"}; !equalValues(got, want) {
		t.Errorf("chosen %v, want %v", got, want)
	}
	if got := selected(m); got != "title" {
		t.Errorf("cursor on %v, want title", got)
	}
	if got := m.left.Len(); got != 3 {
		t.Errorf("left pane lists %d items, want 3", got)
	}
	// The last item has nothing after it, so the cursor goes up.
	m, _ = press(t, m, letter('G'), space)
	if got := selected(m); got != "title" {
		t.Errorf("cursor on %v after the last tick, want title", got)
	}
	if got, want := values(m), []any{"updated", "comments"}; !equalValues(got, want) {
		t.Errorf("chosen %v, want %v", got, want)
	}
}

func TestTickInAFilteredList(t *testing.T) {
	m := open(t)
	m, _ = press(t, m, letter('/'), letter('t'), letter('i'), letter('t'), enter)
	if got := m.left.Len(); got != 1 {
		t.Fatalf("filter lists %d items, want 1", got)
	}
	m, _ = press(t, m, space)
	if got, want := values(m), []any{"title"}; !equalValues(got, want) {
		t.Errorf("chosen %v, want %v", got, want)
	}
	if got := m.left.Query().Text; got != "tit" {
		t.Errorf("query %q after the tick, want it kept", got)
	}
	if got := m.left.Len(); got != 0 {
		t.Errorf("left pane lists %d items for the query, want 0", got)
	}
}

func TestDirectionStartsNatural(t *testing.T) {
	m := open(t)
	m, _ = press(t, m, space, letter('j'), letter('j'), space)
	if got := m.Chosen(); !slices.Equal(got, []Choice{{Value: "created", Desc: true}, {Value: "comments", Desc: true}}) {
		t.Errorf("chosen %v", got)
	}
}

func TestMoveStopsAtTheEnds(t *testing.T) {
	m := open(t)
	m, _ = press(t, m, space, space, space, tab)
	if got, want := values(m), []any{"created", "updated", "title"}; !equalValues(got, want) {
		t.Fatalf("chosen %v, want %v", got, want)
	}
	// The cursor is on the last item: J has nowhere to go.
	m, _ = press(t, m, letter('J'))
	if got, want := values(m), []any{"created", "updated", "title"}; !equalValues(got, want) {
		t.Errorf("chosen %v after J at the end, want %v", got, want)
	}
	m, _ = press(t, m, letter('K'), letter('K'), letter('K'))
	if got, want := values(m), []any{"title", "created", "updated"}; !equalValues(got, want) {
		t.Errorf("chosen %v after K, want %v", got, want)
	}
	if m.cursor != 0 {
		t.Errorf("cursor %d, want the item to carry it to 0", m.cursor)
	}
	m, _ = press(t, m, letter('J'))
	if got, want := values(m), []any{"created", "title", "updated"}; !equalValues(got, want) {
		t.Errorf("chosen %v after J, want %v", got, want)
	}
}

func TestSpaceFlipsTheDirection(t *testing.T) {
	m := open(t)
	m, _ = press(t, m, space, tab, space)
	if got := m.Chosen(); !slices.Equal(got, []Choice{{Value: "created", Desc: false}}) {
		t.Errorf("chosen %v, want created flipped to ascending", got)
	}
	m, _ = press(t, m, space)
	if got := m.Chosen(); !slices.Equal(got, []Choice{{Value: "created", Desc: true}}) {
		t.Errorf("chosen %v, want created natural again", got)
	}
}

func TestRemoveReturnsTheItemToItsPlace(t *testing.T) {
	for _, k := range []any{letter('x'), del} {
		m := open(t)
		m, _ = press(t, m, letter('j'), space, space, tab)
		if got, want := values(m), []any{"updated", "title"}; !equalValues(got, want) {
			t.Fatalf("chosen %v, want %v", got, want)
		}
		// The cursor is on the item chosen last, so go up to the first.
		m, _ = press(t, m, letter('k'), k)
		if got, want := values(m), []any{"title"}; !equalValues(got, want) {
			t.Errorf("chosen %v after remove, want %v", got, want)
		}
		var titles []string
		for _, it := range m.available() {
			titles = append(titles, it.Title)
		}
		if want := []string{"Created", "Updated", "Comments"}; !slices.Equal(titles, want) {
			t.Errorf("left pane lists %v, want %v in their own order", titles, want)
		}
		if got := selected(m); got != "updated" {
			t.Errorf("left cursor on %v, want the removed item", got)
		}
	}
}

func TestSingleModeReplaces(t *testing.T) {
	m := open(t, WithSingle(true))
	m, _ = press(t, m, space, space)
	if got, want := values(m), []any{"updated"}; !equalValues(got, want) {
		t.Errorf("chosen %v, want %v", got, want)
	}
	titles := make([]string, 0, len(m.available()))
	for _, it := range m.available() {
		titles = append(titles, it.Title)
	}
	if want := []string{"Created", "Title", "Comments"}; !slices.Equal(titles, want) {
		t.Errorf("left pane lists %v, want %v", titles, want)
	}
}

func TestTabSwitchesPanes(t *testing.T) {
	m := open(t)
	if m.Pane() != PaneAvailable || !m.left.Focused() {
		t.Fatal("the left pane should start in focus")
	}
	m, _ = press(t, m, tab)
	if m.Pane() != PaneChosen || m.left.Focused() {
		t.Error("tab should move the focus to the right pane")
	}
	m, _ = press(t, m, tab)
	if m.Pane() != PaneAvailable || !m.left.Focused() {
		t.Error("tab should move the focus back")
	}
	// While the filter types, tab is no pane key.
	m, _ = press(t, m, letter('/'), tab)
	if m.Pane() != PaneAvailable {
		t.Error("tab switched panes while the search typed")
	}
}

func TestEnterSendsTheOrderAndDirections(t *testing.T) {
	m := open(t)
	m, _ = press(t, m, letter('j'), space, space, tab, space, letter('J'))
	for _, from := range []string{"right", "left"} {
		_, sent := press(t, m, enter)
		// Space flipped Title, the item under the cursor, to descending.
		want := ChosenMsg{ID: m.ID(), Choices: []Choice{{Value: "updated", Desc: true}, {Value: "title", Desc: true}}}
		if len(sent) != 1 {
			t.Fatalf("%s pane sent %v, want one ChosenMsg", from, sent)
		}
		got := sent[0].(ChosenMsg)
		if got.ID != want.ID || !slices.Equal(got.Choices, want.Choices) {
			t.Errorf("%s pane sent %+v, want %+v", from, got, want)
		}
		m, _ = press(t, m, tab)
	}
}

func TestEscClearsTheQueryThenCancels(t *testing.T) {
	m := open(t)
	m, _ = press(t, m, letter('/'), letter('t'), enter)
	m, sent := press(t, m, esc)
	if len(sent) != 0 || m.left.Query().Text != "" {
		t.Fatalf("esc sent %v with the query %q, want it cleared", sent, m.left.Query().Text)
	}
	m, sent = press(t, m, esc)
	if want := (CancelMsg{ID: m.ID()}); len(sent) != 1 || sent[0] != want {
		t.Errorf("second esc sent %v, want %v", sent, want)
	}
	m, sent = press(t, m, tab, esc)
	if len(sent) != 1 {
		t.Errorf("esc in the right pane sent %v, want a CancelMsg", sent)
	}
}

func TestBackIsLeftToTheParent(t *testing.T) {
	m := open(t)
	m, sent := press(t, m, bksp)
	if len(sent) != 0 {
		t.Errorf("backspace sent %v, want nothing", sent)
	}
	if msg := m.Cancel()(); msg != (CancelMsg{ID: m.ID()}) {
		t.Errorf("Cancel sent %v", msg)
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := open(t)
	m.Blur()
	m, _ = press(t, m, space)
	if len(m.Chosen()) != 0 {
		t.Error("a blurred picker ticked an item")
	}
}

func TestSizeScrollsTheCursorIntoView(t *testing.T) {
	m := open(t, WithSize(80, 5))
	m, _ = press(t, m, space, space, space, space, tab)
	if m.rows() != 1 || m.top != 3 {
		t.Errorf("%d rows from %d, want 1 from 3", m.rows(), m.top)
	}
	m, _ = press(t, m, letter('g'))
	if m.top != 0 {
		t.Errorf("top %d after g, want 0", m.top)
	}
}
