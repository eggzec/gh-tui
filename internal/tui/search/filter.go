package search

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// languages are the languages the filter offers; others are typed in its
// query as language:name.
var languages = []string{
	"Go", "Python", "JavaScript", "TypeScript", "Rust", "Java", "C", "C++", "C#",
	"Ruby", "PHP", "Shell", "Swift", "Kotlin", "Lua", "Nix",
}

// languageField offers languages, and the language of query when it is
// another, so that the form shows it.
func languageField(query string) filterform.Field {
	items := []filterform.Item{{Label: "Any"}}
	known := false
	current := ""
	for _, t := range filterform.Tokenize(query) {
		if strings.EqualFold(t.Qualifier, "language") && t.Value != "" {
			current = t.Value
		}
	}
	for _, l := range languages {
		items = append(items, filterform.Item{Label: l, Value: strings.ToLower(l)})
		known = known || strings.EqualFold(l, current)
	}
	if current != "" && !known {
		items = append(items, filterform.Item{Label: current, Value: strings.ToLower(current)})
	}
	return filterform.Field{Key: "language", Label: "Language", Kind: filterform.Choice, Qualifier: "language", Options: items}
}

func textField(key, label, qualifier, hint string) filterform.Field {
	return filterform.Field{Key: key, Label: label, Kind: filterform.Text, Qualifier: qualifier, Hint: hint}
}

func personField(key, label, qualifier string) filterform.Field {
	return filterform.Field{
		Key: key, Label: label, Kind: filterform.Person, Qualifier: qualifier,
		Options: []filterform.Item{ui.MeItem}, Hint: "anyone",
	}
}

// spec returns the fields the filter of kind edits in query. Each writes a
// qualifier of GitHub's search, and each sort is one GitHub's search takes
// as a sort: qualifier.
func spec(kind core.SearchKind, query string) filterform.Spec {
	switch kind {
	case core.SearchRepos:
		return filterform.Spec{
			Fields: []filterform.Field{
				languageField(query),
				{
					Key: "stars", Label: "Stars", Kind: filterform.Choice, Qualifier: "stars",
					Options: []filterform.Item{
						{Label: "Any"}, {Label: "> 10", Value: ">10"}, {Label: "> 100", Value: ">100"},
						{Label: "> 1k", Value: ">1000"}, {Label: "> 10k", Value: ">10000"},
					},
				},
				{
					Key: "forks", Label: "Forks", Kind: filterform.Choice, Qualifier: "fork",
					Options: []filterform.Item{
						{Label: "Hide"}, {Label: "Include", Value: "true"}, {Label: "Only", Value: "only"},
					},
				},
				{Key: "archived", Label: "Archived", Kind: filterform.Toggle, Qualifier: "archived:false", Hint: "hide archived"},
				{
					Key: "visibility", Label: "Visibility", Kind: filterform.Choice, Qualifier: "is",
					Options: []filterform.Item{{Label: "All"}, {Label: "Public", Value: "public"}, {Label: "Private", Value: "private"}},
				},
				textField("owner", "Owner", "user", "any user or organization"),
			},
			Sort: &filterform.SortField{
				Options: []filterform.SortOption{
					ui.BestMatch,
					ui.SortByCount("Stars", "stars"),
					ui.SortByCount("Forks", "forks"),
					ui.SortByTime("Updated", "updated"),
				},
				Default: filterform.Sort{Desc: true},
			},
		}
	case core.SearchIssues, core.SearchPulls:
		pulls := kind == core.SearchPulls
		states := []filterform.Item{{Label: "Any"}, {Label: "Open", Value: "open"}, {Label: "Closed", Value: "closed"}}
		if pulls {
			states = append(states, filterform.Item{Label: "Merged", Value: "merged"})
		}
		fields := []filterform.Field{
			{Key: "state", Label: "State", Kind: filterform.Choice, Qualifier: "is", Options: states},
			personField("author", "Author", "author"),
			personField("assignee", "Assignee", "assignee"),
			textField("label", "Label", "label", "any"),
			textField("repo", "Repository", "repo", "any, as owner/name"),
			textField("org", "Organization", "org", "any"),
		}
		if pulls {
			fields = append(fields,
				filterform.Field{Key: "draft", Label: "Drafts", Kind: filterform.Toggle, Qualifier: "is:draft", Hint: "drafts only"},
				filterform.Field{
					Key: "review", Label: "Review", Kind: filterform.Choice,
					Options: []filterform.Item{
						{Label: "Any"},
						{Label: "Required", Value: "review:required"},
						{Label: "Approved", Value: "review:approved"},
						{Label: "Changes requested", Value: "review:changes_requested"},
						{Label: "Requested of you", Value: "review-requested:@me"},
						{Label: "Reviewed by you", Value: "reviewed-by:@me"},
					},
				},
			)
		}
		return filterform.Spec{
			Fields: fields,
			Sort: &filterform.SortField{
				Options: []filterform.SortOption{
					ui.BestMatch,
					ui.SortByTime("Updated", "updated"),
					ui.SortByTime("Created", "created"),
					ui.SortByCount("Comments", "comments"),
				},
				Default: filterform.Sort{Desc: true},
			},
		}
	default:
		// Code search takes no sort.
		return filterform.Spec{Fields: []filterform.Field{
			languageField(query),
			textField("repo", "Repository", "repo", "any, as owner/name"),
			textField("org", "Organization", "org", "any"),
			textField("path", "Path", "path", "anywhere"),
		}}
	}
}

// Filter implements ui.Filterable: the form edits the qualifiers of the
// query that the kind on view takes, and keeps the rest of it as free
// text.
func (s *Section) Filter() (ui.Filter, bool) {
	q := strings.Join(strings.Fields(s.input.Value()), " ")
	return ui.Filter{Spec: spec(s.kind, q), Query: q, Subject: kindTitles[s.kind]}, true
}

// ApplyFilter implements ui.Filterable. The query becomes what the form
// wrote, which is searched at once, as on enter.
func (s *Section) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	if msg.Query == strings.Join(strings.Fields(s.input.Value()), " ") {
		return nil
	}
	cmd := s.setQuery(msg.Query)
	s.render()
	return cmd
}
