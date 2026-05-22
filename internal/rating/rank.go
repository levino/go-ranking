package rating

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Rank conversion uses the OGS "log" rating-to-rank system
// (analysis/util/RatingMath.py, configure_rating_to_rank, default
// "auto" → "log"), with the CLI default constants a = 525, c = 23.15:
//
//	rating = 525 * exp(rank / 23.15)
//	rank   = ln(rating / 525) * 23.15
//
// The continuous rank number follows the OGS convention: rank 0 = 30k,
// rank 29 = 1k, rank 30 = 1d, rank 38 = 9d. This is why the system
// reaches all the way down to 30 kyu — rank 0 is its natural floor.
const (
	ogsRankA = 525.0
	ogsRankC = 23.15
)

// RankToRating maps an OGS continuous rank number to a rating.
func RankToRating(rank float64) float64 {
	return ogsRankA * math.Exp(rank/ogsRankC)
}

// RatingToRank maps a rating to an OGS continuous rank number.
func RatingToRank(rating float64) float64 {
	if rating < 1 {
		rating = 1
	}
	return math.Log(rating/ogsRankA) * ogsRankC
}

// FromRank converts a kyu/dan grade like "15k" or "1d" into a rating.
// Supported down to 30k (rank 0) — the OGS floor.
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
		if num > 30 {
			num = 30
		}
		return RankToRating(float64(30 - num)), nil
	case 'd':
		return RankToRating(float64(29 + num)), nil
	case 'p':
		// Pro ranks aren't on the amateur scale; place 1p just above 9d.
		return RankToRating(float64(38 + num)), nil
	}
	return 0, fmt.Errorf("invalid rank %q", s)
}

// FormatRank returns a human-readable kyu/dan grade for a rating.
func FormatRank(rating float64) string {
	rank := int(math.Round(RatingToRank(rating)))
	if rank >= 30 {
		return fmt.Sprintf("%dd", rank-29)
	}
	k := 30 - rank
	if k > 30 {
		k = 30
	}
	if k < 1 {
		k = 1
	}
	return fmt.Sprintf("%dk", k)
}

// FormatRankPrecise returns a fractional grade like "11.0k" or "2.4d",
// matching the OGS profile display.
func FormatRankPrecise(rating float64) string {
	rank := RatingToRank(rating)
	if rank >= 29.5 {
		return fmt.Sprintf("%.1fd", rank-29)
	}
	k := 30 - rank
	if k > 30 {
		k = 30
	}
	return fmt.Sprintf("%.1fk", k)
}

// RankUncertainty expresses a rating deviation as a +/- span in rank
// units, the way the OGS profile shows "± 1.5".
func RankUncertainty(rating, deviation float64) float64 {
	hi := RatingToRank(rating + deviation)
	lo := RatingToRank(rating - deviation)
	return (hi - lo) / 2
}

// RatingFromLegacyGoR converts an old EGF-GoR value (the pre-OGS rating
// scale, 1d = 2050, 100 GoR per rank) into an OGS rating. Used once by
// the v2→v3 store migration.
func RatingFromLegacyGoR(gor float64) float64 {
	// EGF rank number (kyu/dan) → OGS continuous rank: gor/100 + 9.5.
	return RankToRating(gor/100 + 9.5)
}
