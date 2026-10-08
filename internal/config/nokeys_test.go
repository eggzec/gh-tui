package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// keysAllowPath lists the key names that code of the module may hold, as
// the declaration that holds them, with its reason.
const keysAllowPath = "testdata/keys.allow"

// TestNoKeysInGo fails on a key name written in the code of the module
// that keys.allow doesn't list, and on a line of the list that no longer
// names one. A key name is written when code
//
//   - passes a string, a constant or a list of strings to key.WithKeys or
//     SetKeys, spread or not;
//   - binds an action with Keymap.Set to a list of strings;
//   - writes a config.Keymap literal;
//   - compares what a key press's String returns to a string, or switches
//     on it.
//
// Keys belong in default.yaml, where the user sees and changes them. The
// editing keys of the text input and area stay with the library.
func TestNoKeysInGo(t *testing.T) {
	found := keyLiterals(t)
	allow, problems := readAllow(t, keysAllowPath)
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
		t.Errorf("these write a key in Go: move it to default.yaml and read it from the config, or list the declaration in internal/config/%s with keep: and why the key can't be configured:\n%s",
			keysAllowPath, strings.Join(add, "\n"))
	}
	if len(remove) > 0 {
		t.Errorf("internal/config/%s lists what is gone; remove:\n%s", keysAllowPath, strings.Join(remove, "\n"))
	}
}

// keyLiterals returns the top-level declarations of the code of the module
// that write a key name, as path:Name.
func keyLiterals(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, dir := range []string{"cmd", "internal", "pkg"} {
		byDir := map[string][]*ast.File{}
		paths := map[*ast.File]string{}
		fset := token.NewFileSet()
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			byDir[filepath.Dir(path)] = append(byDir[filepath.Dir(path)], f)
			paths[f] = path
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, files := range byDir {
			strs := stringNames(files)
			for _, f := range files {
				rel, err := filepath.Rel(root, paths[f])
				if err != nil {
					t.Fatal(err)
				}
				for _, name := range keyDecls(f, strs) {
					out = append(out, filepath.ToSlash(rel)+":"+name)
				}
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// stringNames returns the names of the package-level constants and
// variables of files that hold a string or a list of strings written in
// the code, so that passing one to WithKeys writes a key too.
func stringNames(files []*ast.File) map[string]bool {
	names := map[string]bool{}
	// A name may be defined from another, in any order, so go on until
	// nothing is added.
	for added := true; added; {
		added = false
		for _, f := range files {
			for _, decl := range f.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.CONST && gd.Tok != token.VAR {
					continue
				}
				for _, spec := range gd.Specs {
					vs := spec.(*ast.ValueSpec)
					for i, id := range vs.Names {
						if i < len(vs.Values) && !names[id.Name] && isWritten(vs.Values[i], names) {
							names[id.Name] = true
							added = true
						}
					}
				}
			}
		}
	}
	return names
}

// isWritten reports whether e is a string or a list of strings written in
// the code: a literal, a name of one, a concatenation of them, a list
// literal of them, or such a list with others appended.
func isWritten(e ast.Expr, names map[string]bool) bool {
	switch e := e.(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.Ident:
		return names[e.Name]
	case *ast.ParenExpr:
		return isWritten(e.X, names)
	case *ast.BinaryExpr:
		return e.Op == token.ADD && (isWritten(e.X, names) || isWritten(e.Y, names))
	case *ast.CompositeLit:
		if arr, ok := e.Type.(*ast.ArrayType); !ok || !isString(arr.Elt) {
			return false
		}
		return slices.ContainsFunc(e.Elts, func(x ast.Expr) bool { return isWritten(x, names) })
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "append" {
			return slices.ContainsFunc(e.Args, func(x ast.Expr) bool { return isWritten(x, names) })
		}
	}
	return false
}

// isString reports whether t is the type string.
func isString(t ast.Expr) bool {
	id, ok := t.(*ast.Ident)
	return ok && id.Name == "string"
}

// keyDecls returns the names of the top-level declarations of f that write
// a key name, given strs, the names of the package's strings (see
// stringNames). A method is named by its receiver and name, as in
// Model.Update.
func keyDecls(f *ast.File, strs map[string]bool) []string {
	var out []string
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if writesKey(d, strs) {
				out = append(out, funcName(d))
			}
		case *ast.GenDecl:
			// A grouped declaration names each of its specs.
			for _, spec := range d.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) > 0 && writesKey(vs, strs) {
					out = append(out, vs.Names[0].Name)
				}
			}
		}
	}
	return out
}

// funcName names a function, or a method with its receiver.
func funcName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	t := d.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if idx, ok := t.(*ast.IndexExpr); ok {
		t = idx.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + d.Name.Name
	}
	return d.Name.Name
}

