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
	complete      Complete
	history       []string
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

// WithComplete sets the function that completes the line. Without one,
// the command line shows no candidates.
func WithComplete(f Complete) Option {
	return func(s *settings) { s.complete = f }
}

// WithHistory sets the lines the command line recalls with up and down,
// oldest first, such as the History saved in an earlier session. It keeps
// a copy, up to the limit.
func WithHistory(lines []string) Option {
	return func(s *settings) { s.history = lines }
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
