package filterform

// Capture is what takes the keys while the form captures them.
type Capture int

const (
	// CaptureNone is no capture: the form takes only its own keys.
	CaptureNone Capture = iota
	// CaptureQuery is the query line, which types its text.
	CaptureQuery
	// CaptureEditor is the editor of a field, which types its text.
	CaptureEditor
	// CapturePicker is the filter of a dropdown, which types its text.
	CapturePicker
)

// CapturedBy reports what takes every key now, or CaptureNone when
// Capturing is false.
func (m Model) CapturedBy() Capture {
	switch {
	case !m.Capturing():
		return CaptureNone
	case m.picking:
		return CapturePicker
	case m.mode == insertMode && m.row != m.queryRow():
		return CaptureEditor
	}
	return CaptureQuery
}
