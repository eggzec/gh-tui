package filterform

import (
	"slices"
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		in   string
		want []Token
	}{
		{"", nil},
		{"  fix \t crash ", []Token{{Raw: "fix", Value: "fix"}, {Raw: "crash", Value: "crash"}}},
		{"is:open", []Token{{Raw: "is:open", Qualifier: "is", Value: "open"}}},
		{`label:"good first issue"`, []Token{{Raw: `label:"good first issue"`, Qualifier: "label", Value: "good first issue"}}},
		{`"exact phrase" -label:wontfix`, []Token{
			{Raw: `"exact phrase"`, Value: "exact phrase"},
			{Raw: "-label:wontfix", Qualifier: "-label", Value: "wontfix"},
		}},
		{`label:"no end`, []Token{{Raw: `label:"no end`, Qualifier: "label", Value: "no end"}}},
		{"http://x a:b:c", []Token{
			{Raw: "http://x", Qualifier: "http", Value: "//x"},
			{Raw: "a:b:c", Qualifier: "a", Value: "b:c"},
		}},
		{`"a:b" :x`, []Token{{Raw: `"a:b"`, Value: "a:b"}, {Raw: ":x", Value: ":x"}}},
	}
	for _, tt := range tests {
		got := Tokenize(tt.in)
		for i := range got {
			got[i].rawValue = ""
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("Tokenize(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestTokenValues(t *testing.T) {
	tok := Tokenize(`label:bug,"good first issue",,docs`)[0]
	if got, want := tok.Values(), []string{"bug", "good first issue", "docs"}; !slices.Equal(got, want) {
		t.Errorf("Values = %q, want %q", got, want)
	}
}

// SetQuery then Query normalizes the query, and reading the result again
// gives the same fields.
func TestQueryRoundTrip(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", "sort:updated-desc"},
		{"defaults", prDefaults, prDefaults},
		{"reordered", "base:main label:bug is:open", "is:open label:bug base:main sort:updated-desc"},
		{"quoted label", `label:"good first issue"`, `label:"good first issue" sort:updated-desc`},
		{"list with quotes", `label:bug,"good first issue"`, `label:bug,"good first issue" sort:updated-desc`},
		{"labels merge", "label:bug label:docs,bug", "label:bug,docs sort:updated-desc"},
		{"quoted text", `base:"release 1.0"`, `base:"release 1.0" sort:updated-desc`},
		{"free text kept in order", "fix is:closed crash repo:cli/cli -label:wontfix",
			"is:closed sort:updated-desc fix crash repo:cli/cli -label:wontfix"},
		{"quoted free text", `"exact phrase" is:merged`, `is:merged sort:updated-desc "exact phrase"`},
		{"unknown choice value is free", "is:issue", "sort:updated-desc is:issue"},
		{"qualifier case", "IS:Closed Label:Bug", "is:closed label:Bug sort:updated-desc"},
		{"whole-token choice", "review:approved", "review:approved sort:updated-desc"},
		{"toggle", "-is:draft", "-is:draft sort:updated-desc"},
		{"last single value wins", "base:main base:dev author:a author:b", "author:b base:dev sort:updated-desc"},
		{"sort asc", "sort:comments-asc", "sort:comments-asc"},
		{"sort without direction", "sort:created", "sort:created-desc"},
		{"unknown sort is free", "sort:reactions", "sort:updated-desc sort:reactions"},
		{"empty value is free", "label: base:", "sort:updated-desc label: base:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(prSpec(nil))
			m.SetQuery(tt.in)
			got := m.Query()
			if got != tt.want {
				t.Fatalf("Query = %q, want %q", got, tt.want)
			}
			values, sort, free := m.Values(), m.Sort(), m.FreeText()
			m.SetQuery(got)
			if m.Query() != got || !equalValues(m.Values(), values) || m.Sort() != sort || !slices.Equal(m.FreeText(), free) {
				t.Errorf("reading %q again changed the fields: %q", got, m.Query())
			}
		})
	}
}

