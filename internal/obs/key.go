package obs

import "strings"

// LogKey returns key, the key of a cache entry or of a read, as a record
// names it: without its query, since that may hold what the user typed,
// such as a filter or a search, and a debug log is what a user pastes
// into an issue. Keys put such text after a '?' for that reason.
func LogKey(key string) string {
	entry, _, _ := strings.Cut(key, "?")
	return entry
}
