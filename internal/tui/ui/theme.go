package ui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/tabs"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
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
// borders, and takes its colors from the palette.

// Tabs returns the styles of the tab bar.
func (t Theme) Tabs() tabs.Styles {
	s := tabs.DefaultStyles(t.Dark)
	s.Tab = s.Tab.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Active = s.Active.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Badge = s.Badge.Foreground(lipgloss.Color(t.Palette.Subtle))
	s.ActiveBadge = s.ActiveBadge.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Rule = s.Rule.Foreground(lipgloss.Color(t.Palette.Border))
	s.Indicator = s.Indicator.Foreground(lipgloss.Color(t.Palette.Accent))
	return s
}

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
func (t Theme) Feed() feed.Styles {
	s := feed.DefaultStyles(t.Dark)
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
func (t Theme) Thread() thread.Styles {
	s := thread.DefaultStyles(t.Dark)
	s.Spinner = s.Spinner.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Loading = s.Loading.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Empty = s.Empty.Foreground(lipgloss.Color(t.Palette.Muted))
	s.Error = s.Error.Foreground(lipgloss.Color(t.Palette.Error))
	s.Key = s.Key.Foreground(lipgloss.Color(t.Palette.Accent))
	s.Hint = s.Hint.Foreground(lipgloss.Color(t.Palette.Subtle))
	return s
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
