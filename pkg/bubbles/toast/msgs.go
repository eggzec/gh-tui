package toast

// ExpireMsg removes a toast when its time is up. Forward it to the
// [Model.Update] of the instance that sent it; other instances ignore it.
type ExpireMsg struct {
	id  int
	seq int
}
