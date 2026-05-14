// Package rating implements the EGF Go Rating (GoR) algorithm.
//
// The implementation follows the official EGF specification as
// described at https://www.europeangodatabase.eu/EGD/EGF_rating_system.php
// and as referenced by the Apache-licensed reference implementation
// in barcicki/GorCalculator (see Calculator.java in that repo) and
// the Rust skillratings::egf crate. The closed-form variant (since
// April 2021) is used here.
package rating

import "math"

// EGF constants, taken verbatim from the official spec.
const (
	maxGor   = 3300.0 // way above 9p
	bonusGor = 2300.0 // anchor for the bonus function (~ 3 dan)
	minGor   = -900.0 // theoretical floor
)

// con is the rating-volatility coefficient: con(R) = ((3300 - R) / 200)^1.6.
// Lower-rated players have higher con (more volatile rating).
func con(R float64) float64 {
	return math.Pow((maxGor-R)/200, 1.6)
}

// bonus is the small upward correction that keeps the rating from
// deflating in pools of mostly-improving beginners. Formula:
//
//	bonus(R) = ln(1 + exp((2300 - R) / 80)) / 5
func bonus(R float64) float64 {
	return math.Log(1+math.Exp((bonusGor-R)/80)) / 5
}

// beta is the EGF-specific transform that flattens the high end of
// the rating scale (so a 9p vs 9p game produces a finite expected
// score). Defined as beta(R) = -7 * ln(3300 - R).
func beta(R float64) float64 {
	// Clamp R to (minGor, maxGor) so log doesn't go NaN/Inf at the edges.
	if R >= maxGor {
		R = maxGor - 1
	}
	return -7 * math.Log(maxGor-R)
}

// expectedScore returns A's expected fraction of the result. A and B
// are already adjusted for handicap (the handicapped player's rating
// is raised by 100 * (stones - 0.5) — EGF convention).
//
//	SE(A, B) = 1 / (1 + exp(beta(B) - beta(A)))
func expectedScore(A, B float64) float64 {
	return 1 / (1 + math.Exp(beta(B)-beta(A)))
}

// Update returns the new ratings for black and white after a single
// game. handicapStones is the number of placed handicap stones (0 for
// an even game) that go to BLACK. blackWon indicates whether black
// won. Tournament-class modifier is fixed at 1.0 here — we have no
// notion of tournament classes.
//
// Note: the EGF spec models the handicap as a temporary boost to the
// weaker side's rating, scaled at 100 GoR per stone regardless of
// board size. Our calling code (service.RecordGame) does already
// guard against applying a bonus to the stronger player.
func Update(black, white, blackHcpBonus float64, blackWon bool) (newBlack, newWhite float64) {
	effBlack := black + blackHcpBonus
	expBlack := expectedScore(effBlack, white)
	expWhite := 1 - expBlack

	var actBlack, actWhite float64
	if blackWon {
		actBlack, actWhite = 1, 0
	} else {
		actBlack, actWhite = 0, 1
	}

	newBlack = black + con(black)*(actBlack-expBlack) + bonus(black)
	newWhite = white + con(white)*(actWhite-expWhite) + bonus(white)

	// Soft floor at 50 — bonus keeps very-low ratings climbing anyway.
	if newBlack < 50 {
		newBlack = 50
	}
	if newWhite < 50 {
		newWhite = 50
	}
	return
}
