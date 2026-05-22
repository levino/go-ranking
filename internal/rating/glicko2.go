// Package rating implements the Online-Go.com (OGS) rating system.
//
// The Glicko-2 core and the handicap/komi model are ported 1:1 from
// the open-source OGS rating repository online-go/goratings
// (MIT License, © 2020 online-go.com):
//
//   - Glicko-2 core: goratings/math/glicko2.py
//   - Handicap/komi: analysis/util/RatingMath.py
//
// OGS deploys Glicko-2 with aging disabled (AGING_PERIOD_SECONDS = None),
// so the timestamp/aging machinery from glicko2.py is omitted here: with
// aging off it only ever expands a deviation by the negligible factor
// sqrt(phi^2 + volatility^2). Every game is its own one-game rating
// period, exactly as in OGS v5.
package rating

import "math"

// Glicko-2 constants — verbatim from goratings/math/glicko2.py, except
// MIN_RD which uses the value the deployed OGS analysis config applies
// (analysis/util/Config.py: min_rd = 10.0).
const (
	glickoEpsilon   = 0.000001
	glickoTau       = 0.5
	glickoMaxRD     = 500.0
	glickoMinRD     = 10.0
	glickoMinVol    = 0.01
	glickoMaxVol    = 0.15
	glickoMinRating = 100.0
	glickoMaxRating = 6000.0

	// Glicko2Scale converts between the internal mu/phi scale and the
	// user-facing rating scale (r = 173.7178*mu + 1500).
	Glicko2Scale = 173.7178

	// DefaultRating / DefaultDeviation / DefaultVolatility are the
	// Glicko-2 starting values for a player with no prior games.
	DefaultRating     = 1500.0
	DefaultDeviation  = 350.0
	DefaultVolatility = 0.06
)

// Glicko2Entry is a player's rating state within one rating category.
type Glicko2Entry struct {
	Rating     float64
	Deviation  float64
	Volatility float64
}

// NewGlicko2Entry returns an entry, defaulting any zero field to the
// Glicko-2 starting values.
func NewGlicko2Entry(rating, deviation, volatility float64) Glicko2Entry {
	if rating == 0 {
		rating = DefaultRating
	}
	if deviation == 0 {
		deviation = DefaultDeviation
	}
	if volatility == 0 {
		volatility = DefaultVolatility
	}
	return Glicko2Entry{Rating: rating, Deviation: deviation, Volatility: volatility}
}

func (e Glicko2Entry) mu() float64  { return (e.Rating - 1500) / Glicko2Scale }
func (e Glicko2Entry) phi() float64 { return e.Deviation / Glicko2Scale }

// withRatingAdjustment returns a copy of the entry with its rating
// shifted — used to fold a handicap adjustment into an opponent.
func (e Glicko2Entry) withRatingAdjustment(adj float64) Glicko2Entry {
	e.Rating += adj
	return e
}

// ExpectedWinProbability is player e's expected score against white,
// with white's rating already shifted by handicapAdjustment.
//
// Ported from Glicko2Entry.expected_win_probability. OGS calls it with
// ignore_g = true, which (due to the inverted flag in the original)
// applies the real Glicko g-factor.
func (e Glicko2Entry) ExpectedWinProbability(white Glicko2Entry, handicapAdjustment float64) float64 {
	g := 1 / math.Sqrt(1+(3*white.phi()*white.phi())/(math.Pi*math.Pi))
	return 1 / (1 + math.Exp(-g*(e.Rating+handicapAdjustment-white.Rating)/Glicko2Scale))
}

// match pairs an opponent's entry with the outcome (1 win, 0 loss).
type match struct {
	opponent Glicko2Entry
	outcome  float64
}

