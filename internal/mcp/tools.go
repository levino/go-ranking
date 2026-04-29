package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/levino/go-ranking/internal/rating"
)

func toolDefs() []Tool {
	return []Tool{
		{
			Name:        "record_game",
			Description: "Trage eine Partie aus einer Session ein. Gibt die neuen GoR-Werte beider Spieler zurück. Vorgabe und Komi werden automatisch aus dem Session-Snapshot berechnet.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"passphrase":    map[string]any{"type": "string", "description": "Session-Passphrase (z.B. \"jumping-hippo\")."},
					"black_number":  map[string]any{"type": "integer", "description": "Spielernummer Schwarz aus der Vorgabe-Matrix."},
					"white_number":  map[string]any{"type": "integer", "description": "Spielernummer Weiß aus der Vorgabe-Matrix."},
					"board_size":    map[string]any{"type": "string", "enum": []string{"9", "13", "19"}, "description": "Brettgröße: 9, 13 oder 19."},
					"winner":        map[string]any{"type": "string", "enum": []string{"black", "white"}, "description": "Sieger: 'black' oder 'white'."},
				},
				"required": []string{"passphrase", "black_number", "white_number", "board_size", "winner"},
			},
		},
		{
			Name:        "list_players",
			Description: "Aktuelle Spielerliste einer Gruppe mit GoR und Rang.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group": map[string]any{"type": "string", "description": "Gruppen-Slug. Optional, falls Default gesetzt."},
				},
			},
		},
		{
			Name:        "ranking",
			Description: "Sortierte Rangliste einer Gruppe (höchster GoR zuerst).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group": map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:        "get_session",
			Description: "Details einer Session inkl. Snapshot und bisheriger Ergebnisse.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"passphrase": map[string]any{"type": "string"},
				},
				"required": []string{"passphrase"},
			},
		},
		{
			Name:        "create_session",
			Description: "Erstellt eine neue Session für eine Gruppe und gibt Passphrase und PDF-Links zurück.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group": map[string]any{"type": "string"},
				},
			},
		},
	}
}

func (s *Server) callTool(ctx context.Context, p ToolCallParams) (*ToolCallResult, error) {
	switch p.Name {
	case "record_game":
		return s.toolRecordGame(ctx, p.Arguments)
	case "list_players":
		return s.toolListPlayers(ctx, p.Arguments)
	case "ranking":
		return s.toolRanking(ctx, p.Arguments)
	case "get_session":
		return s.toolGetSession(ctx, p.Arguments)
	case "create_session":
		return s.toolCreateSession(ctx, p.Arguments)
	}
	return nil, fmt.Errorf("unknown tool: %s", p.Name)
}

func (s *Server) toolRecordGame(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	pass, _ := args["passphrase"].(string)
	if pass == "" {
		return errorResult("passphrase required"), nil
	}
	bn, ok1 := numArg(args["black_number"])
	wn, ok2 := numArg(args["white_number"])
	if !ok1 || !ok2 {
		return errorResult("black_number and white_number must be integers"), nil
	}
	boardStr, _ := args["board_size"].(string)
	if boardStr == "" {
		if v, ok := numArg(args["board_size"]); ok {
			boardStr = fmt.Sprintf("%d", v)
		}
	}
	board, err := rating.ParseBoardSize(boardStr)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	winner, _ := args["winner"].(string)
	winner = strings.ToLower(winner)
	if winner != "black" && winner != "white" {
		return errorResult("winner must be 'black' or 'white'"), nil
	}
	sess, err := s.Service.Store.SessionByPassphrase(ctx, pass)
	if err != nil {
		return errorResult("unknown session: " + pass), nil
	}
	black, err := s.Service.PlayerByNumber(sess, bn)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	white, err := s.Service.PlayerByNumber(sess, wn)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	g, err := s.Service.RecordResult(ctx, sess.ID, black.PlayerID, white.PlayerID, board, winner == "black")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	out := fmt.Sprintf(
		"Eingetragen: %s (#%d, Schwarz) vs %s (#%d, Weiß) auf %s. "+
			"Vorgabe %d, Komi %.1f, Sieger: %s.\n"+
			"Schwarz GoR: %.0f → %.0f (%+.1f), Weiß GoR: %.0f → %.0f (%+.1f).",
		black.Name, bn, white.Name, wn, board, g.Handicap, g.Komi, g.Winner,
		g.BlackGoRBefore, g.BlackGoRAfter, g.BlackGoRAfter-g.BlackGoRBefore,
		g.WhiteGoRBefore, g.WhiteGoRAfter, g.WhiteGoRAfter-g.WhiteGoRBefore,
	)
	return textResult(out), nil
}

func (s *Server) toolListPlayers(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, err := s.resolveGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	players, err := s.Service.Store.ListPlayers(ctx, g.ID, true)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Gruppe %s (%d Spieler):\n", g.Name, len(players))
	for _, p := range players {
		status := ""
		if !p.Active {
			status = " [inaktiv]"
		}
		fmt.Fprintf(&b, "  %-25s GoR %4.0f  (%s)%s\n", p.Name, p.GoR, rating.FormatRank(p.GoR), status)
	}
	return textResult(b.String()), nil
}

func (s *Server) toolRanking(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, err := s.resolveGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	players, err := s.Service.Store.ListPlayers(ctx, g.ID, false)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Rangliste %s:\n", g.Name)
	for i, p := range players {
		fmt.Fprintf(&b, "  %2d. %-25s %4.0f  (%s)\n", i+1, p.Name, p.GoR, rating.FormatRank(p.GoR))
	}
	return textResult(b.String()), nil
}

func (s *Server) toolGetSession(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	pass, _ := args["passphrase"].(string)
	if pass == "" {
		return errorResult("passphrase required"), nil
	}
	sess, err := s.Service.Store.SessionByPassphrase(ctx, pass)
	if err != nil {
		return errorResult("unknown session: " + pass), nil
	}
	games, _ := s.Service.Store.ListGamesBySession(ctx, sess.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "Session %s (%s):\n", sess.Passphrase, sess.CreatedAt.Format("02.01.2006 15:04"))
	fmt.Fprintf(&b, "Spieler:\n")
	for _, e := range sess.Snapshot {
		fmt.Fprintf(&b, "  #%d %-25s GoR %.0f\n", e.Number, e.Name, e.GoR)
	}
	fmt.Fprintf(&b, "Partien (%d):\n", len(games))
	pn := map[int64]string{}
	for _, e := range sess.Snapshot {
		pn[e.PlayerID] = e.Name
	}
	for _, g := range games {
		fmt.Fprintf(&b, "  %s: %s (S) vs %s (W) auf %s — Vorgabe %d, Komi %.1f, Sieger %s\n",
			g.PlayedAt.Format("15:04"),
			pn[g.BlackPlayerID], pn[g.WhitePlayerID], g.BoardSize, g.Handicap, g.Komi, g.Winner)
	}
	return textResult(b.String()), nil
}

func (s *Server) toolCreateSession(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, err := s.resolveGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	sess, err := s.Service.CreateSession(ctx, g.ID)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return textResult(fmt.Sprintf(
		"Neue Session: %s\nMatrix-PDF:    /g/%s/sessions/%s/matrix.pdf\nErgebnis-PDF:  /g/%s/sessions/%s/scorecard.pdf",
		sess.Passphrase, g.Slug, sess.Passphrase, g.Slug, sess.Passphrase)), nil
}

func numArg(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	case string:
		var n int
		_, err := fmt.Sscanf(x, "%d", &n)
		return n, err == nil
	}
	return 0, false
}
