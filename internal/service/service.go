// Package service is the application-layer glue between storage and
// the rating algorithm.  It contains the only business logic that
// must remain consistent across the web UI and the MCP API.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levino/go-ranking/internal/passphrase"
	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/store"
)

type Service struct {
	Store *store.Store
}

func New(s *store.Store) *Service { return &Service{Store: s} }

// CreateGroup creates a group with a normalized slug.
func (s *Service) CreateGroup(ctx context.Context, name string) (*store.Group, error) {
	slug := slugify(name)
	if slug == "" {
		return nil, errors.New("invalid group name")
	}
	return s.Store.CreateGroup(ctx, slug, name)
}

// CreateGroupWithSlug creates a group using a caller-supplied slug.
// The slug is normalized lightly (lowercase, hyphens only) to avoid
// surprises in URLs.
func (s *Service) CreateGroupWithSlug(ctx context.Context, slug, name string) (*store.Group, error) {
	s2 := slugify(slug)
	if s2 == "" {
		return nil, errors.New("invalid slug")
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("name required")
	}
	return s.Store.CreateGroup(ctx, s2, name)
}

// CreateSession freezes a snapshot of the group's active players and
// generates a fresh passphrase.  The passphrase is regenerated on the
// (very rare) event of a uniqueness collision.
func (s *Service) CreateSession(ctx context.Context, groupID int64) (*store.Session, error) {
	players, err := s.Store.ListPlayers(ctx, groupID, false)
	if err != nil {
		return nil, err
	}
	snap := make([]store.SnapshotEntry, len(players))
	for i, p := range players {
		snap[i] = store.SnapshotEntry{
			PlayerID: p.ID,
			Number:   i + 1,
			Name:     p.Name,
			GoR:      p.GoR,
		}
	}
	for try := 0; try < 5; try++ {
		pass := passphrase.New()
		sess, err := s.Store.CreateSession(ctx, groupID, pass, snap)
		if err == nil {
			return sess, nil
		}
		if !strings.Contains(err.Error(), "UNIQUE") {
			return nil, err
		}
	}
	return nil, errors.New("could not generate unique passphrase")
}

// RecordResult enters a single game in the given session, recomputing
// both players' GoR. Vorgabe and Komi are looked up from the session
// snapshot — the caller cannot override them.
func (s *Service) RecordResult(ctx context.Context, sessionID, blackID, whiteID int64, board rating.BoardSize, blackWon bool) (*store.Game, error) {
	sess, err := s.Store.SessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	black, white, err := s.lookupSessionPlayers(ctx, sess, blackID, whiteID)
	if err != nil {
		return nil, err
	}
	// Determine handicap from the SNAPSHOT GoRs (frozen at session
	// creation), not from current player ratings.
	bSnap, wSnap := snapshotEntry(sess, blackID), snapshotEntry(sess, whiteID)
	if bSnap == nil || wSnap == nil {
		return nil, fmt.Errorf("player not in session snapshot")
	}
	stronger, weaker := wSnap.GoR, bSnap.GoR
	if bSnap.GoR > wSnap.GoR {
		stronger, weaker = bSnap.GoR, wSnap.GoR
	}
	h := rating.Recommended(stronger, weaker, board)

	// Black should be the weaker player when handicap is given. If the
	// caller swapped colours (which is invalid in the matrix), we still
	// rate the game with whatever the actual handicap was set to.
	hcpBonus := h.HandicapBonus(board)
	// The bonus only applies when black is the weaker player. If black
	// is actually stronger, the handicap is irrelevant for them.
	if bSnap.GoR > wSnap.GoR {
		hcpBonus = 0
	}

	newBlack, newWhite := rating.Update(black.GoR, white.GoR, hcpBonus, blackWon)

	winner := "black"
	if !blackWon {
		winner = "white"
	}
	g := store.Game{
		SessionID:      sessionID,
		BlackPlayerID:  blackID,
		WhitePlayerID:  whiteID,
		BoardSize:      board,
		Handicap:       h.Stones,
		Komi:           h.Komi,
		Winner:         winner,
		BlackGoRBefore: black.GoR,
		WhiteGoRBefore: white.GoR,
		BlackGoRAfter:  newBlack,
		WhiteGoRAfter:  newWhite,
	}
	return s.Store.RecordGame(ctx, g)
}

func (s *Service) lookupSessionPlayers(ctx context.Context, sess *store.Session, blackID, whiteID int64) (*store.Player, *store.Player, error) {
	if blackID == whiteID {
		return nil, nil, errors.New("black and white must differ")
	}
	black, err := s.Store.PlayerByID(ctx, blackID)
	if err != nil {
		return nil, nil, fmt.Errorf("black player: %w", err)
	}
	white, err := s.Store.PlayerByID(ctx, whiteID)
	if err != nil {
		return nil, nil, fmt.Errorf("white player: %w", err)
	}
	if snapshotEntry(sess, blackID) == nil {
		return nil, nil, fmt.Errorf("black player not in session")
	}
	if snapshotEntry(sess, whiteID) == nil {
		return nil, nil, fmt.Errorf("white player not in session")
	}
	return black, white, nil
}

// PlayerByNumber looks up a player by their session-local number.
func (s *Service) PlayerByNumber(sess *store.Session, number int) (*store.SnapshotEntry, error) {
	for i := range sess.Snapshot {
		if sess.Snapshot[i].Number == number {
			return &sess.Snapshot[i], nil
		}
	}
	return nil, fmt.Errorf("no player with number %d", number)
}

func snapshotEntry(sess *store.Session, id int64) *store.SnapshotEntry {
	for i := range sess.Snapshot {
		if sess.Snapshot[i].PlayerID == id {
			return &sess.Snapshot[i]
		}
	}
	return nil
}

// slugify produces a URL-safe slug for group names.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r == 'ä':
			b.WriteString("ae")
			prevHyphen = false
		case r == 'ö':
			b.WriteString("oe")
			prevHyphen = false
		case r == 'ü':
			b.WriteString("ue")
			prevHyphen = false
		case r == 'ß':
			b.WriteString("ss")
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteRune('-')
				prevHyphen = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		// fall back to something stable
		return fmt.Sprintf("group-%d", time.Now().UnixNano())
	}
	return out
}
