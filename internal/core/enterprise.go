package core

import (
	"strconv"
	"strings"
)

// MinEnterprise is the oldest GitHub Enterprise Server version that
// gh-tui supports, for REST and GraphQL alike. GitHub supports each
// release for about a year and ships one a quarter; this is the oldest
// release GitHub supports less two, since servers lag behind their
// releases. Raise it as releases reach the end of their support, about a
// quarter apart. The schema check in CI checks what gh-tui sends against
// the schemas of this version and of github.com.
const MinEnterprise = "3.16"

// EnterpriseSupported reports whether version, such as 3.17.4, as GitHub
// Enterprise Server tells it, is MinEnterprise or newer. A version that
// doesn't read as one is taken for supported, since nothing says it isn't.
func EnterpriseSupported(version string) bool {
	have, ok := majorMinor(version)
	if !ok {
		return true
	}
	want, _ := majorMinor(MinEnterprise)
	return have[0] > want[0] || have[0] == want[0] && have[1] >= want[1]
}

// majorMinor returns the major and minor numbers of version.
func majorMinor(version string) ([2]int, bool) {
	parts := strings.SplitN(strings.TrimSpace(version), ".", 3)
	if len(parts) < 2 {
		return [2]int{}, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	return [2]int{major, minor}, err1 == nil && err2 == nil
}
