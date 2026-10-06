package tui

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// allowDir holds the collisions that are meant, each with its reason, in
// a file per context, so that changes to different contexts don't touch
// the same file. allowRoot names it from the root of the module.
//
// The file of a context is named by contextFile. It holds one collision
// per line:
//
//	context | key | loser | winner | reason
//
// context names where the keys are, as keyContexts in keyctx_test.go
// does. loser is the binding that doesn't get key, and winner the one that
// does, each as "layer source: description", the way the help shows them.
// reason says in a line why the collision is meant. Blank lines and lines
// that start with # are left out.
//
// A binding shadowed by an earlier layer, such as the app's, needs a line
// as much as a conflict within one layer: the help marks both.
const (
	allowDir  = "testdata/collisions"
	allowRoot = "internal/tui/" + allowDir
)

// contextFile returns the base name of the files of a context, such as
// "owner-stars" for "owner: stars".
func contextFile(context string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(context, ":", "")), "-"))
}

// allowFile returns the allowlist file of a context, from the root of the
// module.
func allowFile(context string) string {
	return path.Join(allowRoot, contextFile(context)+".allow")
}

// collision is a key that one binding loses to another, in a context: a
// conflict within one layer, or a binding shadowed by an earlier layer.
// The help marks both with ⚠, so both must be meant.
type collision struct {
	context, key string
	// loser and winner name each binding as its layer's source and its
	// description, as the help shows them.
	loser, winner string
}

// String formats c as a line of the allowlist, without its reason.
func (c collision) String() string {
	return strings.Join([]string{c.context, c.key, c.loser, c.winner}, " | ")
}

// collisionsOf returns the collisions of layers in context, as Analyze
// finds them for the help.
func collisionsOf(context string, layers []keyhelp.Layer) []collision {
	var out []collision
	for _, r := range keyhelp.Analyze(layers) {
		for _, l := range r.Lost {
			if l.Status != keyhelp.Conflict && l.Status != keyhelp.Shadowed {
				continue
			}
			out = append(out, collision{
				context: context, key: l.Key,
				loser:  r.Source + ": " + r.Binding.Help().Desc,
				winner: l.Source + ": " + l.By.Help().Desc,
			})
		}
	}
	return out
}

// todo stands for the reason of a line the test asks to add, which the
// developer must replace.
const todo = "TODO say why"

// parseAllow reads an allowlist: a collision per line and its reason,
// separated by " | ". Blank lines and those starting with # are left out.
// It returns what it read, and the problems of the lines it couldn't.
func parseAllow(r io.Reader, name string) (allow map[collision]string, problems []string) {
	allow = map[collision]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, " | ")
		if len(fields) != 5 {
			problems = append(problems, fmt.Sprintf("%s:%d: want context | key | loser | winner | reason, got %q", name, n, line))
			continue
		}
		c := collision{context: fields[0], key: fields[1], loser: fields[2], winner: fields[3]}
		switch reason := strings.TrimSpace(fields[4]); {
		case reason == "" || reason == todo:
			problems = append(problems, fmt.Sprintf("%s:%d: say why %s is meant", name, n, c))
		case allow[c] != "":
			problems = append(problems, fmt.Sprintf("%s:%d: %s is listed twice", name, n, c))
		default:
			allow[c] = reason
		}
	}
	if err := sc.Err(); err != nil {
		problems = append(problems, fmt.Sprintf("%s: %v", name, err))
	}
	return allow, problems
}

// checkAllow returns the lines to add to allow for the collisions found
// that it doesn't list, and those to remove, which list collisions no
// longer found, each sorted.
func checkAllow(found map[collision]bool, allow map[collision]string) (add, remove []string) {
	for c := range found {
		if _, ok := allow[c]; !ok {
			add = append(add, c.String()+" | "+todo)
		}
	}
	for c, reason := range allow {
		if !found[c] {
			remove = append(remove, c.String()+" | "+reason)
		}
	}
	slices.Sort(add)
	slices.Sort(remove)
	return add, remove
}

