package imgcaps

// Cell is the size in pixels of a cell of the terminal, which an image is
// scaled by to cover a whole number of cells.
type Cell struct{ Width, Height int }

// FallbackCell is the cell assumed when the terminal tells neither the
// size of its cells nor that of its window in pixels: a common 8×16 font.
var FallbackCell = Cell{Width: 8, Height: 16}

// Valid reports whether c is a size at all.
func (c Cell) Valid() bool { return c.Width > 0 && c.Height > 0 }

// CellFromWindow gives the cell of a window width×height pixels that holds
// cols×rows cells, as XTWINOPS 14 reports the window, and false when
// either size is unknown. Padding around the cells counts in the window,
// so the cell may come out a pixel large, which a scaled image survives.
func CellFromWindow(width, height, cols, rows int) (Cell, bool) {
	if cols <= 0 || rows <= 0 {
		return Cell{}, false
	}
	c := Cell{Width: width / cols, Height: height / rows}
	return c, c.Valid()
}
