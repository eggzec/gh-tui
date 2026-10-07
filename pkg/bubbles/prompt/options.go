package prompt

// Option configures a prompt in [New].
type Option func(*settings)

type settings struct {
	mode          Mode
	title         string
	value         string
	placeholder   string
	charLimit     int
	width, height int
	keys          KeyMap
	styles        Styles
}

// WithMode sets whether the prompt takes one line or many. The default is
// [MultiLine].
func WithMode(mode Mode) Option {
	return func(s *settings) { s.mode = mode }
}

// WithTitle sets the title shown above the input.
func WithTitle(title string) Option {
	return func(s *settings) { s.title = title }
}

// WithValue sets the text the input starts with. The cursor starts after it.
func WithValue(value string) Option {
	return func(s *settings) { s.value = value }
}

// WithPlaceholder sets the text shown while the input is empty.
func WithPlaceholder(placeholder string) Option {
	return func(s *settings) { s.placeholder = placeholder }
}

// WithCharLimit sets the most characters the input takes. Zero or less,
// the default, means no limit.
func WithCharLimit(n int) Option {
	return func(s *settings) { s.charLimit = max(n, 0) }
}

// WithSize sets the width and height of the prompt.
func WithSize(width, height int) Option {
	return func(s *settings) { s.width, s.height = width, height }
}

// WithKeyMap sets the key bindings, which NewKeyMap makes. Without them
// every binding is disabled, and no key acts.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) { s.keys = k }
}

// WithStyles sets the styles. The default is DefaultStyles(true).
func WithStyles(st Styles) Option {
	return func(s *settings) { s.styles = st }
}
