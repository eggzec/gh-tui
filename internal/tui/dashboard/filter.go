package dashboard

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sahilm/fuzzy"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/dashboard"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// defaultRepoSort is the order GitHub lists repositories in, which the
// query leaves out.
const defaultRepoSort = "sort:updated-desc"

// repoFilter is the filter of the repositories pane, read from its query.
// It runs over the repositories read of an owner, so it costs no request
// of its own. Its fields hold what the query says, so the zero value keeps
// every repository in GitHub's order.
type repoFilter struct {
	// query is the query the filter was read from, without the default
	// sort.
	query string
	// visibility is "public", "private" or "" for both.
	visibility string
	// forks is "false" to hide forks and "only" to keep only them, and
	// archived "false" or "true" likewise. Other values keep both.
	forks, archived string
	templates       bool
	// language is kept lower-case, as GitHub matches it.
	language string
	sort     filterform.Sort
	// words match the names of the repositories fuzzily.
	words []string
}

// parseRepoFilter reads the GitHub-like qualifiers of query: is:public or
// is:private, fork:false, archived:false, template:true, language:go and
// sort:stars-desc. Free words match names. Other tokens are kept in the
// query but match every repository.
func parseRepoFilter(query string) repoFilter {
	f := repoFilter{query: query}
	for _, t := range filterform.Tokenize(query) {
		v := strings.ToLower(t.Value)
		switch strings.ToLower(t.Qualifier) {
		case "":
			f.words = append(f.words, t.Value)
		case "is":
			switch v {
			case "public", "private":
				f.visibility = v
			case "template":
				f.templates = true
			}
		case "fork":
			f.forks = v
		case "archived":
			f.archived = v
		case "template":
			f.templates = v == "true"
		case "language":
			f.language = v
		case "sort":
			f.sort = repoSort(v)
		}
	}
	return f
}

// repoSort reads the value of a sort: qualifier, such as stars-desc, which
// is descending without a direction.
func repoSort(v string) filterform.Sort {
	by, dir, _ := strings.Cut(v, "-")
	switch by {
	case "updated", "stars", "name":
		return filterform.Sort{By: by, Desc: dir != "asc"}
	}
	return filterform.Sort{}
}

// active reports whether the filter leaves anything out or reorders it.
func (f *repoFilter) active() bool { return f.query != "" }

// keeps reports whether r passes every qualifier but the words.
func (f *repoFilter) keeps(r *core.Repo) bool {
	switch {
	case f.visibility == "public" && r.Private, f.visibility == "private" && !r.Private:
		return false
	case f.forks == "false" && r.Fork, f.forks == "only" && !r.Fork:
		// fork:true, as on GitHub, adds forks to the rest, which the pane
		// lists already.
		return false
	case f.archived == "false" && r.Archived, f.archived == "true" && !r.Archived:
		return false
	case f.templates && !r.Template:
		return false
	case f.language != "" && !strings.EqualFold(r.Language, f.language):
		return false
	}
	return true
}

// apply returns the repositories of repos the filter keeps, in its order.
// Without a sort, the best matches of the words come first, and GitHub's
// order breaks ties.
func (f *repoFilter) apply(repos []core.Repo) []core.Repo {
	out := make([]core.Repo, 0, len(repos))
	for i := range repos {
		if f.keeps(&repos[i]) {
			out = append(out, repos[i])
		}
	}
	if len(f.words) > 0 {
		out = f.match(out)
	}
	switch f.sort.By {
	case "updated":
		slices.SortStableFunc(out, func(a, b core.Repo) int { return a.UpdatedAt.Compare(b.UpdatedAt) })
	case "stars":
		slices.SortStableFunc(out, func(a, b core.Repo) int { return cmp.Compare(a.Stars, b.Stars) })
	case "name":
		slices.SortStableFunc(out, func(a, b core.Repo) int {
			return cmp.Compare(strings.ToLower(a.Ref.Name), strings.ToLower(b.Ref.Name))
		})
	default:
		return out
	}
	if f.sort.Desc {
		slices.Reverse(out)
	}
	return out
}

// match keeps the repositories whose names match every word, the best
// matches first.
func (f *repoFilter) match(repos []core.Repo) []core.Repo {
	names := make(names, len(repos))
	for i := range repos {
		names[i] = repos[i].Ref.Name
	}
	score := make([]int, len(repos))
	hits := make([]int, len(repos))
	for _, w := range f.words {
		for _, m := range fuzzy.FindFromNoSort(w, names) {
			score[m.Index] += m.Score
			hits[m.Index]++
		}
	}
	idx := make([]int, 0, len(repos))
	for i := range repos {
		if hits[i] == len(f.words) {
			idx = append(idx, i)
		}
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(score[b], score[a]) })
	out := make([]core.Repo, len(idx))
	for i, j := range idx {
		out[i] = repos[j]
	}
	return out
}

// names lets fuzzy match the names of repositories.
type names []string

func (n names) String(i int) string { return n[i] }
func (n names) Len() int            { return len(n) }

