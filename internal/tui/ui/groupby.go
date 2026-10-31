package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// GroupField is a way to group the rows of a list: the feed's field, and the
// words the modal lists it by.
type GroupField[T any] struct {
	feed.Field[T]
	Title string
}

func groupField[T any](name, title string, key func(T) string) GroupField[T] {
	f := GroupField[T]{Title: title}
	f.Name, f.Key = name, key
	return f
}

// Grouping is the grouping of a list: the fields it offers, the levels in
// force, outermost first, and whether the grouping is switched off with the
// levels kept.
type Grouping[T any] struct {
	Fields []GroupField[T]
	Levels []orderpick.Choice
	Off    bool
}

// Groups returns the groups the list takes: those of the levels, or none
// while the grouping is off.
func (g *Grouping[T]) Groups() []feed.Group[T] {
	if g.Off {
		return nil
	}
	fields := make([]feed.Field[T], len(g.Fields))
	for i, f := range g.Fields {
		fields[i] = f.Field
	}
	return feed.Groups(fields, OrderLevels(g.Levels))
}

// Items returns the fields the modal offers.
func (g *Grouping[T]) Items() []orderpick.Item {
	items := make([]orderpick.Item, len(g.Fields))
	for i, f := range g.Fields {
		items[i] = OrderByName(f.Title, f.Name)
	}
	return items
}

// Modal returns the modal that picks the levels; apply takes those chosen.
func (g *Grouping[T]) Modal(section string, keys orderpick.KeyMap, icons Icons, apply func([]orderpick.Choice) tea.Cmd) Modal {
	return NewGroupModal(section, g.Items(), g.Levels, keys, icons, apply)
}

// Choose takes levels, which switches the grouping on.
func (g *Grouping[T]) Choose(levels []orderpick.Choice) {
	g.Levels = append([]orderpick.Choice(nil), levels...)
	g.Off = false
}

// Toggle switches the grouping off, and back on. It returns false when there
// are no levels to switch.
func (g *Grouping[T]) Toggle() bool {
	if len(g.Levels) == 0 {
		return false
	}
	g.Off = !g.Off
	return true
}

// Chip returns the chip that tells the grouping is off, or "" while it is not.
func (g *Grouping[T]) Chip(sep string) string {
	if !g.Off || len(g.Levels) == 0 {
		return ""
	}
	names := make([]string, 0, len(g.Levels))
	for _, c := range g.Levels {
		name, _ := c.Value.(string)
		names = append(names, name)
	}
	return GroupOffChip(names, sep)
}

// GroupOffChip words a grouping by names that is switched off for a title.
func GroupOffChip(names []string, sep string) string {
	return OffChip("by "+strings.Join(names, ", "), sep)
}

// RepoGroupFields returns the ways to group repositories, in the order the
// modal lists them.
func RepoGroupFields() []GroupField[core.Repo] {
	return []GroupField[core.Repo]{
		groupField("owner", "Owner", func(r core.Repo) string { return r.Ref.Owner }),
		groupField("language", "Language", func(r core.Repo) string {
			if r.Language == "" {
				return "none"
			}
			return r.Language
		}),
		groupField("visibility", "Visibility", func(r core.Repo) string {
			if r.Private {
				return "private"
			}
			return "public"
		}),
		groupField("kind", "Kind", RepoKind),
		groupField("role", "Role", func(r core.Repo) string {
			if r.Caps.Permission == "" {
				return "unknown"
			}
			return strings.ToLower(string(r.Caps.Permission))
		}),
	}
}

// RepoKind returns the one kind of r, by priority: archived, mirror, fork,
// template, then source.
func RepoKind(r core.Repo) string {
	switch {
	case r.Archived:
		return "archived"
	case r.Mirror:
		return "mirror"
	case r.Fork:
		return "fork"
	case r.Template:
		return "template"
	}
	return "source"
}

// HitGroupFields returns the fields of repositories as they apply to the
// repositories inside search hits.
func HitGroupFields() []GroupField[core.SearchHit] {
	repo := RepoGroupFields()
	out := make([]GroupField[core.SearchHit], len(repo))
	for i, f := range repo {
		out[i] = groupField(f.Name, f.Title, func(h core.SearchHit) string { return f.Key(h.Repo) })
		out[i].Label = f.Label
	}
	return out
}
