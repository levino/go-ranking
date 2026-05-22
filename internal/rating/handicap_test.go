package rating

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestHandicapRankDifferencePerfectKomi(t *testing.T) {
	// Komi 6 is perfect for territory → zero advantage on any board.
	for _, b := range []BoardSize{Board9, Board13, Board19} {
		if d := HandicapRankDifference(0, b, perfectKomi); !approx(d, 0) {
			t.Errorf("%s komi 6: rank diff = %v, want 0", b, d)
		}
	}
}

func TestHandicapRankDifferenceBoardSize(t *testing.T) {
	// 0 stones, komi 0.5: head start 5.5 points.
	// 9x9 ×6/12 = 2.75 ranks, 13x13 ×3/12 = 1.375, 19x19 ×1/12 ≈ 0.458.
	cases := []struct {
		board BoardSize
		want  float64
	}{
		{Board9, 2.75},
		{Board13, 1.375},
		{Board19, 5.5 / 12},
	}
	for _, c := range cases {
		if d := HandicapRankDifference(0, c.board, 0.5); !approx(d, c.want) {
			t.Errorf("%s komi 0.5: rank diff = %v, want %v", c.board, d, c.want)
		}
	}
}

func TestHandicapRankDifferenceStones(t *testing.T) {
	// 1 stone counts as 0 placed stones (OGS num_extra_moves = h-1).
	if d1, d0 := HandicapRankDifference(1, Board19, 6), HandicapRankDifference(0, Board19, 6); !approx(d1, d0) {
		t.Errorf("1 stone (%v) should equal 0 stones (%v)", d1, d0)
	}
	// 2 stones on 9x9, komi 0.5: head 6-0.5+12 = 17.5 → ×6/12 = 8.75.
	if d := HandicapRankDifference(2, Board9, 0.5); !approx(d, 8.75) {
		t.Errorf("2 stones 9x9: %v, want 8.75", d)
	}
}

func TestHandicapAdjustmentZero(t *testing.T) {
	// Perfect komi, no stones → no rating adjustment.
	for _, b := range []BoardSize{Board9, Board13, Board19} {
		if a := HandicapAdjustment("black", 1000, 0, b, perfectKomi); math.Abs(a) > 1e-6 {
			t.Errorf("%s: adjustment = %v, want 0", b, a)
		}
	}
}

func TestRecommendedNeverOneStone(t *testing.T) {
	for _, board := range []BoardSize{Board9, Board13, Board19} {
		for d := 0.0; d < 1500; d += 25 {
			h := Recommended(1000+d, 1000, board)
			if h.Stones == 1 {
				t.Errorf("%s gap %.0f: got 1 stone (forbidden)", board, d)
			}
		}
	}
}

func TestRecommendedSymmetric(t *testing.T) {
	a, b := 1500.0, 900.0
	for _, board := range []BoardSize{Board9, Board13, Board19} {
		if ha, hb := Recommended(a, b, board), Recommended(b, a, board); ha != hb {
			t.Errorf("asymmetric on %s: %v vs %v", board, ha, hb)
		}
	}
}

// On 9x9 a stone is worth six ranks, so small gaps stay at 0 stones and
// are carried by komi alone.
func TestRecommended9x9KomiFirst(t *testing.T) {
	weak := FromRankMust(t, "20k")
	for _, rk := range []string{"20k", "19k", "18k", "17k", "16k", "15k"} {
		strong := FromRankMust(t, rk)
		h := Recommended(strong, weak, Board9)
		if h.Stones != 0 {
			t.Errorf("9x9 %s vs 20k: %d stones, expected 0 (komi-carried)", rk, h.Stones)
		}
	}
}

func TestRecommendedConsistentWithRating(t *testing.T) {
	// A recommendation should roughly neutralise the gap: applying its
	// handicap rank difference should land near the actual rank gap.
	strong, weak := FromRankMust(t, "8k"), FromRankMust(t, "15k")
	for _, board := range []BoardSize{Board9, Board13, Board19} {
		h := Recommended(strong, weak, board)
		gap := RatingToRank(strong) - RatingToRank(weak)
		got := HandicapRankDifference(h.Stones, board, h.Komi)
		if math.Abs(got-gap) > 1.0 {
			t.Errorf("%s: handicap covers %.2f ranks, gap is %.2f", board, got, gap)
		}
	}
}

func TestParseBoardSize(t *testing.T) {
	for _, s := range []string{"9", "9x9", "9X9"} {
		if b, err := ParseBoardSize(s); err != nil || b != Board9 {
			t.Errorf("%q -> %v, %v", s, b, err)
		}
	}
	if _, err := ParseBoardSize("99"); err == nil {
		t.Error("expected error for 99")
	}
}

func TestFromRankRoundtrip(t *testing.T) {
	for _, rk := range []string{"30k", "20k", "15k", "10k", "1k", "1d", "5d"} {
		r, err := FromRank(rk)
		if err != nil {
			t.Fatalf("FromRank(%q): %v", rk, err)
		}
		if got := FormatRank(r); got != rk {
			t.Errorf("FormatRank(FromRank(%q)) = %q", rk, got)
		}
	}
}

func TestFromRank30k(t *testing.T) {
	r, err := FromRank("30k")
	if err != nil {
		t.Fatal(err)
	}
	if !approx(r, 525) {
		t.Errorf("30k = %v, want 525 (OGS rank 0)", r)
	}
}

func TestFromRankErrors(t *testing.T) {
	for _, bad := range []string{"", "abc", "5x", "0k", "-1d"} {
		if _, err := FromRank(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func FromRankMust(t *testing.T, s string) float64 {
	t.Helper()
	r, err := FromRank(s)
	if err != nil {
		t.Fatalf("FromRank(%q): %v", s, err)
	}
	return r
}
