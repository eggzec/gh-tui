package ui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/graph"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// Theme is the user's palette turned into styles, for the bubbles and for
// the rows that sections render. Build it once per palette change; the
// styles are meant to be reused on every frame.
type Theme struct {
	Dark    bool
	Palette config.Palette

	// Text styles for section content.
	Title   lipgloss.Style
	Text    lipgloss.Style
	Muted   lipgloss.Style
	Subtle  lipgloss.Style
	Accent  lipgloss.Style
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
}

// NewTheme returns the theme for palette p on a dark or light terminal.
func NewTheme(p config.Palette, dark bool) Theme {
	fg := lipgloss.NewStyle().Foreground
	return Theme{
		Dark:    dark,
		Palette: p,
		Title:   fg(lipgloss.Color(p.Foreground)).Bold(true),
		Text:    fg(lipgloss.Color(p.Foreground)),
		Muted:   fg(lipgloss.Color(p.Muted)),
		Subtle:  fg(lipgloss.Color(p.Subtle)),
		Accent:  fg(lipgloss.Color(p.Accent)),
		Success: fg(lipgloss.Color(p.Success)),
		Warning: fg(lipgloss.Color(p.Warning)),
		Error:   fg(lipgloss.Color(p.Error)),
	}
}

// Each bubble keeps the shape of its default styles, such as glyphs and
// borders, and takes its colors from the palette. A bubble that marks what
// went wrong takes the mark from the icons, as the error lines of the
// sections do.

// Toast returns the styles of the toasts.
func (t Theme) Toast() toast.Styles {
	s := toast.DefaultStyles(t.Dark)
	s.Text = s.Text.Foreground(lipgloss.Color(t.Palette.Foreground))
	s.Count = s.Count.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Info.Color = lipgloss.Color(t.Palette.Accent)
	s.Success.Color = lipgloss.Color(t.Palette.Success)
	s.Warning.Color = lipgloss.Color(t.Palette.Warning)
	s.Error.Color = lipgloss.Color(t.Palette.Error)
	return s
}

// Feed returns the styles of a list.
func (t Theme) Feed(ic Icons) feed.Styles {
	s := feed.DefaultStyles(t.Dark)
	s.ErrorGlyph = ic.Error
	s.ErrorSeparator, s.ErrorEllipsis = ic.Separator, ic.Ellipsis
	s.Cursor = s.Cursor.Foreground(lipgloss.Color(t.Palette.Accent))
	s.BlurredCursor = s.BlurredCursor.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Placeholder = s.Placeholder.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Spinner = s.Spinner.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Loading = s.Loading.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Empty = s.Empty.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Error = s.Error.Foreground(lipgloss.Color(t.Palette.Error))
	s.Hint = s.Hint.Foreground(lipgloss.Color(t.Palette.Subtle))
	return s
}

// Thread returns the styles of a document with comments.
func (t Theme) Thread(ic Icons) thread.Styles {
	s := thread.DefaultStyles(t.Dark)
	s.ErrorGlyph = ic.Error
	s.ErrorSeparator, s.ErrorEllipsis = ic.Separator, ic.Ellipsis
	s.Spinner = s.Spinner.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Loading = s.Loading.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Empty = s.Empty.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Error = s.Error.Foreground(lipgloss.Color(t.Palette.Error))
	s.Key = s.Key.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Hint = s.Hint.Foreground(lipgloss.Color(t.Palette.Subtle))
	return s
}

// Tree returns the styles of a tree, such as the files of a repository.
func (t Theme) Tree(ic Icons) tree.Styles {
	s := tree.DefaultStyles(t.Dark)
	s.ErrorGlyph = ic.Error
	s.ErrorSeparator, s.ErrorEllipsis = ic.Separator, ic.Ellipsis
	s.Cursor = s.Cursor.Foreground(lipgloss.Color(t.Palette.Accent))
	s.BlurredCursor = s.BlurredCursor.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Guide = s.Guide.Foreground(lipgloss.Color(t.Palette.Border))
	s.Marker = s.Marker.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Branch = s.Branch.Foreground(lipgloss.Color(t.Palette.Foreground))
	s.Leaf = s.Leaf.Foreground(lipgloss.Color(t.Palette.Foreground))
	s.Detail = s.Detail.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Spinner = s.Spinner.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Loading = s.Loading.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Empty = s.Empty.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Error = s.Error.Foreground(lipgloss.Color(t.Palette.Error))
	s.Hint = s.Hint.Foreground(lipgloss.Color(t.Palette.Subtle))
	return s
}

