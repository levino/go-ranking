// Package tournament is the pure pairing engine for go-liga tournaments.
//
// It is deliberately storage- and rating-agnostic: it works on plain
// numeric IDs and scores so it can be unit-tested in isolation and
// reused from both the web UI and (later) the MCP API. Two formats are
// supported:
//
//   - Round Robin — everyone plays everyone once. The whole schedule is
//     generated up front with the circle method; for an odd field a
//     "ghost" opponent produces one bye per round.
//   - McMahon — players start with a rank-derived score and are paired
//     each round against the nearest-scoring opponent they have not met.
//     Generated one round at a time from the results so far.
//
// The engine assigns nominal Black/White for colour balance in even
// (ebenbürtig) tournaments. In handicap tournaments the caller re-derives
// the real colours (weaker player takes Black) and the handicap; the
// engine's colour choice is then only a hint.
package tournament

import (
	"math"
	"sort"
)

// Format identifies a tournament pairing system.
type Format string

const (
	RoundRobin Format = "round_robin"
	McMahon    Format = "mcmahon"
)

// Valid reports whether f is a known format.
func (f Format) Valid() bool { return f == RoundRobin || f == McMahon }

// ByePoints is what a player scores for a bye (no opponent in a round).
// Casual club practice — and the friendliest choice for a kids' event —
// is to count a bye as a win; EGF rules would use ½. Kept as a named
// constant so it is trivial to change.
const ByePoints = 1.0

// Player is the engine's view of a competitor. Rating drives seeding and
// the McMahon starting score; StartScore is the McMahon starting score
// (0 for Round Robin, where only wins count).
type Player struct {
	ID         int64
	Rating     float64
	StartScore float64
}

// Pairing is one board of one round. A bye is expressed as WhiteID == 0
// (the lone player, in BlackID, sits the round out and scores ByePoints).
type Pairing struct {
	Board   int
	BlackID int64
	WhiteID int64 // 0 → BlackID has a bye this round
}

// IsBye reports whether this pairing is a bye rather than a real game.
func (p Pairing) IsBye() bool { return p.WhiteID == 0 }

// Result records a completed pairing so later rounds and the standings
// can be derived. For a bye set WhiteID == 0 and WinnerID == BlackID.
type Result struct {
	Round    int
	BlackID  int64
	WhiteID  int64 // 0 → bye for BlackID
	WinnerID int64 // player id of the winner; == BlackID for a bye
}

// --- Round Robin ----------------------------------------------------------

// RoundRobinSchedule returns the full list of rounds for a round-robin
// among the given player ids (in seeding order). Uses the circle method:
// player 0 stays fixed while the rest rotate, so every player meets every
// other exactly once. An odd field gets a ghost opponent, giving each
// player exactly one bye across the schedule. Colours alternate by round
// for a rough balance. Fewer than two players yields no rounds.
func RoundRobinSchedule(ids []int64) [][]Pairing {
	if len(ids) < 2 {
		return nil
	}
	arr := append([]int64{}, ids...)
	if len(arr)%2 == 1 {
		arr = append(arr, 0) // ghost → produces one bye per round
	}
	n := len(arr)
	if n < 2 {
		return nil
	}
	half := n / 2
	rounds := n - 1
	schedule := make([][]Pairing, 0, rounds)
	for r := 0; r < rounds; r++ {
		var ps []Pairing
		board := 1
		for i := 0; i < half; i++ {
			a, b := arr[i], arr[n-1-i]
			if a == 0 || b == 0 {
				who := a
				if who == 0 {
					who = b
				}
				ps = append(ps, Pairing{BlackID: who}) // bye
				continue
			}
			black, white := a, b
			if r%2 == 1 {
				black, white = white, black
			}
			ps = append(ps, Pairing{Board: board, BlackID: black, WhiteID: white})
			board++
		}
		schedule = append(schedule, ps)
		rotate(arr)
	}
	return schedule
}

// rotate keeps arr[0] fixed and rotates arr[1:] one step "clockwise",
// the standard circle-method advance.
func rotate(arr []int64) {
	if len(arr) <= 2 {
		return
	}
	last := arr[len(arr)-1]
	copy(arr[2:], arr[1:len(arr)-1])
	arr[1] = last
}

