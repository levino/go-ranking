package rating

import "testing"

func TestRecommended9x9MaxStones(t *testing.T) {
	// 9x9 caps at 4 stones with komi adjustment for huge gaps.
	h := Recommended(2000, 100, Board9)
	if h.Stones != 4 {
		t.Fatalf("9x9 should cap at 4 stones, got %d", h.Stones)
	}
	if h.Komi != -5.5 {
		t.Fatalf("9x9 ≥420 should give komi -5.5, got %.1f", h.Komi)
	}
}

func TestRecommended13x13Sample(t *testing.T) {
	cases := []struct {
		gap  float64
		want int
		komi float64
	}{
		{50, 0, 6.5},
		{150, 0, 0.5},
		{250, 0, -5.5}, // Rückkomi statt 1 Stein
		{350, 2, 0.5},
		{500, 3, 0.5},
		{700, 4, 0.5},
		{900, 5, 0.5},
		{1500, 6, 0.5},
	}
	for _, c := range cases {
		h := Recommended(1000+c.gap, 1000, Board13)
		if h.Stones != c.want || h.Komi != c.komi {
			t.Errorf("13x13 gap %.0f: got %d/%v, want %d/%v",
				c.gap, h.Stones, h.Komi, c.want, c.komi)
		}
	}
}

func TestRecommended19x19Spec(t *testing.T) {
	cases := []struct {
		gap  float64
		want int
		komi float64
	}{
		{0, 0, 6.5},
		{150, 0, 0.5},
		{250, 0, -5.5}, // Rückkomi statt 1 Stein
		{350, 2, 0.5},
		{450, 3, 0.5},
		{550, 4, 0.5},
		{650, 5, 0.5},
		{750, 6, 0.5},
		{850, 7, 0.5},
		{950, 8, 0.5},
		{2000, 9, 0.5},
	}
	for _, c := range cases {
		h := Recommended(2000+c.gap, 2000, Board19)
		if h.Stones != c.want || h.Komi != c.komi {
			t.Errorf("19x19 gap %.0f: got %d/%v, want %d/%v",
				c.gap, h.Stones, h.Komi, c.want, c.komi)
		}
	}
}

// Es darf nie ein 1-Stein-Vorschlag entstehen — der einzelne Vorgabe-
// stein ist sinnlos. Stattdessen wird über Komi (inkl. Rückkomi)
// ausgeglichen, bis ab 2 Steinen wieder eine echte Vorgabe greift.
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
	// Recommended should not depend on the order of (a, b).
	a, b := 1500.0, 1100.0
	for _, board := range []BoardSize{Board9, Board13, Board19} {
		ha := Recommended(a, b, board)
		hb := Recommended(b, a, board)
		if ha != hb {
			t.Errorf("asymmetric on %s: %v vs %v", board, ha, hb)
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
	cases := map[string]float64{
		"1d":  2050,
		"2d":  2150,
		"1k":  1950,
		"5k":  1550,
		"15k": 550,
		"30k": 100,
		"1p":  2700,
	}
	for s, want := range cases {
		got, err := FromRank(s)
		if err != nil || got != want {
			t.Errorf("FromRank(%q) = %.1f, %v; want %.1f", s, got, err, want)
		}
	}
}

func TestFromRankErrors(t *testing.T) {
	for _, bad := range []string{"", "abc", "5x", "0k", "-1d"} {
		if _, err := FromRank(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestFormatRank(t *testing.T) {
	cases := map[float64]string{
		2050: "1d",
		2150: "2d",
		1950: "1k",
		1550: "5k",
		100:  "20k",
		2700: "1p",
	}
	for gor, want := range cases {
		if got := FormatRank(gor); got != want {
			t.Errorf("FormatRank(%.0f) = %s, want %s", gor, got, want)
		}
	}
}

func TestHandicapBonusAcrossBoards(t *testing.T) {
	h := Handicap{Stones: 4, Komi: 0.5}
	if g := h.HandicapBonus(Board9); g != (4-0.5)*50 {
		t.Errorf("9x9 4-stone bonus: %f", g)
	}
	if g := h.HandicapBonus(Board13); g != (4-0.5)*70 {
		t.Errorf("13x13 4-stone bonus: %f", g)
	}
	if g := h.HandicapBonus(Board19); g != (4-0.5)*100 {
		t.Errorf("19x19 4-stone bonus: %f", g)
	}
	if g := (Handicap{}).HandicapBonus(Board19); g != 0 {
		t.Errorf("0-stone bonus must be 0, got %f", g)
	}
}