// readAllow reads every file of allowDir, as parseAllow does, and finds a
// problem in a line that is in the file of another context than its own.
func readAllow() (allow map[collision]string, problems []string) {
	allow = map[collision]string{}
	files, err := filepath.Glob(filepath.Join(allowDir, "*.allow"))
	if err != nil {
		return nil, []string{err.Error()}
	}
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		file := path.Join(allowRoot, filepath.Base(name))
		read, more := parseAllow(f, file)
		f.Close()
		problems = append(problems, more...)
		for c, reason := range read {
			if want := allowFile(c.context); want != file {
				problems = append(problems, fmt.Sprintf("%s: %s belongs in %s", file, c, want))
				continue
			}
			allow[c] = reason
		}
	}
	return allow, problems
}

// byFile returns lines of the allowlist under the file each belongs in.
func byFile(lines []string) string {
	files := map[string][]string{}
	for _, l := range lines {
		context, _, _ := strings.Cut(l, " | ")
		files[allowFile(context)] = append(files[allowFile(context)], l)
	}
	var b strings.Builder
	for _, f := range slices.Sorted(maps.Keys(files)) {
		b.WriteString(f + ":\n" + strings.Join(files[f], "\n") + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// TestCollisions checks every key that two bindings hold in a context of
// the app against the allowlist: a collision it doesn't list fails, and
// so does a line that lists one no context has.
func TestCollisions(t *testing.T) {
	allow, problems := readAllow()
	for _, p := range problems {
		t.Error(p)
	}
	found := map[collision]bool{}
	for _, c := range keyContexts() {
		synctest.Test(t, func(t *testing.T) {
			for _, col := range collisionsOf(c.name, c.layers(t)) {
				found[col] = true
			}
		})
	}
	add, remove := checkAllow(found, allow)
	if len(add) > 0 {
		t.Errorf("new collisions: rebind the key, or add these lines to the files named, each with the reason it is meant in place of %q:\n%s",
			todo, byFile(add))
	}
	if len(remove) > 0 {
		t.Errorf("no context has these collisions any more: remove these lines from the files named:\n%s",
			byFile(remove))
	}
}

// TestAllowList checks that the allowlist is read line by line, that a
// line without a reason or listed twice is a problem, and that what to add
// and remove is named exactly.
func TestAllowList(t *testing.T) {
	a := collision{context: "issues", key: "f", loser: "list: page down", winner: "app: filter"}
	b := collision{context: "issues", key: "s", loser: "Issues: sort", winner: "app: sort"}
	c := collision{context: "pull requests", key: "f", loser: "list: page down", winner: "app: filter"}
	text := strings.Join([]string{
		"# a comment",
		"",
		a.String() + " | f opens the filter",
		b.String() + " | the app sorts",
		b.String() + " | again",
		"issues | x | too few",
		c.String() + " | " + todo,
		c.String() + " | ",
	}, "\n")
	allow, problems := parseAllow(strings.NewReader(text), "allow")
	want := []string{
		"allow:5: " + b.String() + " is listed twice",
		`allow:6: want context | key | loser | winner | reason, got "issues | x | too few"`,
		"allow:7: say why " + c.String() + " is meant",
		`allow:8: want context | key | loser | winner | reason, got "` + c.String() + ` |"`,
	}
	if !slices.Equal(problems, want) {
		t.Errorf("problems:\n%s\nwant:\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
	if len(allow) != 2 || allow[a] != "f opens the filter" || allow[b] != "the app sorts" {
		t.Errorf("read %v", allow)
	}
	add, remove := checkAllow(map[collision]bool{a: true, c: true}, allow)
	if want := []string{c.String() + " | " + todo}; !slices.Equal(add, want) {
		t.Errorf("add %q, want %q", add, want)
	}
	if want := []string{b.String() + " | the app sorts"}; !slices.Equal(remove, want) {
		t.Errorf("remove %q, want %q", remove, want)
	}
}
