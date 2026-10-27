package ui

import (
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// MeItem is the option of a person field for the signed-in user.
var MeItem = filterform.Item{Label: "@me", Value: "@me", Detail: "you"}

// LabelItems returns labels as the options of a filter field.
func LabelItems(labels []core.Label) []filterform.Item {
	items := make([]filterform.Item, len(labels))
	for i, l := range labels {
		items[i] = filterform.Item{Label: l.Name, Value: l.Name, Detail: l.Description}
	}
	return items
}

// PersonItems returns users as the options of a person field, by login,
// with their names beside. It leaves out viewer, the signed-in user, whom
// MeItem offers already; an empty viewer leaves out no one.
func PersonItems(users []core.User, viewer string) []filterform.Item {
	items := make([]filterform.Item, 0, len(users))
	for _, u := range users {
		if viewer != "" && strings.EqualFold(u.Login, viewer) {
			continue
		}
		items = append(items, filterform.Item{Label: u.Login, Value: u.Login, Detail: u.Name})
	}
	return items
}

// Without returns query without the tokens drop matches, such as the state
// that a list shows in its tabs, or its default sort. What is left is
// written as it was, so the same filters give the same query.
func Without(query string, drop func(filterform.Token) bool) string {
	toks := filterform.Tokenize(query)
	kept := make([]string, 0, len(toks))
	for _, t := range toks {
		if !drop(t) {
			kept = append(kept, t.Raw)
		}
	}
	return strings.Join(kept, " ")
}

// ChipSpec is what a list's title says of its filter, sort and grouping.
type ChipSpec struct {
	// Filter is the chips of the filter in force; while FilterOn is false,
	// the chips of the filter that was switched off.
	Filter   string
	FilterOn bool
	// Sort is the chip that words the sort, such as "sort updated"; empty
	// when the sort is part of Filter or has no words.
	Sort   string
	SortOn bool
	// Group is the chip of the grouping, built by Grouping.Chip or
	// GroupOffChip, or "".
	Group string
}

// QueryChips returns the chips of a list's title for s, so that every list
// words its filter, sort and grouping, and their off state, the same way.
// A filter that is off shows alone, dim, with "off" after it. The caller
// joins the chips with Icons.Separator.
func QueryChips(icons Icons, s ChipSpec) []string {
	sep := icons.Separator
	var out []string
	switch {
	case !s.FilterOn:
		out = append(out, OffChip(s.Filter, sep))
	case s.Filter != "":
		out = append(out, s.Filter)
	}
	switch {
	case s.SortOn && s.Sort != "":
		out = append(out, s.Sort)
	case !s.SortOn && s.Sort != "":
		out = append(out, OffChip(s.Sort, sep))
	case !s.SortOn && s.FilterOn:
		// A kept filter holds its sort, so only a filter in force shows
		// the sort off on its own.
		out = append(out, SortOffChip(sep))
	}
	if s.Group != "" {
		out = append(out, s.Group)
	}
	return out
}

// Chips returns the filters of query as short chips for a pane's title,
// apart by sep, such as "@me · bug" for author:@me label:bug: a label by
// its name, an author as @login, and the rest as written.
func Chips(query, sep string) string {
	toks := filterform.Tokenize(query)
	chips := make([]string, 0, len(toks))
	for _, t := range toks {
		switch strings.ToLower(t.Qualifier) {
		case "label":
			chips = append(chips, strings.Join(t.Values(), ","))
		case "author":
			chips = append(chips, "@"+strings.TrimPrefix(t.Value, "@"))
		default:
			chips = append(chips, t.Raw)
		}
	}
	return strings.Join(chips, sep)
}
