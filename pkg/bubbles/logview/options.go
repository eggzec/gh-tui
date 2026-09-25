package logview

// Option configures a log view in [New].
type Option func(*settings)

// TimeMode is how a log view shows the time of each line.
type TimeMode int

const (
	// TimeHidden shows no times.
	TimeHidden TimeMode = iota
	// TimeRelative shows the time since the start of the line's section, or
	// of the log outside sections.
	TimeRelative
	// TimeAbsolute shows the time of day, in the location of the line's
	// time.
	TimeAbsolute
)

// next returns the mode the times key moves to.
func (t TimeMode) next() TimeMode {
	switch t {
	case TimeHidden:
		return TimeRelative
	case TimeRelative:
		return TimeAbsolute
	default:
		return TimeHidden
	}
}

type settings struct {
	width, height int
	keys          KeyMap
	styles        Styles
	tabWidth      int
	lineNumbers   bool
	wrap          bool
	times         TimeMode
	follow        bool
	focusFailed   bool
}

// DefaultTabWidth is the number of columns between tab stops by default,
// the same as a terminal's.
const DefaultTabWidth = 8

func defaultSettings() settings {
	return settings{
		keys:        DefaultKeyMap(),
		styles:      DefaultStyles(true),
		tabWidth:    DefaultTabWidth,
		lineNumbers: true,
		follow:      true,
	}
}

// WithSize sets the width and height, including the status line.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = width, height
	}
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) {
		s.keys = k
	}
}

// WithStyles sets the styles.
func WithStyles(st Styles) Option {
	return func(s *settings) {
		s.styles = st
	}
}

// WithTabWidth sets the number of columns between tab stops. It applies to
// lines set afterwards.
func WithTabWidth(n int) Option {
	return func(s *settings) {
		s.tabWidth = max(n, 1)
	}
}

// WithLineNumbers sets whether the line numbers are shown. They are by
// default.
func WithLineNumbers(show bool) Option {
	return func(s *settings) {
		s.lineNumbers = show
	}
}

// WithWrap sets whether long lines are soft-wrapped instead of scrolled
// sideways. They are scrolled by default.
func WithWrap(wrap bool) Option {
	return func(s *settings) {
		s.wrap = wrap
	}
}

// WithTimeMode sets how the time of each line is shown. Times are hidden
// by default.
func WithTimeMode(t TimeMode) Option {
	return func(s *settings) {
		s.times = t
	}
}

// WithFollow sets whether the view follows appended lines while its cursor
// is on the last line. It does by default.
func WithFollow(follow bool) Option {
	return func(s *settings) {
		s.follow = follow
	}
}

// WithFocusFailed makes every [Model.SetLines] call [Model.FocusFailed], so
// a log opens on what went wrong.
func WithFocusFailed(focus bool) Option {
	return func(s *settings) {
		s.focusFailed = focus
	}
}
