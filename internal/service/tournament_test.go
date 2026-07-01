package service

import (
	"context"
	"testing"

	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/tournament"
)

func TestRoundRobinTournamentFullFlow(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "Klub")
	var ids []int64
	for _, r := range []float64{1600, 1200, 800, 400} {
		p, _ := svc.Store.CreatePlayer(ctx, g.ID, "P", r)
		ids = append(ids, p.ID)
	}

	tr, err := svc.CreateTournament(ctx, g.ID, "Weihnachtsturnier", tournament.RoundRobin, true, rating.Board9, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.StartTournament(ctx, tr.ID, ids); err != nil {
		t.Fatal(err)
	}
	tr, _ = svc.Store.TournamentByID(ctx, tr.ID)
	if tr.Status != "running" {
		t.Fatalf("status = %s, want running", tr.Status)
	}
	if tr.Rounds != 3 {
		t.Fatalf("4 players round robin → 3 rounds, got %d", tr.Rounds)
	}

	pairings, _ := svc.Store.ListPairings(ctx, tr.ID)
	if len(pairings) != 6 {
		t.Fatalf("4 players → 6 games total, got %d pairings", len(pairings))
	}
	// A handicap tournament: the weaker player must be Black in each game.
	for _, p := range pairings {
		if p.IsBye() {
			continue
		}
		bp, _ := svc.Store.PlayerByID(ctx, p.BlackPlayerID)
		wp, _ := svc.Store.PlayerByID(ctx, p.WhitePlayerID)
		if bp.GoR > wp.GoR {
			t.Errorf("handicap game: stronger player got Black (%v vs %v)", bp.GoR, wp.GoR)
		}
	}

	// Record every game: Black always wins (weaker player, with handicap).
	for _, p := range pairings {
		if p.IsBye() {
			continue
		}
		if err := svc.RecordPairingResult(ctx, tr.ID, p.ID, "black"); err != nil {
			t.Fatalf("record pairing %d: %v", p.ID, err)
		}
	}

	// Recording twice must fail.
	for _, p := range pairings {
		if p.IsBye() {
			continue
		}
		if err := svc.RecordPairingResult(ctx, tr.ID, p.ID, "black"); err == nil {
			t.Errorf("expected error re-recording pairing %d", p.ID)
		}
		break
	}

	if err := svc.GenerateNextRound(ctx, tr.ID); err != nil {
		t.Fatalf("finish: %v", err)
	}
	tr, _ = svc.Store.TournamentByID(ctx, tr.ID)
	if tr.Status != "finished" {
		t.Fatalf("all games played → status should be finished, got %s", tr.Status)
	}

	standings, err := svc.TournamentStandings(ctx, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(standings) != 4 {
		t.Fatalf("want 4 standings rows, got %d", len(standings))
	}
	total := 0
	for _, st := range standings {
		total += st.Wins
	}
	if total != 6 {
		t.Errorf("6 games → 6 wins total, got %d", total)
	}
}

func TestMcMahonTournamentGeneratesRoundsIncrementally(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "Klub")
	var ids []int64
	for _, r := range []float64{2100, 1800, 1500, 1200} {
		p, _ := svc.Store.CreatePlayer(ctx, g.ID, "P", r)
		ids = append(ids, p.ID)
	}

	tr, _ := svc.CreateTournament(ctx, g.ID, "McMahon", tournament.McMahon, false, rating.Board19, 2)
	if err := svc.StartTournament(ctx, tr.ID, ids); err != nil {
		t.Fatal(err)
	}

	// Only round 1 exists at the start.
	pairings, _ := svc.Store.ListPairings(ctx, tr.ID)
	if mr := maxRound(pairings); mr != 1 {
		t.Fatalf("McMahon should start with only round 1, got max round %d", mr)
	}

	// Advancing before the round is complete must fail.
	if err := svc.GenerateNextRound(ctx, tr.ID); err == nil {
		t.Fatal("expected error advancing an incomplete round")
	}

	// Play round 1 (Black wins each), then advance.
	for _, p := range pairings {
		if p.RoundNo == 1 && !p.IsBye() {
			if err := svc.RecordPairingResult(ctx, tr.ID, p.ID, "black"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := svc.GenerateNextRound(ctx, tr.ID); err != nil {
		t.Fatalf("advance: %v", err)
	}
	pairings, _ = svc.Store.ListPairings(ctx, tr.ID)
	if mr := maxRound(pairings); mr != 2 {
		t.Fatalf("after advancing, want round 2, got %d", mr)
	}

	// McMahon starting scores must be stored and non-uniform (graded field).
	roster, _ := svc.Store.ListTournamentPlayers(ctx, tr.ID)
	distinct := map[float64]bool{}
	for _, r := range roster {
		distinct[r.StartScore] = true
	}
	if len(distinct) < 2 {
		t.Error("McMahon start scores should grade the field, but all are equal")
	}
}

func TestCreateTournamentValidation(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "Klub")
	if _, err := svc.CreateTournament(ctx, g.ID, "  ", tournament.RoundRobin, true, rating.Board9, 0); err == nil {
		t.Error("empty name should be rejected")
	}
	if _, err := svc.CreateTournament(ctx, g.ID, "X", tournament.Format("bogus"), true, rating.Board9, 0); err == nil {
		t.Error("invalid format should be rejected")
	}
}

func TestStartTournamentNeedsTwoPlayers(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "Klub")
	p, _ := svc.Store.CreatePlayer(ctx, g.ID, "Solo", 1000)
	tr, _ := svc.CreateTournament(ctx, g.ID, "X", tournament.RoundRobin, true, rating.Board9, 0)
	if err := svc.StartTournament(ctx, tr.ID, []int64{p.ID}); err == nil {
		t.Error("a one-player tournament should be rejected")
	}
}
