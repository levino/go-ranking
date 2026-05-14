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

// Per-stone GoR-equivalent value used when converting a placed
// handicap into a rating bonus for the expected-score calculation.
//
// 19×19 uses the EGF value of 100 GoR per stone — one rank in the EGF
// system maps to one handicap stone on the full board.
//
// Smaller boards use deliberately lower values for the RATING
// computation, even though the THEORETICAL stone value is higher on
// small boards (Wikipedia: 13×13 ≈ 2.5–3 ranks/stone, 9×9 ≈ 6
// ranks/stone). The theoretical scaling drives only when stones are
// recommended (rec9/rec13 are conservative with stone counts); for
// rating points we use lower values to reflect how much we TRUST the
// placed handicap to actually neutralize the gap — on tiny boards a
// single overlooked move flips the result, so the rating shift on a
// won/lost handicap game should be smaller, not larger.
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

// Vorgabe-Tabelle: 1 Stein gibt es bewusst nicht. Ein einzelner
// Vorgabestein ist im Go sinnlos — er ist zu schwach für nennenswerten
// Ausgleich, aber zu viel, um als "fast ebene" Partie durchzugehen.
// Stattdessen wird der schmale Bereich zwischen "Komi 6,5" und
// "2 Steine" durch Komi-Anpassung überbrückt: Rückkomi (negatives
// Komi für Schwarz) bzw. reduziertes Komi für Weiß.

func rec9(d float64) Handicap {
	switch {
	case d < 70:
		return Handicap{0, 6.5}
	case d < 140:
		return Handicap{0, 0.5} // leichter Ausgleich über Komi
	case d < 210:
		return Handicap{0, -5.5} // Rückkomi statt 1 Stein
	case d < 280:
		return Handicap{2, 0.5}
	case d < 350:
		return Handicap{3, 0.5}
	case d < 420:
		return Handicap{4, 0.5}
	}
	return Handicap{4, -5.5}
}

func rec13(d float64) Handicap {
	switch {
	case d < 100:
		return Handicap{0, 6.5}
	case d < 200:
		return Handicap{0, 0.5} // leichter Ausgleich über Komi
	case d < 300:
		return Handicap{0, -5.5} // Rückkomi statt 1 Stein
	case d < 450:
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

// rec19 — auf dem vollen Brett wird auf jeden Fall mit Steinen
// ausgeglichen, **nicht** mit Rückkomi. Die Steine-Skala reicht bis 9;
// erst wenn diese ausgeschöpft sind, kommt Rückkomi als Ergänzung
// dazu (siehe Sensei's Library / EGF-Praxis).
//
// Zwischen "0 Steine + Komi 0,5" und "2 Steine + Komi 0,5" liegt nur
// eine Komi-Stufe (kein 1-Stein), entsprechend der Go-Konvention.
func rec19(d float64) Handicap {
	switch {
	case d < 100:
		return Handicap{0, 6.5}
	case d < 200:
		return Handicap{0, 0.5} // Komi-Adjust statt 1 Stein
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
	case d < 1000:
		return Handicap{9, 0.5}
	}
	// >1000 GoR diff: 9 Steine sind ausgeschöpft, Rückkomi als zusätzlicher
	// Ausgleich für den Schwächeren (Komi für Weiß wird negativ).
	return Handicap{9, -5.5}
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
