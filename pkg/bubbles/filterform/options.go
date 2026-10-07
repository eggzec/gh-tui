package filterform

import "context"

// Option configures a form in [New].
type Option func(*settings)

type settings struct {
	parent        context.Context
	query         string
	hasQuery      bool
	tab           Tab
	helpLine      bool
	tabBar        bool
	editorHeight  int
	width, height int
	keys          KeyMap
	keyName       func(string) string
	styles        Styles
	errorText     func(error) (text, hint string)
}

// DefaultEditorHeight is the height of a Multi or Person field's picker.
const DefaultEditorHeight = 7

func defaultSettings() settings {
	return settings{
		parent:       context.Background(),
		helpLine:     true,
		tabBar:       true,
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

// WithKeyNames sets how the help line writes the names of keys and what
// they do, such as "↵" or "↑/k", for a parent whose icon set writes them
// in words. The default writes them as the key map labels them.
func WithKeyNames(name func(string) string) Option {
	return func(s *settings) {
		s.keyName = name
	}
}

// WithTab sets the tab the form opens on. The default is FiltersTab; a
// form without a sort has no other.
func WithTab(t Tab) Option {
	return func(s *settings) {
		s.tab = t
	}
}

// WithTabBar sets whether a form with tabs shows them on its first line.
// The default is true; turn it off when the parent shows the tabs itself,
// from Tabs and Tab.
func WithTabBar(show bool) Option {
	return func(s *settings) {
		s.tabBar = show
	}
}

// WithEditorHeight sets the height of the picker a Multi or Person field
// opens, frame included. The default is DefaultEditorHeight.
func WithEditorHeight(h int) Option {
	return func(s *settings) {
		s.editorHeight = max(h, 3)
	}
}

// WithErrorText sets how a field whose options failed to load reads in
// its open editor, and a failed search of its picker. say returns the
// words for err and a hint, such as "enter to retry", or "" for none; the
// editor shows the hint on the line below, and the picker leaves it out,
// since typing searches again. An empty text shows no error. By default
// the editor says "Couldn't load" and the field, then the first line of
// the error, and names the edit key, which retries.
func WithErrorText(say func(error) (text, hint string)) Option {
	return func(s *settings) {
		s.errorText = say
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
