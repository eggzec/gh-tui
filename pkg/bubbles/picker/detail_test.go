package picker

import (
	"slices"
	"testing"

	"charm.land/lipgloss/v2"
)

var described = []Item{
	{Title: "bug", Detail: "Something isn't working", Value: "bug"},
	{Title: "docs", Detail: "Improvements to documentation", Value: "docs"},
	{Title: "cache", Value: "cache"},
}

func TestDetailMatchListsTitlesFirstThenDetails(t *testing.T) {
	t.Parallel()
	m := open(t, nil, WithItems(described), WithDetailMatch(true))
	m = typeText(t, m, "work")
	if got, want := titlesOf(m), []string{"bug"}; !slices.Equal(got, want) {
		t.Fatalf("detail match: got %v, want %v", got, want)
	}
	m = open(t, nil, WithItems(described))
	m = typeText(t, m, "work")
	if got := titlesOf(m); len(got) != 0 {
		t.Fatalf("without the option the detail is not searched, got %v", got)
	}
}

func TestDetailMatchIgnoresCase(t *testing.T) {
	t.Parallel()
	m := open(t, nil, WithItems(described), WithDetailMatch(true))
	m = typeText(t, m, "DOCUMENTATION")
	if got, want := titlesOf(m), []string{"docs"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTitleStyleReceivesTheItem(t *testing.T) {
	t.Parallel()
	var seen []any
	m := open(t, nil, WithItems(described), WithTitleStyle(func(it Item, base lipgloss.Style) lipgloss.Style {
		seen = append(seen, it.Value)
		return base.Underline(true)
	}))
	m.SetSize(40, 10)
	_ = m.View()
	for _, v := range []any{"bug", "docs", "cache"} {
		if !slices.Contains(seen, v) {
			t.Errorf("style was not asked for %v, asked for %v", v, seen)
		}
	}
}

func TestFollowingIsTheItemBelowTheSelection(t *testing.T) {
	t.Parallel()
	m := open(t, nil, WithItems(described))
	if it, ok := m.Following(); !ok || it.Value != "docs" {
		t.Fatalf("Following = %v, %v; want docs", it, ok)
	}
	m.Select("cache")
	if it, ok := m.Following(); ok {
		t.Fatalf("Following after the last item = %v, want none", it)
	}
}

func TestPrecedingIsTheItemAboveTheSelection(t *testing.T) {
	t.Parallel()
	m := open(t, nil, WithItems(described))
	if it, ok := m.Preceding(); ok {
		t.Fatalf("Preceding before the first item = %v, want none", it)
	}
	m.Select("docs")
	if it, ok := m.Preceding(); !ok || it.Value != "bug" {
		t.Fatalf("Preceding = %v, %v; want bug", it, ok)
	}
}
