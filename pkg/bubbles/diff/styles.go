package diff

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
)

// gutterWidth is the width of the cursor gutter left of every row.
const gutterWidth = 2

// Styles holds the styles and the glyphs of a diff view.
type Styles struct {
	// Cursor styles the CursorGlyph that marks the cursor's row while the
	// view is focused, and BlurredCursor while it is not.
	Cursor        lipgloss.Style
	BlurredCursor lipgloss.Style
	// CursorGlyph marks the cursor's row, cut or padded to one cell. The
	// default is "▌".
	CursorGlyph string

	// FileHeader styles the header of a file, and HunkHeader that of a hunk.
	FileHeader lipgloss.Style
	HunkHeader lipgloss.Style
	// Added, Deleted and Context style the marker and the text of a line.
	Added   lipgloss.Style
	Deleted lipgloss.Style
	Context lipgloss.Style
	// AddedText, DeletedText and ContextText style the code of a line
	// once it is highlighted, under the colors of its tokens: give the
	// first two a background to tint the lines. The marker keeps Added
	// and Deleted.
	AddedText   lipgloss.Style
	DeletedText lipgloss.Style
	ContextText lipgloss.Style
	// Syntax colors the tokens of highlighted lines; only its foreground
	// colors and font styles are used. Nil shows every line plain.
	Syntax *chroma.Style
	// NoNewline styles the "No newline at end of file" marker, and Note
	// the note that stands in for a patch that is missing.
	NoNewline lipgloss.Style
	Note      lipgloss.Style
	// LineNumber styles the line numbers of the gutter.
	LineNumber lipgloss.Style
	// Status styles the line that says where the cursor is.
	Status lipgloss.Style

	// Match styles the matches of a search, and CurrentMatch the one the
	// last jump went to. They replace the colors of the match.
	Match        lipgloss.Style
	CurrentMatch lipgloss.Style
	// Prompt styles the "/" before the search input, and InputCursor its
	// cursor, with its foreground.
	Prompt      lipgloss.Style
	InputCursor lipgloss.Style

	// Loading, Empty, Error and Hint style the rows that say that files
	// are being fetched, that there are none, that a fetch failed, and
	// what to do about it.
	Loading lipgloss.Style
	Empty   lipgloss.Style
	Error   lipgloss.Style
	Hint    lipgloss.Style

	// ErrorGlyph starts the error row, FoldOpen and FoldClosed start the
	// header of a file that is unfolded or folded, RenameArrow stands
	// between the old and the new path of a renamed file, Separator
	// stands between the parts of the status line, and Ellipsis ends a
	// path that was cut. The defaults are "✗", "▾", "▸", "→", " · " and
	// "…". Each glyph is cut or padded to its place.
	ErrorGlyph  string
	FoldOpen    string
	FoldClosed  string
	RenameArrow string
	Separator   string
	Ellipsis    string
}

// DefaultStyles returns the default styles for a light or dark terminal.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	green := ld(lipgloss.Color("#1a7f37"), lipgloss.Color("#7ec699"))
	red := ld(lipgloss.Color("#c0392b"), lipgloss.Color("#ef7d7d"))
	cyan := ld(lipgloss.Color("#0b7285"), lipgloss.Color("#7dcfff"))

	match := ld(lipgloss.Color("#f6e7a8"), lipgloss.Color("#4a4430"))
	current := ld(lipgloss.Color("#f2c14e"), lipgloss.Color("#e0af68"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	onCurrent := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#1a1b26"))

	syntax := "github"
	if isDark {
		syntax = "github-dark"
	}
	return Styles{
		Syntax:        chromastyles.Get(syntax),
		AddedText:     lipgloss.NewStyle(),
		DeletedText:   lipgloss.NewStyle(),
		ContextText:   lipgloss.NewStyle(),
		Cursor:        lipgloss.NewStyle().Foreground(accent),
		BlurredCursor: lipgloss.NewStyle().Foreground(subtle),
		CursorGlyph:   "▌",
		FileHeader:    lipgloss.NewStyle().Bold(true),
		HunkHeader:    lipgloss.NewStyle().Foreground(cyan),
		Added:         lipgloss.NewStyle().Foreground(green),
		Deleted:       lipgloss.NewStyle().Foreground(red),
		Context:       lipgloss.NewStyle(),
		NoNewline:     lipgloss.NewStyle().Foreground(subtle).Italic(true),
		Note:          lipgloss.NewStyle().Foreground(muted).Italic(true),
		LineNumber:    lipgloss.NewStyle().Foreground(subtle),
		Status:        lipgloss.NewStyle().Foreground(muted),
		Match:         lipgloss.NewStyle().Foreground(text).Background(match),
		CurrentMatch:  lipgloss.NewStyle().Foreground(onCurrent).Background(current),
		Prompt:        lipgloss.NewStyle().Foreground(accent),
		InputCursor:   lipgloss.NewStyle().Foreground(accent),
		Loading:       lipgloss.NewStyle().Foreground(muted),
		Empty:         lipgloss.NewStyle().Foreground(muted),
		Error:         lipgloss.NewStyle().Foreground(red),
		Hint:          lipgloss.NewStyle().Foreground(subtle),
		ErrorGlyph:    "✗",
		FoldOpen:      "▾",
		FoldClosed:    "▸",
		RenameArrow:   "→",
		Separator:     " · ",
		Ellipsis:      "…",
	}
}

// sgr is what a style writes before and after text, found once so that
// drawing a row costs no styling. It is exact for the colors and text
// attributes of a style, which is what a row's style should hold.
type sgr struct{ pre, suf string }

func wrapOf(s lipgloss.Style) sgr {
	pre, suf, _ := strings.Cut(s.Render("x"), "x")
	return sgr{pre, suf}
}

// on styles text, which holds no escape sequences of its own.
func (w sgr) on(text string) string { return w.pre + text + w.suf }

// wraps are the styles of rows as sgr, made by [newWraps].
type wraps struct {
	FileHeader, HunkHeader, Added, Deleted, Context, NoNewline, Note sgr
	LineNumber, Status, Loading, Empty, Error, Hint                  sgr
	match, current                                                   sgr
	// code is how the code of a context, an added and a deleted line
	// looks once highlighted.
	code [3]tokenStyles
}

// The kinds of line that take their own code style, as indexes of code.
const (
	codeContext = iota
	codeAdded
	codeDeleted
)

func newWraps(s Styles) wraps {
	return wraps{
		FileHeader: wrapOf(s.FileHeader), HunkHeader: wrapOf(s.HunkHeader),
		Added: wrapOf(s.Added), Deleted: wrapOf(s.Deleted), Context: wrapOf(s.Context),
		NoNewline: wrapOf(s.NoNewline), Note: wrapOf(s.Note),
		LineNumber: wrapOf(s.LineNumber), Status: wrapOf(s.Status),
		Loading: wrapOf(s.Loading), Empty: wrapOf(s.Empty),
		Error: wrapOf(s.Error), Hint: wrapOf(s.Hint),
		match: wrapOf(s.Match), current: wrapOf(s.CurrentMatch),
		code: [3]tokenStyles{
			codeContext: newTokenStyles(s.ContextText, s.Syntax),
			codeAdded:   newTokenStyles(s.AddedText, s.Syntax),
			codeDeleted: newTokenStyles(s.DeletedText, s.Syntax),
		},
	}
}
