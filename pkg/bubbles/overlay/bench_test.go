package overlay

import (
	"fmt"
	"testing"
)

func BenchmarkCenter(b *testing.B) {
	for _, size := range [][2]int{{80, 24}, {200, 60}} {
		w, h := size[0], size[1]
		b.Run(fmt.Sprintf("%dx%d", w, h), func(b *testing.B) {
			// A modal of about two thirds of the frame, as a file preview
			// would take.
			bg, fg := frame(w, h), box(frame(w*2/3, h*2/3))
			b.ReportAllocs()
			for b.Loop() {
				_ = Center(bg, fg, w, h)
			}
		})
	}
}
