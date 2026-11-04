package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// plainRows strips the colours from rows.
func plainRows(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = ansi.Strip(r)
	}
	return out
}

// threadRows draws a thread of one comment with body at width.
func threadRows(o *diffOverlay, body string, width int) []string {
	th := threadOn("t-md", body, func(t *core.Thread) { t.Line = 2 })
	return o.threadBlock(&th).Render(width, false)
}

func TestThreadBlockWrapsALongCommentWithoutEllipsis(t *testing.T) {
	t.Parallel()
	_, _, o := findingDiff(t, diffFindingChecks(), false)
	body := strings.Repeat("word ", 40)
	rows := plainRows(threadRows(o, body, 40))
	if len(rows) < 4 {
		t.Fatalf("a long comment drew %d rows, want it wrapped:\n%s", len(rows), strings.Join(rows, "\n"))
	}
	for _, r := range rows {
		if w := ansi.StringWidth(r); w > 40 {
			t.Errorf("row %q is %d cells, want at most 40", r, w)
		}
		if strings.Contains(r, "…") {
			t.Errorf("row %q is cut with an ellipsis", r)
		}
	}
	if got := strings.Count(strings.Join(rows, " "), "word"); got != 40 {
		t.Errorf("%d words shown, want all 40", got)
	}
}

func TestFindingBlockWrapsALongMessageWithoutEllipsis(t *testing.T) {
	t.Parallel()
	_, _, o := findingDiff(t, diffFindingChecks(), false)
	msg := strings.Repeat("word ", 40)
	rows := plainRows(o.findingBlock("a.go", 2, []files.Note{{Line: 2, Level: core.AnnotationWarning, App: "Actions", Message: msg}}).Render(40, false))
	if len(rows) < 4 {
		t.Fatalf("a long message drew %d rows, want it wrapped:\n%s", len(rows), strings.Join(rows, "\n"))
	}
	for _, r := range rows {
		if w := ansi.StringWidth(r); w > 40 {
			t.Errorf("row %q is %d cells, want at most 40", r, w)
		}
		if strings.Contains(r, "…") {
			t.Errorf("row %q is cut with an ellipsis", r)
		}
	}
	if got := strings.Count(strings.Join(rows, " "), "word"); got != 40 {
		t.Errorf("%d words shown, want all 40", got)
	}
}

func TestThreadBlockDrawsMarkdown(t *testing.T) {
	t.Parallel()
	_, _, o := findingDiff(t, diffFindingChecks(), false)
	raw := threadRows(o, "this is **bold** text\n\n- first\n- second", 60)
	text := strings.Join(plainRows(raw), "\n")
	if strings.Contains(text, "**") {
		t.Errorf("the bold markers show:\n%s", text)
	}
	if strings.Contains(text, "- first") {
		t.Errorf("the list marker is not drawn as a bullet:\n%s", text)
	}
	for _, want := range []string{"bold", "first", "second"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q is missing:\n%s", want, text)
		}
	}
	if !strings.Contains(strings.Join(raw, "\n"), "\x1b[") {
		t.Errorf("the rows have no styling")
	}
}

func TestThreadBlockCleansCarriageReturnsAndTabs(t *testing.T) {
	t.Parallel()
	_, _, o := findingDiff(t, diffFindingChecks(), false)
	raw := threadRows(o, "one\r\ntwo\tgap\r\n", 60)
	for _, r := range raw {
		if strings.ContainsAny(r, "\r\t") {
			t.Errorf("row %q keeps a carriage return or a tab", r)
		}
	}
	text := strings.Join(plainRows(raw), "\n")
	for _, want := range []string{"one", "two", "gap"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q is missing:\n%s", want, text)
		}
	}
}

func TestDiffMarkdownFollowsTheThemeBeforeTheConversationStarts(t *testing.T) {
	t.Parallel()
	_, m, o := findingDiff(t, diffFindingChecks(), false)
	// The diff may open before the Conversation does: draw with no renderer
	// of the pane yet.
	m.conv.started, m.conv.md = false, nil
	before := strings.Join(threadRows(o, "a **bold** word", 60), "\n")
	light, _ := config.Default().Palette(false)
	m.SetTheme(ui.NewTheme(light, false))
	after := strings.Join(threadRows(o, "a **bold** word!", 60), "\n")
	strip := func(s string) string { return strings.ReplaceAll(ansi.Strip(s), "!", "") }
	if strip(before) != strip(after) {
		t.Fatalf("the text changed:\n%q\n%q", before, after)
	}
	if strings.ReplaceAll(after, "!", "") == before {
		t.Errorf("the rows keep the style of the old theme: %q", before)
	}
}
