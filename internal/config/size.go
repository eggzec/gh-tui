package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Size is a number of bytes. In YAML it is a plain integer of bytes or a
// string with a unit, such as "64KiB" or "1MB": B, kB, MB and GB count in
// powers of 1000, KiB, MiB and GiB in powers of 1024. Units ignore case.
type Size int64

// Common sizes.
const (
	KiB Size = 1 << 10
	MiB Size = 1 << 20
	GiB Size = 1 << 30
)

var sizeUnits = map[string]Size{
	"":    1,
	"b":   1,
	"kb":  1000,
	"mb":  1000 * 1000,
	"gb":  1000 * 1000 * 1000,
	"kib": KiB,
	"mib": MiB,
	"gib": GiB,
}

// ParseSize parses a size such as "512", "64KiB" or "1.5 MB".
func ParseSize(s string) (Size, error) {
	t := strings.TrimSpace(s)
	i := strings.IndexFunc(t, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if i < 0 {
		i = len(t)
	}
	num, unit := t[:i], strings.ToLower(strings.TrimSpace(t[i:]))
	mul, ok := sizeUnits[unit]
	if num == "" || !ok {
		return 0, fmt.Errorf("invalid size %q: want bytes or a number with a unit like 64KiB or 1MB", s)
	}
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	v := n * float64(mul)
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("invalid size %q: too large", s)
	}
	return Size(v), nil
}

// UnmarshalYAML reads an integer of bytes or a size string.
func (s *Size) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: want a size like 64KiB", n.Line)
	}
	v, err := ParseSize(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*s = v
	return nil
}

// String formats s in the largest binary unit that divides it, such as
// "64KiB", or in bytes.
func (s Size) String() string {
	for _, u := range []struct {
		size Size
		name string
	}{{GiB, "GiB"}, {MiB, "MiB"}, {KiB, "KiB"}} {
		if s != 0 && s%u.size == 0 {
			return strconv.FormatInt(int64(s/u.size), 10) + u.name
		}
	}
	return strconv.FormatInt(int64(s), 10) + "B"
}
