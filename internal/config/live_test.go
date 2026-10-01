package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestLiveSettingsAreRead fails on a setting that the set command changes
// while the app runs, since it isn't tagged startup, and that the path the
// set command applies settings through doesn't read: a setting must be
// applied there, or be tagged when:"startup" with why. So dropping the
// tag of a setting read only at startup fails too.
//
// The path is what reads the settings each time they change:
//   - the functions named configure, Configure, applySettings and
//     applyTheme in internal/tui, which the set command's settings reach;
//   - in cmd/gh-tui, the function given to tui.WithSettings, what reads
//     live.cfg, the config as the set command left it, and the functions,
//     declared or held by a variable, that are given live.cfg.
//
// It reads the code, not the types: a setting counts as read where a
// chain of selectors from the config names it, as c.History.Row, through
// a variable, as p := c.Details.Prefetch then p.Rows, or through an
// accessor of this package, as c.DashboardPrefetch(). A group of settings
// read whole, as c.History given to the History modal, counts for each
// setting in it; the whole config given on doesn't.
func TestLiveSettingsAreRead(t *testing.T) {
	read := liveReads(t)
	for _, group := range unapplied {
		for key := range read {
			if key == group || strings.HasPrefix(key, group+".") {
				t.Errorf("%s is applied now, so take %s off the list of groups nothing applies yet", key, group)
				break
			}
		}
	}
	for _, key := range Keys() {
		if _, ok := Startup(key); ok || isUnapplied(key) {
			continue
		}
		if !read[key] {
			t.Errorf("%s changes while the app runs, but nothing applies it when the settings change: apply it where they are, or tag it when:\"startup\" with why", key)
		}
	}
}

// unapplied are the groups of settings that the config holds, checks and
// resolves, and that nothing reads yet. A group leaves the list once
// anything applies it.
var unapplied []string

// isUnapplied reports whether key is in a group of unapplied.
func isUnapplied(key string) bool {
	for _, group := range unapplied {
		if key == group || strings.HasPrefix(key, group+".") {
			return true
		}
	}
	return false
}

// liveReads returns the settings that the path the set command applies
// settings through reads.
func liveReads(t *testing.T) map[string]bool {
	t.Helper()
	acc := accessors(t)
	read := map[string]bool{}
	fset := token.NewFileSet()
	for _, dir := range []string{"internal/tui", "cmd/gh-tui"} {
		pkgs := map[string][]*ast.File{}
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			pkgs[filepath.Dir(path)] = append(pkgs[filepath.Dir(path)], f)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, files := range pkgs {
			for _, scope := range applyScopes(files) {
				scope.reads(acc, read)
			}
		}
	}
	return read
}

// scope is a function on the path that applies settings, and the names
// that hold the config in it.
type scope struct {
	body  ast.Node
	roots map[string]bool
}

// applyScopes returns the functions of the files of one package that are
// on the path that applies settings.
func applyScopes(files []*ast.File) []scope {
	var out []scope
	given := map[string]bool{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// Functions given live.cfg read the session's config.
			if id, ok := call.Fun.(*ast.Ident); ok && hasArg(call, "live.cfg") {
				given[id.Name] = true
			}
			// What tui.WithSettings is given runs when the settings change.
			if chain(call.Fun) == "tui.WithSettings" && len(call.Args) == 1 {
				if lit, ok := call.Args[0].(*ast.FuncLit); ok {
					out = append(out, scope{body: lit.Body, roots: configParams(lit.Type, "live.cfg")})
				}
			}
			return true
		})
	}
	for _, f := range files {
		// Functions held by a variable, as historyOpts := func(c config.Config).
		ast.Inspect(f, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != len(as.Rhs) {
				return true
			}
			for i, l := range as.Lhs {
				id, ok := l.(*ast.Ident)
				lit, isLit := as.Rhs[i].(*ast.FuncLit)
				if ok && isLit && given[id.Name] {
					out = append(out, scope{body: lit.Body, roots: configParams(lit.Type, "live.cfg")})
				}
			}
			return true
		})
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			switch name := fn.Name.Name; {
			case name == "configure" || name == "Configure" || name == "applySettings" || name == "applyTheme" || given[name]:
				// The root model's config is the session's.
				out = append(out, scope{body: fn.Body, roots: configParams(fn.Type, "live.cfg", "m.cfg")})
			default:
				// live.cfg is the session's config wherever it is read.
				out = append(out, scope{body: fn.Body, roots: configParams(&ast.FuncType{Params: &ast.FieldList{}}, "live.cfg")})
			}
		}
	}
	return out
}

// configParams returns the names of the parameters of ft of type
// config.Config, and more.
func configParams(ft *ast.FuncType, more ...string) map[string]bool {
	out := map[string]bool{}
	for _, m := range more {
		out[m] = true
	}
	for _, p := range ft.Params.List {
		if chain(p.Type) == "config.Config" {
			for _, n := range p.Names {
				out[n.Name] = true
			}
		}
	}
	return out
}

func hasArg(call *ast.CallExpr, want string) bool {
	for _, a := range call.Args {
		if chain(a) == want {
			return true
		}
	}
	return false
}

