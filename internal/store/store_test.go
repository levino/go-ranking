package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/levino/go-ranking/internal/rating"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestGroupCRUD(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	g, err := st.CreateGroup(ctx, "blue-school", "Blue School")
	if err != nil {
		t.Fatal(err)
	}
	if g.ID == 0 {
		t.Fatal("zero id")
	}

	got, err := st.GroupBySlug(ctx, "blue-school")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Blue School" {
		t.Fatalf("got %q", got.Name)
	}

	if _, err := st.GroupBySlug(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	gs, err := st.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 1 {
		t.Fatalf("expected 1 group, got %d", len(gs))
	}
}

func TestPlayerCRUD(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	g, _ := st.CreateGroup(ctx, "g1", "G1")

	p, err := st.CreatePlayer(ctx, g.ID, "Alice", 800)
	if err != nil {
		t.Fatal(err)
	}
	if p.GoR != 800 {
		t.Fatal("gor not set")
	}
	q, _ := st.CreatePlayer(ctx, g.ID, "Bob", 200)

	ps, err := st.ListPlayers(ctx, g.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("got %d", len(ps))
	}
	if ps[0].ID != p.ID { // sorted by GoR DESC
		t.Fatalf("alice should be first")
	}

	// Deactivate Bob
	if err := st.UpdatePlayer(ctx, q.ID, "Bob", false); err != nil {
		t.Fatal(err)
	}
	ps, _ = st.ListPlayers(ctx, g.ID, false)
	if len(ps) != 1 {
		t.Fatalf("inactive should be excluded, got %d", len(ps))
	}
}

func TestRecordGameUpdatesRatingsAtomically(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	g, _ := st.CreateGroup(ctx, "x", "X")
	p1, _ := st.CreatePlayer(ctx, g.ID, "P1", 1000)
	p2, _ := st.CreatePlayer(ctx, g.ID, "P2", 800)

	gm := Game{
		GroupID: g.ID, BlackPlayerID: p2.ID, WhitePlayerID: p1.ID,
		BoardSize: rating.Board13, Handicap: 2, Komi: 0.5, Winner: "black",
		BlackGoRBefore: 800, WhiteGoRBefore: 1000,
		BlackGoRAfter: 850, WhiteGoRAfter: 970,
	}
	bOverall := RatingState{Rating: 850, Deviation: 200, Volatility: 0.06}
	wOverall := RatingState{Rating: 970, Deviation: 200, Volatility: 0.06}
	bCat := CategoryRating{PlayerID: p2.ID, Category: "13x13",
		RatingState: RatingState{Rating: 850, Deviation: 200, Volatility: 0.06, Games: 1}}
	wCat := CategoryRating{PlayerID: p1.ID, Category: "13x13",
		RatingState: RatingState{Rating: 970, Deviation: 200, Volatility: 0.06, Games: 1}}
	if _, err := st.RecordGame(ctx, gm, bOverall, wOverall, bCat, wCat); err != nil {
		t.Fatal(err)
	}
	if u, _ := st.PlayerByID(ctx, p2.ID); u.GoR != 850 {
		t.Fatalf("p2 rating not updated: %.1f", u.GoR)
	}
	if u, _ := st.PlayerByID(ctx, p1.ID); u.GoR != 970 {
		t.Fatalf("p1 rating not updated: %.1f", u.GoR)
	}

	// The board-size category rating must be written too.
	cr, err := st.CategoryRating(ctx, p2.ID, "13x13")
	if err != nil {
		t.Fatalf("category rating not stored: %v", err)
	}
	if cr.Rating != 850 || cr.Games != 1 {
		t.Fatalf("category rating wrong: %+v", cr)
	}

	games, err := st.ListRecentGames(ctx, g.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
	pg, _ := st.ListGamesByPlayer(ctx, p1.ID)
	if len(pg) != 1 {
		t.Fatalf("ListGamesByPlayer = %d", len(pg))
	}
}

func TestUserUpsertByOIDC(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	u1, err := st.UpsertUserByOIDC(ctx, "sub-1", "alice@example.com", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if u1.ID == 0 || u1.Email != "alice@example.com" || u1.Name != "Alice" {
		t.Fatalf("bad user: %+v", u1)
	}

	// Second call with same subject updates email/name in place.
	u2, err := st.UpsertUserByOIDC(ctx, "sub-1", "alice2@example.com", "Alice II")
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u1.ID {
		t.Fatalf("upsert created a new row: %d vs %d", u1.ID, u2.ID)
	}
	if u2.Email != "alice2@example.com" || u2.Name != "Alice II" {
		t.Fatalf("upsert did not refresh fields: %+v", u2)
	}

	got, err := st.UserByEmail(ctx, "alice2@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u1.ID {
		t.Fatalf("UserByEmail returned wrong row")
	}
}

func TestGroupAdminsMembership(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	g1, _ := st.CreateGroup(ctx, "g1", "G1")
	g2, _ := st.CreateGroup(ctx, "g2", "G2")
	u1, _ := st.UpsertUserByOIDC(ctx, "u1", "u1@example.com", "U1")
	u2, _ := st.UpsertUserByOIDC(ctx, "u2", "u2@example.com", "U2")

	if err := st.AddGroupAdmin(ctx, u1.ID, g1.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddGroupAdmin(ctx, u1.ID, g2.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddGroupAdmin(ctx, u2.ID, g1.ID); err != nil {
		t.Fatal(err)
	}
	// Idempotent: re-adding is a no-op.
	if err := st.AddGroupAdmin(ctx, u1.ID, g1.ID); err != nil {
		t.Fatal(err)
	}

	ok, err := st.IsGroupAdmin(ctx, u1.ID, g1.ID)
	if err != nil || !ok {
		t.Fatalf("u1 should admin g1: %v %v", ok, err)
	}
	ok, _ = st.IsGroupAdmin(ctx, u2.ID, g2.ID)
	if ok {
		t.Fatal("u2 should NOT admin g2")
	}

	groups, err := st.ListAdminGroups(ctx, u1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("u1 admins 2 groups, got %d", len(groups))
	}

	admins, err := st.ListGroupAdmins(ctx, g1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(admins) != 2 {
		t.Fatalf("g1 has 2 admins, got %d", len(admins))
	}

	if err := st.RemoveGroupAdmin(ctx, u2.ID, g1.ID); err != nil {
		t.Fatal(err)
	}
	admins, _ = st.ListGroupAdmins(ctx, g1.ID)
	if len(admins) != 1 || admins[0].ID != u1.ID {
		t.Fatalf("removal failed: %+v", admins)
	}
}
