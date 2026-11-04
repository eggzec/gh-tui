package pulls

// The kind of entry the timeline of a pull request is kept as, and the
// version of its value, as for the other kinds in service.go.
const (
	kindTimeline = "pulltimeline"
	// timelineSchema 1 is the first, 2 keeps the reactions, and
	// 3 whether the viewer gave each.
	timelineSchema = 3
)
