package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/tournament"
)

// Deleting a game must leave every rating exactly as if the game had
// never been recorded — including the games played after it.
func TestDeleteGameRewindsRatingsAsIfNeverPlayed(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	type game struct {
		b, w     int
		blackWon bool
	}
	history := []game{{0, 1, true}, {1, 2, false}, {0, 2, true}, {2, 1, true}}
	setup := func(name string, skip int) ([]int64, []int64) {
		g, _ := svc.CreateGroup(ctx, name)
		var ids []int64
		for _, r := range []float64{1200, 1000, 900} {
			p, _ := svc.Store.CreatePlayer(ctx, g.ID, "P", r)
			ids = append(ids, p.ID)
		}
		var gameIDs []int64
		for i, h := range history {
			if i == skip {
				continue
			}
			gm, err := svc.RecordGame(ctx, g.ID, ids[h.b], ids[h.w], rating.Board9, 0, math.NaN(), h.blackWon)
			if err != nil {
				t.Fatal(err)
			}
			gameIDs = append(gameIDs, gm.ID)
		}
		return ids, gameIDs
	}
	ids, gameIDs := setup("With", -1)
	refIDs, _ := setup("Without", 1)

	gWith, _ := svc.Store.GroupBySlug(ctx, "with")
	before, _ := svc.Store.PlayerByID(ctx, ids[0])

	plan, err := svc.PlanDeleteGame(ctx, gWith.ID, gameIDs[1])
	if err != nil {
		t.Fatal(err)
	}
	if plan.LaterGames != 2 {
		t.Errorf("LaterGames = %d, want 2", plan.LaterGames)
	}
	if len(plan.Changes) == 0 {
		t.Error("expected rating changes in the preview")
	}
	// The dry run must not write.
	if n, _ := svc.Store.CountGames(ctx, gWith.ID); n != 4 {
		t.Fatalf("preview deleted something: %d games left", n)
	}
	if p, _ := svc.Store.PlayerByID(ctx, ids[0]); p.GoR != before.GoR {
		t.Fatal("preview changed a rating")
	}

	if _, err := svc.DeleteGame(ctx, gWith.ID, gameIDs[1]); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.Store.CountGames(ctx, gWith.ID); n != 3 {
		t.Fatalf("games left = %d, want 3", n)
	}
	for i := range ids {
		got, _ := svc.Store.PlayerByID(ctx, ids[i])
		want, _ := svc.Store.PlayerByID(ctx, refIDs[i])
		if math.Abs(got.GoR-want.GoR) > 1e-9 || math.Abs(got.Deviation-want.Deviation) > 1e-9 {
			t.Errorf("player %d: got %.4f/%.4f, want %.4f/%.4f", i, got.GoR, got.Deviation, want.GoR, want.Deviation)
		}
		gc, _ := svc.Store.CategoryRating(ctx, ids[i], "9x9")
		wc, _ := svc.Store.CategoryRating(ctx, refIDs[i], "9x9")
		if math.Abs(gc.Rating-wc.Rating) > 1e-9 || gc.Games != wc.Games {
			t.Errorf("player %d 9x9: got %.4f (%d), want %.4f (%d)", i, gc.Rating, gc.Games, wc.Rating, wc.Games)
		}
	}
	if _, err := svc.DeleteGame(ctx, gWith.ID, gameIDs[1]); !errors.Is(err, ErrGameNotInGroup) {
		t.Fatalf("second delete: err = %v, want ErrGameNotInGroup", err)
	}
}

func TestDeleteGameRejectsOldGames(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "Klub")
	a, _ := svc.Store.CreatePlayer(ctx, g.ID, "A", 1000)
	b, _ := svc.Store.CreatePlayer(ctx, g.ID, "B", 1000)
	gm, _ := svc.RecordGame(ctx, g.ID, a.ID, b.ID, rating.Board9, 0, math.NaN(), true)

	svc.Now = func() time.Time { return time.Now().Add(GameDeletionWindow - time.Hour) }
	if _, err := svc.PlanDeleteGame(ctx, g.ID, gm.ID); err != nil {
		t.Fatalf("inside the window: %v", err)
	}
	svc.Now = func() time.Time { return time.Now().Add(GameDeletionWindow + time.Hour) }
	if _, err := svc.DeleteGame(ctx, g.ID, gm.ID); !errors.Is(err, ErrGameTooOld) {
		t.Fatalf("err = %v, want ErrGameTooOld", err)
	}
	if n, _ := svc.Store.CountGames(ctx, g.ID); n != 1 {
		t.Fatal("old game was deleted")
	}
}

func TestDeleteGameRejectsOtherGroupAndTournamentGames(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "Klub")
	other, _ := svc.CreateGroup(ctx, "Andere")
	var ids []int64
	for _, r := range []float64{1200, 1000} {
		p, _ := svc.Store.CreatePlayer(ctx, g.ID, "P", r)
		ids = append(ids, p.ID)
	}
	tr, _ := svc.CreateTournament(ctx, g.ID, "Cup", tournament.RoundRobin, false, rating.Board9, 0)
	if err := svc.StartTournament(ctx, tr.ID, ids); err != nil {
		t.Fatal(err)
	}
	pairings, _ := svc.Store.ListPairings(ctx, tr.ID)
	if err := svc.RecordPairingResult(ctx, tr.ID, pairings[0].ID, "black"); err != nil {
		t.Fatal(err)
	}
	pairings, _ = svc.Store.ListPairings(ctx, tr.ID)
	gameID := pairings[0].GameID

	if _, err := svc.DeleteGame(ctx, other.ID, gameID); !errors.Is(err, ErrGameNotInGroup) {
		t.Fatalf("other group: err = %v, want ErrGameNotInGroup", err)
	}
	if _, err := svc.DeleteGame(ctx, g.ID, gameID); !errors.Is(err, ErrGameInTournament) {
		t.Fatalf("tournament game: err = %v, want ErrGameInTournament", err)
	}
}