// Graph returns the styles of a commit graph. The lanes keep their
// default hues, which tell lanes apart, except the first, which takes the
// accent.
func (t Theme) Graph(ic Icons) graph.Styles {
	s := graph.DefaultStyles(t.Dark)
	s.ErrorGlyph = ic.Error
	s.ErrorSeparator, s.ErrorEllipsis = ic.Separator, ic.Ellipsis
	accent := lipgloss.Color(t.Palette.Accent)
	s.Cursor = s.Cursor.Foreground(accent)
	s.BlurredCursor = s.BlurredCursor.Foreground(lipgloss.Color(t.Palette.Subtle))
	if len(s.Lanes) > 0 {
		s.Lanes[0] = s.Lanes[0].Foreground(accent)
	}
	s.Overflow = s.Overflow.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Short = s.Short.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Title = s.Title.Foreground(lipgloss.Color(t.Palette.Foreground))
	s.Detail = s.Detail.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Right = s.Right.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Spinner = s.Spinner.Foreground(accent)
	s.Loading = s.Loading.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Empty = s.Empty.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Error = s.Error.Foreground(lipgloss.Color(t.Palette.Error))
	s.Hint = s.Hint.Foreground(lipgloss.Color(t.Palette.Subtle))
	return s
}

// Pager returns the styles of a file viewer. The syntax colors and the
// search highlights, which need backgrounds the palette doesn't have, keep
// their defaults for a light or dark terminal.
func (t Theme) Pager(ic Icons) pager.Styles {
	s := pager.DefaultStyles(t.Dark)
	s.ErrorGlyph = ic.Error
	s.ErrorSeparator, s.ErrorEllipsis = ic.Separator, ic.Ellipsis
	s.Text = s.Text.Foreground(lipgloss.Color(t.Palette.Foreground))
	s.LineNumber = s.LineNumber.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Name = s.Name.Foreground(lipgloss.Color(t.Palette.Foreground))
	s.Status = s.Status.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Notice = s.Notice.Foreground(lipgloss.Color(t.Palette.Error))
	s.Message = s.Message.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Spinner = s.Spinner.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Error = s.Error.Foreground(lipgloss.Color(t.Palette.Error))
	s.Prompt = s.Prompt.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Cursor = s.Cursor.Foreground(lipgloss.Color(t.Palette.Accent))
	return s
}

// LogView returns the styles of a job log. The search highlights, which
// need backgrounds the palette doesn't have, keep their defaults for a
// light or dark terminal.
func (t Theme) LogView(ic Icons) logview.Styles {
	s := logview.DefaultStyles(t.Dark)
	s.ErrorGlyph = ic.Error
	s.ErrorSeparator, s.ErrorEllipsis = ic.Separator, ic.Ellipsis
	c := lipgloss.Color
	p := t.Palette
	s.Text = s.Text.Foreground(c(p.Foreground))
	s.Command = s.Command.Foreground(c(p.Accent))
	s.ErrorLine = s.ErrorLine.Foreground(c(p.Error))
	s.WarningLine = s.WarningLine.Foreground(c(p.Warning))
	s.NoticeLine = s.NoticeLine.Foreground(c(p.Foreground))
	s.Group = s.Group.Foreground(c(p.Foreground))
	s.Section = s.Section.Foreground(c(p.Foreground))
	s.FailedSection = s.FailedSection.Foreground(c(p.Error))
	s.Duration = s.Duration.Foreground(c(p.Subtle))
	s.Marker = s.Marker.Foreground(c(p.Muted))
	s.ErrorMark = s.ErrorMark.Foreground(c(p.Error))
	s.WarningMark = s.WarningMark.Foreground(c(p.Warning))
	s.NoticeMark = s.NoticeMark.Foreground(c(p.Accent))
	s.Cursor = s.Cursor.Foreground(c(p.Accent))
	s.BlurredCursor = s.BlurredCursor.Foreground(c(p.Subtle))
	s.LineNumber = s.LineNumber.Foreground(c(p.Subtle))
	s.Time = s.Time.Foreground(c(p.Subtle))
	s.Title = s.Title.Foreground(c(p.Foreground))
	s.Status = s.Status.Foreground(c(p.Muted))
	s.NoMatches = s.NoMatches.Foreground(c(p.Error))
	s.Message = s.Message.Foreground(c(p.Muted))
	s.Spinner = s.Spinner.Foreground(c(p.Accent))
	s.LoadError = s.LoadError.Foreground(c(p.Error))
	s.Prompt = s.Prompt.Foreground(c(p.Accent))
	s.InputCursor = s.InputCursor.Foreground(c(p.Accent))
	return s
}