// glicko2Update computes a player's new entry after the given matches.
// Ported verbatim from glicko2_update in goratings/math/glicko2.py
// (no-aging path: timestamp is always None in the deployed config).
func glicko2Update(player Glicko2Entry, matches []match) Glicko2Entry {
	if len(matches) == 0 {
		return player
	}

	// Step 3/4: accumulate v and delta.
	vSum, deltaSum := 0.0, 0.0
	for _, m := range matches {
		gPhiJ := 1 / math.Sqrt(1+(3*m.opponent.phi()*m.opponent.phi())/(math.Pi*math.Pi))
		E := 1 / (1 + math.Exp(-gPhiJ*(player.mu()-m.opponent.mu())))
		vSum += gPhiJ * gPhiJ * E * (1 - E)
		deltaSum += gPhiJ * (m.outcome - E)
	}
	v := 9999.0
	if vSum != 0 {
		v = 1.0 / vSum
	}
	delta := v * deltaSum

	// Step 5: iterate to find the new volatility.
	phi := player.phi()
	a := math.Log(player.Volatility * player.Volatility)
	f := func(x float64) float64 {
		ex := math.Exp(x)
		return (ex*(delta*delta-phi*phi-v-ex))/(2*((phi*phi+v+ex)*(phi*phi+v+ex))) - (x-a)/(glickoTau*glickoTau)
	}

	A := a
	var B float64
	if delta*delta > phi*phi+v {
		B = math.Log(delta*delta - phi*phi - v)
	} else {
		k := 1.0
		for f(a-k*glickoTau) < 0 {
			k++
		}
		B = a - k*glickoTau
	}

	fA, fB := f(A), f(B)
	for math.Abs(B-A) > glickoEpsilon {
		C := A + (A-B)*fA/(fB-fA)
		fC := f(C)
		if fC*fB <= 0 {
			A, fA = B, fB
		} else {
			fA = fA / 2
		}
		B, fB = C, fC
	}
	newVolatility := math.Exp(A / 2)

	// Step 6/7: new deviation and rating.
	phiStar := math.Sqrt(phi*phi + newVolatility*newVolatility)
	phiPrime := 1 / math.Sqrt(1/(phiStar*phiStar)+1/v)
	muPrime := player.mu() + phiPrime*phiPrime*deltaSum

	return Glicko2Entry{
		Rating:     clamp(Glicko2Scale*muPrime+1500, glickoMinRating, glickoMaxRating),
		Deviation:  clamp(Glicko2Scale*phiPrime, glickoMinRD, glickoMaxRD),
		Volatility: clamp(newVolatility, glickoMinVol, glickoMaxVol),
	}
}

// UpdateGame returns player's new entry after a single game against
// opponent. opponentHandicapAdjustment shifts the opponent's rating to
// account for the handicap (see HandicapAdjustment).
func UpdateGame(player, opponent Glicko2Entry, opponentHandicapAdjustment float64, playerWon bool) Glicko2Entry {
	outcome := 0.0
	if playerWon {
		outcome = 1
	}
	return glicko2Update(player, []match{
		{opponent.withRatingAdjustment(opponentHandicapAdjustment), outcome},
	})
}

// GameSetup describes one game for RateGame.
type GameSetup struct {
	Board    BoardSize
	Stones   int
	Komi     float64
	BlackWon bool
}

// RateGame computes new Glicko-2 entries for both players after a
// single game, for both the overall category and the board-size
// category. Following the OGS v5 rule, the opponent in every update is
// taken from the overall category — only the player's own side differs
// between the overall and the per-board update.
func RateGame(blackOverall, whiteOverall, blackCat, whiteCat Glicko2Entry, g GameSetup) (newBlackOverall, newWhiteOverall, newBlackCat, newWhiteCat Glicko2Entry) {
	whiteAdj := HandicapAdjustment("white", whiteOverall.Rating, g.Stones, g.Board, g.Komi)
	blackAdj := HandicapAdjustment("black", blackOverall.Rating, g.Stones, g.Board, g.Komi)

	newBlackOverall = UpdateGame(blackOverall, whiteOverall, whiteAdj, g.BlackWon)
	newWhiteOverall = UpdateGame(whiteOverall, blackOverall, blackAdj, !g.BlackWon)
	newBlackCat = UpdateGame(blackCat, whiteOverall, whiteAdj, g.BlackWon)
	newWhiteCat = UpdateGame(whiteCat, blackOverall, blackAdj, !g.BlackWon)
	return
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
