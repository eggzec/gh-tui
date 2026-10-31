package feed

import (
	"slices"
	"strings"
	"testing"
	"time"
)

type levelItem struct {
	id, kind string
	at       time.Time
}

var levelFields = []Field[levelItem]{
	{Name: "kind", Key: func(i levelItem) string { return i.kind }, Label: strings.ToUpper},
	{Name: "id", Key: func(i levelItem) string { return i.id }},
	{Name: "at", Time: func(i levelItem) time.Time { return i.at }},
}

func levelAt(n int) time.Time { return time.Unix(int64(n), 0) }

func TestGroups(t *testing.T) {
	t.Parallel()
	got := Groups(levelFields, []Level{{"id", true}, {"nope", false}, {"at", false}, {"kind", false}})
	if len(got) != 2 {
		t.Fatalf("got %d groups, want 2", len(got))
	}
	if !got[0].Desc || got[0].Label != nil || got[0].Key(levelItem{id: "x"}) != "x" {
		t.Errorf("first group wrong: %+v", got[0])
	}
	if got[1].Desc || got[1].Label("a") != "A" {
		t.Errorf("second group wrong: %+v", got[1])
	}
}

func TestLess(t *testing.T) {
	t.Parallel()
	tie := func(i levelItem) time.Time { return i.at }
	tests := []struct {
		name   string
		levels []Level
		tie    func(levelItem) time.Time
		in     []levelItem
		want   string
	}{
		{"key ascending", []Level{{"id", false}}, nil,
			[]levelItem{{id: "b"}, {id: "a"}, {id: "c"}}, "abc"},
		{"key desc", []Level{{"id", true}}, nil,
			[]levelItem{{id: "b"}, {id: "a"}, {id: "c"}}, "cba"},
		{"empty last ascending", []Level{{"id", false}}, nil,
			[]levelItem{{id: ""}, {id: "b"}, {id: "a"}}, "ab."},
		{"empty last descending", []Level{{"id", true}}, nil,
			[]levelItem{{id: ""}, {id: "a"}, {id: "b"}}, "ba."},
		{"time newest first", []Level{{"at", false}}, nil,
			[]levelItem{{id: "1", at: levelAt(1)}, {id: "3", at: levelAt(3)}, {id: "2", at: levelAt(2)}}, "321"},
		{"time desc", []Level{{"at", true}}, nil,
			[]levelItem{{id: "3", at: levelAt(3)}, {id: "1", at: levelAt(1)}, {id: "2", at: levelAt(2)}}, "123"},
		{"zero time oldest", []Level{{"at", false}}, nil,
			[]levelItem{{id: "0"}, {id: "1", at: levelAt(1)}}, "10"},
		{"tie falls to next level", []Level{{"kind", false}, {"id", false}}, nil,
			[]levelItem{{id: "b", kind: "x"}, {id: "a", kind: "x"}, {id: "c", kind: "w"}}, "cab"},
		{"final tie-break newest first", []Level{{"kind", false}}, tie,
			[]levelItem{{id: "1", kind: "k", at: levelAt(1)}, {id: "2", kind: "k", at: levelAt(2)}}, "21"},
		{"nil tie keeps source order", []Level{{"kind", false}}, nil,
			[]levelItem{{id: "1", kind: "k", at: levelAt(1)}, {id: "2", kind: "k", at: levelAt(2)}}, "12"},
		{"unknown level skipped", []Level{{"nope", false}, {"id", false}}, nil,
			[]levelItem{{id: "b"}, {id: "a"}}, "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := slices.Clone(tt.in)
			slices.SortStableFunc(in, Less(levelFields, tt.levels, tt.tie))
			var sb strings.Builder
			for _, i := range in {
				if i.id == "" {
					sb.WriteString(".")
				} else {
					sb.WriteString(i.id)
				}
			}
			if sb.String() != tt.want {
				t.Errorf("got %q, want %q", sb.String(), tt.want)
			}
		})
	}
}