// FilterForm returns the styles of a filter form. The accent marks only
// what is in focus, and its pickers take the styles of the search popups.
func (t Theme) FilterForm(ic Icons) filterform.Styles {
	s := filterform.DefaultStyles(t.Dark)
	s.ErrorGlyph = ic.Error
	s.ErrorSeparator, s.ErrorEllipsis = ic.Separator, ic.Ellipsis
	c := lipgloss.Color
	p := t.Palette
	s.Tab = s.Tab.Foreground(c(p.Muted))
	s.ActiveTab = s.ActiveTab.Foreground(c(p.Accent))
	s.Gutter = s.Gutter.Foreground(c(p.Accent))
	s.Label = s.Label.Foreground(c(p.Muted))
	s.FocusedLabel = s.FocusedLabel.Foreground(c(p.Foreground))
	s.Option = s.Option.Foreground(c(p.Subtle))
	s.Selected = s.Selected.Foreground(c(p.Foreground))
	s.Active = s.Active.Foreground(c(p.Accent))
	s.Chip = s.Chip.Foreground(c(p.Foreground))
	s.ActiveChip = s.ActiveChip.Foreground(c(p.Accent))
	s.Remove = s.Remove.Foreground(c(p.Subtle))
	s.Add = s.Add.Foreground(c(p.Subtle))
	s.Value = s.Value.Foreground(c(p.Foreground))
	s.Hint = s.Hint.Foreground(c(p.Subtle))
	s.Rule = s.Rule.Foreground(c(p.Border))
	s.Query = s.Query.Foreground(c(p.Foreground))
	s.Cursor = s.Cursor.Foreground(c(p.Accent))
	s.Spinner = s.Spinner.Foreground(c(p.Accent))
	s.Error = s.Error.Foreground(c(p.Error))
	s.Help = t.Help()
	frame := s.Picker.Frame
	s.Picker = t.Picker(ic)
	s.Picker.Frame = frame.BorderForeground(c(p.Border))
	return s
}

// Prompt returns the styles of an input panel, such as the one a comment is
// written in. Only its edge and cursor take the accent.
func (t Theme) Prompt() prompt.Styles {
	s := prompt.DefaultStyles(t.Dark)
	s.Frame = s.Frame.BorderForeground(lipgloss.Color(t.Palette.Accent))
	s.BlurredFrame = s.BlurredFrame.BorderForeground(lipgloss.Color(t.Palette.Border))
	s.Title = s.Title.Foreground(lipgloss.Color(t.Palette.Foreground))
	s.Text = s.Text.Foreground(lipgloss.Color(t.Palette.Foreground))
	s.Placeholder = s.Placeholder.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Cursor = s.Cursor.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Key = s.Key.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Hint = s.Hint.Foreground(lipgloss.Color(t.Palette.Subtle))
	return s
}

// Picker returns the styles of a search popup. Its frame takes the Border
// color, and the accent marks the prompt, the scope, the selection and the
// matches.
func (t Theme) Picker(ic Icons) picker.Styles {
	s := picker.DefaultStyles(t.Dark)
	s.ErrorGlyph = ic.Error
	s.ErrorSeparator, s.ErrorEllipsis = ic.Separator, ic.Ellipsis
	fg, accent := lipgloss.Color(t.Palette.Foreground), lipgloss.Color(t.Palette.Accent)
	muted, subtle := lipgloss.Color(t.Palette.Muted), lipgloss.Color(t.Palette.Subtle)
	s.Frame = s.Frame.BorderForeground(lipgloss.Color(t.Palette.Border))
	s.Prompt = s.Prompt.Foreground(accent)
	s.Text = s.Text.Foreground(fg)
	s.Placeholder = s.Placeholder.Foreground(subtle)
	s.Cursor = s.Cursor.Foreground(accent)
	s.Scope = s.Scope.Foreground(subtle)
	s.ActiveScope = s.ActiveScope.Foreground(accent)
	s.Status = s.Status.Foreground(subtle)
	s.Spinner = s.Spinner.Foreground(accent)
	s.Header = s.Header.Foreground(muted)
	s.Gutter = s.Gutter.Foreground(accent)
	s.Title = s.Title.Foreground(fg)
	s.SelectedTitle = s.SelectedTitle.Foreground(fg)
	s.Match = s.Match.Foreground(accent)
	s.Detail = s.Detail.Foreground(subtle)
	s.Empty = s.Empty.Foreground(muted)
	s.Error = s.Error.Foreground(lipgloss.Color(t.Palette.Error))
	return s
}

