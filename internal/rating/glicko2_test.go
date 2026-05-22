package rating

import (
	"math"
	"testing"
)

// TestGlicko2Update is the canonical Glickman worked example, ported
// from goratings unit_tests/test_glicko2.py (test_glicko2). A player
// rated 1500±200 plays three opponents (win, loss, loss).
func TestGlicko2Update(t *testing.T) {
	player := NewGlicko2Entry(1500, 200, 0.06)
	a := NewGlicko2Entry(1400, 30, 0.06)
	b := NewGlicko2Entry(1550, 100, 0.06)
	c := NewGlicko2Entry(1700, 300, 0.06)

	got := glicko2Update(player, []match{
		{a, 1},
		{b, 0},
		{c, 0},
	})

	if r := math.Round(got.Rating*10) / 10; r != 1464.1 {
		t.Errorf("rating = %.4f, want 1464.1", got.Rating)
	}
	if d := math.Round(got.Deviation*10) / 10; d != 151.5 {
		t.Errorf("deviation = %.4f, want 151.5", got.Deviation)
	}
}

func TestExpectedWinProbabilitySelf(t *testing.T) {
	p := NewGlicko2Entry(1500, 200, 0.06)
	if got := p.ExpectedWinProbability(p, 0); got != 0.5 {
		t.Errorf("self win probability = %v, want 0.5", got)
	}
}

func TestGlicko2UpdateNoMatches(t *testing.T) {
	p := NewGlicko2Entry(1500, 200, 0.06)
	if got := glicko2Update(p, nil); got != p {
		t.Errorf("no-match update changed entry: %+v", got)
	}
}

// A win against a stronger player should raise the rating; a loss
// against a weaker one should lower it.
func TestGlicko2UpdateDirection(t *testing.T) {
	p := NewGlicko2Entry(1500, 120, 0.06)
	stronger := NewGlicko2Entry(1800, 120, 0.06)
	weaker := NewGlicko2Entry(1200, 120, 0.06)

	if up := glicko2Update(p, []match{{stronger, 1}}); up.Rating <= p.Rating {
		t.Errorf("win vs stronger did not raise rating: %.1f", up.Rating)
	}
	if down := glicko2Update(p, []match{{weaker, 0}}); down.Rating >= p.Rating {
		t.Errorf("loss vs weaker did not lower rating: %.1f", down.Rating)
	}
}