// Fields make a query that reads back into the same fields.
func TestFieldsRoundTrip(t *testing.T) {
	spec := prSpec(nil)
	tests := []state{
		defaults(&spec),
		{values: []Value{TextValue("merged"), TextValue("octo cat"), TextValue(""), ListValue("good first issue", "a,b", "x"), BoolValue(true), TextValue("release 1.0")}, sort: Sort{By: "comments"}},
		{values: []Value{{}, {}, TextValue("review:changes_requested"), {}, {}, {}}, sort: Sort{By: "created", Desc: true}, free: []string{"fix", `"two words"`}},
	}
	for _, st := range tests {
		q := st.query(&spec)
		got := parse(&spec, q)
		if !slices.EqualFunc(got.values, st.values, Value.Equal) || got.sort != st.sort || !slices.Equal(got.free, st.free) {
			t.Errorf("parse(%q) = %+v, want %+v", q, got, st)
		}
	}
}

func TestEach(t *testing.T) {
	spec := Spec{Fields: []Field{{Key: "l", Kind: Multi, Qualifier: "label", Each: true}}}
	m := New(spec, WithQuery(`label:bug,docs label:"good first issue"`))
	if got, want := m.Query(), `label:bug label:docs label:"good first issue"`; got != want {
		t.Errorf("Query = %q, want %q", got, want)
	}
}

func TestFormatAndParse(t *testing.T) {
	// A number of comments, written as comments:>N.
	spec := Spec{Fields: []Field{{
		Key: "comments", Label: "Comments", Kind: Text,
		Format: func(v Value) string {
			if v.Text() == "" {
				return ""
			}
			return "comments:>" + v.Text()
		},
		Parse: func(tok Token, v Value) (Value, bool) {
			n, ok := strings.CutPrefix(tok.Value, ">")
			if tok.Qualifier != "comments" || !ok {
				return v, false
			}
			return TextValue(n), true
		},
	}}}
	m := New(spec, WithQuery("comments:>10 comments:5"))
	if v, _ := m.Value("comments"); v.Text() != "10" {
		t.Errorf("comments = %q, want 10", v.Text())
	}
	if got, want := m.Query(), "comments:>10 comments:5"; got != want {
		t.Errorf("Query = %q, want %q", got, want)
	}
}

// A Choice with no empty option, and a sort with none, can't be left out,
// so an empty query keeps their defaults.
func TestClearedKeepsRequiredDefaults(t *testing.T) {
	spec := Spec{
		Fields: []Field{{Key: "s", Kind: Choice, Qualifier: "is", Options: []Item{{"Open", "open", ""}, {"Closed", "closed", ""}}, Default: TextValue("open")}},
		Sort:   &SortField{Options: []SortOption{{Label: "Best match"}, {Label: "Updated", Value: "updated"}}, Default: Sort{By: "updated", Desc: true}},
	}
	m := New(spec, WithQuery(""))
	if got := m.Query(); got != "is:open" {
		t.Errorf("Query = %q, want is:open", got)
	}
}

// A sort that can be left out keeps its default direction when the query
// has none, so choosing an option sorts the default way.
func TestClearedKeepsTheSortDirection(t *testing.T) {
	spec := Spec{Sort: &SortField{
		Options: []SortOption{{Label: "Best match"}, {Label: "Stars", Value: "stars"}},
		Default: Sort{Desc: true},
	}}
	m := New(spec, WithQuery("tea"), WithTab(SortTab))
	m.Focus()
	m, _ = m.Update(right)
	if got := m.Query(); got != "sort:stars-desc tea" {
		t.Errorf("Query = %q, want the stars descending", got)
	}
}

func TestValue(t *testing.T) {
	m := New(prSpec(nil))
	v, ok := m.Value("labels")
	if !ok || !slices.Equal(v.List(), []string{"bug", "enhancement"}) {
		t.Fatalf("labels = %v, %v", v.List(), ok)
	}
	v.List()[0] = "changed"
	if w, _ := m.Value("labels"); w.List()[0] != "bug" {
		t.Error("changing a returned list changed the form")
	}
	if _, ok := m.Value("nope"); ok {
		t.Error("Value found a field that isn't there")
	}
	if !BoolValue(false).IsZero() || TextValue("a").IsZero() || ListValue().IsZero() == false {
		t.Error("IsZero is wrong")
	}
}

func equalValues(a, b map[string]Value) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || !v.Equal(w) {
			return false
		}
	}
	return true
}
