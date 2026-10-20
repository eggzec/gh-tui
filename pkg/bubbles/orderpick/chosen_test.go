package orderpick

import (
	"slices"
	"testing"
)

func TestWithChosen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		opts   []Option
		chosen []Choice
		want   []any
		desc   []bool
	}{
		{"in order", nil, []Choice{{Value: "title"}, {Value: "created", Desc: true}}, []any{"title", "created"}, []bool{false, true}},
		{"unknown and repeated are left out", nil, []Choice{{Value: "nope"}, {Value: "title"}, {Value: "title", Desc: true}}, []any{"title"}, []bool{false}},
		{"single keeps the first", []Option{WithSingle(true)}, []Choice{{Value: "updated", Desc: true}, {Value: "title"}}, []any{"updated"}, []bool{true}},
		{"none", nil, nil, []any{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := open(t, append(tt.opts, WithChosen(tt.chosen...))...)
			if got := values(m); !slices.Equal(got, tt.want) {
				t.Fatalf("chosen = %v, want %v", got, tt.want)
			}
			for i, c := range m.Chosen() {
				if c.Desc != tt.desc[i] {
					t.Errorf("choice %d desc = %v, want %v", i, c.Desc, tt.desc[i])
				}
			}
			for _, it := range m.available() {
				if slices.Contains(tt.want, fields[it.Value.(int)].Value) {
					t.Errorf("%q is chosen and still in the left pane", it.Title)
				}
			}
		})
	}
}

func TestSingleTickReplacesTheChosen(t *testing.T) {
	t.Parallel()
	m := open(t, WithSingle(true), WithChosen(Choice{Value: "created", Desc: true}))
	m, _ = press(t, m, space)
	if got := values(m); len(got) != 1 || got[0] == "created" {
		t.Fatalf("chosen = %v, want one item that replaced created", got)
	}
}
