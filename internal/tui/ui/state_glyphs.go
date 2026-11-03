package ui

// StateGlyphs returns the glyph of each state, drawn in the state's color.
func StateGlyphs(t Theme, icons Icons) [NumStates]string {
	var glyphs [NumStates]string
	for i := range glyphs {
		s := State(i)
		glyphs[i] = t.State(s).Render(icons.State(s))
	}
	return glyphs
}
