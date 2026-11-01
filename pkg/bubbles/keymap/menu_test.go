package keymap_test

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

type menuKeys struct {
	Open  key.Binding `menu:"item"`
	Plain key.Binding
	Edit  key.Binding `menu:"item"`
	other key.Binding `menu:"item"`
}

func TestMenuListsOnlyTaggedBindings(t *testing.T) {
	t.Parallel()
	k := menuKeys{
		Open:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open")),
		Plain: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "plain")),
		Edit:  key.NewBinding(key.WithHelp("", "edit")),
		other: key.NewBinding(key.WithKeys("x")),
	}
	_ = k.other
	for name, in := range map[string]any{"value": k, "pointer": &k} {
		got := keymap.Menu(in)
		if len(got) != 2 || got[0].Help().Desc != "open" || got[1].Help().Desc != "edit" {
			t.Errorf("%s: Menu = %v, want open then edit", name, got)
		}
	}
}

func TestMenuOfNonStructIsEmpty(t *testing.T) {
	t.Parallel()
	if got := keymap.Menu(3); len(got) != 0 {
		t.Errorf("Menu(3) = %v, want none", got)
	}
}
