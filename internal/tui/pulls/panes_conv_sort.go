package pulls

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// convOrders are the orders of the conversation in the order the sort key steps
// through them, with what the toast says of each.
var convOrders = []struct {
	order conv.Order
	said  string
}{
	{conv.OrderOldest, "oldest first"},
	{conv.OrderNewest, "newest first"},
	{conv.OrderActivity, "recent activity first"},
}

// stepConvSort changes the conversation to the next order, which wraps around
// from the last to the first, and says which it is.
func (m *panesModal) stepConvSort() tea.Cmd {
	c := &m.conv
	if !c.started {
		return nil
	}
	cur := c.model.Order()
	next := 0
	for i, o := range convOrders {
		if o.order == cur {
			next = (i + 1) % len(convOrders)
		}
	}
	c.model.SetOrder(convOrders[next].order)
	m.refreshConv()
	return ui.Notify(toast.Info, convOrders[next].said)
}
