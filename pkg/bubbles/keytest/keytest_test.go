package keytest

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
)

// recorder is a testing.TB that keeps the errors it gets.
type recorder struct {
	testing.TB
	errs []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

type inner struct {
	Next key.Binding
}

// keys is a key map with a nested one, an unexported binding that help
// may leave out, and a way to leave out or repeat one of its own.
type keys struct {
	Up, Down key.Binding
	Nested   inner
	hidden   key.Binding
	// full is what FullHelp returns.
	full func(k keys) [][]key.Binding
}

func (k keys) ShortHelp() []key.Binding  { return []key.Binding{k.Up} }
func (k keys) FullHelp() [][]key.Binding { return k.full(k) }

func newKeys(full func(k keys) [][]key.Binding) keys {
	return keys{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑", "up")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓", "down")),
		Nested: inner{Next: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next"))},
		hidden: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "hidden")),
		full:   full,
	}
}

func TestComplete(t *testing.T) {
	tests := []struct {
		name string
		full func(k keys) [][]key.Binding
		want []string
	}{
		{name: "complete", full: func(k keys) [][]key.Binding {
			return [][]key.Binding{{k.Up, k.Down}, {k.Nested.Next}}
		}},
		{name: "disabled still counts", full: func(k keys) [][]key.Binding {
			k.Down.SetEnabled(false)
			return [][]key.Binding{{k.Up, k.Down, k.Nested.Next}}
		}},
		{name: "missing nested", full: func(k keys) [][]key.Binding {
			return [][]key.Binding{{k.Up, k.Down}}
		}, want: []string{"Nested.Next 0 times"}},
		{name: "twice", full: func(k keys) [][]key.Binding {
			return [][]key.Binding{{k.Up, k.Down, k.Down, k.Nested.Next}}
		}, want: []string{"Down 2 times"}},
		{name: "not a field", full: func(k keys) [][]key.Binding {
			return [][]key.Binding{{k.Up, k.Down, k.Nested.Next, k.hidden}}
		}, want: []string{"that are no field"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{TB: t}
			Complete(r, newKeys(tt.full))
			if len(r.errs) != len(tt.want) {
				t.Fatalf("errors %q, want %d", r.errs, len(tt.want))
			}
			for i, w := range tt.want {
				if !strings.Contains(r.errs[i], w) {
					t.Errorf("error %q, want it to say %q", r.errs[i], w)
				}
			}
		})
	}
}

func TestNoConflicts(t *testing.T) {
	r := &recorder{TB: t}
	NoConflicts(r, newKeys(func(k keys) [][]key.Binding {
		return [][]key.Binding{{k.Up, k.Down, k.Nested.Next}}
	}))
	if len(r.errs) != 0 {
		t.Errorf("distinct keys: errors %q", r.errs)
	}
	r = &recorder{TB: t}
	NoConflicts(r, newKeys(func(k keys) [][]key.Binding {
		k.Down.SetKeys("down", "k")
		return [][]key.Binding{{k.Up, k.Down, k.Nested.Next}}
	}))
	if len(r.errs) != 1 || !strings.Contains(r.errs[0], `"up" (up k) and "down" (down k) both hold k`) {
		t.Errorf("shared k: errors %q", r.errs)
	}
}

type tagged struct {
	Up     key.Binding `keymap:"up" help:"up"`
	Down   key.Binding `keymap:"down" help:"next"`
	Bare   key.Binding
	Nested struct {
		Left key.Binding
	}
}

func TestTagged(t *testing.T) {
	r := &recorder{TB: t}
	Tagged(r, tagged{})
	want := []string{"Bare has no keymap tag", "Nested.Left has no keymap tag"}
	if strings.Join(r.errs, "|") != strings.Join(want, "|") {
		t.Errorf("errors = %q, want %q", r.errs, want)
	}
}

func TestHelpTags(t *testing.T) {
	km := tagged{
		Up:   key.NewBinding(key.WithHelp("↑", "up")),
		Down: key.NewBinding(key.WithHelp("↓", "down")),
	}
	r := &recorder{TB: t}
	HelpTags(r, km)
	if len(r.errs) != 1 || !strings.HasPrefix(r.errs[0], "Down: help tag is \"next\"") {
		t.Errorf("errors = %q, want one about Down", r.errs)
	}
}

func TestTable(t *testing.T) {
	look := Table(map[string][]string{"down": {"j", "down"}})
	if got := look.Of("down"); len(got) != 2 || got[0] != "j" {
		t.Errorf("down = %v, want [j down]", got)
	}
	if got := look.Of("up"); got != nil {
		t.Errorf("up = %v, want none", got)
	}
}