// writesKey reports whether n writes a key name: see TestNoKeysInGo.
func writesKey(n ast.Node, strs map[string]bool) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n := n.(type) {
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			var name string
			if ok {
				name = sel.Sel.Name
			} else if id, isID := n.Fun.(*ast.Ident); isID {
				name = id.Name
			}
			switch {
			case name == "WithKeys" || name == "SetKeys":
				found = slices.ContainsFunc(n.Args, func(a ast.Expr) bool { return isWritten(a, strs) })
			case name == "Set" && ok && len(n.Args) == 2:
				_, list := n.Args[1].(*ast.CompositeLit)
				found = list && isWritten(n.Args[1], strs)
			}
		case *ast.CompositeLit:
			found = isKeymapType(n.Type)
		case *ast.BinaryExpr:
			if n.Op == token.EQL || n.Op == token.NEQ {
				found = isStringCall(n.X) && isWritten(n.Y, strs) || isStringCall(n.Y) && isWritten(n.X, strs)
				found = found && !isEmptyString(n.X) && !isEmptyString(n.Y)
			}
		case *ast.SwitchStmt:
			if isStringCall(n.Tag) {
				for _, c := range n.Body.List {
					for _, e := range c.(*ast.CaseClause).List {
						found = found || isWritten(e, strs) && !isEmptyString(e)
					}
				}
			}
		}
		return !found
	})
	return found
}

// isEmptyString reports whether e is "".
func isEmptyString(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && (lit.Value == `""` || lit.Value == "``")
}

// isStringCall reports whether e is a call of a method named String
// without arguments, as a key press names its key.
func isStringCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "String"
}

// isKeymapType reports whether t is config.Keymap, Keymap, or the map it
// is.
func isKeymapType(t ast.Expr) bool {
	switch t := t.(type) {
	case *ast.SelectorExpr:
		pkg, ok := t.X.(*ast.Ident)
		return ok && pkg.Name == "config" && t.Sel.Name == "Keymap"
	case *ast.Ident:
		return t.Name == "Keymap"
	case *ast.MapType:
		inner, ok := t.Value.(*ast.MapType)
		if !ok {
			return false
		}
		list, ok := inner.Value.(*ast.ArrayType)
		return ok && isString(list.Elt)
	}
	return false
}

// TestKeyDecls pins each way of writing a key name, and some that write
// none, each as a fixture of its own.
func TestKeyDecls(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"literal", `var quit = key.NewBinding(key.WithKeys("ctrl+c"))`, []string{"quit"}},
		{"constant", `const k = "x"
func f() { key.NewBinding(key.WithKeys(k)) }`, []string{"f"}},
		{"constant expression", `const k = "x"
func f() { key.NewBinding(key.WithKeys(k + "y")) }`, []string{"f"}},
		{"constant from another", `const a = "x"
const b = a
func f() { b2 := key.WithKeys(b); _ = b2 }`, []string{"f"}},
		{"list literal spread", `func f() { key.NewBinding(key.WithKeys([]string{"a", "b"}...)) }`, []string{"f"}},
		{"list variable spread", `var ks = []string{"a"}
func f() { key.NewBinding(key.WithKeys(ks...)) }`, []string{"f"}},
		{"list with append", `func f() { b.SetKeys(append([]string{"a"}, more...)...) }`, []string{"f"}},
		{"grouped", `var (
	a = key.WithKeys("x")
	b = key.WithKeys(other...)
	c = key.WithKeys("y")
)`, []string{"a", "c"}},
		{"method", `func (m *Model[T]) Update() { b.SetKeys("x") }`, []string{"Model.Update"}},
		{"keys from elsewhere", `func f(k []string) { b.SetKeys(k...); key.WithKeys(other.Keys()...) }`, nil},
		{"keymap set", `func f(k config.Keymap) { k.Set("pulls.merge", []string{"m"}) }`, []string{"f"}},
		{"keymap set from elsewhere", `func f(k config.Keymap, ks []string) { k.Set("pulls.merge", ks) }`, nil},
		{"keymap literal", `var k = config.Keymap{"pulls": {"merge": nil}}`, []string{"k"}},
		{"keymap map literal", `var k = map[string]map[string][]string{}`, []string{"k"}},
		{"keymap make", `func f() { _ = make(config.Keymap) }`, nil},
		{"string compare", `func f(msg tea.KeyPressMsg) bool { return msg.String() == "q" }`, []string{"f"}},
		{"string compare reversed", `func f(msg tea.KeyPressMsg) bool { return "q" != msg.String() }`, []string{"f"}},
		{"string compare constant", `const q = "q"
func f(msg tea.KeyPressMsg) bool { return msg.String() == q }`, []string{"f"}},
		{"string compare with empty", `func f(v fmt.Stringer) bool { return v.String() == "" }`, nil},
		{"string switch", `func f(msg tea.KeyPressMsg) { switch msg.String() { case "q": } }`, []string{"f"}},
		{"switch on something else", `func f(s string) { switch s { case "q": } }`, nil},
		{"compare something else", `func f(s string) bool { return s == "q" }`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parser.ParseFile(token.NewFileSet(), "p.go", "package p\n"+tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := keyDecls(f, stringNames([]*ast.File{f})); !slices.Equal(got, tt.want) {
				t.Errorf("keyDecls = %v, want %v", got, tt.want)
			}
		})
	}
}
