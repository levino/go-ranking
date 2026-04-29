package rating

import (
	"math"
	"testing"
)

func TestUpdateEvenGameReasonable(t *testing.T) {
	// 100 vs 100 even game, black wins -> black gains, white loses,
	// the change should be sizable for low-rated players.
	nb, nw := Update(100, 100, 0, true)
	if nb <= 100 || nw >= 100 {
		t.Fatalf("expected gain/loss; got %.2f / %.2f", nb, nw)
	}
	delta := nb - 100
	if delta < 30 || delta > 80 {
		t.Fatalf("delta out of plausible range: %.2f", delta)
	}
}

func TestUpdateUpsetSwingsMore(t *testing.T) {
	// Strong player loses to weak player -> big rating swing.
	weakWon := func(black, white, hcp float64) (float64, float64) {
		return Update(black, white, hcp, true)
	}
	nb1, nw1 := weakWon(500, 1500, 0)
	if nb1 <= 500 || nw1 >= 1500 {
		t.Fatalf("upset should benefit black: %.2f / %.2f", nb1, nw1)
	}
	if nb1-500 < 50 {
		t.Fatalf("upset gain too small: %.2f", nb1-500)
	}
}

func TestExpectedSymmetric(t *testing.T) {
	a := expected(1000, 1000)
	if math.Abs(a-0.5) > 1e-9 {
		t.Fatalf("expected 0.5, got %.6f", a)
	}
}

func TestHandicapBonusAt19x19(t *testing.T) {
	h := Handicap{Stones: 4, Komi: 0.5}
	if got := h.HandicapBonus(Board19); got != 350 {
		t.Fatalf("19x19 4-stone bonus expected 350, got %.1f", got)
	}
}

func TestRecommendedMonotone(t *testing.T) {
	// Larger differences must never give fewer stones.
	for _, b := range []BoardSize{Board9, Board13, Board19} {
		prev := -1
		for d := 0.0; d <= 1500; d += 50 {
			h := Recommended(1500+d, 1500, b)
			if h.Stones < prev {
				t.Fatalf("non-monotone on %s at d=%.0f: %d after %d", b, d, h.Stones, prev)
			}
			prev = h.Stones
		}
	}
}
