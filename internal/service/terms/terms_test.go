package terms

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	got := Parse(`author:@me  -is:draft label:"good first issue",bug crash Sort:created-asc "two words"`)
	want := []Term{
		{Key: "author", Value: "@me", raw: "@me"},
		{Key: "is", Not: true, Value: "draft", raw: "draft"},
		{Key: "label", Value: "good first issue,bug", raw: `"good first issue",bug`},
		{Value: "crash", raw: "crash"},
		{Key: "sort", Value: "created-asc", raw: "created-asc"},
		{Value: "two words", raw: `"two words"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse =\n%+v\nwant\n%+v", got, want)
	}
	if v := got[2].Values(); !reflect.DeepEqual(v, []string{"good first issue", "bug"}) {
		t.Errorf("Values = %q, want both labels", v)
	}
	if len(Parse("   ")) != 0 {
		t.Error("Parse of spaces returned terms")
	}
}

func TestParseNotQualifiers(t *testing.T) {
	for _, word := range []string{":x", "a.b:c", "é!:x"} {
		if got := Parse(word); len(got) != 1 || got[0].Key != "" {
			t.Errorf("Parse(%q) = %+v, want a free word", word, got)
		}
	}
}

func TestSort(t *testing.T) {
	tests := []struct {
		in  string
		by  string
		asc bool
	}{
		{"updated-desc", "updated", false},
		{"created-asc", "created", true},
		{"comments", "comments", false},
		{"Created-ASC", "created", true},
	}
	for _, tt := range tests {
		if by, asc := Sort(tt.in); by != tt.by || asc != tt.asc {
			t.Errorf("Sort(%q) = %q, %v; want %q, %v", tt.in, by, asc, tt.by, tt.asc)
		}
	}
}
