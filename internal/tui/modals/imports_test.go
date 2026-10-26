package modals_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	modulePath = "github.com/eggzec/gh-tui/"
	modalsPath = modulePath + "internal/tui/modals"
	tuiPath    = modulePath + "internal/tui"
)

// importsOf returns the import paths of each non-test file of dir, or of its
// test files when tests is set.
func importsOf(t *testing.T, dir string, tests bool) map[string][]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") != tests {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			out[f] = append(out[f], p)
		}
	}
	return out
}

func TestOnlyTheRootImportsModals(t *testing.T) {
	t.Parallel()
	dirs, err := filepath.Glob("../*")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() || filepath.Base(dir) == "modals" {
			continue
		}
		for _, tests := range []bool{false, true} {
			for file, imps := range importsOf(t, dir, tests) {
				for _, p := range imps {
					if p == modalsPath {
						t.Errorf("%s imports %s; only internal/tui may", file, modalsPath)
					}
				}
			}
		}
	}
}

func TestModalsImportsOnlyUI(t *testing.T) {
	t.Parallel()
	for file, imps := range importsOf(t, ".", false) {
		for _, p := range imps {
			if !strings.HasPrefix(p, tuiPath) {
				continue
			}
			if p != tuiPath+"/ui" {
				t.Errorf("%s imports %s; only internal/tui/ui is allowed", file, p)
			}
		}
	}
}
