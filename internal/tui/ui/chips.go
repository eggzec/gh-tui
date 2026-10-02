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
