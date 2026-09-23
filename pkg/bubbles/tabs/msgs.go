package tabs

// ChangeMsg reports that the active tab changed. Parents use it to load the
// section's content lazily, and check ID to tell tab bars apart.
type ChangeMsg struct {
	ID    int
	Index int
}
