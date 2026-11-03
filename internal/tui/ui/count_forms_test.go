package ui

import (
	"slices"
	"testing"
)

func TestCountForms(t *testing.T) {
	cases := []struct {
		name string
		spec CountSpec
		want []string
	}{
		{"zero", CountSpec{}, []string{"0"}},
		{"one unknown", CountSpec{Loaded: 1}, []string{"1"}},
		{"more pages", CountSpec{Loaded: 30, More: true}, []string{"30+"}},
		{"999 loaded", CountSpec{Loaded: 999}, []string{"999"}},
		{"1000 loaded", CountSpec{Loaded: 1000}, []string{"1k"}},
		{"total beside loaded", CountSpec{Total: 2000, Loaded: 20, More: true}, []string{"2k [20+]", "2k"}},
		{"all loaded", CountSpec{Total: 30, Loaded: 30}, []string{"30"}},
		{"total below loaded", CountSpec{Total: 5, Loaded: 30}, []string{"30"}},
		{"filter no total all loaded", CountSpec{Loaded: 30, Matched: 30, Filtered: true}, []string{"30"}},
		{"filter no total", CountSpec{Loaded: 30, Matched: 7, Filtered: true}, []string{"7 [30]", "7"}},
		{"filter total all matched", CountSpec{Total: 2000, Loaded: 20, Matched: 20, More: true, Filtered: true}, []string{"20/2k", "20"}},
		{"filter total", CountSpec{Total: 2000, Loaded: 20, Matched: 3, Filtered: true}, []string{"3/2k [20]", "3/2k", "3"}},
		{"filter zero matched", CountSpec{Total: 40, Loaded: 20, Matched: 0, More: true, Filtered: true}, []string{"0/40 [20+]", "0/40", "0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CountForms(c.spec); !slices.Equal(got, c.want) {
				t.Fatalf("CountForms(%+v) = %q, want %q", c.spec, got, c.want)
			}
		})
	}
}