// chips names what the filter does in a few words, for the pane's title.
func (f *repoFilter) chips() string {
	if !f.active() {
		return ""
	}
	var parts []string
	if len(f.words) > 0 {
		parts = append(parts, `"`+strings.Join(f.words, " ")+`"`)
	}
	if f.visibility != "" {
		parts = append(parts, f.visibility)
	}
	switch f.forks {
	case "false":
		parts = append(parts, "no forks")
	case "only":
		parts = append(parts, "forks")
	}
	switch f.archived {
	case "false":
		parts = append(parts, "no archived")
	case "true":
		parts = append(parts, "archived")
	}
	if f.templates {
		parts = append(parts, "templates")
	}
	if f.language != "" {
		parts = append(parts, f.language)
	}
	if f.sort.By != "" {
		dir := "↓"
		if !f.sort.Desc {
			dir = "↑"
		}
		parts = append(parts, f.sort.By+" "+dir)
	}
	if len(parts) == 0 {
		// Only tokens the filter doesn't read, which it keeps as typed.
		return ui.Chips(f.query)
	}
	return strings.Join(parts, " · ")
}

// spec returns the fields of the filter of the repositories.
func (t *repoTabs) spec() filterform.Spec {
	return filterform.Spec{
		Fields: []filterform.Field{
			{
				Key: "name", Label: "Name", Kind: filterform.Text, Hint: "any name",
				Format: writeWords, Parse: claimWord,
			},
			{
				Key: "visibility", Label: "Visibility", Kind: filterform.Choice, Qualifier: "is",
				Options: []filterform.Item{{Label: "All"}, {Label: "Public", Value: "public"}, {Label: "Private", Value: "private"}},
			},
			{Key: "forks", Label: "Forks", Kind: filterform.Toggle, Qualifier: "fork:false", Hint: "hide forks"},
			{Key: "archived", Label: "Archived", Kind: filterform.Toggle, Qualifier: "archived:false", Hint: "hide archived"},
			{Key: "templates", Label: "Templates", Kind: filterform.Toggle, Qualifier: "template:true", Hint: "templates only"},
			{
				Key: "language", Label: "Language", Kind: filterform.Choice, Qualifier: "language",
				Options: t.languageItems(),
			},
		},
		Sort: &filterform.SortField{
			Options: []filterform.Item{
				{Label: "Updated", Value: "updated"},
				{Label: "Stars", Value: "stars"},
				{Label: "Name", Value: "name"},
			},
			Default: filterform.Sort{By: "updated", Desc: true},
		},
	}
}

// writeWords writes the words of the name field as free text.
func writeWords(v filterform.Value) string { return strings.Join(strings.Fields(v.Text()), " ") }

// claimWord takes the free words of a query for the name field.
func claimWord(tok filterform.Token, v filterform.Value) (filterform.Value, bool) {
	if tok.Qualifier != "" {
		return v, false
	}
	return filterform.TextValue(strings.TrimSpace(v.Text() + " " + tok.Raw)), true
}

// languageItems offers the languages of the repositories read of the tab
// on view, the most common first, and the language of the filter.
func (t *repoTabs) languageItems() []filterform.Item {
	counts := map[string]int{}
	label := map[string]string{}
	repos := t.read(t.current())
	for i := range repos {
		lang := repos[i].Language
		if lang == "" {
			continue
		}
		k := strings.ToLower(lang)
		counts[k]++
		label[k] = lang
	}
	if l := t.filter().language; l != "" {
		if _, ok := counts[l]; !ok {
			counts[l], label[l] = 0, l
		}
	}
	langs := slices.SortedFunc(maps.Keys(counts), func(a, b string) int {
		return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b))
	})
	items := make([]filterform.Item, 0, len(langs)+1)
	items = append(items, filterform.Item{Label: "Any"})
	for _, k := range langs {
		it := filterform.Item{Label: label[k], Value: k}
		if n := counts[k]; n > 0 {
			it.Detail = strconv.Itoa(n)
		}
		items = append(items, it)
	}
	return items
}

// read returns the repositories of o that are cached, from the first,
// without I/O.
func (t *repoTabs) read(o *owner) []core.Repo {
	q := o.q
	q.Cursor = ""
	p, _ := t.s.svc.CachedAllRepos(q, dashboard.MaxOwnerRepos)
	return p.Items
}

// filter returns the filter in force.
func (t *repoTabs) filter() *repoFilter {
	if f := t.current().filter.Load(); f != nil {
		return f
	}
	return &repoFilter{}
}

// setFilter filters every tab with the filter of query, unless it does
// already, and lists them again.
func (t *repoTabs) setFilter(query string) tea.Cmd {
	if query == t.filter().query {
		return nil
	}
	f := parseRepoFilter(query)
	cmds := make([]tea.Cmd, 0, len(t.tabs))
	for _, o := range t.tabs {
		o.filter.Store(&f)
		o.feed.SetEmptyText(o.emptyText(&f))
		if o.started {
			cmds = append(cmds, o.feed.Reset())
		}
	}
	return tea.Batch(cmds...)
}

// Filter implements ui.Filterable while the repositories have the focus.
func (s *Section) Filter() (ui.Filter, bool) {
	if s.focus != reposPane {
		return ui.Filter{}, false
	}
	return ui.Filter{Spec: s.repos.spec(), Query: s.repos.filter().query, Subject: paneTitles[reposPane]}, true
}

// ApplyFilter implements ui.Filterable. The default sort is left out of
// the query, so that it alone filters nothing.
func (s *Section) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	query := ui.Without(msg.Query, func(t filterform.Token) bool { return strings.EqualFold(t.Raw, defaultRepoSort) })
	cmd := s.repos.setFilter(query)
	s.render()
	return cmd
}