// --- McMahon --------------------------------------------------------------

// standing is the engine's running tally for one player.
type standing struct {
	p         Player
	score     float64
	wins      int
	played    int
	opponents map[int64]bool
	blacks    int
	whites    int
	hadBye    bool
}

// tally reduces players + results to per-player standings.
func tally(players []Player, results []Result) map[int64]*standing {
	m := make(map[int64]*standing, len(players))
	for _, p := range players {
		m[p.ID] = &standing{p: p, score: p.StartScore, opponents: map[int64]bool{}}
	}
	for _, r := range results {
		b := m[r.BlackID]
		if b == nil {
			continue
		}
		if r.WhiteID == 0 { // bye
			b.score += ByePoints
			b.hadBye = true
			continue
		}
		w := m[r.WhiteID]
		if w == nil {
			continue
		}
		b.opponents[r.WhiteID] = true
		w.opponents[r.BlackID] = true
		b.blacks++
		w.whites++
		b.played++
		w.played++
		if r.WinnerID == r.BlackID {
			b.score++
			b.wins++
		} else {
			w.score++
			w.wins++
		}
	}
	return m
}

// McMahonRound generates the pairings for the next round from the players
// and the results of all prior rounds. Players are sorted by current
// score (then rating); each is paired top-down with the nearest-scoring
// opponent they have not yet met, avoiding rematches until unavoidable.
// An odd field gives a bye to the lowest-standing player who has not had
// one yet. Returns nil when fewer than two players remain.
func McMahonRound(players []Player, results []Result, round int) []Pairing {
	if len(players) < 2 {
		return nil
	}
	tallies := tally(players, results)

	ordered := make([]*standing, 0, len(players))
	for _, p := range players {
		ordered = append(ordered, tallies[p.ID])
	}
	sortStandings(ordered)

	// Odd field → pull out the bye player first: the lowest standing that
	// has not had a bye yet, falling back to the very lowest.
	var byeID int64
	if len(ordered)%2 == 1 {
		idx := -1
		for i := len(ordered) - 1; i >= 0; i-- {
			if !ordered[i].hadBye {
				idx = i
				break
			}
		}
		if idx == -1 {
			idx = len(ordered) - 1
		}
		byeID = ordered[idx].p.ID
		ordered = append(ordered[:idx], ordered[idx+1:]...)
	}

	remaining := make([]*standing, len(ordered))
	copy(remaining, ordered)

	var pairings []Pairing
	board := 1
	for len(remaining) >= 2 {
		a := remaining[0]
		// Find the best opponent for a: the first later player a hasn't
		// met; if a has met everyone, take the nearest (first) one.
		oppIdx := 1
		for j := 1; j < len(remaining); j++ {
			if !a.opponents[remaining[j].p.ID] {
				oppIdx = j
				break
			}
		}
		b := remaining[oppIdx]
		black, white := assignColours(a, b)
		pairings = append(pairings, Pairing{Board: board, BlackID: black, WhiteID: white})
		board++
		// Remove a (index 0) and b (index oppIdx); remove the higher index first.
		remaining = append(remaining[:oppIdx], remaining[oppIdx+1:]...)
		remaining = remaining[1:]
	}
	if byeID != 0 {
		pairings = append(pairings, Pairing{BlackID: byeID})
	}
	return pairings
}

// assignColours picks Black/White for an even game: the player with fewer
// prior Blacks takes Black; on a tie the weaker player (lower rating)
// takes Black, matching Go convention. Returns (blackID, whiteID).
func assignColours(a, b *standing) (int64, int64) {
	switch {
	case a.blacks < b.blacks:
		return a.p.ID, b.p.ID
	case b.blacks < a.blacks:
		return b.p.ID, a.p.ID
	case a.p.Rating <= b.p.Rating:
		return a.p.ID, b.p.ID
	default:
		return b.p.ID, a.p.ID
	}
}

func sortStandings(s []*standing) {
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].score != s[j].score {
			return s[i].score > s[j].score
		}
		return s[i].p.Rating > s[j].p.Rating
	})
}

