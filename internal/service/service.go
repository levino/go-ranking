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
// players' Glicko-2 ratings atomically — the overall rating and the
// board-size category rating. Handicap (stones) is caller-supplied;
// komi is optional — pass NaN to default it (6.5 on even games, 0.5
// once stones are placed). Komi may be negative (Rückkomi); it is fully
// accounted for in the rating via the OGS handicap formula.
func (s *Service) RecordGame(ctx context.Context, groupID, blackID, whiteID int64, board rating.BoardSize, stones int, komi float64, blackWon bool) (*store.Game, error) {
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
	if isNaN(komi) {
		komi = defaultKomi(stones)
	}

	category := board.String()
	blackCat, blackCatGames := s.loadCategory(ctx, black, category)
	whiteCat, whiteCatGames := s.loadCategory(ctx, white, category)

	blackOverall := rating.NewGlicko2Entry(black.GoR, black.Deviation, black.Volatility)
	whiteOverall := rating.NewGlicko2Entry(white.GoR, white.Deviation, white.Volatility)

	nbO, nwO, nbC, nwC := rating.RateGame(blackOverall, whiteOverall, blackCat, whiteCat,
		rating.GameSetup{Board: board, Stones: stones, Komi: komi, BlackWon: blackWon})

	winner := "black"
	if !blackWon {
		winner = "white"
	}
	g := store.Game{
		GroupID:        groupID,
		BlackPlayerID:  blackID,
		WhitePlayerID:  whiteID,
		BoardSize:      board,
		Handicap:       stones,
		Komi:           komi,
		Winner:         winner,
		BlackGoRBefore: blackOverall.Rating,
		WhiteGoRBefore: whiteOverall.Rating,
		BlackGoRAfter:  nbO.Rating,
		WhiteGoRAfter:  nwO.Rating,
	}
	return s.Store.RecordGame(ctx, g,
		entryState(nbO, 0), entryState(nwO, 0),
		store.CategoryRating{PlayerID: blackID, Category: category, RatingState: entryState(nbC, blackCatGames+1)},
		store.CategoryRating{PlayerID: whiteID, Category: category, RatingState: entryState(nwC, whiteCatGames+1)},
	)
}

func entryState(e rating.Glicko2Entry, games int) store.RatingState {
	return store.RatingState{Rating: e.Rating, Deviation: e.Deviation, Volatility: e.Volatility, Games: games}
}

// loadCategory returns the player's board-category Glicko-2 entry, plus
// the number of games already played in that category. A player with
// no games in the category yet is seeded from their strength estimate.
func (s *Service) loadCategory(ctx context.Context, p *store.Player, category string) (rating.Glicko2Entry, int) {
	cr, err := s.Store.CategoryRating(ctx, p.ID, category)
	if err != nil {
		return rating.NewGlicko2Entry(p.SeedRating, rating.DefaultDeviation, rating.DefaultVolatility), 0
	}
	return rating.NewGlicko2Entry(cr.Rating, cr.Deviation, cr.Volatility), cr.Games
}

type catKey struct {
	player   int64
	category string
}

// RecomputeGroup replays a group's entire game history through the OGS
// engine, rebuilding every player's overall and per-board rating from
// their seed. Deterministic — the same games always produce the same
// result — and safe to run repeatedly.
func (s *Service) RecomputeGroup(ctx context.Context, groupID int64) error {
	players, err := s.Store.ListPlayers(ctx, groupID, true)
	if err != nil {
		return err
	}
	games, err := s.Store.ListGamesByGroupAsc(ctx, groupID)
	if err != nil {
		return err
	}

	overall := map[int64]rating.Glicko2Entry{}
	seed := map[int64]float64{}
	for _, p := range players {
		overall[p.ID] = rating.NewGlicko2Entry(p.SeedRating, rating.DefaultDeviation, rating.DefaultVolatility)
		seed[p.ID] = p.SeedRating
	}
	cats := map[catKey]rating.Glicko2Entry{}
	catGames := map[catKey]int{}
	seedCat := func(player int64) rating.Glicko2Entry {
		return rating.NewGlicko2Entry(seed[player], rating.DefaultDeviation, rating.DefaultVolatility)
	}

	for i := range games {
		g := &games[i]
		cat := g.BoardSize.String()
		bk, wk := catKey{g.BlackPlayerID, cat}, catKey{g.WhitePlayerID, cat}
		bO, wO := overall[g.BlackPlayerID], overall[g.WhitePlayerID]
		bC, ok := cats[bk]
		if !ok {
			bC = seedCat(g.BlackPlayerID)
		}
		wC, ok := cats[wk]
		if !ok {
			wC = seedCat(g.WhitePlayerID)
		}
		nbO, nwO, nbC, nwC := rating.RateGame(bO, wO, bC, wC,
			rating.GameSetup{Board: g.BoardSize, Stones: g.Handicap, Komi: g.Komi, BlackWon: g.Winner == "black"})
		g.BlackGoRBefore, g.WhiteGoRBefore = bO.Rating, wO.Rating
		g.BlackGoRAfter, g.WhiteGoRAfter = nbO.Rating, nwO.Rating
		overall[g.BlackPlayerID], overall[g.WhitePlayerID] = nbO, nwO
		cats[bk], cats[wk] = nbC, nwC
		catGames[bk]++
		catGames[wk]++
	}

	outPlayers := make([]store.Player, 0, len(players))
	for _, p := range players {
		e := overall[p.ID]
		p.GoR, p.Deviation, p.Volatility = e.Rating, e.Deviation, e.Volatility
		outPlayers = append(outPlayers, p)
	}
	outCats := make([]store.CategoryRating, 0, len(cats))
	for k, e := range cats {
		outCats = append(outCats, store.CategoryRating{
			PlayerID: k.player, Category: k.category,
			RatingState: entryState(e, catGames[k]),
		})
	}
	return s.Store.SaveRecompute(ctx, groupID, outPlayers, outCats, games)
}

// DefaultKomi exposes the conventional komi for a given handicap so
// templates can pre-fill the komi field.
func (s *Service) DefaultKomi(stones int) float64 { return defaultKomi(stones) }

func isNaN(f float64) bool { return f != f }

// defaultKomi returns the conventional komi for a given number of
// handicap stones: 6.5 on even games, 0.5 once stones are placed
// (OGS getDefaultKomi, japanese rules).
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
