package config

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// allowPath lists the package-level constants and variables of the module
// that look like defaults but aren't settings, each with its reason, and
// root is the root of the module.
const (
	allowPath = "testdata/constants.allow"
	root      = "../.."
)

// defaultName matches the names of constants and variables that look like
// defaults.
var defaultName = regexp.MustCompile(`(?i)^default|TTL$|Interval$|Delay$|Timeout$|Wait$|PageSize$|Size$|Memory$|Capacity$`)

// TestNoDefaultsInGo fails on a package-level constant or variable in the
// code of the module that looks like a default, by its name, its type or
// its value, and that constants.allow doesn't list, and on a line of the
// list that no longer names one. A default belongs in default.yaml, where
// the user sees it and may change it; a new constant must either move
// there or be justified in the list as a true constant.
func TestNoDefaultsInGo(t *testing.T) {
	found := defaultLike(t)
	allow, problems := readAllow(t)
	for _, p := range problems {
		t.Error(p)
	}
	var add, remove []string
	for _, c := range found {
		if _, ok := allow[c]; !ok {
			add = append(add, c)
		}
	}
	for c := range allow {
		if !slices.Contains(found, c) {
			remove = append(remove, c)
		}
	}
	slices.Sort(remove)
	if len(add) > 0 {
		t.Errorf("these look like defaults: move each to default.yaml, or list it in internal/config/%s with keep: and why it is a true constant:\n%s",
			allowPath, strings.Join(add, "\n"))
	}
	if len(remove) > 0 {
		t.Errorf("internal/config/%s lists what is gone; remove:\n%s", allowPath, strings.Join(remove, "\n"))
	}
}

// defaultLike returns the package-level constants and variables of the
// code of the module that look like defaults, as path:Name.
func defaultLike(t *testing.T) []string {
	t.Helper()
	var out []string
	fset := token.NewFileSet()
	for _, dir := range []string{"cmd", "internal", "pkg"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			for _, name := range defaultDecls(f) {
				out = append(out, filepath.ToSlash(rel)+":"+name)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(out)
	return out
}

// defaultDecls returns the names of the package-level constants and
// variables of f that look like defaults.
func defaultDecls(f *ast.File) []string {
	var out []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST && gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, id := range vs.Names {
				if id.Name == "_" {
					continue
				}
				var value ast.Expr
				if i < len(vs.Values) {
					value = vs.Values[i]
				}
				if defaultName.MatchString(id.Name) || isQuantityType(vs.Type) || isQuantity(value) {
					out = append(out, id.Name)
				}
			}
		}
	}
	return out
}

// isQuantityType reports whether t is a duration or a size.
func isQuantityType(t ast.Expr) bool {
	sel, ok := t.(*ast.SelectorExpr)
	if !ok {
		id, ok := t.(*ast.Ident)
		return ok && id.Name == "Size"
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && (pkg.Name == "time" && sel.Sel.Name == "Duration" || pkg.Name == "config" && sel.Sel.Name == "Size")
}

// isQuantity reports whether e is a duration or a byte size, such as
// 5 * time.Minute or 64 * config.KiB.
func isQuantity(e ast.Expr) bool {
	if e == nil {
		return false
	}
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit, *ast.CompositeLit:
			// A function or a table holds values of its own.
			return false
		case *ast.SelectorExpr:
			if pkg, ok := n.X.(*ast.Ident); ok && pkg.Name == "time" && timeUnits[n.Sel.Name] {
				found = true
			}
		case *ast.Ident:
			if sizeUnitNames[n.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

var (
	timeUnits     = map[string]bool{"Nanosecond": true, "Microsecond": true, "Millisecond": true, "Second": true, "Minute": true, "Hour": true}
	sizeUnitNames = map[string]bool{"KiB": true, "MiB": true, "GiB": true}
)

// readAllow reads constants.allow: one path:Name per line, then keep: and
// why it is a true constant, or move: and the key it moves to. Blank
// lines and # comments are skipped.
func readAllow(t *testing.T) (allow map[string]string, problems []string) {
	t.Helper()
	f, err := os.Open(allowPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	allow = map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, reason, ok := strings.Cut(line, " keep: ")
		if !ok {
			name, reason, ok = strings.Cut(line, " move: ")
		}
		name, reason = strings.TrimSpace(name), strings.TrimSpace(reason)
		switch {
		case !ok || reason == "":
			problems = append(problems, fmt.Sprintf("%s:%d: want path:Name  keep: reason, or path:Name  move: key, got %q", allowPath, n, line))
		case allow[name] != "":
			problems = append(problems, fmt.Sprintf("%s:%d: %s is listed twice", allowPath, n, name))
		default:
			allow[name] = reason
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return allow, problems
}

func TestDefaultDecls(t *testing.T) {
	src := `package p

import "time"

const (
	DefaultLimit = 3
	pollInterval = 10
	rest         = 150 * time.Millisecond
	bound        = 8 * config.MiB
	name         = "x"
	_            = time.Second
)

var (
	wait  time.Duration
	size  config.Size
	table = map[string]time.Duration{"a": time.Second}
	count = 3
)

func f() { const inner = time.Second }
`
	f, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"DefaultLimit", "pollInterval", "rest", "bound", "wait", "size"}
	if got := defaultDecls(f); !slices.Equal(got, want) {
		t.Errorf("defaultDecls = %v, want %v", got, want)
	}
}