// --- Standings ------------------------------------------------------------

// Standing is one row of the final (or in-progress) result table.
type Standing struct {
	PlayerID int64
	Score    float64
	Wins     int
	Played   int
	SOS      float64 // Sum of Opponents' Scores
	SOSOS    float64 // Sum of Opponents' SOS
	Place    int     // 1-based rank after tie-breaks (ties share a place)
}

// Standings computes the ordered result table. Ordering: score, then SOS,
// then SOSOS, then rating. A bye contributes ByePoints to the player's
// score but nothing to any opponent's SOS. Places are 1-based and shared
// on an exact (score, SOS, SOSOS) tie.
func Standings(players []Player, results []Result) []Standing {
	tallies := tally(players, results)

	// SOS needs every player's final score first, then SOSOS needs SOS.
	sos := make(map[int64]float64, len(players))
	for id, st := range tallies {
		var s float64
		for opp := range st.opponents {
			if o := tallies[opp]; o != nil {
				s += o.score
			}
		}
		sos[id] = s
	}
	sosos := make(map[int64]float64, len(players))
	for id, st := range tallies {
		var s float64
		for opp := range st.opponents {
			s += sos[opp]
		}
		sosos[id] = s
	}

	rows := make([]Standing, 0, len(players))
	for _, p := range players {
		st := tallies[p.ID]
		rows = append(rows, Standing{
			PlayerID: p.ID,
			Score:    st.score,
			Wins:     st.wins,
			Played:   st.played,
			SOS:      sos[p.ID],
			SOSOS:    sosos[p.ID],
		})
	}
	ratingOf := func(id int64) float64 { return tallies[id].p.Rating }
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.SOS != b.SOS {
			return a.SOS > b.SOS
		}
		if a.SOSOS != b.SOSOS {
			return a.SOSOS > b.SOSOS
		}
		return ratingOf(a.PlayerID) > ratingOf(b.PlayerID)
	})
	// Assign places; equal (score, SOS, SOSOS) share a place.
	for i := range rows {
		if i > 0 && sameRank(rows[i-1], rows[i]) {
			rows[i].Place = rows[i-1].Place
		} else {
			rows[i].Place = i + 1
		}
	}
	return rows
}

func sameRank(a, b Standing) bool {
	return a.Score == b.Score && a.SOS == b.SOS && a.SOSOS == b.SOSOS
}

// --- McMahon starting scores ----------------------------------------------

// StartScores derives McMahon starting scores from ratings via a
// rank→score mapping. rankOf converts a rating to an OGS continuous rank
// number (higher = stronger); the caller passes rating.RatingToRank.
//
// Every player at or above the bar rating is flattened onto the same top
// score (the McMahon bar), so any of them can still win the event; the
// weakest player is normalised to 0. Ranks are rounded to whole McMahon
// groups. Pass barRating <= 0 to place the bar at the strongest player
// (the default: the field is simply graded by strength).
func StartScores(players []Player, barRating float64, rankOf func(float64) float64) map[int64]float64 {
	if len(players) == 0 {
		return map[int64]float64{}
	}
	ranks := make(map[int64]float64, len(players))
	minRank := math.Inf(1)
	maxRank := math.Inf(-1)
	for _, p := range players {
		r := math.Round(rankOf(p.Rating))
		ranks[p.ID] = r
		minRank = math.Min(minRank, r)
		maxRank = math.Max(maxRank, r)
	}
	bar := maxRank
	if barRating > 0 {
		bar = math.Round(rankOf(barRating))
	}
	out := make(map[int64]float64, len(players))
	for id, r := range ranks {
		if r > bar {
			r = bar
		}
		out[id] = r - minRank
	}
	return out
}

// SuggestedRounds returns a sensible default number of McMahon rounds for
// a field of n players: enough to separate the field but not exhausting.
// Roughly ceil(log2(n)) + 1, clamped to [1, n-1].
func SuggestedRounds(n int) int {
	if n < 2 {
		return 0
	}
	r := int(math.Ceil(math.Log2(float64(n)))) + 1
	if r > n-1 {
		r = n - 1
	}
	if r < 1 {
		r = 1
	}
	return r
}
