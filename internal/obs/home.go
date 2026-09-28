package obs

import (
	"os"
	"path/filepath"
	"strings"
)

// ShortHome returns s, a path or a text that holds paths such as an
// error, with the user's home directory written as ~, so that what is
// logged or shown doesn't spell out where the home directory is, which
// often names the user. Only a whole path is shortened, not one that the
// home directory's path merely starts, such as /home/alice for /home/ali.
func ShortHome(s string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return s
	}
	// A home at the root would make every path start with ~.
	if home = filepath.Clean(home); filepath.Dir(home) == home {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, home)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := i + len(home)
		whole := (i == 0 || !pathByte(s[i-1])) && (end == len(s) || s[end] == filepath.Separator || !pathByte(s[end]))
		if whole {
			b.WriteString(s[:i])
			b.WriteByte('~')
		} else {
			b.WriteString(s[:end])
		}
		s = s[end:]
	}
}

// pathByte reports whether c may be part of a path segment, or separate
// two, so that a home directory next to it is part of a longer path.
func pathByte(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte(`/\._-~`, c) >= 0
}
