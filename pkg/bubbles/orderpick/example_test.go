package orderpick_test

import (
	"fmt"

	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

func Example() {
	m := orderpick.New(
		orderpick.WithItems([]orderpick.Item{
			{Value: "updated", Title: "Updated", AscLabel: "oldest first", DescLabel: "newest first", Desc: true},
			{Value: "title", Title: "Title", AscLabel: "A to Z", DescLabel: "Z to A"},
		}),
		orderpick.WithSize(80, 12),
	)
	_ = m.Focus()

	// Forward messages to m.Update while it is open, and close it on
	// orderpick.ChosenMsg or orderpick.CancelMsg with m.ID(). The keys come
	// from orderpick.NewKeyMap, filled from the config.
	// ChosenMsg.Choices lists the values in order, each with its direction.
	fmt.Println(len(m.Chosen()))
	// Output:
	// 0
}
