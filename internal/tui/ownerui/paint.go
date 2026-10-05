package ownerui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Paint is a style rendered once into the sequences around its text, so
// rows are styled by concatenation. It suits styles of one line without
// padding or borders.
type Paint struct {
	pre, post string
}

// NewPaint renders st once into a paint.
func NewPaint(st lipgloss.Style) Paint {
	pre, post, _ := strings.Cut(st.Render("x"), "x")
	return Paint{pre: pre, post: post}
}

// Render styles s, and leaves it empty when it is.
func (p Paint) Render(s string) string {
	if s == "" {
		return ""
	}
	return p.pre + s + p.post
}

// Write writes s, styled, to b.
func (p Paint) Write(b *strings.Builder, s string) {
	if s == "" {
		return
	}
	b.WriteString(p.pre)
	b.WriteString(s)
	b.WriteString(p.post)
}

// Styles are the paints of the profile and the cards.
type Styles struct {
	Name, Login, Text, Muted, Subtle, Accent Paint
	// Cursor and Blurred mark the selected card, while its pane is focused
	// and while it isn't.
	Cursor, Blurred string
}
