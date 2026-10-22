package search

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// usersSection returns a page over accounts, on the Users kind with the
// results focused, and the query not yet submitted.
func usersSection(t *testing.T) (*Section, *fakeService) {
	t.Helper()
	f := newFake()
	f.users = []core.SearchUser{
		{Login: "teaman", Kind: core.OwnerUser},
		{Login: "teaworks", Kind: core.OwnerOrg},
	}
	s := newSection(t, f, 100, 24)
	press(t, s, "esc")
	press(t, s, strconv.Itoa(slices.Index(kinds, core.SearchUsers)+1))
	return s, f
}

func TestUsersKindLoadsOnSubmit(t *testing.T) {
	t.Parallel()
	s, f := usersSection(t)
	typeOnly(t, s, "tea")
	if len(f.userQueries) != 0 {
		t.Fatalf("searched users %v before the query was submitted", f.userQueries)
	}
	press(t, s, "enter")
	if len(f.userQueries) != 1 || f.userQueries[0] != (search.UsersQuery{Text: "tea", Kind: core.SearchAll}) {
		t.Fatalf("searched users with %+v, want one query for both kinds", f.userQueries)
	}
}

func TestUserRowGlyphAndLogin(t *testing.T) {
	t.Parallel()
	s, _ := usersSection(t)
	typeText(t, s, "tea")
	view := s.View()
	var user, org string
	for line := range strings.Lines(view) {
		switch {
		case strings.Contains(line, "teaman"):
			user = line
		case strings.Contains(line, "teaworks"):
			org = line
		}
	}
	if !strings.Contains(user, s.icons.Person) || strings.Contains(user, s.icons.Org) {
		t.Errorf("row of a user = %q, want the person glyph", user)
	}
	if !strings.Contains(org, s.icons.Org) || strings.Contains(org, s.icons.Person) {
		t.Errorf("row of an organization = %q, want the organization glyph", org)
	}
}

func TestUserEnterOpensOwner(t *testing.T) {
	t.Parallel()
	s, _ := usersSection(t)
	typeText(t, s, "tea")
	press(t, s, "down")
	app := press(t, s, "enter")
	if len(app) != 1 {
		t.Fatalf("enter sent %v, want one message", app)
	}
	if got, ok := app[0].(ui.OwnerMsg); !ok || got.Login != "teaworks" {
		t.Errorf("enter sent %#v, want the owner view of teaworks", app[0])
	}
}

func TestUsersFilterSpec(t *testing.T) {
	t.Parallel()
	sp := spec(core.SearchUsers, "")
	keys := make([]string, 0, len(sp.Fields))
	for _, f := range sp.Fields {
		keys = append(keys, f.Key)
	}
	if !slices.Contains(keys, "type") {
		t.Errorf("fields %v lack type", keys)
	}
	if sp.Sort == nil || len(sp.Sort.Options) != 4 {
		t.Fatalf("sort = %+v, want best match, followers, repositories and joined", sp.Sort)
	}
}

func TestUsersNoCount(t *testing.T) {
	t.Parallel()
	s, _ := usersSection(t)
	typeText(t, s, "tea")
	if m := regexp.MustCompile(`Users\W*\d`).FindString(s.View()); m != "" {
		t.Errorf("view shows a count for the users: %q", m)
	}
}
