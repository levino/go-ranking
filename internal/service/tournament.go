package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/store"
	"github.com/levino/go-ranking/internal/tournament"
)

// CreateTournament creates a tournament in the "setup" state. The roster
// and the first round are generated later by StartTournament. rounds may
// be 0 for McMahon (a sensible default is chosen at start) and is ignored
// for round robin (the schedule fixes it).
func (s *Service) CreateTournament(ctx context.Context, groupID int64, name string, format tournament.Format, handicap bool, board rating.BoardSize, rounds int) (*store.Tournament, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("tournament name required")
	}
	if !format.Valid() {
		return nil, fmt.Errorf("invalid format %q", format)
	}
	return s.Store.CreateTournament(ctx, store.Tournament{
		GroupID:   groupID,
		Name:      strings.TrimSpace(name),
		Format:    string(format),
		Handicap:  handicap,
		BoardSize: board,
		Rounds:    rounds,
		Status:    "setup",
	})
}

// StartTournament fixes the roster, computes McMahon starting scores,
// generates the opening round(s) and flips the tournament to "running".
// For round robin the whole schedule is laid out at once; for McMahon
// only round 1 is generated (later rounds depend on results). Requires at
// least two distinct players, all in the tournament's group.
func (s *Service) StartTournament(ctx context.Context, tournamentID int64, playerIDs []int64) error {
	t, err := s.Store.TournamentByID(ctx, tournamentID)
	if err != nil {
		return err
	}
	if t.Status != "setup" {
		return errors.New("tournament already started")
	}
	roster, err := s.loadRoster(ctx, t, playerIDs)
	if err != nil {
		return err
	}
	if len(roster) < 2 {
		return errors.New("a tournament needs at least two players")
	}

	// Engine players, in seeding order (strongest first).
	sort.SliceStable(roster, func(i, j int) bool { return roster[i].GoR > roster[j].GoR })
	eng := make([]tournament.Player, 0, len(roster))
	byID := make(map[int64]*store.Player, len(roster))
	for i := range roster {
		p := &roster[i]
		byID[p.ID] = p
		eng = append(eng, tournament.Player{ID: p.ID, Rating: p.GoR})
	}

	// McMahon starting scores from rank; round robin starts everyone at 0.
	starts := map[int64]float64{}
	if tournament.Format(t.Format) == tournament.McMahon {
		starts = tournament.StartScores(eng, 0, rating.RatingToRank)
		for i := range eng {
			eng[i].StartScore = starts[eng[i].ID]
		}
	}
	tp := make([]store.TournamentPlayer, 0, len(eng))
	for _, e := range eng {
		tp = append(tp, store.TournamentPlayer{TournamentID: t.ID, PlayerID: e.ID, StartScore: starts[e.ID]})
	}
	if err := s.Store.SetTournamentRoster(ctx, t.ID, tp); err != nil {
		return err
	}

	// Generate the opening pairings and the planned round count.
	var firstRound []tournament.Pairing
	rounds := t.Rounds
	switch tournament.Format(t.Format) {
	case tournament.RoundRobin:
		schedule := tournament.RoundRobinSchedule(idsOf(eng))
		rounds = len(schedule)
		var all []store.Pairing
		for r, round := range schedule {
			all = append(all, s.storePairings(t, round, r+1, byID)...)
		}
		if err := s.Store.InsertPairings(ctx, all); err != nil {
			return err
		}
	case tournament.McMahon:
		if rounds <= 0 {
			rounds = tournament.SuggestedRounds(len(eng))
		}
		firstRound = tournament.McMahonRound(eng, nil, 1)
		if err := s.Store.InsertPairings(ctx, s.storePairings(t, firstRound, 1, byID)); err != nil {
			return err
		}
	}
	if err := s.Store.UpdateTournamentRounds(ctx, t.ID, rounds); err != nil {
		return err
	}
	return s.Store.UpdateTournamentStatus(ctx, t.ID, "running")
}