// chain returns e spelled as a chain of selectors, such as "c.UI.Icons",
// or "" if it is something else.
func chain(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if x := chain(e.X); x != "" {
			return x + "." + e.Sel.Name
		}
	}
	return ""
}

// reads adds to read the settings that s reads.
func (s scope) reads(acc map[string][][]string, read map[string]bool) {
	// Variables that hold a group of settings, as p := c.Details.Prefetch.
	alias := map[string][]string{}
	skip := map[ast.Expr]bool{}
	ast.Inspect(s.body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.DEFINE || len(as.Lhs) != len(as.Rhs) {
			return true
		}
		for i, l := range as.Lhs {
			id, ok := l.(*ast.Ident)
			if !ok {
				continue
			}
			if p := s.resolve(as.Rhs[i], alias); len(p) > 0 && isGroup(p) {
				alias[id.Name] = p
				skip[as.Rhs[i]] = true
			}
		}
		return true
	})
	ast.Inspect(s.body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if !skip[sel] {
			if p := s.resolve(sel, alias); len(p) > 0 {
				markRead(p, acc, read)
			}
		}
		// The longest chain alone says what is read.
		return false
	})
}

// resolve returns the names that e selects from the config, such as
// [Details Prefetch Rows], or nil if e doesn't start from it.
func (s scope) resolve(e ast.Expr, alias map[string][]string) []string {
	c := chain(e)
	if c == "" {
		return nil
	}
	for root := range s.roots {
		if rest, ok := strings.CutPrefix(c, root+"."); ok {
			return strings.Split(rest, ".")
		}
	}
	first, rest, _ := strings.Cut(c, ".")
	if p, ok := alias[first]; ok {
		if rest == "" {
			return p
		}
		return append(append([]string(nil), p...), strings.Split(rest, ".")...)
	}
	return nil
}

// isGroup reports whether the names p select a group of settings from
// Config.
func isGroup(p []string) bool {
	t := reflect.TypeFor[Config]()
	for _, name := range p {
		f, ok := t.FieldByName(name)
		if !ok {
			return false
		}
		t = f.Type
	}
	return t.Kind() == reflect.Struct
}

// markRead marks read the settings that the names p select from Config:
// the setting, every setting of the group, or, for an accessor, what the
// accessor reads.
func markRead(p []string, acc map[string][][]string, read map[string]bool) {
	t := reflect.TypeFor[Config]()
	for i, name := range p {
		if name == "" {
			return
		}
		f, ok := t.FieldByName(name)
		if !ok {
			for _, sub := range acc[t.Name()+"."+name] {
				markRead(append(append([]string(nil), p[:i]...), sub...), acc, read)
			}
			return
		}
		t = f.Type
	}
	for _, key := range Keys() {
		if g := goPath(key); len(g) >= len(p) && slices.Equal(g[:len(p)], p) {
			read[key] = true
		}
	}
}

// accessors returns, for each method of this package that reads settings
// for others, such as Config.DashboardPrefetch, what it reads, relative
// to its receiver, where an empty path is the receiver whole. Validate, Get, Set and Values read every setting, and
// so say nothing of whether one is applied.
func accessors(t *testing.T) map[string][][]string {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][][]string{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil || len(fn.Recv.List[0].Names) == 0 {
				continue
			}
			typ := fn.Recv.List[0].Type
			if star, ok := typ.(*ast.StarExpr); ok {
				typ = star.X
			}
			switch fn.Name.Name {
			case "Validate", "Get", "Set", "Values", "validate":
				continue
			}
			recv := fn.Recv.List[0].Names[0].Name
			s := scope{body: fn.Body, roots: map[string]bool{recv: true}}
			name := chain(typ) + "." + fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.Ident:
					// The prefetch settings used whole, as Resolve
					// gives them to reflect, read every setting in them.
					// Other methods that take their receiver whole, as
					// clone does, apply nothing.
					if n.Name == recv && chain(typ) == "PrefetchLayers" {
						out[name] = append(out[name], []string{})
					}
				case *ast.SelectorExpr:
					if p := s.resolve(n, nil); len(p) > 0 {
						out[name] = append(out[name], p)
					}
					return false
				}
				return true
			})
		}
	}
	return out
}

// goPath returns the names of the fields of Config that key names, such
// as [Details Prefetch Rows].
func goPath(key string) []string {
	var out []string
	t := reflect.TypeFor[Config]()
	for part := range strings.SplitSeq(key, ".") {
		f, ok := fieldByYAML(t, part)
		if !ok {
			return nil
		}
		out = append(out, f.Name)
		t = f.Type
	}
	return out
}

// TestResolveReadsEveryKnob checks that a list that applies its settings
// with Resolve counts as reading every knob of prefetch, which Resolve
// reaches through reflection rather than by name.
func TestResolveReadsEveryKnob(t *testing.T) {
	read := map[string]bool{}
	markRead([]string{"Prefetch", "Resolve"}, accessors(t), read)
	for _, key := range []string{"prefetch.enabled", "prefetch.pulls.window.after", "prefetch.files.preview.rest"} {
		if !read[key] {
			t.Errorf("Resolve doesn't count as reading %s", key)
		}
	}
}
