package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/modals"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fakePriorities ranks the fakes of the tests. It is a table of the tests
// only: rankFakes adds it to the copy each test model keeps, so the table of
// the app never lists a fake.
var fakePriorities = map[reflect.Type]modals.Priority{}

// rankFakes gives m the ranks of the fakes of the tests.
func rankFakes(m *Model) { maps.Copy(m.priorities, fakePriorities) }

// The fakes of the tests rank as the real modals they stand for do: a content
// modal by default, a detail where the fake is a pager.
func init() {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*fakeModal](),
		reflect.TypeFor[*backModal](),
		reflect.TypeFor[*disabledBackModal](),
		reflect.TypeFor[*leavingModal](),
		reflect.TypeFor[*tabbedModal](),
		reflect.TypeFor[*overlayingModal](),
		reflect.TypeFor[*hidingModal](),
		reflect.TypeFor[*settlingModal](),
		reflect.TypeFor[*ctxModal](),
		reflect.TypeFor[*linkedModal](),
		reflect.TypeFor[*pullsModal](),
		reflect.TypeFor[*screenModal](),
		reflect.TypeFor[*savingModal](),
	} {
		fakePriorities[typ] = modals.PriorityContent
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*pagerModal](),
		reflect.TypeFor[*sourceModal](),
	} {
		fakePriorities[typ] = modals.PriorityDetail
	}
}

// previewModal is a backModal that ranks as a detail over a content modal,
// so closing it returns to what is under it.
type previewModal struct{ backModal }

func init() { fakePriorities[reflect.TypeFor[*previewModal]()] = modals.PriorityDetail }

// Update and Act close the preview itself, not the backModal inside it, which
// the manager does not know.
func (p *previewModal) Update(msg tea.Msg) tea.Cmd {
	p.msgs = append(p.msgs, msg)
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyEscape {
		return ui.CloseModal(p)
	}
	return nil
}

func (p *previewModal) Act(action string) (tea.Cmd, bool) {
	if action == ui.ActQuit {
		return ui.CloseModal(p), true
	}
	return nil, false
}

// TestEveryRealModalIsRanked checks that the table ranks the modals the app
// opens, and that a type it lacks is refused rather than opened.
func TestEveryRealModalIsRanked(t *testing.T) {
	t.Parallel()
	// The numbers are pinned here, apart from the constants, so a change of
	// the table shows up in review. Keys are the type each constructor
	// returns, a pointer.
	want := map[string]int{
		"search.Modal":        -10,
		"notifications.Modal": -10,
		"files.finderModal":   10,
		"pulls.panesModal":    20,
		"issues.detailModal":  20,
		"releases.Modal":      20,
		"history.Modal":       20,
		"actions.Modal":       20,
		"files.preview":       25,
		"actions.logModal":    25,
		"tui.textModal":       25,
		"ui.FilterModal":      30,
		"ui.OrderModal":       30,
		"tui.menuModal":       30,
		"pulls.mergeDialog":   40,
		"ui.ConfirmModal":     50,
		"tui.authModal":       60,
	}
	got := map[string]int{}
	for typ, p := range modalPriorities {
		if typ.Kind() != reflect.Pointer {
			t.Errorf("%v is not a pointer type", typ)
			continue
		}
		got[typ.Elem().String()] = int(p)
	}
	if !maps.Equal(got, want) {
		t.Errorf("table:\n got  %v\n want %v", got, want)
	}
	if n := len(modalPriorities); n != 17 {
		t.Errorf("the table ranks %d types, want 17", n)
	}

	found := modalTypes(t)
	// The job view is what the job log modal holds, never opened by itself.
	delete(found, "jobview.Model")
	for name := range found {
		if _, ok := got[name]; !ok {
			t.Errorf("modal type %s has no rank in modalPriorities", name)
		}
	}
	for name := range got {
		if !found[name] {
			t.Errorf("modalPriorities ranks %s, which is no modal type", name)
		}
	}

	m, _ := newTestApp(t)
	if _, ok := m.priorityOf(struct{ *fakeModal }{}); ok {
		t.Error("an unlisted type has a rank")
	}
	if _, ok := modalPriorities[reflect.TypeFor[*fakeModal]()]; ok {
		t.Error("the table of the app lists a fake")
	}
}

// modalMethods are those of ui.Modal that no other interface of the app has
// all of together.
var modalMethods = []string{"Title", "Update", "View", "SetSize", "SetTheme", "KeyLayers"}

// modalTypes returns the types, as "pkg.Name", of the non-test files under
// internal/tui whose method set has those of ui.Modal. An alias counts as
// the type it names, as reflect does.
func modalTypes(t *testing.T) map[string]bool {
	t.Helper()
	type pkgTypes struct {
		methods  map[string]map[string]bool // type -> method names
		embedded map[string][]string        // type -> embedded type names
		aliases  map[string]string
	}
	pkgs := map[string]*pkgTypes{} // directory -> types
	names := map[string]string{}   // directory -> package name
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		p := pkgs[dir]
		if p == nil {
			p = &pkgTypes{map[string]map[string]bool{}, map[string][]string{}, map[string]string{}}
			pkgs[dir], names[dir] = p, f.Name.Name
		}
		for _, decl := range f.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil || len(decl.Recv.List) == 0 {
					continue
				}
				recv := typeName(decl.Recv.List[0].Type)
				if p.methods[recv] == nil {
					p.methods[recv] = map[string]bool{}
				}
				p.methods[recv][decl.Name.Name] = true
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if ts.Assign.IsValid() {
						p.aliases[ts.Name.Name] = typeName(ts.Type)
					}
					if st, ok := ts.Type.(*ast.StructType); ok {
						for _, fld := range st.Fields.List {
							if len(fld.Names) == 0 {
								p.embedded[ts.Name.Name] = append(p.embedded[ts.Name.Name], typeName(fld.Type))
							}
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var has func(p *pkgTypes, typ, method string, depth int) bool
	has = func(p *pkgTypes, typ, method string, depth int) bool {
		if p.methods[typ][method] {
			return true
		}
		if depth > 4 {
			return false
		}
		for _, e := range p.embedded[typ] {
			if has(p, e, method, depth+1) {
				return true
			}
		}
		return false
	}
	found := map[string]bool{}
	for dir, p := range pkgs {
		for typ := range p.methods {
			all := true
			for _, method := range modalMethods {
				all = all && has(p, typ, method, 0)
			}
			// A section has the same methods and takes the focus; a modal
			// does not.
			if !all || has(p, typ, "Focus", 0) {
				continue
			}
			target := typ
			if to, ok := p.aliases[typ]; ok {
				target = to
			}
			found[names[dir]+"."+target] = true
		}
		for _, to := range p.aliases {
			all := true
			for _, method := range modalMethods {
				all = all && has(p, to, method, 0)
			}
			if all && !has(p, to, "Focus", 0) {
				found[names[dir]+"."+to] = true
			}
		}
	}
	return found
}

// typeName is the name a type expression names, without pointer or package.
func typeName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.StarExpr:
		return typeName(e.X)
	case *ast.IndexExpr:
		return typeName(e.X)
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}
