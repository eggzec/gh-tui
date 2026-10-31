package markdown

import (
	"fmt"
	"image/color"
	"reflect"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestRefs(t *testing.T) {
	issue := func(n string) Ref { return Ref{RefIssue, n} }
	user := func(n string) Ref { return Ref{RefUser, n} }
	commit := func(n string) Ref { return Ref{RefCommit, n} }
	tests := []struct {
		name, in string
		want     []Ref
	}{
		{"issue", "fixes #123.", []Ref{issue("123")}},
		{"issue at start", "#7 and #7 again", []Ref{issue("7")}},
		{"nine digits", "#123456789", []Ref{issue("123456789")}},
		{"ten digits", "#1234567890", nil},
		{"leading zero", "#0123 #0", nil},
		{"issue then letters", "#12abc", nil},
		{"heading", "# Title", nil},
		{"user", "thanks @octocat!", []Ref{user("octocat")}},
		{"user hyphen", "@a-b and @-a and @a-", []Ref{user("a-b")}},
		{"user too long", "@" + strings.Repeat("a", 40), nil},
		{"user longest", "@" + strings.Repeat("a", 39), []Ref{user(strings.Repeat("a", 39))}},
		{"team", "@org/team", nil},
		{"email", "mail me@example.com now", nil},
		{"plus email", "me+gh@example.com a.b=@c d%@e", nil},
		{"definition", "[WIP]: fixes #12", []Ref{issue("12")}},
		{"link definition", "[x]: https://example.com/#2 \"t #3\"", nil},
		{"lone backtick then code", "Use the ` key.\n\nRun `git log #1` now #2", []Ref{issue("2")}},
		{"lone backtick, list items", "- a `\n- `b #1` #2", []Ref{issue("2")}},
		{"underscore", "@user_name", nil},
		{"hash", "see 0123abc and ABCDEF1", []Ref{commit("0123abc"), commit("abcdef1")}},
		{"hash 40", strings.Repeat("a1", 20), []Ref{commit(strings.Repeat("a1", 20))}},
		{"hash 41", strings.Repeat("a1", 20) + "b", nil},
		{"hash short", "abc123", nil},
		{"hash digits only", "1234567", nil},
		{"hash letters only", "abcdefa", nil},
		{"hash in word", "x0123abc 0123abcx 0123abc_", nil},
		{"hash in path", "a/0123abc 0123abc/b", nil},
		{"code span", "`#1 @a 0123abc` #2", []Ref{issue("2")}},
		{"code span over lines", "`a\n#1` #2", []Ref{issue("2")}},
		{"fence", "```\n#1\n```\n#2", []Ref{issue("2")}},
		{"indented code", "text\n\n    #1\n\n#2", []Ref{issue("2")}},
		{"link text", "[see #1](https://example.com/#2) #3", []Ref{issue("3")}},
		{"reference link", "[see #1][x] #3\n\n[x]: https://example.com/#2", []Ref{issue("3")}},
		{"autolink", "<https://example.com/#1> #2", []Ref{issue("2")}},
		{"bare url", "https://example.com/0123abc/#1 @a", []Ref{user("a")}},
		{"html tag", `<a href="#1" title="@a">x</a> #2`, []Ref{issue("2")}},
		{"entity", "&#123; &amp;", nil},
		{"escaped", `\#1 #2`, []Ref{issue("2")}},
		{"order", "@b #2 @a #1 @b", []Ref{user("b"), issue("2"), user("a"), issue("1")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Refs(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Refs(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestRefsCap(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= maxRefs+20; i++ {
		fmt.Fprintf(&b, "#%d ", i)
	}
	got := Refs(b.String())
	if len(got) != maxRefs || got[0] != (Ref{RefIssue, "1"}) {
		t.Errorf("got %d refs, first %v; want %d", len(got), got[0], maxRefs)
	}
}

// known is an Autolink that knows the references in set.
func known(set map[Ref]bool) Autolink {
	return func(kind RefKind, ref string) (string, bool) {
		return "https://example.com/" + ref, set[Ref{kind, ref}]
	}
}

func addrs(out string) int { return strings.Count(out, "\x1b]8;;https://example.com/") }

func TestRenderAutolink(t *testing.T) {
	linksOn(t, true)
	r := New(DefaultStyle(true))
	r.SetAutolink(known(map[Ref]bool{
		{RefIssue, "1"}:            true,
		{RefUser, "octocat"}:       true,
		{RefCommit, "0123abcdef0"}: true,
		{RefIssue, "3"}:            true,
	}))
	out := r.Render("See #1, #2, @octocat, @ghost and 0123abcdef0.\n\n`#3` and\n\n```\n#3\n```", 80)
	plain := strings.Join(strings.Fields(xansi.Strip(out)), " ")
	if !strings.Contains(plain, "See #1, #2, @octocat, @ghost and 0123abc.") {
		t.Errorf("text: %q", plain)
	}
	if strings.Contains(plain, "example.com") {
		t.Errorf("an address shows: %q", plain)
	}
	if n := addrs(out); n != 3 {
		t.Errorf("%d links, want 3: %q", n, out)
	}
	for _, want := range []string{"/1\x07", "/octocat\x07", "/0123abcdef0\x07"} {
		if !strings.Contains(out, want) && !strings.Contains(out, strings.TrimSuffix(want, "\x07")+"\x1b\\") {
			t.Errorf("no link to %q: %q", want, out)
		}
	}
}

func TestRenderAutolinkLoneBacktick(t *testing.T) {
	linksOn(t, true)
	r := New(DefaultStyle(true))
	r.SetAutolink(known(map[Ref]bool{{RefIssue, "1"}: true}))
	out := r.Render("Use the ` key.\n\nRun `git log #1` now", 80)
	if n := addrs(out); n != 0 || strings.Contains(xansi.Strip(out), "example.com") {
		t.Errorf("a ref in code is linked: %q", out)
	}
}

func TestRenderAutolinkOff(t *testing.T) {
	const src = "See #1 and @octocat and 0123abcdef0."
	all := known(map[Ref]bool{{RefIssue, "1"}: true, {RefUser, "octocat"}: true, {RefCommit, "0123abcdef0"}: true})
	for _, on := range []bool{false, true} {
		linksOn(t, on)
		plain := New(DefaultStyle(true))
		hooked := New(DefaultStyle(true))
		hooked.SetAutolink(all)
		want, got := plain.Render(src, 80), hooked.Render(src, 80)
		if on && got == want || !on && got != want {
			t.Errorf("links %v: hook output %q, without %q", on, got, want)
		}
	}
	// A hook that knows nothing changes nothing.
	linksOn(t, true)
	r := New(DefaultStyle(true))
	r.SetAutolink(known(nil))
	if got, want := r.Render(src, 80), New(DefaultStyle(true)).Render(src, 80); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderAutolinkAnswerFlips(t *testing.T) {
	linksOn(t, true)
	set := map[Ref]bool{}
	r := New(DefaultStyle(true))
	r.SetAutolink(known(set))
	const src = "See #5."
	if n := addrs(r.Render(src, 80)); n != 0 {
		t.Fatalf("%d links before it is known", n)
	}
	renders := r.Renders()
	r.Render(src, 80)
	if r.Renders() != renders {
		t.Errorf("rendered again with no new answer")
	}
	set[Ref{RefIssue, "5"}] = true
	if n := addrs(r.Render(src, 80)); n != 1 {
		t.Errorf("%d links once it is known, want 1", n)
	}
	delete(set, Ref{RefIssue, "5"})
	if n := addrs(r.Render(src, 80)); n != 0 {
		t.Errorf("%d links once it is gone, want 0", n)
	}
}

func TestRenderLinkColor(t *testing.T) {
	const src = "See [the docs](https://example.com/docs) now."
	linksOn(t, true)
	r := New(DefaultStyle(true))
	plain := r.Render(src, 80)
	r.SetLinkColor(color.RGBA{R: 1, G: 2, B: 3, A: 255})
	out := r.Render(src, 80)
	i := strings.Index(out, "the docs")
	if i < 0 || !strings.Contains(out[:i], "38;2;1;2;3m") {
		t.Errorf("the text is not coloured: %q", out)
	}
	if !strings.Contains(out[i:], "\x1b[4m") && !strings.Contains(out[:i], "\x1b[4m") {
		t.Errorf("the underline is gone: %q", out)
	}
	r.SetLinkColor(nil)
	if got := r.Render(src, 80); got != plain {
		t.Errorf("with no colour got %q, want %q", got, plain)
	}
	// With links off the colour isn't used.
	linksOn(t, false)
	r.SetLinkColor(color.RGBA{R: 1, G: 2, B: 3, A: 255})
	if got := r.Render(src, 80); strings.Contains(got, "38;2;1;2;3m") {
		t.Errorf("coloured with links off: %q", got)
	}
}

// With links on, a link of a body shows its text only.
func TestRenderHidesAddress(t *testing.T) {
	linksOn(t, true)
	got := xansi.Strip(New(DefaultStyle(true)).Render("Go to [the docs](https://example.com/docs) now.", 80))
	if got != "Go to the docs now." {
		t.Errorf("got %q", got)
	}
}
