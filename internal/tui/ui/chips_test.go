package ui

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPersonItemsLeaveOutTheViewer(t *testing.T) {
	users := []core.User{{Login: "octocat", Name: "The Octocat"}, {Login: "Hubot"}, {Login: "monalisa"}}
	tests := []struct {
		name, viewer string
		want         []string
	}{
		{"unknown viewer", "", []string{"octocat", "Hubot", "monalisa"}},
		{"viewer listed", "octocat", []string{"Hubot", "monalisa"}},
		{"viewer in another case", "hubot", []string{"octocat", "monalisa"}},
		{"viewer not listed", "someone", []string{"octocat", "Hubot", "monalisa"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := PersonItems(users, tt.viewer)
			got := make([]string, len(items))
			for i, it := range items {
				got[i] = it.Value
			}
			if len(got) != len(tt.want) {
				t.Fatalf("PersonItems(%q) = %v, want %v", tt.viewer, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("PersonItems(%q) = %v, want %v", tt.viewer, got, tt.want)
				}
			}
		})
	}
	if it := PersonItems(users[:1], "")[0]; it.Label != "octocat" || it.Detail != "The Octocat" {
		t.Errorf("item = %+v, want the login with the name beside", it)
	}
}
