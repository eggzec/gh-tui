package calendar

// Option configures a calendar in [New].
type Option func(*settings)

type settings struct {
	weeks     [][]Day
	days      int
	total     int
	width     int
	height    int
	glyph     string
	emptyText string
	keyMap    KeyMap
	styles    Styles
	focused   bool
}

func defaultSettings() settings {
	return settings{
		total:     -1,
		glyph:     DefaultGlyph,
		emptyText: "No contributions to show.",
		keyMap:    NewKeyMap(unbound),
		styles:    DefaultStyles(true),
	}
}

// WithWeeks sets the days to show. See [Model.SetWeeks].
func WithWeeks(weeks [][]Day) Option {
	return func(s *settings) {
		s.weeks = weeks
	}
}

// WithRange shows only the most recent days. See [Model.SetRange].
func WithRange(days int) Option {
	return func(s *settings) {
		s.days = max(days, 0)
	}
}

// WithTotal sets the total shown above the grid. See [Model.SetTotal].
func WithTotal(total int) Option {
	return func(s *settings) {
		s.total = max(total, -1)
	}
}

// WithSize sets the width and height of the calendar in cells.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = max(width, 0), max(height, 0)
	}
}

// WithGlyph sets the glyph of a day, for terminals that draw
// [DefaultGlyph] badly. It must be one cell wide, such as "▪" or "#";
// other glyphs fall back to DefaultGlyph.
func WithGlyph(glyph string) Option {
	return func(s *settings) {
		if glyph != "" {
			s.glyph = glyph
		}
	}
}

// WithEmptyText sets the text shown when there are no days.
func WithEmptyText(text string) Option {
	return func(s *settings) {
		s.emptyText = text
	}
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) {
		s.keyMap = k
	}
}

// WithStyles sets the styles.
func WithStyles(st Styles) Option {
	return func(s *settings) {
		s.styles = st
	}
}

// WithFocused sets whether the calendar starts focused and reacts to keys.
func WithFocused(focused bool) Option {
	return func(s *settings) {
		s.focused = focused
	}
}