// Finder returns the styles of a file finder.
func (t Theme) Finder(ic Icons) finder.Styles {
	s := finder.DefaultStyles(t.Dark)
	s.ErrorGlyph = ic.Error
	s.ErrorSeparator, s.ErrorEllipsis = ic.Separator, ic.Ellipsis
	fg, accent := lipgloss.Color(t.Palette.Foreground), lipgloss.Color(t.Palette.Accent)
	muted, subtle := lipgloss.Color(t.Palette.Muted), lipgloss.Color(t.Palette.Subtle)
	s.Prompt = s.Prompt.Foreground(accent)
	s.Text = s.Text.Foreground(fg)
	s.Placeholder = s.Placeholder.Foreground(subtle)
	s.Cursor = s.Cursor.Foreground(accent)
	s.Gutter = s.Gutter.Foreground(accent)
	s.Dir = s.Dir.Foreground(muted)
	s.Name = s.Name.Foreground(fg)
	s.SelectedName = s.SelectedName.Foreground(fg)
	s.Match = s.Match.Foreground(accent)
	s.Detail = s.Detail.Foreground(subtle)
	s.Status = s.Status.Foreground(subtle)
	s.Note = s.Note.Foreground(lipgloss.Color(t.Palette.Warning))
	s.Spinner = s.Spinner.Foreground(accent)
	s.Empty = s.Empty.Foreground(muted)
	s.Error = s.Error.Foreground(lipgloss.Color(t.Palette.Error))
	return s
}

// KeyHelp returns the styles of the help, which the accent marks as it
// does the finder: its query and the keys.
func (t Theme) KeyHelp() keyhelp.Styles {
	s := keyhelp.DefaultStyles(t.Dark)
	fg, accent := lipgloss.Color(t.Palette.Foreground), lipgloss.Color(t.Palette.Accent)
	muted, subtle := lipgloss.Color(t.Palette.Muted), lipgloss.Color(t.Palette.Subtle)
	s.Title = s.Title.Foreground(fg)
	s.Count = s.Count.Foreground(subtle)
	s.Prompt = s.Prompt.Foreground(accent)
	s.Text = s.Text.Foreground(fg)
	s.Placeholder = s.Placeholder.Foreground(subtle)
	s.Cursor = s.Cursor.Foreground(accent)
	s.Capture = s.Capture.Foreground(accent)
	s.Key = s.Key.Foreground(accent)
	s.Desc = s.Desc.Foreground(fg)
	s.Source = s.Source.Foreground(muted)
	s.Disabled = s.Disabled.Foreground(subtle)
	s.Conflict = s.Conflict.Foreground(lipgloss.Color(t.Palette.Error))
	s.Shadowed = s.Shadowed.Foreground(lipgloss.Color(t.Palette.Warning))
	s.Typed = s.Typed.Foreground(subtle)
	s.Empty = s.Empty.Foreground(muted)
	return s
}

// Calendar returns the styles of a contribution calendar. The levels keep
// their green scale, which reads as contributions whatever the palette, and
// the words take the palette's colors.
func (t Theme) Calendar() calendar.Styles {
	s := calendar.DefaultStyles(t.Dark)
	s.Total = s.Total.Foreground(lipgloss.Color(t.Palette.Foreground))
	s.Month = s.Month.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Weekday = s.Weekday.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Legend = s.Legend.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.Status = s.Status.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Empty = s.Empty.Foreground(lipgloss.Color(t.Palette.Muted))
	return s
}

// Cmdline returns the styles of the command line. The accent marks the
// prompt, the cursor and the chosen candidate, as it does in the finder.
func (t Theme) Cmdline() cmdline.Styles {
	s := cmdline.DefaultStyles(t.Dark)
	fg, accent := lipgloss.Color(t.Palette.Foreground), lipgloss.Color(t.Palette.Accent)
	subtle := lipgloss.Color(t.Palette.Subtle)
	s.Prompt = s.Prompt.Foreground(accent)
	s.Text = s.Text.Foreground(fg)
	s.Placeholder = s.Placeholder.Foreground(subtle)
	s.Cursor = s.Cursor.Foreground(accent)
	s.Candidate = s.Candidate.Foreground(fg)
	s.Detail = s.Detail.Foreground(subtle)
	s.Selected = s.Selected.Background(accent)
	s.More = s.More.Foreground(accent)
	return s
}

// Frame returns the style of the frame around a modal, without its top
// edge, which carries the modal's title and is drawn in Accent.
func (t Theme) Frame() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), false, true, true).
		BorderForeground(lipgloss.Color(t.Palette.Accent)).
		Padding(0, 1)
}

// Help returns the styles of the help line.
func (t Theme) Help() help.Styles {
	s := help.DefaultStyles(t.Dark)
	s.ShortKey = s.ShortKey.Foreground(lipgloss.Color(t.Palette.Muted))
	s.ShortDesc = s.ShortDesc.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.ShortSeparator = s.ShortSeparator.Foreground(lipgloss.Color(t.Palette.Border))
	s.FullKey = s.FullKey.Foreground(lipgloss.Color(t.Palette.Muted))
	s.FullDesc = s.FullDesc.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.FullSeparator = s.FullSeparator.Foreground(lipgloss.Color(t.Palette.Border))
	s.Ellipsis = s.Ellipsis.Foreground(lipgloss.Color(t.Palette.Subtle))
	return s
}
