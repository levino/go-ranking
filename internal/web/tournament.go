package web

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/store"
	"github.com/levino/go-ranking/internal/tournament"
)

// PairingView is one board rendered on the tournament detail page.
type PairingView struct {
	ID        int64
	Board     int
	IsBye     bool
	Played    bool
	BlackID   int64
	WhiteID   int64
	BlackName string
	WhiteName string
	BlackRank string
	WhiteRank string
	Handicap  int
	Komi      float64
	Winner    string // "black" | "white" | "bye" | ""
}

// RoundView groups a round's pairings for display.
type RoundView struct {
	No       int
	Pairings []PairingView
	Complete bool
	Current  bool
}

// StandingView is one row of the standings table.
type StandingView struct {
	Place  int
	Name   string
	Rank   string
	Score  float64
	Wins   int
	Played int
	SOS    float64
}

func (s *Server) handleTournamentsList(w http.ResponseWriter, r *http.Request, g *store.Group) {
	loc := s.loc(r)
	tours, _ := s.Service.Store.ListTournaments(r.Context(), g.ID)
	players, _ := s.Service.Store.ListPlayers(r.Context(), g.ID, false)
	ctx := pageContext{
		Title:       loc.T("tourn.title", g.Name),
		User:        userOf(r),
		Group:       g,
		Tournaments: tours,
		Players:     players,
		Loc:         loc,
	}
	if f := r.URL.Query().Get("flash"); f != "" {
		ctx.Flash = f
	}
	s.render(w, r, "tournaments", ctx)
}

func (s *Server) handleTournamentCreate(w http.ResponseWriter, r *http.Request, g *store.Group) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	name := r.FormValue("name")
	format := tournament.Format(r.FormValue("format"))
	handicap := r.FormValue("handicap") == "on" || r.FormValue("handicap") == "1"
	board, err := rating.ParseBoardSize(r.FormValue("board"))
	if err != nil {
		board = rating.Board9
	}
	rounds, _ := parseInt64(r.FormValue("rounds"))
	tr, err := s.Service.CreateTournament(r.Context(), g.ID, name, format, handicap, board, int(rounds))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/g/%s/t/%d", g.Slug, tr.ID), http.StatusFound)
}

func (s *Server) handleTournamentDetail(w http.ResponseWriter, r *http.Request, g *store.Group) {
	loc := s.loc(r)
	id, _ := parseInt64(r.PathValue("id"))
	t, err := s.Service.Store.TournamentByID(r.Context(), id)
	if err != nil || t.GroupID != g.ID {
		http.NotFound(w, r)
		return
	}
	players, _ := s.Service.Store.ListPlayers(r.Context(), g.ID, false)
	pn := map[int64]string{}
	pr := map[int64]float64{}
	for _, p := range players {
		pn[p.ID] = p.Name
		pr[p.ID] = p.GoR
	}
	ctx := pageContext{
		Title:       t.Name,
		User:        userOf(r),
		Group:       g,
		Tournament:  t,
		Players:     players,
		PlayerNames: pn,
		Loc:         loc,
	}

	if t.Status != "setup" {
		pairings, _ := s.Service.Store.ListPairings(r.Context(), t.ID)
		ctx.Rounds = buildRounds(pairings, pn, pr)
		standings, _ := s.Service.TournamentStandings(r.Context(), t.ID)
		ctx.Standings = buildStandings(standings, pn, pr)
		ctx.CanAdvance = t.Status == "running" && currentRoundComplete(pairings)
	}
	s.render(w, r, "tournament", ctx)
}

func (s *Server) handleTournamentStart(w http.ResponseWriter, r *http.Request, g *store.Group) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	id, _ := parseInt64(r.PathValue("id"))
	var ids []int64
	for _, v := range r.Form["player"] {
		if pid, err := parseInt64(v); err == nil {
			ids = append(ids, pid)
		}
	}
	if err := s.Service.StartTournament(r.Context(), id, ids); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/g/%s/tournaments?flash=%s", g.Slug, s.loc(r).T("tourn.err.start")), http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/g/%s/t/%d", g.Slug, id), http.StatusFound)
}

func (s *Server) handleTournamentResult(w http.ResponseWriter, r *http.Request, g *store.Group) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	id, _ := parseInt64(r.PathValue("id"))
	pairingID, _ := parseInt64(r.FormValue("pairing"))
	winner := r.FormValue("winner")
	if err := s.Service.RecordPairingResult(r.Context(), id, pairingID, winner); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/g/%s/t/%d", g.Slug, id), http.StatusFound)
}

func (s *Server) handleTournamentNext(w http.ResponseWriter, r *http.Request, g *store.Group) {
	id, _ := parseInt64(r.PathValue("id"))
	if err := s.Service.GenerateNextRound(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/g/%s/t/%d", g.Slug, id), http.StatusFound)
}

// ---- view builders -------------------------------------------------------

func buildRounds(pairings []store.Pairing, names map[int64]string, ratings map[int64]float64) []RoundView {
	byRound := map[int][]store.Pairing{}
	maxRound := 0
	for _, p := range pairings {
		byRound[p.RoundNo] = append(byRound[p.RoundNo], p)
		if p.RoundNo > maxRound {
			maxRound = p.RoundNo
		}
	}
	nums := make([]int, 0, len(byRound))
	for n := range byRound {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	rank := func(id int64) string {
		if r, ok := ratings[id]; ok {
			return rating.FormatRank(r)
		}
		return ""
	}
	var out []RoundView
	for _, n := range nums {
		ps := byRound[n]
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].BoardNo < ps[j].BoardNo })
		rv := RoundView{No: n, Current: n == maxRound, Complete: true}
		for _, p := range ps {
			pv := PairingView{
				ID: p.ID, Board: p.BoardNo, IsBye: p.IsBye(), Played: p.Played(),
				BlackID: p.BlackPlayerID, WhiteID: p.WhitePlayerID,
				BlackName: names[p.BlackPlayerID], WhiteName: names[p.WhitePlayerID],
				BlackRank: rank(p.BlackPlayerID), WhiteRank: rank(p.WhitePlayerID),
				Handicap: p.Handicap, Komi: p.Komi, Winner: p.Winner,
			}
			if !p.IsBye() && !p.Played() {
				rv.Complete = false
			}
			rv.Pairings = append(rv.Pairings, pv)
		}
		out = append(out, rv)
	}
	// Show newest round first.
	sort.SliceStable(out, func(i, j int) bool { return out[i].No > out[j].No })
	return out
}

func buildStandings(standings []tournament.Standing, names map[int64]string, ratings map[int64]float64) []StandingView {
	out := make([]StandingView, 0, len(standings))
	for _, st := range standings {
		rank := ""
		if r, ok := ratings[st.PlayerID]; ok {
			rank = rating.FormatRank(r)
		}
		out = append(out, StandingView{
			Place: st.Place, Name: names[st.PlayerID], Rank: rank,
			Score: st.Score, Wins: st.Wins, Played: st.Played, SOS: st.SOS,
		})
	}
	return out
}

// currentRoundComplete reports whether the highest round has all its
// non-bye games recorded — the condition for advancing.
func currentRoundComplete(pairings []store.Pairing) bool {
	maxRound := 0
	for _, p := range pairings {
		if p.RoundNo > maxRound {
			maxRound = p.RoundNo
		}
	}
	if maxRound == 0 {
		return false
	}
	for _, p := range pairings {
		if p.RoundNo == maxRound && !p.IsBye() && !p.Played() {
			return false
		}
	}
	return true
}
