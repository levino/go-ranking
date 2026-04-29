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

func TestSessionAndGames(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	g, _ := st.CreateGroup(ctx, "x", "X")
	p1, _ := st.CreatePlayer(ctx, g.ID, "P1", 1000)
	p2, _ := st.CreatePlayer(ctx, g.ID, "P2", 800)

	snap := []SnapshotEntry{
		{PlayerID: p1.ID, Number: 1, Name: "P1", GoR: 1000},
		{PlayerID: p2.ID, Number: 2, Name: "P2", GoR: 800},
	}
	sess, err := st.CreateSession(ctx, g.ID, "lucky-fox", snap)
	if err != nil {
		t.Fatal(err)
	}

	got, err := st.SessionByPassphrase(ctx, "lucky-fox")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Snapshot) != 2 || got.Snapshot[0].Name != "P1" {
		t.Fatalf("snapshot bad: %+v", got.Snapshot)
	}

	// Insert a game; player ratings should be updated atomically.
	g1 := Game{
		SessionID: sess.ID, BlackPlayerID: p2.ID, WhitePlayerID: p1.ID,
		BoardSize: rating.Board13, Handicap: 2, Komi: 0.5, Winner: "black",
		BlackGoRBefore: 800, WhiteGoRBefore: 1000,
		BlackGoRAfter: 850, WhiteGoRAfter: 970,
	}
	if _, err := st.RecordGame(ctx, g1); err != nil {
		t.Fatal(err)
	}

	updated, _ := st.PlayerByID(ctx, p2.ID)
	if updated.GoR != 850 {
		t.Fatalf("p2 GoR not updated: %.1f", updated.GoR)
	}
	updated2, _ := st.PlayerByID(ctx, p1.ID)
	if updated2.GoR != 970 {
		t.Fatalf("p1 GoR not updated: %.1f", updated2.GoR)
	}

	games, err := st.ListGamesBySession(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games", len(games))
	}
}

func TestUserCRUDAndAdminFlag(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if any, _ := st.HasAnyUsers(ctx); any {
		t.Fatal("fresh DB must report no users")
	}

	if _, err := st.CreateUser(ctx, "admin", "hash", nil, true); err != nil {
		t.Fatal(err)
	}
	if any, _ := st.HasAnyUsers(ctx); !any {
		t.Fatal("expected users after create")
	}

	g, _ := st.CreateGroup(ctx, "s", "S")
	if _, err := st.CreateUser(ctx, "teacher", "hash", &g.ID, false); err != nil {
		t.Fatal(err)
	}

	u, err := st.UserByUsername(ctx, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	if !u.GroupID.Valid || u.GroupID.Int64 != g.ID {
		t.Fatal("teacher group not set")
	}
	if u.IsAdmin {
		t.Fatal("teacher must not be admin")
	}
}
