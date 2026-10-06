package github

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestSharedShapesDecode(t *testing.T) {
	const data = `{
		"author": {"login": "octocat", "name": "The Octocat"},
		"labels": {"nodes": [{"name": "bug", "color": "d73a4a", "description": "Something is wrong"}]},
		"pageInfo": {"hasNextPage": true, "endCursor": "Y3Vyc29y"}
	}`
	var v struct {
		Author   user         `json:"author"`
		Labels   nodes[label] `json:"labels"`
		PageInfo pageInfo     `json:"pageInfo"`
	}
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		t.Fatal(err)
	}

	if got, want := v.Author.core(), (core.User{Login: "octocat", Name: "The Octocat"}); got != want {
		t.Errorf("author = %+v, want %+v", got, want)
	}
	wantLabels := []core.Label{{Name: "bug", Color: "d73a4a", Description: "Something is wrong"}}
	if got := convert(v.Labels.Nodes, label.core); !slices.Equal(got, wantLabels) {
		t.Errorf("labels = %+v, want %+v", got, wantLabels)
	}
	if got := v.PageInfo.next(); got != "Y3Vyc29y" {
		t.Errorf("next = %q, want the end cursor", got)
	}
}

// TestUserKinds checks that an app is told from a person in the shapes of
// both APIs: GraphQL's actor and REST's account.
func TestUserKinds(t *testing.T) {
	tests := []struct {
		data string
		rest bool
		want core.User
	}{
		{data: `{"__typename": "Bot", "login": "dependabot"}`, want: core.User{Login: "dependabot", Bot: true}},
		{data: `{"__typename": "User", "login": "octocat", "name": "The Octocat"}`, want: core.User{Login: "octocat", Name: "The Octocat"}},
		{data: `{"login": "dependabot[bot]", "type": "Bot"}`, rest: true, want: core.User{Login: "dependabot[bot]", Bot: true}},
		{data: `{"login": "octocat", "type": "User"}`, rest: true, want: core.User{Login: "octocat"}},
	}
	for _, tt := range tests {
		var got core.User
		var err error
		if tt.rest {
			var a account
			err = json.Unmarshal([]byte(tt.data), &a)
			got = a.core()
		} else {
			var a actor
			err = json.Unmarshal([]byte(tt.data), &a)
			got = a.core()
		}
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.data, got, tt.want)
		}
	}
}

func TestPageInfoLastPage(t *testing.T) {
	p := pageInfo{HasNextPage: false, EndCursor: "Y3Vyc29y"}
	if got := p.next(); got != "" {
		t.Errorf("next = %q, want empty on the last page", got)
	}
}

func TestConvertEmpty(t *testing.T) {
	if got := convert([]user{}, user.core); got != nil {
		t.Errorf("convert(empty) = %v, want nil", got)
	}
}