// loadRoster validates and loads the chosen players for a tournament,
// deduplicating ids and rejecting players from another group.
func (s *Service) loadRoster(ctx context.Context, t *store.Tournament, playerIDs []int64) ([]store.Player, error) {
	seen := map[int64]bool{}
	var out []store.Player
	for _, id := range playerIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		p, err := s.Store.PlayerByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("player %d: %w", id, err)
		}
		if p.GroupID != t.GroupID {
			return nil, fmt.Errorf("player %d is not in this group", id)
		}
		out = append(out, *p)
	}
	return out, nil
}

// storePairings converts engine pairings into persisted rows for one
// round, deciding colours, handicap and komi. In a handicap tournament
// the weaker player (lower rating) takes Black and the recommended
// Vorgabe is applied so weaker players have a real chance; in an even
// tournament the engine's colours stand with a default 6.5 komi. Byes are
// stored resolved (winner = "bye").
func (s *Service) storePairings(t *store.Tournament, pairings []tournament.Pairing, round int, byID map[int64]*store.Player) []store.Pairing {
	out := make([]store.Pairing, 0, len(pairings))
	for _, p := range pairings {
		sp := store.Pairing{
			TournamentID: t.ID,
			RoundNo:      round,
			BoardNo:      p.Board,
			Komi:         defaultKomi(0),
		}
		if p.IsBye() {
			sp.BlackPlayerID = p.BlackID
			sp.Winner = "bye"
			out = append(out, sp)
			continue
		}
		black, white := p.BlackID, p.WhiteID
		if t.Handicap {
			// Weaker player takes Black; apply the recommended handicap.
			bp, wp := byID[black], byID[white]
			if bp.GoR > wp.GoR {
				black, white = white, black
				bp, wp = wp, bp
			}
			h := rating.Recommended(wp.GoR, bp.GoR, t.BoardSize)
			sp.Handicap = h.Stones
			sp.Komi = h.Komi
		}
		sp.BlackPlayerID = black
		sp.WhitePlayerID = white
		out = append(out, sp)
	}
	return out
}

// RecordPairingResult records the winner of a pairing: it writes the
// underlying rating game (moving both players' ratings via RecordGame)
// and links it to the pairing. winnerColor must be "black" or "white".
// Byes and already-played pairings are rejected.
func (s *Service) RecordPairingResult(ctx context.Context, tournamentID, pairingID int64, winnerColor string) error {
	if winnerColor != "black" && winnerColor != "white" {
		return errors.New("winner must be black or white")
	}
	t, err := s.Store.TournamentByID(ctx, tournamentID)
	if err != nil {
		return err
	}
	p, err := s.Store.PairingByID(ctx, pairingID)
	if err != nil {
		return err
	}
	if p.TournamentID != tournamentID {
		return errors.New("pairing is not in this tournament")
	}
	if p.IsBye() {
		return errors.New("a bye has no result to record")
	}
	if p.Played() {
		return errors.New("result already recorded")
	}
	gm, err := s.RecordGame(ctx, t.GroupID,
		p.BlackPlayerID, p.WhitePlayerID, t.BoardSize, p.Handicap, p.Komi, winnerColor == "black")
	if err != nil {
		return err
	}
	return s.Store.SetPairingResult(ctx, pairingID, winnerColor, gm.ID)
}

