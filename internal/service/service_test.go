package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/store"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st)
}

func TestSlugifyNormalisesGermanNames(t *testing.T) {
	cases := map[string]string{
		"Go-AG München":     "go-ag-muenchen",
		"  Schöne Schule  ": "schoene-schule",
		"Über Straße":       "ueber-strasse",
	}
	for in, want := range cases {
		got := slugify(in)
		if got != want {
			t.Errorf("slugify(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestRecommendPicksWeakerAsBlack(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "Group")
	stronger, _ := svc.Store.CreatePlayer(ctx, g.ID, "Strong", 1500)
	weaker, _ := svc.Store.CreatePlayer(ctx, g.ID, "Weak", 500)

	rec, err := svc.Recommend(ctx, stronger.ID, weaker.ID, rating.Board19)
	if err != nil {
		t.Fatal(err)
	}
	if rec.BlackPlayer.ID != weaker.ID {
		t.Fatal("weaker player should play Black")
	}
	if rec.Stones < 1 {
		t.Fatalf("expected handicap stones > 0, got %d", rec.Stones)
	}
}

func TestRecordGameUpdatesRatings(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "Group")
	weaker, _ := svc.Store.CreatePlayer(ctx, g.ID, "Weak", 800)
	stronger, _ := svc.Store.CreatePlayer(ctx, g.ID, "Strong", 1500)

	gm, err := svc.RecordGame(ctx, g.ID, weaker.ID, stronger.ID, rating.Board19, 7, true)
	if err != nil {
		t.Fatal(err)
	}
	if gm.Winner != "black" {
		t.Errorf("winner = %s", gm.Winner)
	}
	if gm.Handicap != 7 {
		t.Errorf("handicap = %d", gm.Handicap)
	}
	if gm.Komi != 0.5 {
		t.Errorf("komi for handicap game should be 0.5, got %.1f", gm.Komi)
	}

	wAfter, _ := svc.Store.PlayerByID(ctx, weaker.ID)
	sAfter, _ := svc.Store.PlayerByID(ctx, stronger.ID)
	if wAfter.GoR <= 800 {
		t.Errorf("weaker should gain when winning Black with handicap: %.0f", wAfter.GoR)
	}
	if sAfter.GoR >= 1500 {
		t.Errorf("stronger should lose: %.0f", sAfter.GoR)
	}
}

func TestRecordGameEvenGameUsesKomi65(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "Group")
	a, _ := svc.Store.CreatePlayer(ctx, g.ID, "A", 1000)
	b, _ := svc.Store.CreatePlayer(ctx, g.ID, "B", 1000)

	gm, err := svc.RecordGame(ctx, g.ID, a.ID, b.ID, rating.Board13, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if gm.Komi != 6.5 {
		t.Errorf("even-game komi should be 6.5, got %.1f", gm.Komi)
	}
}

func TestRecordGameRejectsSamePlayer(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g, _ := svc.CreateGroup(ctx, "G")
	a, _ := svc.Store.CreatePlayer(ctx, g.ID, "A", 1000)
	_, err := svc.RecordGame(ctx, g.ID, a.ID, a.ID, rating.Board9, 0, true)
	if err == nil || !strings.Contains(err.Error(), "differ") {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestRecordGameRejectsForeignPlayer(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	g1, _ := svc.CreateGroup(ctx, "G1")
	g2, _ := svc.CreateGroup(ctx, "G2")
	a, _ := svc.Store.CreatePlayer(ctx, g1.ID, "A", 1000)
	b, _ := svc.Store.CreatePlayer(ctx, g2.ID, "B", 1000)
	if _, err := svc.RecordGame(ctx, g1.ID, a.ID, b.ID, rating.Board9, 0, true); err == nil {
		t.Fatal("expected error: players in different groups")
	}
}
