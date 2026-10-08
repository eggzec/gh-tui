package core

import (
	"slices"
	"strings"
	"testing"
)

func TestScanRefs(t *testing.T) {
	here := RepoRef{Owner: "eggzec", Name: "gh-tui"}
	other := RepoRef{Owner: "cli", Name: "go-gh"}
	r := func(repo RepoRef, n int) Target { return Target{Repo: repo, Number: n} }
	pull := func(repo RepoRef, n int) Target { return Target{Repo: repo, Number: n, Kind: KindPull} }
	issue := func(repo RepoRef, n int) Target { return Target{Repo: repo, Number: n, Kind: KindIssue} }
	tests := []struct {
		name string
		host string
		text string
		want []Target
	}{
		{name: "same repository", text: "Fixes #12.", want: []Target{r(here, 12)}},
		{name: "other repository", text: "See cli/go-gh#412, too", want: []Target{r(other, 412)}},
		{name: "in order, and more than once", text: "#3 then cli/go-gh#2 and #3", want: []Target{r(here, 3), r(other, 2), r(here, 3)}},
		{name: "after punctuation", text: "(#1) [#2] {#3},#4;#5:#6", want: []Target{r(here, 1), r(here, 2), r(here, 3), r(here, 4), r(here, 5), r(here, 6)}},
		{name: "at the start of a line", text: "x\n#7", want: []Target{r(here, 7)}},
		{name: "not a reference", text: "abc#12 #12abc # 5 #0 #99999999999 deadbeef", want: nil},
		{
			name: "links", text: "https://github.com/cli/go-gh/pull/412 and https://github.com/eggzec/gh-tui/issues/9.",
			want: []Target{pull(other, 412), issue(here, 9)},
		},
		{name: "a link in markdown", text: "[the fix](https://github.com/cli/go-gh/pull/412)!", want: []Target{pull(other, 412)}},
		{name: "a link to a comment", text: "https://github.com/cli/go-gh/issues/5#issuecomment-123", want: []Target{issue(other, 5)}},
		{name: "a link and a short form in order", text: "#1 https://github.com/cli/go-gh/pull/2 #3", want: []Target{r(here, 1), pull(other, 2), r(here, 3)}},
		{name: "a link is not read twice", text: "https://github.com/cli/go-gh/pull/2", want: []Target{pull(other, 2)}},
		{name: "links to no item", text: "https://github.com/cli/go-gh https://github.com/cli https://github.com/cli/go-gh/pulls", want: nil},
		{name: "another host", text: "https://gitlab.com/o/r/pull/5 https://ghe.example.com/o/r/pull/6", want: nil},
		{
			name: "an Enterprise host", host: "ghe.example.com",
			text: "https://ghe.example.com/o/r/pull/5 and https://ghe.example.com/api/v3/repos/o/r/issues/6",
			want: []Target{pull(RepoRef{Owner: "o", Name: "r"}, 5), issue(RepoRef{Owner: "o", Name: "r"}, 6)},
		},
		{name: "github.com on an Enterprise host", host: "ghe.example.com", text: "https://github.com/o/r/pull/5 #7", want: []Target{r(here, 7)}},
		{name: "a fenced block", text: "a #1\n```\n#2\n```\nb #3", want: []Target{r(here, 1), r(here, 3)}},
		{name: "a fence with a language", text: "```go\n#2\n```\n#3", want: []Target{r(here, 3)}},
		{name: "a tilde fence", text: "~~~\n#2\n~~~\n#3", want: []Target{r(here, 3)}},
		{name: "a fence that doesn't end", text: "#1\n```\n#2\n#3", want: []Target{r(here, 1)}},
		{name: "a longer fence holds a shorter one", text: "````\n```\n#2\n```\n````\n#3", want: []Target{r(here, 3)}},
		{name: "a code span", text: "`#2` and #3", want: []Target{r(here, 3)}},
		{name: "a double code span", text: "``a ` #2`` and #3", want: []Target{r(here, 3)}},
		{name: "a lone backtick", text: "it`s #3", want: []Target{r(here, 3)}},
		{name: "a link in a code span", text: "`https://github.com/cli/go-gh/pull/2` #3", want: []Target{r(here, 3)}},
		{name: "an HTML comment", text: "<!-- Fixes #2 -->\n#3", want: []Target{r(here, 3)}},
		{name: "a comment over lines", text: "<!--\nFixes #2\n-->#3", want: []Target{r(here, 3)}},
		{name: "a comment that doesn't end, mid-sentence", text: "#1 <!-- #2", want: []Target{r(here, 1), r(here, 2)}},
		{name: "an indented block", text: "see:\n\n    #2\n    #4\n\n#3", want: []Target{r(here, 3)}},
		{name: "a tab block", text: "\t#2\n#3", want: []Target{r(here, 3)}},
		{name: "indented text in a list is not code", text: "- item\n\n    more of it #2", want: []Target{r(here, 2)}},
		{name: "indented text after text is not code", text: "word\n    #2", want: []Target{r(here, 2)}},
		{name: "a lone backtick doesn't reach the next paragraph", text: "Don`t do it.\n\nFixes #3 with `code`", want: []Target{r(here, 3)}},
		{name: "a backtick in a comment", text: "<!-- a lone ` here -->\n\nFixes #4 and `x`", want: []Target{r(here, 4)}},
		{name: "a span that opens before a comment is code", text: "a ` b <!-- c ` --> #5", want: []Target{r(here, 5)}},
		{name: "a comment in a span", text: "`<!--` #6 `-->`", want: []Target{r(here, 6)}},
		{name: "an autolink", text: "See GH-12 and (GH-13), not gh-14, GH-x, XGH-15 or foo_GH-24", want: []Target{r(here, 12), r(here, 13)}},
		{name: "an autolink in emphasis", text: "_GH-25_ and *GH-26* and a_#27", want: []Target{r(here, 25), r(here, 26)}},
		{name: "a comment that never ends, mid-sentence", text: "Use <!-- to start one. Fixes #7", want: []Target{r(here, 7)}},
		{name: "a comment that never ends, on a line", text: "Fixes #7\n  <!-- never closed #8\n#9", want: []Target{r(here, 7)}},
		{name: "a span doesn't cross list items", text: "- a ` b\n- c ` and #10", want: []Target{r(here, 10)}},
		{name: "a span over lines of one item", text: "- a `b\n  c` and #11", want: []Target{r(here, 11)}},
		{name: "a link in emphasis", text: "**https://github.com/cli/go-gh/pull/1** and _https://github.com/cli/go-gh/issues/2_ ~~#3~~", want: []Target{pull(other, 1), issue(other, 2), r(here, 3)}},
		{name: "a link without a scheme", text: "see github.com/cli/go-gh/issues/8, or www.github.com/cli/go-gh/pull/9.", want: []Target{issue(other, 8), pull(other, 9)}},
		{name: "a link without a scheme to another host", text: "gitlab.com/o/r/issues/8 xgithub.com/o/r/issues/9", want: nil},
		{name: "an owner with an underscore", text: "octo_acme/tools#4", want: []Target{r(RepoRef{Owner: "octo_acme", Name: "tools"}, 4)}},
		{name: "empty", text: "", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScanRefs(tt.text, tt.host, here); !slices.Equal(got, tt.want) {
				t.Errorf("ScanRefs(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestRefKey(t *testing.T) {
	a := Target{Repo: RepoRef{Owner: "EggZec", Name: "GH-TUI"}, Number: 12, Kind: KindPull}
	b := Target{Repo: RepoRef{Owner: "eggzec", Name: "gh-tui"}, Number: 12, Kind: KindIssue}
	if RefKey(a) != RefKey(b) || RefKey(a) != "eggzec/gh-tui#12" {
		t.Errorf("RefKey = %q and %q, want one key whatever the case and the kind", RefKey(a), RefKey(b))
	}
	if RefKey(a) == RefKey(Target{Repo: a.Repo, Number: 13}) {
		t.Error("RefKey names two numbers alike")
	}
}

func TestScanRefsLongText(t *testing.T) {
	text := strings.Repeat("word `code` ", 1000) + "#5"
	if got := ScanRefs(text, "", RepoRef{Owner: "o", Name: "r"}); len(got) != 1 {
		t.Errorf("ScanRefs found %v, want one", got)
	}
}
