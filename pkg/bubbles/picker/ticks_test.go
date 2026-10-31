package picker

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func tickPicker(tb testing.TB, items []Item) Model {
	tb.Helper()
	return open(tb, nil, WithItems(items), WithMarks("[x]", "[ ]"), WithTicks(), WithModes(true))
}

func tickedTitles(m Model) []string {
	out := make([]string, 0, len(m.Ticked()))
	for _, it := range m.Ticked() {
		out = append(out, it.Title)
	}
	return out
}

func TestToggleTicksTheHighlightedRowInNormalMode(t *testing.T) {
	m := tickPicker(t, catalog)
	m, _ = press(t, m, space)
	if got := tickedTitles(m); !slices.Equal(got, []string{ghTUI.Title}) {
		t.Fatalf("ticked %q after space, want the first row", got)
	}
	m, _ = press(t, m, space)
	if got := m.Ticked(); len(got) != 0 {
		t.Errorf("ticked %v after a second space, want none", got)
	}
}

func TestTickedKeepsTheListedOrder(t *testing.T) {
	m := tickPicker(t, catalog)
	m, _ = press(t, m, down, down, space, up, up, space)
	if got := tickedTitles(m); !slices.Equal(got, []string{ghTUI.Title, crash.Title}) {
		t.Errorf("ticked %q, want the items in listed order", got)
	}
}

func TestTypingModeTypesASpace(t *testing.T) {
	m := tickPicker(t, catalog)
	m, _ = press(t, m, letter('/'))
	m = typeText(t, m, "a")
	m, _ = press(t, m, space)
	if got := m.Query().Text; got != "a " {
		t.Errorf("query %q, want the space typed", got)
	}
	if got := m.Ticked(); len(got) != 0 {
		t.Errorf("ticked %v, want none", got)
	}
}

func TestTicksPersistAcrossSearches(t *testing.T) {
	m := tickPicker(t, catalog)
	m, _ = press(t, m, space, letter('/'))
	m = typeText(t, m, "crash")
	if m.Len() == 0 || strings.Contains(strings.Join(titlesOf(m), "|"), ghTUI.Title) {
		t.Fatalf("results %q, want the first row hidden by the query", titlesOf(m))
	}
	if got := tickedTitles(m); !slices.Equal(got, []string{ghTUI.Title}) {
		t.Errorf("ticked %q, want the hidden row still ticked", got)
	}
}

func TestKnownItemsAreTickedBeforeAnySearchAnswers(t *testing.T) {
	search := func(ctx context.Context, _ Query) ([]Item, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	m := New(search, WithMarks("[x]", "[ ]"), WithTicks(), WithModes(true), WithKnown([]Item{ghTUI}))
	m.SetMarked([]any{ghTUI.Value})
	if got := tickedTitles(m); !slices.Equal(got, []string{ghTUI.Title}) {
		t.Errorf("ticked %q before any answer, want the known item", got)
	}
}

func TestEnterSendsExactlyTheTickedRows(t *testing.T) {
	m := tickPicker(t, catalog)
	m, _ = press(t, m, space, down, down, space, down)
	selected, _ := m.Selected()
	_, sent := press(t, m, enter)
	want := ChosenMsg{ID: m.ID(), Item: selected, Ticked: []Item{ghTUI, crash}}
	got, ok := sent[0].(ChosenMsg)
	if len(sent) != 1 || !ok || !slices.Equal(got.Ticked, want.Ticked) {
		t.Fatalf("sent %v, want the two ticked rows", sent)
	}
	if got.Item != selected {
		t.Errorf("item %v, want the selected row %v", got.Item, selected)
	}
}

func TestMarksWithoutTicksStayDisplayOnly(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithMarks("[x]", "[ ]"), WithModes(true))
	m.SetMarked([]any{ghTUI.Value})
	m, _ = press(t, m, down, space)
	if got := tickedTitles(m); !slices.Equal(got, []string{ghTUI.Title}) {
		t.Errorf("marked %q after space, want only the one set by the parent", got)
	}
}

func TestEnterWithNothingTickedSendsNoRows(t *testing.T) {
	m := tickPicker(t, catalog)
	_, sent := press(t, m, enter)
	if len(sent) != 1 {
		t.Fatalf("sent %v, want one message", sent)
	}
	if got, ok := sent[0].(ChosenMsg); !ok || len(got.Ticked) != 0 {
		t.Errorf("sent %v, want a ChosenMsg with no ticked rows", sent[0])
	}
}

func TestLockedRowKeepsItsTick(t *testing.T) {
	locked := ghTUI
	locked.Locked = true
	items := []Item{locked, dotfiles}
	m := tickPicker(t, items)
	m.SetMarked([]any{locked.Value})
	m, _ = press(t, m, space)
	if got := tickedTitles(m); !slices.Equal(got, []string{locked.Title}) {
		t.Errorf("ticked %q after space on a locked ticked row, want it kept", got)
	}
	m2 := tickPicker(t, items)
	m2, _ = press(t, m2, space)
	if got := m2.Ticked(); len(got) != 0 {
		t.Errorf("ticked %v after space on a locked unticked row, want none", got)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, DefaultStyles(true).LockGlyph) {
		t.Errorf("view lacks the lock marker:\n%s", v)
	}
}

func TestToggleDoesNothingWithoutMarks(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true))
	m, sent := press(t, m, space)
	if len(sent) != 0 || len(m.Ticked()) != 0 || !reflect.DeepEqual(titlesOf(m), titlesOf(open(t, nil, WithItems(catalog), WithModes(true)))) {
		t.Errorf("space changed a picker without marks: %v", sent)
	}
}