// GenerateNextRound advances a McMahon tournament: if the current round is
// fully played and rounds remain, it pairs the next round from the results
// so far; if no rounds remain (or a round robin has all games in) it marks
// the tournament finished. Returns an error if the current round is still
// incomplete.
func (s *Service) GenerateNextRound(ctx context.Context, tournamentID int64) error {
	t, err := s.Store.TournamentByID(ctx, tournamentID)
	if err != nil {
		return err
	}
	if t.Status != "running" {
		return errors.New("tournament is not running")
	}
	pairings, err := s.Store.ListPairings(ctx, t.ID)
	if err != nil {
		return err
	}
	current := maxRound(pairings)
	if !roundComplete(pairings, current) {
		return errors.New("the current round still has open games")
	}

	// Round robin: everything is pre-generated. Finish once all is played.
	if tournament.Format(t.Format) == tournament.RoundRobin {
		if current >= t.Rounds {
			return s.Store.UpdateTournamentStatus(ctx, t.ID, "finished")
		}
		return nil // later rounds already exist
	}

	// McMahon: finish, or pair the next round from results.
	if current >= t.Rounds {
		return s.Store.UpdateTournamentStatus(ctx, t.ID, "finished")
	}
	roster, err := s.Store.ListTournamentPlayers(ctx, t.ID)
	if err != nil {
		return err
	}
	eng, byID, err := s.enginePlayers(ctx, roster)
	if err != nil {
		return err
	}
	results := engineResults(pairings)
	next := tournament.McMahonRound(eng, results, current+1)
	if len(next) == 0 {
		return s.Store.UpdateTournamentStatus(ctx, t.ID, "finished")
	}
	return s.Store.InsertPairings(ctx, s.storePairings(t, next, current+1, byID))
}

// enginePlayers rebuilds the engine roster (with fixed starting scores and
// current ratings for seeding) from stored roster rows.
func (s *Service) enginePlayers(ctx context.Context, roster []store.TournamentPlayer) ([]tournament.Player, map[int64]*store.Player, error) {
	eng := make([]tournament.Player, 0, len(roster))
	byID := make(map[int64]*store.Player, len(roster))
	for _, r := range roster {
		p, err := s.Store.PlayerByID(ctx, r.PlayerID)
		if err != nil {
			return nil, nil, err
		}
		byID[p.ID] = p
		eng = append(eng, tournament.Player{ID: p.ID, Rating: p.GoR, StartScore: r.StartScore})
	}
	return eng, byID, nil
}

// TournamentStandings computes the current standings for a tournament.
func (s *Service) TournamentStandings(ctx context.Context, tournamentID int64) ([]tournament.Standing, error) {
	roster, err := s.Store.ListTournamentPlayers(ctx, tournamentID)
	if err != nil {
		return nil, err
	}
	eng, _, err := s.enginePlayers(ctx, roster)
	if err != nil {
		return nil, err
	}
	pairings, err := s.Store.ListPairings(ctx, tournamentID)
	if err != nil {
		return nil, err
	}
	return tournament.Standings(eng, engineResults(pairings)), nil
}

// ---- helpers -------------------------------------------------------------

func idsOf(ps []tournament.Player) []int64 {
	out := make([]int64, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

// engineResults converts played pairings into engine results for scoring.
func engineResults(pairings []store.Pairing) []tournament.Result {
	var out []tournament.Result
	for _, p := range pairings {
		if !p.Played() {
			continue
		}
		if p.Winner == "bye" {
			out = append(out, tournament.Result{Round: p.RoundNo, BlackID: p.BlackPlayerID, WinnerID: p.BlackPlayerID})
			continue
		}
		winner := p.BlackPlayerID
		if p.Winner == "white" {
			winner = p.WhitePlayerID
		}
		out = append(out, tournament.Result{
			Round: p.RoundNo, BlackID: p.BlackPlayerID, WhiteID: p.WhitePlayerID, WinnerID: winner,
		})
	}
	return out
}

func maxRound(pairings []store.Pairing) int {
	m := 0
	for _, p := range pairings {
		if p.RoundNo > m {
			m = p.RoundNo
		}
	}
	return m
}

// roundComplete reports whether every non-bye pairing of the round has a
// result. A round with no pairings is not complete.
func roundComplete(pairings []store.Pairing, round int) bool {
	found := false
	for _, p := range pairings {
		if p.RoundNo != round {
			continue
		}
		found = true
		if !p.Played() {
			return false
		}
	}
	return found
}
