package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

const timeFmt = "2006-01-02 15:04 UTC"

func winnerDE(w string) string {
	if w == "black" {
		return "Schwarz"
	}
	return "Weiß"
}

func describeGame(g store.Game, black, white string) string {
	return fmt.Sprintf("#%d  %s  %s (Schwarz) vs %s (Weiß), %s, Vorgabe %d, Komi %.1f, Sieger: %s",
		g.ID, g.PlayedAt.Format(timeFmt), black, white, g.BoardSize, g.Handicap, g.Komi, winnerDE(g.Winner))
}

func (s *Server) playerNames(ctx context.Context, groupID int64) (map[int64]string, error) {
	players, err := s.Service.Store.ListPlayers(ctx, groupID, true)
	if err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(players))
	for _, p := range players {
		names[p.ID] = p.Name
	}
	return names, nil
}

func (s *Server) toolListGames(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, _, err := s.resolveAdminGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	limit := 20
	if v, ok := numArg(args["limit"]); ok && v > 0 {
		limit = min(v, 100)
	}
	names, err := s.playerNames(ctx, g.ID)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	var games []store.Game
	if pn, _ := args["player"].(string); pn != "" {
		p, err := s.Service.Store.PlayerByGroupAndName(ctx, g.ID, pn)
		if err != nil {
			return errorResult(fmt.Sprintf("no player named %q in %s", pn, g.Slug)), nil
		}
		all, err := s.Service.Store.ListGamesByPlayer(ctx, p.ID)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		for i := len(all) - 1; i >= 0 && len(games) < limit; i-- {
			games = append(games, all[i])
		}
	} else {
		games, err = s.Service.Store.ListRecentGames(ctx, g.ID, limit)
		if err != nil {
			return errorResult(err.Error()), nil
		}
	}
	if len(games) == 0 {
		return textResult(fmt.Sprintf("Keine Partien in %s.", g.Slug)), nil
	}
	now := s.Service.CurrentTime()
	var b strings.Builder
	fmt.Fprintf(&b, "Partien %s (neueste zuerst, %d):\n", g.Name, len(games))
	for _, gm := range games {
		status := ""
		inT, err := s.Service.Store.GameInTournament(ctx, gm.ID)
		switch {
		case err != nil:
			return errorResult(err.Error()), nil
		case inT:
			status = "Turnierpartie, nicht hier löschbar"
		default:
			until := gm.PlayedAt.Add(service.GameDeletionWindow)
			if until.After(now) {
				status = "löschbar bis " + until.Format(timeFmt)
			} else {
				status = "nicht mehr löschbar"
			}
		}
		fmt.Fprintf(&b, "  %s  [%s]\n", describeGame(gm, names[gm.BlackPlayerID], names[gm.WhitePlayerID]), status)
	}
	return textResult(b.String()), nil
}

func (s *Server) toolDeleteGame(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, user, err := s.resolveAdminGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	id, ok := numArg(args["game_id"])
	if !ok || id <= 0 {
		return errorResult("game_id must be a positive integer — use list_games to find it"), nil
	}
	confirm, _ := args["confirm"].(bool)

	var plan *service.GameDeletion
	if confirm {
		plan, err = s.Service.DeleteGame(ctx, g.ID, int64(id))
	} else {
		plan, err = s.Service.PlanDeleteGame(ctx, g.ID, int64(id))
	}
	switch {
	case errors.Is(err, service.ErrGameNotInGroup):
		return errorResult(fmt.Sprintf("Partie #%d gibt es in %s nicht. Mit list_games die ID prüfen.", id, g.Slug)), nil
	case errors.Is(err, service.ErrGameTooOld):
		return errorResult(fmt.Sprintf("Partie #%d kann nicht mehr gelöscht werden: Löschen ist nur bis %d Tage nach dem Eintragen möglich (%v).",
			id, int(service.GameDeletionWindow.Hours()/24), err)), nil
	case errors.Is(err, service.ErrGameInTournament):
		return errorResult(fmt.Sprintf("Partie #%d ist ein Turnierergebnis und kann hier nicht gelöscht werden — sonst stimmen Turniertabelle und Paarungen nicht mehr.", id)), nil
	case errors.Is(err, store.ErrConcurrentChange):
		return errorResult("Während des Löschens wurde eine andere Partie eingetragen. Nichts wurde gelöscht — bitte Vorschau erneut holen."), nil
	case err != nil:
		return errorResult(err.Error()), nil
	}

	var b strings.Builder
	if confirm {
		log.Printf("mcp: user %d deleted game %d in group %q: %s", user.ID, id, g.Slug,
			describeGame(plan.Game, plan.Black.Name, plan.White.Name))
		fmt.Fprintf(&b, "Gelöscht: %s\n", describeGame(plan.Game, plan.Black.Name, plan.White.Name))
		fmt.Fprintf(&b, "Ratings wurden aus der verbleibenden Historie neu berechnet (%d spätere Partien neu bewertet).\n", plan.LaterGames)
	} else {
		fmt.Fprintf(&b, "VORSCHAU — noch nichts gelöscht.\n")
		fmt.Fprintf(&b, "Partie: %s\n", describeGame(plan.Game, plan.Black.Name, plan.White.Name))
		fmt.Fprintf(&b, "Löschbar bis: %s\n", plan.DeletableUntil.Format(timeFmt))
		fmt.Fprintf(&b, "Beim Löschen wird die Historie ohne diese Partie neu durchgerechnet; %d später eingetragene Partien werden neu bewertet.\n", plan.LaterGames)
	}
	if len(plan.Changes) == 0 {
		b.WriteString("Rating-Änderungen: keine nennenswerten.\n")
	} else {
		b.WriteString("Rating-Änderungen:\n")
		for _, c := range plan.Changes {
			fmt.Fprintf(&b, "  %-25s %5.0f → %5.0f (%+.0f)\n", c.Player.Name, c.Before, c.After, c.After-c.Before)
		}
	}
	if !confirm {
		b.WriteString("\nDas Löschen ist endgültig und kann nicht rückgängig gemacht werden. " +
			"Zeige dem Nutzer diese Vorschau und frage ausdrücklich nach. Nur wenn er zustimmt, " +
			"delete_game mit denselben Angaben und confirm=true aufrufen.")
	}
	return textResult(b.String()), nil
}
