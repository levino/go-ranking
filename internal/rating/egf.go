// Package rating implements the EGF Go Rating (GoR) algorithm.
//
// References:
//   https://www.europeangodatabase.eu/EGD/EGF_rating_system.php
package rating

import "math"

// con returns the rating-volatility coefficient for a given GoR.
// Values follow the official EGF table in 100-point steps.
func con(gor float64) float64 {
	switch {
	case gor < 100:
		return 116
	case gor < 200:
		return 110
	case gor < 300:
		return 105
	case gor < 400:
		return 100
	case gor < 500:
		return 95
	case gor < 600:
		return 90
	case gor < 700:
		return 85
	case gor < 800:
		return 80
	case gor < 900:
		return 75
	case gor < 1000:
		return 70
	case gor < 1100:
		return 65
	case gor < 1200:
		return 60
	case gor < 1300:
		return 55
	case gor < 1400:
		return 51
	case gor < 1500:
		return 47
	case gor < 1600:
		return 43
	case gor < 1700:
		return 39
	case gor < 1800:
		return 35
	case gor < 1900:
		return 31
	case gor < 2000:
		return 27
	case gor < 2100:
		return 24
	case gor < 2200:
		return 21
	case gor < 2300:
		return 18
	case gor < 2400:
		return 15
	case gor < 2500:
		return 13
	case gor < 2600:
		return 11
	}
	return 10
}

// a returns the steepness parameter for the expected-score curve.
func a(gor float64) float64 {
	switch {
	case gor < 100:
		return 200
	case gor < 200:
		return 195
	case gor < 300:
		return 190
	case gor < 400:
		return 185
	case gor < 500:
		return 180
	case gor < 600:
		return 175
	case gor < 700:
		return 170
	case gor < 800:
		return 165
	case gor < 900:
		return 160
	case gor < 1000:
		return 155
	case gor < 1100:
		return 150
	case gor < 1200:
		return 145
	case gor < 1300:
		return 140
	case gor < 1400:
		return 135
	case gor < 1500:
		return 130
	case gor < 1600:
		return 125
	case gor < 1700:
		return 120
	case gor < 1800:
		return 115
	case gor < 1900:
		return 110
	case gor < 2000:
		return 105
	case gor < 2100:
		return 100
	case gor < 2200:
		return 95
	case gor < 2300:
		return 90
	case gor < 2400:
		return 85
	case gor < 2500:
		return 80
	case gor < 2600:
		return 75
	}
	return 70
}

// bonus is the small upward correction applied to low-rated players,
// counteracting deflation in beginner pools.
func bonus(gor float64) float64 {
	return math.Log(1+math.Exp((2300-gor)/80)) / 5
}

// expected returns the expected score for the player rated p against q,
// with handicap-equivalent rating points already absorbed into q.
func expected(p, q float64) float64 {
	return 1.0 / (1.0 + math.Exp((q-p)/a(p)))
}

// Update returns the new ratings for black and white after a single game.
//
// blackHcp is the rating-equivalent of the handicap (in GoR points)
// granted to the weaker player. It must be non-negative; pass 0 for an
// even game.  blackWon indicates whether black won.
func Update(black, white, blackHcp float64, blackWon bool) (newBlack, newWhite float64) {
	// The handicap effectively raises the weaker player's rating for
	// the purpose of the expected-score calculation.
	effBlack := black + blackHcp

	expBlack := expected(effBlack, white)
	expWhite := 1 - expBlack

	var actBlack, actWhite float64
	if blackWon {
		actBlack, actWhite = 1, 0
	} else {
		actBlack, actWhite = 0, 1
	}

	newBlack = black + con(black)*(actBlack-expBlack) + bonus(black)
	newWhite = white + con(white)*(actWhite-expWhite) + bonus(white)
	// Soft floor: brand-new player can't drop below 50.  In practice
	// the bonus keeps very-low ratings rising over time anyway.
	if newBlack < 50 {
		newBlack = 50
	}
	if newWhite < 50 {
		newWhite = 50
	}
	return
}
