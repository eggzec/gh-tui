package filterform

import "context"

// Option configures a form in [New].
type Option func(*settings)

type settings struct {
	parent        context.Context
	query         string
	hasQuery      bool
	helpLine      bool
	editorHeight  int
	width, height int
	keys          KeyMap
	styles        Styles
}

// DefaultEditorHeight is the height of a Multi or Person field's picker.
const DefaultEditorHeight = 7

func defaultSettings() settings {
	return settings{
		parent:       context.Background(),
		helpLine:     true,
		editorHeight: DefaultEditorHeight,
		keys:         DefaultKeyMap(),
		styles:       DefaultStyles(true),
	}
}

// WithQuery sets the query the form starts from, in place of the fields'
// defaults, as SetQuery does.
func WithQuery(q string) Option {
	return func(s *settings) {
		s.query, s.hasQuery = q, true
	}
}

// WithHelpLine sets whether the form shows a line of key help at the
// bottom. The default is true; turn it off when the parent shows the
// form's help itself.
func WithHelpLine(show bool) Option {
	return func(s *settings) {
		s.helpLine = show
	}
}

// WithEditorHeight sets the height of the picker a Multi or Person field
// opens, frame included. The default is DefaultEditorHeight.
func WithEditorHeight(h int) Option {
	return func(s *settings) {
		s.editorHeight = max(h, 3)
	}
}

// WithSize sets the width and height of the form.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = max(width, 0), max(height, 0)
	}
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) {
		s.keys = k
	}
}

// WithStyles sets the styles. The default is DefaultStyles(true).
func WithStyles(st Styles) Option {
	return func(s *settings) {
		s.styles = st
	}
}

// WithContext sets the parent context of every Loader call. Cancel it to
// stop the form's work in flight.
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		if ctx != nil {
			s.parent = ctx
		}
	}
}
