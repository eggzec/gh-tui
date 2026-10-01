package pager

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// spy is a lexer that never finishes, as JSONata's and Jungle's don't on
// a lone backslash. It counts what chroma asks of it, claims the files of
// those lexers and more, and would be chroma's guess for any text.
type spy struct {
	configured, tokenised, analysed atomic.Int32
	release                         chan struct{}
}

var (
	spyOnce sync.Once
	theSpy  *spy
)

// registerSpy registers the spy with chroma, once for all tests, and
// returns it with its counts zeroed.
func registerSpy(t *testing.T) *spy {
	t.Helper()
	spyOnce.Do(func() {
		theSpy = &spy{release: make(chan struct{})}
		lexers.GlobalLexerRegistry.Register(theSpy)
	})
	theSpy.reset()
	return theSpy
}

func (s *spy) reset() {
	s.configured.Store(0)
	s.tokenised.Store(0)
	s.analysed.Store(0)
}

// asked returns how often chroma asked the spy anything.
func (s *spy) asked() int32 {
	return s.configured.Load() + s.tokenised.Load() + s.analysed.Load()
}

func (s *spy) Config() *chroma.Config {
	s.configured.Add(1)
	return &chroma.Config{
		Name:      "Spy",
		Aliases:   []string{"spy"},
		Filenames: []string{"*.spy", "*.jsonata", "*.jungle"},
		Priority:  100,
	}
}

func (s *spy) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	s.tokenised.Add(1)
	<-s.release
	return chroma.Literator(), nil
}

func (s *spy) SetRegistry(*chroma.LexerRegistry) chroma.Lexer { return s }
func (s *spy) SetAnalyser(func(string) float32) chroma.Lexer  { return s }

func (s *spy) AnalyseText(string) float32 {
	s.analysed.Add(1)
	return 1
}

// settle waits a while for the goroutines to come down to n, and returns
// how many run.
func settle(n int) int {
	deadline := time.Now().Add(10 * time.Second)
	for runtime.NumGoroutine() > n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	return runtime.NumGoroutine()
}

// Setting content and taking its tokens never reach chroma, which may
// take long even to pick a lexer; only the command does.
func TestUpdateNeverReachesChroma(t *testing.T) {
	s := registerSpy(t)
	m := New(WithSize(40, 5))
	m.Focus()
	// Each in a pager of its own, so no content cancels another's
	// command.
	names := []string{"a.spy", "a.jsonata", "notes", "main.go"}
	cmds := make([]tea.Cmd, 0, len(names)+1)
	for _, name := range names {
		p := New()
		cmds = append(cmds, p.SetContent(name, "\\\n"))
	}
	p := New()
	cmds = append(cmds, p.SetContentSyntax("a.go", "spy", "\\\n"))
	highlight := m.SetContent("main.go", goSource)
	if n := s.asked(); n != 0 {
		t.Fatalf("setting content asked chroma %d times", n)
	}
	msg := highlight()
	s.reset()
	m, _ = m.Update(msg)
	if len(m.spans) != m.Lines() {
		t.Fatal("the tokens weren't taken")
	}
	keys(t, m, "j", "G", "l")
	if n := s.asked(); n != 0 {
		t.Fatalf("Update asked chroma %d times", n)
	}
	for _, c := range cmds {
		c()
	}
	if n, g := s.tokenised.Load(), s.analysed.Load(); n != 0 || g != 0 {
		t.Errorf("the commands ran the spy %d times and guessed %d times", n, g)
	}
}

// A file whose lexer never finishes, by its name or by a guess, shows at
// once and plain, its command ends at once, and nothing is left running.
func TestHostileFilesStayPlain(t *testing.T) {
	syntaxtest.Use(t, nil)
	s := registerSpy(t)
	before := runtime.NumGoroutine()
	for _, name := range []string{"a.jsonata", "a.jungle", "a.spy", "notes", "notes.txt"} {
		for _, text := range []string{"\\\n", "é\n", "\\é\n"} {
			m := New(WithSize(40, 5))
			start := time.Now()
			cmd := m.SetContent(name, text)
			if d := time.Since(start); d > 50*time.Millisecond {
				t.Errorf("%s: setting the content took %v", name, d)
			}
			if cmd == nil {
				continue
			}
			start = time.Now()
			if msg := cmd(); msg != nil {
				t.Errorf("%s: highlighted %q", name, text)
			}
			if d := time.Since(start); d > lexLimit {
				t.Errorf("%s: the command took %v, more than %v", name, d, lexLimit)
			}
		}
	}
	if n, g := s.tokenised.Load(), s.analysed.Load(); n != 0 || g != 0 {
		t.Errorf("chroma ran the spy %d times and guessed %d times", n, g)
	}
	if n := settle(before); n > before {
		t.Errorf("%d goroutines run after the commands, %d before", n, before)
	}
}

