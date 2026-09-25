package filterform_test

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

func Example() {
	// The section adapts its labels service to a Loader; here it returns a
	// fixed list. The form calls it when the user opens the Labels field.
	loadLabels := func(context.Context, string) ([]filterform.Item, error) {
		return []filterform.Item{{Label: "bug", Value: "bug"}, {Label: "good first issue", Value: "good first issue"}}, nil
	}

	spec := filterform.Spec{
		Fields: []filterform.Field{
			{
				Key: "state", Label: "State", Kind: filterform.Choice, Qualifier: "is",
				Options: []filterform.Item{{Label: "Open", Value: "open"}, {Label: "Closed", Value: "closed"}, {Label: "Merged", Value: "merged"}, {Label: "All"}},
				Default: filterform.TextValue("open"),
			},
			{
				Key: "author", Label: "Author", Kind: filterform.Person, Qualifier: "author",
				Options: []filterform.Item{{Label: "@me", Value: "@me"}}, Hint: "anyone",
			},
			{
				// A Choice with no qualifier writes its options whole.
				Key: "review", Label: "Review", Kind: filterform.Choice,
				Options: []filterform.Item{
					{Label: "Any"},
					{Label: "Requested from me", Value: "review-requested:@me"},
					{Label: "Approved", Value: "review:approved"},
				},
			},
			{Key: "labels", Label: "Labels", Kind: filterform.Multi, Qualifier: "label", Load: loadLabels},
			{Key: "drafts", Label: "Drafts", Kind: filterform.Toggle, Qualifier: "-is:draft", Hint: "hide drafts"},
			{Key: "base", Label: "Base", Kind: filterform.Text, Qualifier: "base", Hint: "any branch"},
		},
		Sort: &filterform.SortField{
			Options: []filterform.Item{{Label: "Updated", Value: "updated"}, {Label: "Created", Value: "created"}},
			Desc:    "↓ newest first", Asc: "↑ oldest first",
			Default: filterform.Sort{By: "updated", Desc: true},
		},
	}

	// Open the form on the list's current query. Words no field claims,
	// such as "crash", are kept.
	m := filterform.New(spec,
		filterform.WithQuery(`is:closed label:"good first issue" crash`),
		filterform.WithSize(60, 14),
	)
	_ = m.Focus()

	// Forward messages to m.Update while the form is open, and close it on
	// filterform.AppliedMsg or filterform.CancelMsg with m.ID(). Applied
	// carries the same values as the accessors.
	labels, _ := m.Value("labels")
	fmt.Println(labels.List())
	fmt.Println(m.Query())
	// Output:
	// [good first issue]
	// is:closed label:"good first issue" sort:updated-desc crash
}
