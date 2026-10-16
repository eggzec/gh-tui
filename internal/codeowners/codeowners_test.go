package codeowners

import (
	"os"
	"slices"
	"testing"
)

func owners(names ...string) []Owner {
	out := make([]Owner, 0, len(names))
	for _, n := range names {
		o, ok := parseOwner(n)
		if !ok {
			panic("bad owner " + n)
		}
		out = append(out, o)
	}
	return out
}

func TestParseOwner(t *testing.T) {
	tests := []struct {
		in   string
		want Owner
		ok   bool
	}{
		{"@octocat", Owner{OwnerUser, "octocat"}, true},
		{"@octo-cat_1", Owner{OwnerUser, "octo-cat_1"}, true},
		{"@org/team-a.b", Owner{OwnerTeam, "org/team-a.b"}, true},
		{"a@example.com", Owner{OwnerEmail, "a@example.com"}, true},
		{"octocat", Owner{OwnerEmail, "octocat"}, false},
		{"@", Owner{OwnerUser, ""}, false},
		{"@org/", Owner{OwnerTeam, "org/"}, false},
		{"@a/b/c", Owner{OwnerTeam, "a/b/c"}, false},
		{"@bad name", Owner{OwnerUser, "bad name"}, false},
		{"a@b", Owner{OwnerEmail, "a@b"}, false},
	}
	for _, tt := range tests {
		got, ok := parseOwner(tt.in)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("parseOwner(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestOwnerParts(t *testing.T) {
	team := Owner{OwnerTeam, "octo/reviewers"}
	if team.Org() != "octo" || team.Slug() != "reviewers" || team.String() != "@octo/reviewers" {
		t.Errorf("team = %q, %q, %q", team.Org(), team.Slug(), team)
	}
	user := Owner{OwnerUser, "mona"}
	if user.Org() != "" || user.Slug() != "" || user.String() != "@mona" {
		t.Errorf("user = %q, %q, %q", user.Org(), user.Slug(), user)
	}
	if e := (Owner{OwnerEmail, "a@b.co"}); e.String() != "a@b.co" {
		t.Errorf("email = %q", e)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []Rule
	}{
		{"empty", "", nil},
		{"comments and blanks", "# a\n\n   \n\t# b\n", nil},
		{"owners", "*.go @a @b/c d@e.fg\n", []Rule{{Pattern: "*.go", Owners: owners("@a", "@b/c", "d@e.fg"), Line: 1}}},
		{"inline comment", "*.go @a # why\n", []Rule{{Pattern: "*.go", Owners: owners("@a"), Line: 1}}},
		{"comment without a space", "*.go @a #why\n", []Rule{{Pattern: "*.go", Owners: owners("@a"), Line: 1}}},
		{"no owners clears", "\n/x/\n", []Rule{{Pattern: "/x/", Line: 2}}},
		{"escaped space", "/my\\ docs/ @a\n", []Rule{{Pattern: "/my docs/", Owners: owners("@a"), Line: 1}}},
		{"escaped hash", "\\#n.txt @a\n", []Rule{{Pattern: "#n.txt", Owners: owners("@a"), Line: 1}}},
		{"escaped brackets", "a\\[1\\] @a\n", []Rule{{Pattern: "a\\[1\\]", Owners: owners("@a"), Line: 1}}},
		{"tabs and CRLF", "a\t@x \t@y\r\nb @z\r\n", []Rule{
			{Pattern: "a", Owners: owners("@x", "@y"), Line: 1},
			{Pattern: "b", Owners: owners("@z"), Line: 2},
		}},
		{"leading space", "   a @x\n", []Rule{{Pattern: "a", Owners: owners("@x"), Line: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Parse(tt.text)
			if len(f.Invalid) != 0 {
				t.Errorf("Invalid = %+v, want none", f.Invalid)
			}
			if len(f.Rules) != len(tt.want) {
				t.Fatalf("rules = %+v, want %+v", f.Rules, tt.want)
			}
			for i, r := range f.Rules {
				w := tt.want[i]
				if r.Pattern != w.Pattern || r.Line != w.Line || !slices.Equal(r.Owners, w.Owners) {
					t.Errorf("rule %d = %+v, want %+v", i, r, w)
				}
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	f := Parse("ok @a\n!no @a\n[ab].go @a\nbad nobody\n@a x\ngood @b\n")
	lines := make([]int, 0, len(f.Invalid))
	for _, in := range f.Invalid {
		lines = append(lines, in.Line)
		if in.Reason == "" || in.Text == "" {
			t.Errorf("invalid %+v lacks its text or reason", in)
		}
	}
	if !slices.Equal(lines, []int{2, 3, 4, 5}) {
		t.Errorf("invalid lines = %v, want [2 3 4 5]", lines)
	}
	if len(f.Rules) != 2 || f.Rules[0].Pattern != "ok" || f.Rules[1].Pattern != "good" || f.Rules[1].Line != 6 {
		t.Errorf("rules = %+v, want the valid lines 1 and 6", f.Rules)
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// A pattern with no slash matches at any depth, as a file or a directory.
		{"*", "a", true},
		{"*", "a/b/c.txt", true},
		{"*.js", "x.js", true},
		{"*.js", "a/b/x.js", true},
		{"*.js", "x.jsx", false},
		{"*.js", "a.js/b.txt", true},
		{"docs", "docs", true},
		{"docs", "a/docs", true},
		{"docs", "docs/a.md", true},
		{"docs", "a/docs/b/c.md", true},
		{"docs", "mydocs", false},
		{"docs", "docs.md", false},
		{"README.md", "sub/README.md", true},
		{"a?c", "abc", true},
		{"a?c", "a/c", false},

		// A leading slash anchors to the root.
		{"/docs", "docs", true},
		{"/docs", "docs/a.md", true},
		{"/docs", "a/docs", false},
		{"/docs", "a/docs/b.md", false},
		{"/*.md", "a.md", true},
		{"/*.md", "d/a.md", false},

		// A slash inside also anchors.
		{"docs/guide", "docs/guide", true},
		{"docs/guide", "docs/guide/a.md", true},
		{"docs/guide", "x/docs/guide", false},
		{"src/*.go", "src/a.go", true},
		{"src/*.go", "src/sub/a.go", false},

		// A trailing slash matches a directory and everything under it.
		{"apps/", "apps/a.go", true},
		{"apps/", "x/apps/a.go", true},
		{"apps/", "x/apps/y/z/a.go", true},
		{"apps/", "apps", false},
		{"apps/", "myapps/a.go", false},
		{"/apps/", "apps/a.go", true},
		{"/apps/", "x/apps/a.go", false},
		{"/build/logs/", "build/logs/a/b.txt", true},
		{"/build/logs/", "build/a.txt", false},

		// A trailing /* is the files directly in the directory.
		{"docs/*", "docs/a.md", true},
		{"docs/*", "docs/sub/a.md", false},
		{"/docs/*", "docs/a.md", true},
		{"/docs/*", "docs/sub/a.md", false},
		{"/docs/*", "x/docs/a.md", false},
		// A bare /* is not that: as in gitignore it matches the top-level
		// entries, and a directory owns what is below it, so it owns all.
		{"/*", "a.md", true},
		{"/*", "docs/a.md", true},
		{"/*", "a/b/c/d.go", true},
		{"a\\[1\\]", "a[1]", true},
		{"a\\[1\\]", "a1", false},

		// ** crosses segments.
		{"docs/**", "docs/a.md", true},
		{"docs/**", "docs/a/b/c.md", true},
		{"docs/**", "x/docs/a.md", false},
		{"**/logs", "logs", true},
		{"**/logs", "a/b/logs", true},
		{"**/logs", "a/b/logs/c.txt", true},
		{"**/logs", "a/mylogs", false},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/x/y/b/c", true},
		{"a/**/b", "a/x/y/c", false},
		{"/**/logs/**", "a/logs/b", true},
		{"**/*.md", "a/b.md", true},
		{"**/*.md", "b.md", true},

		// Characters that mean something to a regular expression don't.
		{"a.b", "axb", false},
		{"a+b", "a+b", true},
		{"(x)", "(x)", true},
		{"a\\*b", "a*b", true},
		{"a\\*b", "axb", false},

		// A character range isn't supported, so such a pattern is invalid:
		// see TestParseInvalid.
	}
	for _, tt := range tests {
		f := Parse(tt.pattern + " @o\n")
		if len(f.Rules) != 1 {
			t.Errorf("%q: Parse gave %+v", tt.pattern, f)
			continue
		}
		if got := f.Rules[0].Matches(tt.path); got != tt.want {
			t.Errorf("%q matches %q = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestLastRuleWins(t *testing.T) {
	f := Parse("*  @everyone\n*.go @gophers\n/vendor/ \n/vendor/keep.go @keeper\n")
	tests := []struct {
		path string
		want []Owner
		line int
	}{
		{"README.md", owners("@everyone"), 1},
		{"main.go", owners("@gophers"), 2},
		{"vendor/x.go", nil, 3},
		{"vendor/keep.go", owners("@keeper"), 4},
		{"/vendor/keep.go", owners("@keeper"), 4},
	}
	for _, tt := range tests {
		r, ok := f.RuleFor(tt.path)
		if !ok || r.Line != tt.line {
			t.Errorf("RuleFor(%q) = line %d, %v; want line %d", tt.path, r.Line, ok, tt.line)
		}
		if got := f.OwnersOf(tt.path); !slices.Equal(got, tt.want) {
			t.Errorf("OwnersOf(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
	if _, ok := Parse("/a @x\n").RuleFor("b"); ok {
		t.Error("a path no rule matches has a rule")
	}
	if got := (File{}).OwnersOf("a"); got != nil {
		t.Errorf("an empty file owns %v", got)
	}
}

func TestFixture(t *testing.T) {
	text, err := os.ReadFile("testdata/CODEOWNERS")
	if err != nil {
		t.Fatal(err)
	}
	f := Parse(string(text))
	bad := make([]int, 0, len(f.Invalid))
	for _, in := range f.Invalid {
		bad = append(bad, in.Line)
	}
	if !slices.Equal(bad, []int{26, 27, 28}) {
		t.Errorf("invalid lines = %v, want the last three", bad)
	}
	tests := []struct {
		path string
		want []Owner
	}{
		{"README.md", owners("@global-owner1", "@global-owner2")},
		{"web/app.js", owners("@js-owner")},
		{"web/app.go", owners("docs@example.com")},
		{"docs/intro.md", owners("@octocat/docs-team")},
		{"docs/guide/intro.md", owners("@global-owner1", "@global-owner2")},
		{"x/apps/main.py", owners("@octocat")},
		{"my docs/a.md", owners("@spaced")},
		{"#notes.txt", owners("@hash")},
		{"build/generated/a.pb", nil},
		{"src/a.go", owners("docs@example.com")},
	}
	for _, tt := range tests {
		if got := f.OwnersOf(tt.path); !slices.Equal(got, tt.want) {
			t.Errorf("OwnersOf(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestOwnedPaths(t *testing.T) {
	f := Parse("*.go @b @a/team\n/docs/ @a/team\n/gen/ \n*.md @B\n*.txt @c\n")
	got := f.OwnedPaths([]string{"z.go", "docs/x.md", "gen/a.go", "a.go", "n.bin", "z.go", "t.txt"})
	// docs/x.md is B's, not the team's: the last matching rule is *.md. Names
	// that differ only in case sort with the capital first.
	want := []Owned{
		{Owner{OwnerTeam, "a/team"}, []string{"a.go", "z.go"}},
		{Owner{OwnerUser, "B"}, []string{"docs/x.md"}},
		{Owner{OwnerUser, "b"}, []string{"a.go", "z.go"}},
		{Owner{OwnerUser, "c"}, []string{"t.txt"}},
	}
	if len(got) != len(want) {
		t.Fatalf("OwnedPaths = %+v, want %+v", got, want)
	}
	for i := range got {
		if got[i].Owner != want[i].Owner || !slices.Equal(got[i].Paths, want[i].Paths) {
			t.Errorf("owned %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// The order of the paths doesn't matter.
	rev := f.OwnedPaths([]string{"t.txt", "z.go", "a.go", "n.bin", "gen/a.go", "docs/x.md"})
	for i := range rev {
		if rev[i].Owner != want[i].Owner || !slices.Equal(rev[i].Paths, want[i].Paths) {
			t.Errorf("reversed owned %d = %+v, want %+v", i, rev[i], want[i])
		}
	}
	if got := f.OwnedPaths(nil); len(got) != 0 {
		t.Errorf("no paths gave %+v", got)
	}
}
