package syntax

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
)

// name returns the name of l, or "" for none.
func name(l chroma.Lexer) string {
	if l == nil {
		return ""
	}
	return l.Config().Name
}

func TestLexer(t *testing.T) {
	for lang, want := range map[string]string{
		"go": "Go", "GO": "Go", "golang": "Go", "diff": "Diff", "sh": "Bash",
		// Lexers that never finish on some code, or hand it to one.
		"jsonata": "", "jungle": "", "markdown": "", "http": "", "postgresql": "",
		"no-such-lang": "",
	} {
		if got := name(Lexer(lang)); got != want {
			t.Errorf("Lexer(%q) is %q, want %q", lang, got, want)
		}
	}
}

func TestFile(t *testing.T) {
	tests := []struct {
		file, text, want string
	}{
		{file: "main.go", want: "Go"},
		{file: "dir/a.json", want: "JSON"},
		{file: "Dockerfile", want: "Docker"},
		{file: "Makefile", want: "Makefile"},
		{file: "README.md", want: "markdown"},
		{file: "notes.txt", want: "plaintext"},
		{file: "run", text: "#!/bin/sh\necho hi\n", want: "Bash"},
		{file: "run", text: "#!/usr/bin/env -S python3.12 -u\n", want: "Python"},
		{file: "run", text: "#!/usr/bin/env\n"},
		{file: "run", text: "#!/usr/bin/jsonata\n"},
		{file: "notes", text: "\\"},
		{file: "notes", text: "package main\n\nfunc main() {}\n"},
		{file: "a.jsonata", text: "\\"},
		{file: "a.jungle", text: "\\"},
		// The name picks a lexer, which isn't highlighted.
		{file: "a.jsonata", text: "#!/bin/sh\n"},
	}
	for _, tt := range tests {
		if got := name(File(tt.file, tt.text)); got != tt.want {
			t.Errorf("File(%q, %q) is %q, want %q", tt.file, tt.text, got, tt.want)
		}
	}
}

// Markdown's fences reach only the lexers of common languages.
func TestMarkdownFencesAreConfined(t *testing.T) {
	idle(t)
	before := runtime.NumGoroutine()
	l := File("README.md", "")
	for _, lang := range []string{"jsonata", "jungle", "x.jsonata", "markdown", "http"} {
		code := "# Title\n\n```" + lang + "\n\\\n```\n"
		if toks, err := Head(t.Context(), l, code, 10*time.Second); err != nil || text(toks) != code {
			t.Errorf("%s: lexed %q, %v", lang, text(toks), err)
		}
	}
	if n := settle(before); n > before {
		t.Errorf("%d goroutines run after lexing, %d before", n, before)
	}
	toks, _ := Head(t.Context(), l, "```go\nfunc f() {}\n```\n", 10*time.Second)
	if !has(toks, chroma.KeywordDeclaration) {
		t.Errorf("the Go in a fence isn't highlighted: %v", toks)
	}
}

func text(toks []chroma.Token) string {
	var b strings.Builder
	for _, t := range toks {
		b.WriteString(t.Value)
	}
	return b.String()
}

func has(toks []chroma.Token, typ chroma.TokenType) bool {
	for _, t := range toks {
		if t.Type == typ {
			return true
		}
	}
	return false
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

// idle waits for a lexer that overran to end.
func idle(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	select {
	case lexing <- struct{}{}:
		<-lexing
	case <-ctx.Done():
		t.Fatal("a lexer still runs")
	}
}
