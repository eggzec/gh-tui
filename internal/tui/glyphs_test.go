package tui

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// glyphLiterals are the glyphs that the ASCII icon set replaces. A string
// literal that holds one shows it to a user who chose ASCII.
var glyphLiterals = []string{"·", "…", "...", "✓", "✗", "✔", "✘", "●", "○"}

// TestNoGlyphLiterals fails on a glyph written in a string of non-test Go
// under internal/tui, which must come from ui.Icons instead. The files that
// may hold one, each with its reason, are in testdata/glyphs.allow as
// "path: reason" lines, the path relative to this directory.
func TestNoGlyphLiterals(t *testing.T) {
	t.Parallel()
	allowed := map[string]bool{}
	f, err := os.Open(filepath.Join("testdata", "glyphs.allow"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, reason, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(reason) == "" {
			t.Fatalf("glyphs.allow: %q needs a reason after the path", line)
		}
		allowed[strings.TrimSpace(path)] = true
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		path = filepath.ToSlash(path)
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == "ui/icons.go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, g := range glyphLiterals {
				if !strings.Contains(s, g) {
					continue
				}
				if allowed[path] {
					used[path] = true
				} else {
					t.Errorf("%s: the string %s holds %q; take it from ui.Icons", fset.Position(lit.Pos()), lit.Value, g)
				}
				break
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for p := range allowed {
		if !used[p] {
			t.Errorf("glyphs.allow lists %s, which holds no glyph literal; remove it", p)
		}
	}
}