// highlighted returns the pager with name and text and its tokens.
func highlighted(t *testing.T, name, text string) Model {
	t.Helper()
	m := New(WithSize(40, 5))
	cmd := m.SetContent(name, text)
	if cmd == nil {
		t.Fatalf("%s: no command to highlight", name)
	}
	msg := cmd()
	if msg == nil {
		t.Fatalf("%s: not highlighted", name)
	}
	m, _ = m.Update(msg)
	return m
}

// Common files highlight as they did: by their names, by a shebang, and
// markdown with the code in its fences.
func TestCommonFilesHighlight(t *testing.T) {
	tests := []struct {
		name, text string
		// line is a line with a token of type want.
		line int
		want chroma.TokenType
	}{
		{name: "main.go", text: goSource, line: 0, want: chroma.KeywordNamespace},
		{name: "a.json", text: "{\n  \"a\": true\n}\n", line: 1, want: chroma.KeywordConstant},
		{name: "a.yaml", text: "a: true\n", line: 0, want: chroma.KeywordConstant},
		{name: "a.py", text: "def f():\n    return 1\n", line: 0, want: chroma.Keyword},
		{name: "Makefile", text: "all:\n\techo hi\n", line: 0, want: chroma.NameFunction},
		{name: "run", text: "#!/usr/bin/env python3\nimport os\n", line: 1, want: chroma.KeywordNamespace},
		{name: "README.md", text: "# Title\n\n```go\nfunc f() {}\n```\n", line: 0, want: chroma.GenericHeading},
		{name: "README.md", text: "# Title\n\n```go\nfunc f() {}\n```\n", line: 3, want: chroma.KeywordDeclaration},
	}
	for _, tt := range tests {
		m := highlighted(t, tt.name, tt.text)
		if len(m.spans) != m.Lines() {
			t.Errorf("%s: %d lines of tokens for %d lines", tt.name, len(m.spans), m.Lines())
			continue
		}
		var got []chroma.TokenType
		for _, s := range m.spans[tt.line] {
			got = append(got, s.typ)
		}
		if !strings.Contains(strings.Join(typeNames(got), " "), tt.want.String()) {
			t.Errorf("%s: line %d is %v, want a %v", tt.name, tt.line+1, got, tt.want)
		}
	}
}

func typeNames(ts []chroma.TokenType) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.String()
	}
	return out
}

// Code in a markdown fence reaches only the lexers of common languages.
func TestMarkdownFencesStayPlain(t *testing.T) {
	s := registerSpy(t)
	before := runtime.NumGoroutine()
	for _, lang := range []string{"jsonata", "jungle", "spy", "x.jsonata"} {
		m := New()
		cmd := m.SetContent("README.md", "# Title\n\n```"+lang+"\n\\\n```\n")
		if cmd() == nil {
			t.Errorf("%s: the markdown isn't highlighted", lang)
		}
	}
	if n := s.tokenised.Load(); n != 0 {
		t.Errorf("a fence ran the spy %d times", n)
	}
	if n := settle(before); n > before {
		t.Errorf("%d goroutines run after the commands, %d before", n, before)
	}
}

// A line too long to lex shows plain, and the lines around it highlight.
func TestLongLinesStayPlain(t *testing.T) {
	long := `var s = "` + strings.Repeat("x", maxLexedLine) + `"`
	m := highlighted(t, "main.go", "package main\n\n"+long+"\n\nvar t = 1\n")
	if len(m.spans) != m.Lines() {
		t.Fatalf("%d lines of tokens for %d lines", len(m.spans), m.Lines())
	}
	if m.spans[2] != nil {
		t.Errorf("the long line has tokens %v", m.spans[2])
	}
	if m.spans[0] == nil || m.spans[4] == nil {
		t.Error("the lines around the long one aren't highlighted")
	}
}

// A file opened while the lexer of the one before, cut short at its
// limit, finishes its last token still highlights.
func TestHighlightWaitsForTheLexerBefore(t *testing.T) {
	gate := syntax.Lexing()
	gate <- struct{}{}
	time.AfterFunc(50*time.Millisecond, func() { <-gate })
	if m := highlighted(t, "main.go", goSource); len(m.spans) != m.Lines() {
		t.Errorf("%d lines of tokens for %d lines", len(m.spans), m.Lines())
	}
}
