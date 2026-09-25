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
// with their names beside.
func PersonItems(users []core.User) []filterform.Item {
	items := make([]filterform.Item, len(users))
	for i, u := range users {
		items[i] = filterform.Item{Label: u.Login, Value: u.Login, Detail: u.Name}
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

// Chips returns the filters of query as short chips for a pane's title,
// such as "@me · bug" for author:@me label:bug: a label by its name, an
// author as @login, and the rest as written.
func Chips(query string) string {
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
	return strings.Join(chips, " · ")
}
