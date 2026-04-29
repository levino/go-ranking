package rating

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// FromRank converts a kyu/dan grade like "5k" or "1d" into a GoR value.
//
// We use the EGF convention 1d = 2050, 1k = 1950, with -100 per kyu.
// Anything below GoR 100 is floored.
func FromRank(s string) (float64, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return 0, fmt.Errorf("empty rank")
	}
	last := t[len(t)-1]
	num, err := strconv.Atoi(t[:len(t)-1])
	if err != nil || num < 1 {
		return 0, fmt.Errorf("invalid rank %q", s)
	}
	switch last {
	case 'k':
		gor := 2050.0 - float64(num)*100.0
		if gor < 100 {
			gor = 100
		}
		return gor, nil
	case 'd':
		return 2050.0 + float64(num-1)*100.0, nil
	case 'p':
		return 2700.0 + float64(num-1)*50.0, nil
	}
	return 0, fmt.Errorf("invalid rank %q", s)
}

// FormatRank returns a human-readable kyu/dan grade for a given GoR.
func FormatRank(gor float64) string {
	switch {
	case gor >= 2700:
		return fmt.Sprintf("%dp", int((gor-2700)/50)+1)
	case gor >= 2050:
		return fmt.Sprintf("%dd", int((gor-2050)/100)+1)
	}
	k := int(math.Ceil((2050.0 - gor) / 100.0))
	if k < 1 {
		k = 1
	}
	if k > 30 {
		k = 30
	}
	return fmt.Sprintf("%dk", k)
}
