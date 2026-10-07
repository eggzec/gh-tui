package keymap_test

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

type inner struct {
	Left key.Binding `keymap:"left" help:"week before"`
}

type keys struct {
	Up     key.Binding `keymap:"up" help:"up"`
	Select key.Binding `keymap:"global.select" help:"open"`
	Gone   key.Binding `keymap:"gone" help:"not bound"`
	Plain  key.Binding
	Nested inner
	Other  inner `keymap:"cal"`
}

// from looks actions up in a table.
func from(table map[string][]string) keymap.Lookup {
	return func(action string) []string { return table[action] }
}

func TestFill(t *testing.T) {
	var km keys
	km.Plain = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "kept"))
	keymap.Fill(&km, from(map[string][]string{
		"up":            {"up", "k"},
		"global.select": {"enter"},
		"left":          {"ctrl+b", "h"},
		"cal.left":      {"a"},
	}))

	if got := km.Up.Keys(); !slices.Equal(got, []string{"up", "k"}) {
		t.Errorf("Up keys = %v", got)
	}
	if got := km.Up.Help(); got.Key != "↑/k" || got.Desc != "up" {
		t.Errorf("Up help = %+v", got)
	}
	if got := km.Select.Help(); got.Key != "↵" || got.Desc != "open" {
		t.Errorf("Select help = %+v, want the label of enter and its desc", got)
	}
	if got := km.Nested.Left.Help(); got.Key != "^b/h" || got.Desc != "week before" {
		t.Errorf("nested help = %+v, want the nested map filled too", got)
	}
	if got := km.Other.Left.Keys(); !slices.Equal(got, []string{"a"}) {
		t.Errorf("nested map with its own context has keys %v, want those of cal.left", got)
	}
	if !km.Plain.Enabled() || km.Plain.Help().Desc != "kept" {
		t.Errorf("an untagged binding changed: %+v", km.Plain.Help())
	}
}

func TestFillCopiesKeys(t *testing.T) {
	src := []string{"up", "k"}
	var km keys
	keymap.Fill(&km, from(map[string][]string{"up": src}))
	src[0] = "x"
	if got := km.Up.Keys(); !slices.Equal(got, []string{"up", "k"}) {
		t.Errorf("Up keys = %v after the source changed, want them unchanged", got)
	}
}

func TestFillUnbound(t *testing.T) {
	var km keys
	keymap.Fill(&km, from(nil))
	if km.Gone.Enabled() {
		t.Error("an action without keys is enabled")
	}
	if got := km.Gone.Help(); got.Key != "" || got.Desc != "not bound" {
		t.Errorf("help = %+v, want no key and the desc kept", got)
	}
	if len(km.Gone.Keys()) != 0 {
		t.Errorf("keys = %v", km.Gone.Keys())
	}
}

func TestFillPanics(t *testing.T) {
	type wrong struct {
		Name string `keymap:"name"`
	}
	tests := []struct {
		name string
		km   any
	}{
		{"not a pointer", keys{}},
		{"nil pointer", (*keys)(nil)},
		{"pointer to a non-struct", new(int)},
		{"tagged field that is no binding", &wrong{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("Fill did not panic")
				}
			}()
			keymap.Fill(tt.km, from(nil))
		})
	}
}

func TestNames(t *testing.T) {
	want := []string{"cal.left", "global.select", "gone", "left", "up"}
	if got := keymap.Names(keys{}); !slices.Equal(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
	if got := keymap.Names(&keys{}); !slices.Equal(got, want) {
		t.Errorf("Names of a pointer = %v, want %v", got, want)
	}
}

func TestNamesOfPager(t *testing.T) {
	want := []string{
		"bottom", "edit", "find", "global.dismiss", "global.quit", "half_page_down", "half_page_up",
		"left", "next_match", "option", "page_down", "page_up", "pager_option.cancel", "pager_option.chop",
		"pager_option.ignore_case", "pager_option.line_numbers", "pager_option.smart_case",
		"pager_option.squeeze", "prev_match", "quick_filter", "right", "search_prompt.cancel", "search_prompt.cancel_empty", "search_prompt.run", "top", "up", "down",
	}
	slices.Sort(want)
	if got := keymap.Names(pager.KeyMap{}); !slices.Equal(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
}

func TestLabel(t *testing.T) {
	tests := map[string]string{
		"enter":  "↵",
		"up":     "↑",
		"down":   "↓",
		"left":   "←",
		"right":  "→",
		"esc":    "esc",
		" ":      "space",
		"space":  "space",
		"ctrl+x": "^x",
		"pgup":   "pgup",
		"pgdown": "pgdn",
		"j":      "j",
	}
	for in, want := range tests {
		if got := keymap.Label(in); got != want {
			t.Errorf("Label(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLabels(t *testing.T) {
	tests := []struct {
		keys []string
		want string
	}{
		{[]string{"up", "k"}, "↑/k"},
		{[]string{"ctrl+f", "pgdown"}, "^f/pgdn"},
		{[]string{"-", "left", "h"}, "-/←/h"},
		{[]string{"enter"}, "↵"},
	}
	for _, tt := range tests {
		if got := keymap.Labels(tt.keys); got != tt.want {
			t.Errorf("Labels(%q) = %q, want %q", tt.keys, got, tt.want)
		}
	}
}

func TestNamesOfPickerNormalMode(t *testing.T) {
	got := keymap.Names(picker.KeyMap{})
	for _, want := range []string{"picker_normal.up", "picker_normal.half_page_down", "picker_normal.insert", "picker_normal.append", "up", "choose"} {
		if !slices.Contains(got, want) {
			t.Errorf("Names = %v, missing %q", got, want)
		}
	}
	for _, n := range got {
		if strings.HasPrefix(n, "normal_") {
			t.Errorf("Names has %q, want normal mode under picker_normal", n)
		}
	}
}
