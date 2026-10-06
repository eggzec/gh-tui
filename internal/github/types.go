package github

import (
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// JSON shapes shared by the domain methods. REST and GraphQL use the same
// field names for these, so one shape decodes both.

type user struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

func (u user) core() core.User {
	return core.User{Login: u.Login, Name: u.Name}
}

// actor is the GraphQL author of a pull request or issue, whose
// __typename tells an app, such as dependabot, from a person.
type actor struct {
	user
	Typename string `json:"__typename"`
}

func (a actor) core() core.User {
	u := a.user.core()
	u.Bot = a.Typename == "Bot"
	return u
}

// account is the REST author of an issue, whose type tells an app, such
// as dependabot[bot], from a person.
type account struct {
	user
	Type string `json:"type"`
}

func (a account) core() core.User {
	u := a.user.core()
	u.Bot = a.Type == "Bot"
	return u
}

type label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

func (l label) core() core.Label {
	return core.Label(l)
}

// nodes is a GraphQL connection read without its edges.
type nodes[T any] struct {
	Nodes []T `json:"nodes"`
}

// pageInfo is the pagination block of a GraphQL connection.
type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// next returns the cursor of the next page, or "" on the last page, as
// core.Page expects.
func (p pageInfo) next() string {
	if !p.HasNextPage {
		return ""
	}
	return p.EndCursor
}

// convert maps a decoded slice to core values. It returns nil for an empty
// slice.
func convert[T, U any](in []T, f func(T) U) []U {
	if len(in) == 0 {
		return nil
	}
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

// repoLanguage is the primary language of a repository and its color, such
// as "#00ADD8". Both the repository and the search queries read it.
type repoLanguage struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// viewerCaps is what GitHub says the viewer may do to an issue or pull
// request, and whether it is locked, as GraphQL reads them.
type viewerCaps struct {
	Locked bool `json:"locked"`
	// ActiveLockReason is null for a conversation locked without one.
	ActiveLockReason string `json:"activeLockReason"`
	ViewerCanUpdate  bool   `json:"viewerCanUpdate"`
	ViewerCanClose   bool   `json:"viewerCanClose"`
	ViewerCanReopen  bool   `json:"viewerCanReopen"`
	// ViewerCanLabel is nil when the query left it out, as it does on a
	// server whose schema lacks it.
	ViewerCanLabel  *bool `json:"viewerCanLabel"`
	ViewerDidAuthor bool  `json:"viewerDidAuthor"`
}

func (v viewerCaps) core() core.ItemCaps {
	caps := core.ItemCaps{
		Known:    true,
		Update:   v.ViewerCanUpdate,
		Close:    v.ViewerCanClose,
		Reopen:   v.ViewerCanReopen,
		Authored: v.ViewerDidAuthor,
	}
	if v.ViewerCanLabel != nil {
		caps.Label, caps.LabelKnown = *v.ViewerCanLabel, true
	}
	return caps
}

// repoRef is the repository owner/name as GitHub named it in an answer.
// GitHub keeps both to letters, digits and a few marks, but a hostile
// server needn't, and they are drawn in many places, so they are cleaned
// once here, as termtext.OneLine cleans any text from the API. A real
// name comes back as it is, without allocating.
func repoRef(owner, name string) core.RepoRef {
	return core.RepoRef{Owner: termtext.OneLine(owner), Name: termtext.OneLine(name)}
}
