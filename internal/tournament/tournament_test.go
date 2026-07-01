package tournament

import (
	"fmt"
	"testing"
)

func players(ratings ...float64) []Player {
	ps := make([]Player, len(ratings))
	for i, r := range ratings {
		ps[i] = Player{ID: int64(i + 1), Rating: r}
	}
	return ps
}

func ids(ps []Player) []int64 {
	out := make([]int64, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

// pairKey normalises an unordered pair into a comparable string.
func pairKey(a, b int64) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("%d-%d", a, b)
}

func TestRoundRobinEvenFieldMeetsEveryoneOnce(t *testing.T) {
	ps := players(2000, 1500, 1000, 500) // 4 players
	sched := RoundRobinSchedule(ids(ps))
	if len(sched) != 3 {
		t.Fatalf("4 players → want 3 rounds, got %d", len(sched))
	}
	seen := map[string]int{}
	for _, round := range sched {
		if len(round) != 2 {
			t.Fatalf("round should have 2 boards, got %d", len(round))
		}
		perRound := map[int64]bool{}
		for _, p := range round {
			if p.IsBye() {
				t.Fatal("even field should have no byes")
			}
			seen[pairKey(p.BlackID, p.WhiteID)]++
			if perRound[p.BlackID] || perRound[p.WhiteID] {
				t.Fatal("a player appears twice in one round")
			}
			perRound[p.BlackID] = true
			perRound[p.WhiteID] = true
		}
	}
	// 4 players → 6 distinct pairs, each exactly once.
	if len(seen) != 6 {
		t.Fatalf("want 6 distinct pairs, got %d", len(seen))
	}
	for k, c := range seen {
		if c != 1 {
			t.Errorf("pair %s played %d times, want 1", k, c)
		}
	}
}

func TestRoundRobinOddFieldGivesOneByeEach(t *testing.T) {
	ps := players(2000, 1500, 1000, 800, 500) // 5 players
	sched := RoundRobinSchedule(ids(ps))
	if len(sched) != 5 {
		t.Fatalf("5 players → want 5 rounds, got %d", len(sched))
	}
	byes := map[int64]int{}
	pairs := map[string]int{}
	for _, round := range sched {
		byeThisRound := 0
		for _, p := range round {
			if p.IsBye() {
				byes[p.BlackID]++
				byeThisRound++
				continue
			}
			pairs[pairKey(p.BlackID, p.WhiteID)]++
		}
		if byeThisRound != 1 {
			t.Fatalf("odd field should have exactly one bye per round, got %d", byeThisRound)
		}
	}
	for _, p := range ps {
		if byes[p.ID] != 1 {
			t.Errorf("player %d had %d byes, want 1", p.ID, byes[p.ID])
		}
	}
	// 5 players → 10 distinct pairs, each once.
	if len(pairs) != 10 {
		t.Fatalf("want 10 distinct pairs, got %d", len(pairs))
	}
	for k, c := range pairs {
		if c != 1 {
			t.Errorf("pair %s played %d times, want 1", k, c)
		}
	}
}

func TestRoundRobinTooFewPlayers(t *testing.T) {
	if got := RoundRobinSchedule([]int64{1}); got != nil {
		t.Fatalf("1 player should yield no rounds, got %v", got)
	}
	if got := RoundRobinSchedule(nil); got != nil {
		t.Fatalf("no players should yield no rounds, got %v", got)
	}
}

func TestMcMahonPairsNearestScoreNoRematch(t *testing.T) {
	ps := players(2000, 1800, 1600, 1400) // 4 players, all StartScore 0
	// Round 1.
	r1 := McMahonRound(ps, nil, 1)
	if len(r1) != 2 {
		t.Fatalf("want 2 boards, got %d", len(r1))
	}
	// Feed results: top seed wins both boards' higher player.
	var results []Result
	for _, p := range r1 {
		results = append(results, Result{Round: 1, BlackID: p.BlackID, WhiteID: p.WhiteID, WinnerID: p.BlackID})
	}
	// Round 2: nobody should be paired with a round-1 opponent.
	r2 := McMahonRound(ps, results, 2)
	met := map[int64]map[int64]bool{}
	for _, r := range results {
		if met[r.BlackID] == nil {
			met[r.BlackID] = map[int64]bool{}
		}
		if met[r.WhiteID] == nil {
			met[r.WhiteID] = map[int64]bool{}
		}
		met[r.BlackID][r.WhiteID] = true
		met[r.WhiteID][r.BlackID] = true
	}
	for _, p := range r2 {
		if p.IsBye() {
			continue
		}
		if met[p.BlackID][p.WhiteID] {
			t.Errorf("round 2 repeats pairing %d vs %d", p.BlackID, p.WhiteID)
		}
	}
}

func TestMcMahonOddFieldByeGoesToLowest(t *testing.T) {
	ps := players(2000, 1500, 1000) // 3 players → one bye
	r1 := McMahonRound(ps, nil, 1)
	var bye int64
	boards := 0
	for _, p := range r1 {
		if p.IsBye() {
			bye = p.BlackID
		} else {
			boards++
		}
	}
	if boards != 1 {
		t.Fatalf("3 players → want 1 real board, got %d", boards)
	}
	if bye != 3 { // lowest-rated player id is 3 (rating 1000)
		t.Errorf("bye should go to the lowest standing (id 3), got %d", bye)
	}
}

func TestMcMahonByeNotRepeated(t *testing.T) {
	ps := players(2000, 1500, 1000)
	var results []Result
	byeCount := map[int64]int{}
	for round := 1; round <= 3; round++ {
		pr := McMahonRound(ps, results, round)
		for _, p := range pr {
			if p.IsBye() {
				byeCount[p.BlackID]++
				results = append(results, Result{Round: round, BlackID: p.BlackID, WinnerID: p.BlackID})
			} else {
				results = append(results, Result{Round: round, BlackID: p.BlackID, WhiteID: p.WhiteID, WinnerID: p.BlackID})
			}
		}
	}
	for id, c := range byeCount {
		if c > 1 {
			t.Errorf("player %d got %d byes in 3 rounds; should spread byes", id, c)
		}
	}
}

func TestStandingsScoreThenSOS(t *testing.T) {
	// A and B both win 2; A beat stronger opponents → higher SOS → 1st.
	ps := players(2000, 1900, 1000, 900) // ids 1,2,3,4
	results := []Result{
		// Round 1: 1 beats 3, 2 beats 4
		{Round: 1, BlackID: 1, WhiteID: 3, WinnerID: 1},
		{Round: 1, BlackID: 2, WhiteID: 4, WinnerID: 2},
		// Round 2: 1 beats 2, 3 beats 4
		{Round: 2, BlackID: 1, WhiteID: 2, WinnerID: 1},
		{Round: 2, BlackID: 3, WhiteID: 4, WinnerID: 3},
	}
	st := Standings(ps, results)
	if st[0].PlayerID != 1 {
		t.Fatalf("player 1 (2 wins, beat 2 & 3) should be first, got %d", st[0].PlayerID)
	}
	if st[0].Wins != 2 || st[0].Score != 2 {
		t.Errorf("winner should have 2 wins/score 2, got wins=%d score=%.1f", st[0].Wins, st[0].Score)
	}
	// Player 4 lost both → last.
	if st[len(st)-1].PlayerID != 4 {
		t.Errorf("player 4 (0 wins) should be last, got %d", st[len(st)-1].PlayerID)
	}
}

func TestStandingsByeCountsAsWin(t *testing.T) {
	ps := players(1500, 1400)
	results := []Result{{Round: 1, BlackID: 1, WinnerID: 1}} // bye for player 1
	st := Standings(ps, results)
	var p1 Standing
	for _, s := range st {
		if s.PlayerID == 1 {
			p1 = s
		}
	}
	if p1.Score != ByePoints {
		t.Errorf("bye should score %.1f, got %.1f", ByePoints, p1.Score)
	}
}

func TestStartScoresBarFlattensTop(t *testing.T) {
	// Fake rank function: rank == rating/100 (higher = stronger).
	rankOf := func(r float64) float64 { return r / 100 }
	ps := []Player{
		{ID: 1, Rating: 3000}, // rank 30
		{ID: 2, Rating: 2800}, // rank 28
		{ID: 3, Rating: 1000}, // rank 10 (weakest → 0)
	}
	// Bar at rating 2800 → both 3000 and 2800 flatten to rank 28.
	ss := StartScores(ps, 2800, rankOf)
	if ss[1] != ss[2] {
		t.Errorf("players above/at the bar should share a start score: %v vs %v", ss[1], ss[2])
	}
	if ss[3] != 0 {
		t.Errorf("weakest player should be normalised to 0, got %v", ss[3])
	}
	// Weakest is rank 10, bar rank 28 → 28-10 = 18.
	if ss[1] != 18 {
		t.Errorf("top start score should be 18, got %v", ss[1])
	}
}

func TestSuggestedRounds(t *testing.T) {
	cases := map[int]int{2: 1, 4: 3, 8: 4, 16: 5, 3: 2}
	for n, want := range cases {
		if got := SuggestedRounds(n); got != want {
			t.Errorf("SuggestedRounds(%d) = %d, want %d", n, got, want)
		}
	}
}
