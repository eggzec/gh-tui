package ui

import (
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// MultiField is a filter field that picks any number of items, one
// qualifier of its key each.
func MultiField(key, label string, items []filterform.Item) filterform.Field {
	return filterform.Field{
		Key: key, Label: label, Kind: filterform.Multi, Qualifier: key, Each: true,
		Options: items, Hint: "any",
	}
}

// WithValues returns items with each of vs among them, so that the form
// shows the values typed in the query.
func WithValues(items []filterform.Item, vs []string) []filterform.Item {
	for _, v := range vs {
		if slices.ContainsFunc(items, func(it filterform.Item) bool { return strings.EqualFold(it.Value, v) }) {
			continue
		}
		items = append(slices.Clip(items), filterform.Item{Label: v, Value: v})
	}
	return items
}
