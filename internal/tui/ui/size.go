package ui

import (
	"math"
	"strconv"
)

// Size formats a size in bytes in binary units, as short as ls -h
// does: 512B, 1.2K, 34K, 2.1M.
func Size(n int64) string {
	const units = "KMGT"
	if n < 1024 {
		return strconv.FormatInt(max(n, 0), 10) + "B"
	}
	v := float64(n)
	u := -1
	for v >= 1024 && u < len(units)-1 {
		v /= 1024
		u++
	}
	if v < 9.95 {
		return strconv.FormatFloat(v, 'f', 1, 64) + units[u:u+1]
	}
	return strconv.FormatFloat(math.Round(v), 'f', 0, 64) + units[u:u+1]
}
