package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/levino/go-ranking/internal/store"
)

// GameDeletionWindow is how long after it was recorded a game can still
// be deleted. Deleting is meant for correcting mistakes (wrong winner,
// wrong pairing, a duplicate entry) — a group that meets weekly should
// still be able to fix last session's typo at the next session, hence
// two weeks. Older games are part of the settled history: removing one
// would silently rewrite every rating that was built on top of it.
const GameDeletionWindow = 14 * 24 * time.Hour

// Sentinel errors so callers can tell the user *why* a game cannot go.
var (
	ErrGameTooOld       = errors.New("game is older than the deletion window")
	ErrGameInTournament = errors.New("game belongs to a tournament")
	ErrGameNotInGroup   = errors.New("game not found in this group")
)

// RatingChange is one player's overall rating now vs. after the deletion.
type RatingChange struct {
	Player store.Player
	Before float64
	After  float64
}

// GameDeletion describes what deleting a game would do. It is returned
// both by the dry run (PlanDeleteGame) and by the real deletion.
type GameDeletion struct {
	Game            store.Game
	Black, White    store.Player
	DeletableUntil  time.Time
	LaterGames      int            // games recorded after this one, whose snapshots get recomputed
	Changes         []RatingChange // players whose overall rating moves by ≥ 0.5
	remainingGames  []store.Game
	recomputedPlays []store.Player
	recomputedCats  []store.CategoryRating
}

// PlanDeleteGame is the dry run of DeleteGame: it validates that the
// game may be deleted and computes the ratings the group would end up
// with, without writing anything.
func (s *Service) PlanDeleteGame(ctx context.Context, groupID, gameID int64) (*GameDeletion, error) {
	g, err := s.Store.GameByID(ctx, gameID)
	if err != nil || g.GroupID != groupID {
		return nil, ErrGameNotInGroup
	}
	until := g.PlayedAt.Add(GameDeletionWindow)
	if s.CurrentTime().After(until) {
		return nil, fmt.Errorf("%w (recorded %s, deletable until %s)", ErrGameTooOld,
			g.PlayedAt.Format("2006-01-02"), until.Format("2006-01-02"))
	}
	inTournament, err := s.Store.GameInTournament(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if inTournament {
		return nil, ErrGameInTournament
	}

	players, err := s.Store.ListPlayers(ctx, groupID, true)
	if err != nil {
		return nil, err
	}
	all, err := s.Store.ListGamesByGroupAsc(ctx, groupID)
	if err != nil {
		return nil, err
	}
	remaining := make([]store.Game, 0, len(all))
	later, seen := 0, false
	for _, x := range all {
		if x.ID == gameID {
			seen = true
			continue
		}
		if seen {
			later++
		}
		remaining = append(remaining, x)
	}
	outPlayers, outCats := replay(players, remaining)

	plan := &GameDeletion{
		Game:            *g,
		DeletableUntil:  until,
		LaterGames:      later,
		remainingGames:  remaining,
		recomputedPlays: outPlayers,
		recomputedCats:  outCats,
	}
	byID := map[int64]store.Player{}
	for _, p := range players {
		byID[p.ID] = p
	}
	plan.Black, plan.White = byID[g.BlackPlayerID], byID[g.WhitePlayerID]
	for _, np := range outPlayers {
		old := byID[np.ID]
		if math.Abs(np.GoR-old.GoR) >= 0.5 {
			plan.Changes = append(plan.Changes, RatingChange{Player: old, Before: old.GoR, After: np.GoR})
		}
	}
	return plan, nil
}

// DeleteGame permanently removes a game and replays the group's
// remaining history so every rating is as if the game had never been
// played. The delete and the recompute share one transaction. There is
// no undo — re-recording the game later would put it at the end of the
// history, not back in its original place.
func (s *Service) DeleteGame(ctx context.Context, groupID, gameID int64) (*GameDeletion, error) {
	plan, err := s.PlanDeleteGame(ctx, groupID, gameID)
	if err != nil {
		return nil, err
	}
	if err := s.Store.DeleteGameWithRecompute(ctx, groupID, gameID,
		plan.recomputedPlays, plan.recomputedCats, plan.remainingGames); err != nil {
		return nil, err
	}
	return plan, nil
}
