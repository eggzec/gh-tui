package config

// removedMessage is the error for a setting at path that no longer exists,
// with hint saying what to use instead.
func removedMessage(path, hint string) string {
	return path + ": removed: use " + hint + "."
}
