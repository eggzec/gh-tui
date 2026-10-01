package core

// maxLogin is the longest login ValidLogin takes. GitHub's are at most 39
// characters; an Enterprise Server's, made from a directory's names, may
// be longer.
const maxLogin = 64

// ValidLogin reports whether s can be a GitHub login: 1 to 64 ASCII
// letters, digits, hyphens and underscores. A login is shown on screen
// and kept on disk, so one from an answer or a file that isn't such a
// name, such as one with control characters, is taken for none.
func ValidLogin(s string) bool {
	if s == "" || len(s) > maxLogin {
		return false
	}
	for _, r := range s {
		if !loginRune(r) {
			return false
		}
	}
	return true
}

// loginRune reports whether r may be in a login.
func loginRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return r == '-' || r == '_'
}
