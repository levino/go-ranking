package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/store"
)

func newTestService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st), context.Background()
}

func TestSlugifyGermanCharsAndSpaces(t *testing.T) {
	cases := map[string]string{
		"Blau Schule":  "blau-schule",
		"Föhrenhaus":   "foehrenhaus",
		"  Multi   Space  ": "multi-space",
		"Größe & Stoß":   "groesse-stoss",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestCreateGroupSlugifies(t *testing.T) {
	svc, ctx := newTestService(t)
	g, err := svc.CreateGroup(ctx, "Föhrenhaus")
	if err != nil {
		t.Fatal(err)
	}
	if g.Slug != "foehrenhaus" {
		t.Fatalf("slug = %q", g.Slug)
	}
}

func TestCreateSessionSnapshotsActivePlayers(t *testing.T) {
	svc, ctx := newTestService(t)
	g, _ := svc.CreateGroup(ctx, "Test")
	a, _ := svc.Store.CreatePlayer(ctx, g.ID, "Alice", 1000)
	_, _ = svc.Store.CreatePlayer(ctx, g.ID, "Bob", 500)
	c, _ := svc.Store.CreatePlayer(ctx, g.ID, "Carl", 300)
	_ = svc.Store.UpdatePlayer(ctx, c.ID, "Carl", false) // inactive

	sess, err := svc.CreateSession(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Snapshot) != 2 {
		t.Fatalf("expected 2 active, got %d", len(sess.Snapshot))
	}
	if sess.Snapshot[0].PlayerID != a.ID {
		t.Fatal("ordering by GoR DESC expected")
	}
	// Numbers are 1-based and contiguous.
	for i, e := range sess.Snapshot {
		if e.Number != i+1 {
			t.Errorf("snapshot[%d].Number = %d", i, e.Number)
		}
	}
}

func TestRecordResultUpdatesRatings(t *testing.T) {
	svc, ctx := newTestService(t)
	g, _ := svc.CreateGroup(ctx, "T")
	stronger, _ := svc.Store.CreatePlayer(ctx, g.ID, "S", 1500)
	weaker, _ := svc.Store.CreatePlayer(ctx, g.ID, "W", 800)

	sess, _ := svc.CreateSession(ctx, g.ID)

	// Weaker (Black) wins on 19x19 with the recommended handicap.
	got, err := svc.RecordResult(ctx, sess.ID, weaker.ID, stronger.ID, rating.Board19, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Handicap < 1 {
		t.Errorf("expected non-zero handicap on 19x19 with 700 gap, got %d", got.Handicap)
	}
	// Black (weaker) gained, white (stronger) lost
	if got.BlackGoRAfter <= got.BlackGoRBefore {
		t.Error("weaker player should gain after a win")
	}
	if got.WhiteGoRAfter >= got.WhiteGoRBefore {
		t.Error("stronger player should lose after a loss")
	}

	// Database actually stored the new ratings on the players.
	w, _ := svc.Store.PlayerByID(ctx, weaker.ID)
	if w.GoR != got.BlackGoRAfter {
		t.Errorf("weaker DB GoR %.1f != game GoR %.1f", w.GoR, got.BlackGoRAfter)
	}
}

func TestRecordResultUsesSnapshotForHandicap(t *testing.T) {
	// After a session is created, even if a later game changes player
	// ratings, subsequent games in that session must still derive
	// handicap from the SNAPSHOT.
	svc, ctx := newTestService(t)
	g, _ := svc.CreateGroup(ctx, "T")
	a, _ := svc.Store.CreatePlayer(ctx, g.ID, "A", 1500)
	b, _ := svc.Store.CreatePlayer(ctx, g.ID, "B", 800)

	sess, _ := svc.CreateSession(ctx, g.ID)
	expected := rating.Recommended(1500, 800, rating.Board19)

	g1, err := svc.RecordResult(ctx, sess.ID, b.ID, a.ID, rating.Board19, true)
	if err != nil {
		t.Fatal(err)
	}
	if g1.Handicap != expected.Stones {
		t.Fatalf("first-game handicap %d, expected %d", g1.Handicap, expected.Stones)
	}

	g2, err := svc.RecordResult(ctx, sess.ID, b.ID, a.ID, rating.Board19, true)
	if err != nil {
		t.Fatal(err)
	}
	if g2.Handicap != expected.Stones {
		t.Fatalf("second-game handicap %d, expected %d (session snapshot must be stable)",
			g2.Handicap, expected.Stones)
	}
}

func TestRecordResultRejectsPlayerNotInSession(t *testing.T) {
	svc, ctx := newTestService(t)
	g, _ := svc.CreateGroup(ctx, "T")
	a, _ := svc.Store.CreatePlayer(ctx, g.ID, "A", 1000)
	b, _ := svc.Store.CreatePlayer(ctx, g.ID, "B", 1000)
	sess, _ := svc.CreateSession(ctx, g.ID)
	// Add a third player AFTER the snapshot.
	c, _ := svc.Store.CreatePlayer(ctx, g.ID, "C", 1000)

	if _, err := svc.RecordResult(ctx, sess.ID, c.ID, a.ID, rating.Board13, true); err == nil {
		t.Fatal("expected error for player not in snapshot")
	}
	if _, err := svc.RecordResult(ctx, sess.ID, a.ID, b.ID, rating.Board13, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordResultRejectsSamePlayerBothColours(t *testing.T) {
	svc, ctx := newTestService(t)
	g, _ := svc.CreateGroup(ctx, "T")
	a, _ := svc.Store.CreatePlayer(ctx, g.ID, "A", 1000)
	sess, _ := svc.CreateSession(ctx, g.ID)
	if _, err := svc.RecordResult(ctx, sess.ID, a.ID, a.ID, rating.Board9, true); err == nil {
		t.Fatal("expected error when black=white")
	}
}

func TestPlayerByNumber(t *testing.T) {
	svc, ctx := newTestService(t)
	g, _ := svc.CreateGroup(ctx, "T")
	_, _ = svc.Store.CreatePlayer(ctx, g.ID, "A", 1000)
	_, _ = svc.Store.CreatePlayer(ctx, g.ID, "B", 500)
	sess, _ := svc.CreateSession(ctx, g.ID)
	if e, err := svc.PlayerByNumber(sess, 2); err != nil || e.Name != "B" {
		t.Errorf("number 2 = %+v %v", e, err)
	}
	if _, err := svc.PlayerByNumber(sess, 99); err == nil {
		t.Errorf("expected error for missing number")
	}
}
