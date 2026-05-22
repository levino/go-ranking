package rating

import (
	"fmt"
	"math"
)

// BoardSize is the side length of the playing board.
type BoardSize int

const (
	Board9  BoardSize = 9
	Board13 BoardSize = 13
	Board19 BoardSize = 19
)

func (b BoardSize) String() string { return fmt.Sprintf("%dx%d", int(b), int(b)) }

func ParseBoardSize(s string) (BoardSize, error) {
	switch s {
	case "9", "9x9", "9X9":
		return Board9, nil
	case "13", "13x13", "13X13":
		return Board13, nil
	case "19", "19x19", "19X19":
		return Board19, nil
	}
	return 0, fmt.Errorf("invalid board size %q", s)
}

// Handicap describes a fair handicap setup for one pairing.
type Handicap struct {
	Stones int     // placed handicap stones (0 = even game)
	Komi   float64 // komi awarded to white
}

// Handicap/komi model — ported from OGS analysis/util/RatingMath.py
// (get_handicap_rank_difference / get_handicap_adjustment). Rules are
// fixed to japanese (territory scoring): perfect komi 6, free-stone
// territorial value 12, no handicap scoring bonus.
const (
	perfectKomi = 6.0  // best estimate of perfect territory komi
	stoneValue  = 12.0 // territorial value of a free stone (2 * perfectKomi)
)

// boardMultiplier scales a head-start in points to a rank difference.
// OGS: ×6 for 9x9, ×3 for 13x13, ×1 for 19x19 (and any other size).
func boardMultiplier(b BoardSize) float64 {
	switch b {
	case Board9:
		return 6
	case Board13:
		return 3
	default:
		return 1
	}
}

// HandicapRankDifference returns Black's advantage, in ranks, for a
// game with the given placed stones and komi on the given board.
// Ported from get_handicap_rank_difference (japanese rules branch).
func HandicapRankDifference(stones int, board BoardSize, komi float64) float64 {
	numExtraMoves := 0
	if stones > 1 {
		numExtraMoves = stones - 1
	}
	blackHeadStart := perfectKomi - komi + stoneValue*float64(numExtraMoves)
	return blackHeadStart * boardMultiplier(board) / stoneValue
}

// HandicapAdjustment returns the rating delta to apply to one player so
// the expected-score calculation accounts for the handicap. Ported from
// get_handicap_adjustment: it shifts the player by the handicap rank
// difference in rank space, then converts back to a rating delta.
//
// player must be "black" or "white".
func HandicapAdjustment(player string, rating float64, stones int, board BoardSize, komi float64) float64 {
	rankDiff := HandicapRankDifference(stones, board, komi)
	var effectiveRank float64
	if player == "black" {
		effectiveRank = RatingToRank(rating) + rankDiff
	} else {
		effectiveRank = RatingToRank(rating) - rankDiff
	}
	return RankToRating(effectiveRank) - rating
}

// recommendation komi range: from the even-game komi down to a reverse
// komi of -6.5; beyond that an extra stone is placed instead.
const (
	recKomiMax   = 6.5
	recKomiMin   = -6.5
	recMaxStones = 9
)

// Recommended returns the suggested handicap for a pairing, derived by
// inverting the OGS handicap formula: it finds the (stones, komi) whose
// HandicapRankDifference matches the players' rank gap. Small gaps are
// absorbed by komi alone; a stone is added only once komi would leave
// the playable range. This keeps 9x9 games finely graded — on 9x9 a
// stone is worth six ranks, so komi carries roughly the first six.
func Recommended(stronger, weaker float64, board BoardSize) Handicap {
	gap := RatingToRank(stronger) - RatingToRank(weaker)
	if gap < 0 {
		gap = -gap
	}
	// Target head start (in points): invert rankDiff = head*mult/stoneValue.
	head := gap * stoneValue / boardMultiplier(board)

	for _, stones := range []int{0, 2, 3, 4, 5, 6, 7, 8, 9} {
		numExtra := 0
		if stones > 1 {
			numExtra = stones - 1
		}
		komi := perfectKomi - head + stoneValue*float64(numExtra)
		if komi >= recKomiMin || stones == recMaxStones {
			return Handicap{Stones: stones, Komi: NormalizeKomi(clamp(komi, -50, recKomiMax))}
		}
	}
	return Handicap{Stones: 0, Komi: recKomiMax}
}

// NormalizeKomi snaps a komi value to the nearest half-integer of the
// form n+0.5 (…, -1.5, -0.5, 0.5, 1.5, …). A whole-number komi is never
// produced: with integer area/territory scoring it would permit a drawn
// game (jigo), and go-liga has no result for a draw.
func NormalizeKomi(v float64) float64 {
	return math.Round(v-0.5) + 0.5
}
