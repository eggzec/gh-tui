package cmdline

// Option configures a command line in [New].
type Option func(*settings)

type settings struct {
	prompt        string
	value         string
	placeholder   string
	charLimit     int
	width, height int
	keys          KeyMap
	styles        Styles
}

// WithPrompt sets the prompt shown before the line. The default is ":".
func WithPrompt(p string) Option {
	return func(s *settings) { s.prompt = p }
}

// WithValue sets the text the line starts with. The cursor starts after it.
func WithValue(v string) Option {
	return func(s *settings) { s.value = v }
}

// WithPlaceholder sets the text shown while the line is empty, on one
// line.
func WithPlaceholder(p string) Option {
	return func(s *settings) { s.placeholder = p }
}

// WithCharLimit sets the most characters the line takes. Zero or less,
// the default, means no limit.
func WithCharLimit(n int) Option {
	return func(s *settings) { s.charLimit = max(n, 0) }
}

// WithSize sets the width and the most rows the command line may take,
// as [Model.SetSize] does. The height defaults to [MaxHeight].
func WithSize(width, height int) Option {
	return func(s *settings) { s.width, s.height = width, height }
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) { s.keys = k }
}

// WithStyles sets the styles. The default is DefaultStyles(true).
func WithStyles(st Styles) Option {
	return func(s *settings) { s.styles = st }
}
