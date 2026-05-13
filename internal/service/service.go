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

// Recommendation is the suggested handicap/komi for a pairing at the
// current GoR values. Black plays the weaker player (lower GoR).
type Recommendation struct {
	BlackPlayer *store.Player
	WhitePlayer *store.Player
	Board       rating.BoardSize
	Stones      int     // handicap stones — 0 means even game
	Komi        float64 // komi awarded to white
}

// Recommend returns the suggested handicap pairing for two players on
// the given board. Order of the two players doesn't matter; the
// weaker one is assigned Black.
func (s *Service) Recommend(ctx context.Context, p1ID, p2ID int64, board rating.BoardSize) (*Recommendation, error) {
	p1, err := s.Store.PlayerByID(ctx, p1ID)
	if err != nil {
		return nil, fmt.Errorf("player 1: %w", err)
	}
	p2, err := s.Store.PlayerByID(ctx, p2ID)
	if err != nil {
		return nil, fmt.Errorf("player 2: %w", err)
	}
	if p1.GroupID != p2.GroupID {
		return nil, errors.New("players are in different groups")
	}
	black, white := p1, p2
	if p1.GoR > p2.GoR {
		black, white = p2, p1
	}
	h := rating.Recommended(white.GoR, black.GoR, board)
	return &Recommendation{
		BlackPlayer: black,
		WhitePlayer: white,
		Board:       board,
		Stones:      h.Stones,
		Komi:        h.Komi,
	}, nil
}

// RecordGame writes a single game to the store and updates both
// players' GoR atomically. Handicap (stones) is caller-supplied; komi
// is auto-derived from the stones (EGF convention: 6.5 for even, 0.5
// when handicap stones are placed). Rating bonus from handicap only
// applies when Black is the weaker player.
func (s *Service) RecordGame(ctx context.Context, groupID, blackID, whiteID int64, board rating.BoardSize, stones int, blackWon bool) (*store.Game, error) {
	if blackID == whiteID {
		return nil, errors.New("black and white must differ")
	}
	black, err := s.Store.PlayerByID(ctx, blackID)
	if err != nil {
		return nil, fmt.Errorf("black: %w", err)
	}
	white, err := s.Store.PlayerByID(ctx, whiteID)
	if err != nil {
		return nil, fmt.Errorf("white: %w", err)
	}
	if black.GroupID != groupID || white.GroupID != groupID {
		return nil, errors.New("players are not in this group")
	}

	h := rating.Handicap{Stones: stones, Komi: defaultKomi(stones)}
	hcpBonus := h.HandicapBonus(board)
	// Bonus only applies when Black is actually the weaker player.
	if black.GoR > white.GoR {
		hcpBonus = 0
	}

	newBlack, newWhite := rating.Update(black.GoR, white.GoR, hcpBonus, blackWon)

	winner := "black"
	if !blackWon {
		winner = "white"
	}
	g := store.Game{
		GroupID:        groupID,
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

// defaultKomi returns the EGF-conventional komi for a given number of
// handicap stones: 6.5 on even games, 0.5 once stones are placed.
func defaultKomi(stones int) float64 {
	if stones == 0 {
		return 6.5
	}
	return 0.5
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
		return fmt.Sprintf("group-%d", time.Now().UnixNano())
	}
	return out
}
