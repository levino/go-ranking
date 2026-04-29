package rating

import "fmt"

// BoardSize is the side length of the playing board.
type BoardSize int

const (
	Board9  BoardSize = 9
	Board13 BoardSize = 13
	Board19 BoardSize = 19
)

func (b BoardSize) String() string {
	return fmt.Sprintf("%dx%d", int(b), int(b))
}

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

// Handicap describes a fair handicap setup for a single pairing.
type Handicap struct {
	Stones int     // number of placed handicap stones (0 = even game)
	Komi   float64 // komi awarded to white
}

// Per-stone GoR-equivalent value.  Smaller boards mean less ground
// covered per stone, hence less rating advantage.
const (
	stoneValue9  = 50
	stoneValue13 = 70
	stoneValue19 = 100
)

func stoneValue(b BoardSize) float64 {
	switch b {
	case Board9:
		return stoneValue9
	case Board13:
		return stoneValue13
	}
	return stoneValue19
}

// Recommended returns the recommended handicap for a pairing where
// `stronger` is the higher rated player and `weaker` the other.
func Recommended(stronger, weaker float64, board BoardSize) Handicap {
	d := stronger - weaker
	if d < 0 {
		d = -d
	}
	switch board {
	case Board9:
		return rec9(d)
	case Board13:
		return rec13(d)
	default:
		return rec19(d)
	}
}

func rec9(d float64) Handicap {
	switch {
	case d < 100:
		return Handicap{0, 6.5}
	case d < 200:
		return Handicap{0, 0.5}
	case d < 300:
		return Handicap{1, 0.5}
	case d < 400:
		return Handicap{2, 0.5}
	case d < 500:
		return Handicap{3, 0.5}
	case d < 600:
		return Handicap{4, 0.5}
	}
	return Handicap{4, -5.5}
}

func rec13(d float64) Handicap {
	switch {
	case d < 100:
		return Handicap{0, 6.5}
	case d < 200:
		return Handicap{0, 0.5}
	case d < 400:
		return Handicap{2, 0.5}
	case d < 600:
		return Handicap{3, 0.5}
	case d < 800:
		return Handicap{4, 0.5}
	case d < 1000:
		return Handicap{5, 0.5}
	}
	return Handicap{6, 0.5}
}

func rec19(d float64) Handicap {
	switch {
	case d < 100:
		return Handicap{0, 6.5}
	case d < 200:
		return Handicap{0, 0.5}
	case d < 300:
		return Handicap{2, 0.5}
	case d < 400:
		return Handicap{3, 0.5}
	case d < 500:
		return Handicap{4, 0.5}
	case d < 600:
		return Handicap{5, 0.5}
	case d < 700:
		return Handicap{6, 0.5}
	case d < 800:
		return Handicap{7, 0.5}
	case d < 900:
		return Handicap{8, 0.5}
	}
	return Handicap{9, 0.5}
}

// HandicapBonus returns the GoR-equivalent of the placed handicap
// stones for use in the rating expected-score calculation.
func (h Handicap) HandicapBonus(board BoardSize) float64 {
	if h.Stones == 0 {
		return 0
	}
	// EGF convention: H stones ≈ (H - 0.5) * stoneValue.
	return (float64(h.Stones) - 0.5) * stoneValue(board)
}
