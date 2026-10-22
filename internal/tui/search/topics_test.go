package search

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// topicsSection returns a page over topics, on the Topics kind with the
// results focused, and the query not yet submitted.
func topicsSection(t *testing.T) (*Section, *fakeService) {
	t.Helper()
	f := newFake()
	f.topics = []core.SearchTopic{
		{Name: "tea-brewing", ShortDescription: "Brewing tea"},
		{Name: "tea-leaves"},
	}
	s := newSection(t, f, 100, 24)
	press(t, s, "esc")
	press(t, s, strconv.Itoa(slices.Index(kinds, core.SearchTopics)+1))
	return s, f
}

func TestTopicsKindLoadsOnSubmit(t *testing.T) {
	t.Parallel()
	s, f := topicsSection(t)
	typeOnly(t, s, "tea")
	if len(f.topicQueries) != 0 {
		t.Fatalf("searched topics %v before the query was submitted", f.topicQueries)
	}
	press(t, s, "enter")
	if len(f.topicQueries) != 1 || f.topicQueries[0].Text != "tea" {
		t.Fatalf("searched topics with %+v, want one query for tea", f.topicQueries)
	}
}

func TestTopicRow(t *testing.T) {
	t.Parallel()
	s, _ := topicsSection(t)
	typeText(t, s, "tea")
	var row string
	for line := range strings.Lines(s.View()) {
		if strings.Contains(line, "tea-brewing") {
			row = line
		}
	}
	if !strings.Contains(row, s.icons.Glyph(ui.GlyphLabelEvent)) || !strings.Contains(row, "tea-brewing") {
		t.Errorf("row of a topic = %q, want the glyph and the name", row)
	}
}

func TestTopicEnterSearchesRepos(t *testing.T) {
	t.Parallel()
	s, f := topicsSection(t)
	typeText(t, s, "tea")
	press(t, s, "down")
	app := press(t, s, "enter")
	if len(app) != 0 {
		t.Errorf("enter sent %v, want no app message", app)
	}
	if got := s.input.Value(); got != "topic:tea-leaves" {
		t.Errorf("query = %q, want topic:tea-leaves", got)
	}
	if s.Kind() != core.SearchRepos || s.area != resultsArea {
		t.Errorf("kind %v in area %d, want repositories in the results", s.Kind(), s.area)
	}
	n := 0
	for _, q := range f.queries {
		if q.Kind == core.SearchRepos && q.Text == "topic:tea-leaves" && q.Cursor == "" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("enter issued %d repository searches for the topic, want one (%+v)", n, f.queries)
	}
}

func TestTopicsNoCount(t *testing.T) {
	t.Parallel()
	s, _ := topicsSection(t)
	typeText(t, s, "tea")
	if m := regexp.MustCompile(`Topics\W*\d`).FindString(s.View()); m != "" {
		t.Errorf("view shows a count for the topics: %q", m)
	}
}

func TestTopicsFilterKeyHidden(t *testing.T) {
	t.Parallel()
	s, _ := topicsSection(t)
	if _, ok := s.Filter(); ok {
		t.Error("the topics kind offers a filter, want none")
	}
	keys := s.keys.state(s)
	if keys.Filter.Enabled() || keys.Sort.Enabled() {
		t.Errorf("topics filter enabled = %v, sort enabled = %v, want both off", keys.Filter.Enabled(), keys.Sort.Enabled())
	}
}
