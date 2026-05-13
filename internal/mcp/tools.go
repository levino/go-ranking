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
			Name:        "list_my_groups",
			Description: "Listet alle Gruppen, die der eingeloggte User administriert.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "create_group",
			Description: "Legt eine neue Gruppe an. Der aufrufende User wird automatisch Admin.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"slug": map[string]any{"type": "string", "description": "URL-Kürzel der Gruppe, z.B. 'go-ag-hannover'."},
					"name": map[string]any{"type": "string", "description": "Klartextname der Gruppe."},
				},
				"required": []string{"slug", "name"},
			},
		},
		{
			Name:        "add_admin",
			Description: "Fügt einen weiteren Admin zu einer Gruppe hinzu. Die Person muss sich vorher mindestens einmal via id.levinkeller.de eingeloggt haben.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group": map[string]any{"type": "string", "description": "Gruppen-Slug."},
					"email": map[string]any{"type": "string", "description": "E-Mail-Adresse des neuen Admins."},
				},
				"required": []string{"group", "email"},
			},
		},
		{
			Name:        "remove_admin",
			Description: "Entfernt einen Admin aus einer Gruppe. Der letzte verbleibende Admin kann sich nicht selbst entfernen.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group": map[string]any{"type": "string", "description": "Gruppen-Slug."},
					"email": map[string]any{"type": "string", "description": "E-Mail-Adresse des zu entfernenden Admins."},
				},
				"required": []string{"group", "email"},
			},
		},
		{
			Name:        "update_player",
			Description: "Ändert einen Spieler — umbenennen oder aktiv/inaktiv setzen. Inaktive Spieler werden in neuen Sessions nicht eingeschlossen.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group":    map[string]any{"type": "string"},
					"name":     map[string]any{"type": "string", "description": "Aktueller Name des Spielers."},
					"new_name": map[string]any{"type": "string", "description": "Optional: neuer Name."},
					"active":   map[string]any{"type": "boolean", "description": "Optional: aktiv (true) / inaktiv (false)."},
				},
				"required": []string{"group", "name"},
			},
		},
		{
			Name:        "list_sessions",
			Description: "Listet alle Sessions einer Gruppe (Passphrase, Datum, Spieler- und Partienzahl).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group": map[string]any{"type": "string"},
				},
				"required": []string{"group"},
			},
		},
		{
			Name:        "record_game",
			Description: "Trage eine Partie aus einer Session ein. Gibt die neuen GoR-Werte beider Spieler zurück. Vorgabe und Komi werden automatisch aus dem Session-Snapshot berechnet.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"passphrase":   map[string]any{"type": "string", "description": "Session-Passphrase (z.B. \"jumping-hippo\")."},
					"black_number": map[string]any{"type": "integer", "description": "Spielernummer Schwarz aus der Vorgabe-Matrix."},
					"white_number": map[string]any{"type": "integer", "description": "Spielernummer Weiß aus der Vorgabe-Matrix."},
					"board_size":   map[string]any{"type": "string", "enum": []string{"9", "13", "19"}, "description": "Brettgröße: 9, 13 oder 19."},
					"winner":       map[string]any{"type": "string", "enum": []string{"black", "white"}, "description": "Sieger: 'black' oder 'white'."},
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
					"group": map[string]any{"type": "string", "description": "Gruppen-Slug."},
				},
				"required": []string{"group"},
			},
		},
		{
			Name:        "add_player",
			Description: "Fügt einen Spieler (nur Name, kein Account) zu einer Gruppe hinzu. Der Rang ist eine grobe Schätzung (z.B. '20k' für Anfänger) und wird in einen GoR-Startwert umgesetzt — Kinder haben keinen externen Rang, das ist der Tipp der Trainerin.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group": map[string]any{"type": "string", "description": "Gruppen-Slug."},
					"name":  map[string]any{"type": "string", "description": "Spielername."},
					"rank":  map[string]any{"type": "string", "description": "Optional: geschätzter Startrang, z.B. '15k' oder '3d'. Default '30k' für Anfänger."},
				},
				"required": []string{"group", "name"},
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
				"required": []string{"group"},
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
				"required": []string{"group"},
			},
		},
	}
}

func (s *Server) callTool(ctx context.Context, p ToolCallParams) (*ToolCallResult, error) {
	switch p.Name {
	case "list_my_groups":
		return s.toolListMyGroups(ctx)
	case "create_group":
		return s.toolCreateGroup(ctx, p.Arguments)
	case "add_admin":
		return s.toolAddAdmin(ctx, p.Arguments)
	case "remove_admin":
		return s.toolRemoveAdmin(ctx, p.Arguments)
	case "update_player":
		return s.toolUpdatePlayer(ctx, p.Arguments)
	case "list_sessions":
		return s.toolListSessions(ctx, p.Arguments)
	case "record_game":
		return s.toolRecordGame(ctx, p.Arguments)
	case "list_players":
		return s.toolListPlayers(ctx, p.Arguments)
	case "add_player":
		return s.toolAddPlayer(ctx, p.Arguments)
	case "ranking":
		return s.toolRanking(ctx, p.Arguments)
	case "get_session":
		return s.toolGetSession(ctx, p.Arguments)
	case "create_session":
		return s.toolCreateSession(ctx, p.Arguments)
	}
	return nil, fmt.Errorf("unknown tool: %s", p.Name)
}

func (s *Server) toolListMyGroups(ctx context.Context) (*ToolCallResult, error) {
	user, err := s.callerUser(ctx)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	groups, err := s.Service.Store.ListAdminGroups(ctx, user.ID)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if len(groups) == 0 {
		return textResult("Du administrierst noch keine Gruppe. Mit `create_group` eine anlegen."), nil
	}
	var b strings.Builder
	for _, g := range groups {
		fmt.Fprintf(&b, "  %-30s %s\n", g.Slug, g.Name)
	}
	return textResult(fmt.Sprintf("Gruppen (%d):\n%s", len(groups), b.String())), nil
}

func (s *Server) toolCreateGroup(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	user, err := s.callerUser(ctx)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	slug, _ := args["slug"].(string)
	name, _ := args["name"].(string)
	if slug == "" || name == "" {
		return errorResult("slug and name required"), nil
	}
	g, err := s.Service.CreateGroupWithSlug(ctx, slug, name)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if err := s.Service.Store.AddGroupAdmin(ctx, user.ID, g.ID); err != nil {
		return errorResult(err.Error()), nil
	}
	return textResult(fmt.Sprintf("Gruppe %q (%s) angelegt, du bist Admin.", g.Name, g.Slug)), nil
}

func (s *Server) toolAddAdmin(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, _, err := s.resolveAdminGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	email, _ := args["email"].(string)
	if email == "" {
		return errorResult("email required"), nil
	}
	target, err := s.Service.Store.UserByEmail(ctx, email)
	if err != nil {
		return errorResult(fmt.Sprintf("no user with email %q — they must log in via the web UI first", email)), nil
	}
	if err := s.Service.Store.AddGroupAdmin(ctx, target.ID, g.ID); err != nil {
		return errorResult(err.Error()), nil
	}
	return textResult(fmt.Sprintf("%s ist jetzt Admin von %s.", email, g.Slug)), nil
}

func (s *Server) toolRemoveAdmin(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, me, err := s.resolveAdminGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	email, _ := args["email"].(string)
	if email == "" {
		return errorResult("email required"), nil
	}
	target, err := s.Service.Store.UserByEmail(ctx, email)
	if err != nil {
		return errorResult(fmt.Sprintf("no user with email %q", email)), nil
	}
	if target.ID == me.ID {
		admins, _ := s.Service.Store.ListGroupAdmins(ctx, g.ID)
		if len(admins) <= 1 {
			return errorResult("cannot remove the last admin of a group"), nil
		}
	}
	if err := s.Service.Store.RemoveGroupAdmin(ctx, target.ID, g.ID); err != nil {
		return errorResult(err.Error()), nil
	}
	return textResult(fmt.Sprintf("%s ist nicht mehr Admin von %s.", email, g.Slug)), nil
}

func (s *Server) toolUpdatePlayer(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, _, err := s.resolveAdminGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	name, _ := args["name"].(string)
	if name == "" {
		return errorResult("name required"), nil
	}
	p, err := s.Service.Store.PlayerByGroupAndName(ctx, g.ID, name)
	if err != nil {
		return errorResult(fmt.Sprintf("no player named %q in %s", name, g.Slug)), nil
	}
	newName := p.Name
	if v, _ := args["new_name"].(string); v != "" {
		newName = v
	}
	active := p.Active
	if v, ok := args["active"].(bool); ok {
		active = v
	}
	if err := s.Service.Store.UpdatePlayer(ctx, p.ID, newName, active); err != nil {
		return errorResult(err.Error()), nil
	}
	status := "aktiv"
	if !active {
		status = "inaktiv"
	}
	return textResult(fmt.Sprintf("Spieler %q → %q (%s) in %s.", name, newName, status, g.Slug)), nil
}

func (s *Server) toolListSessions(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, _, err := s.resolveAdminGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	sess, err := s.Service.Store.ListSessions(ctx, g.ID)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if len(sess) == 0 {
		return textResult(fmt.Sprintf("Keine Sessions in %s. Mit `create_session` anlegen.", g.Slug)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sessions in %s (%d):\n", g.Slug, len(sess))
	for _, sx := range sess {
		games, _ := s.Service.Store.ListGamesBySession(ctx, sx.ID)
		fmt.Fprintf(&b, "  %s  %s  %d Spieler  %d Partien\n",
			sx.CreatedAt.Format("02.01.2006 15:04"),
			sx.Passphrase, len(sx.Snapshot), len(games))
	}
	return textResult(b.String()), nil
}

func (s *Server) toolAddPlayer(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, _, err := s.resolveAdminGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	name, _ := args["name"].(string)
	if name == "" {
		return errorResult("name required"), nil
	}
	gor := 100.0
	if rk, _ := args["rank"].(string); rk != "" {
		v, err := rating.FromRank(rk)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		gor = v
	}
	p, err := s.Service.Store.CreatePlayer(ctx, g.ID, name, gor)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return textResult(fmt.Sprintf("Spieler %q angelegt (GoR %.0f, %s) in %s.",
		p.Name, p.GoR, rating.FormatRank(p.GoR), g.Slug)), nil
}

func (s *Server) toolRecordGame(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	pass, _ := args["passphrase"].(string)
	if pass == "" {
		return errorResult("passphrase required"), nil
	}
	sess, err := s.Service.Store.SessionByPassphrase(ctx, pass)
	if err != nil {
		return errorResult("unknown session: " + pass), nil
	}
	// Gate on group admin membership.
	g, err := s.Service.Store.GroupByID(ctx, sess.GroupID)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if _, _, err := s.resolveAdminGroup(ctx, map[string]any{"group": g.Slug}); err != nil {
		return errorResult(err.Error()), nil
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
	black, err := s.Service.PlayerByNumber(sess, bn)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	white, err := s.Service.PlayerByNumber(sess, wn)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	gm, err := s.Service.RecordResult(ctx, sess.ID, black.PlayerID, white.PlayerID, board, winner == "black")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	out := fmt.Sprintf(
		"Eingetragen: %s (#%d, Schwarz) vs %s (#%d, Weiß) auf %s. "+
			"Vorgabe %d, Komi %.1f, Sieger: %s.\n"+
			"Schwarz GoR: %.0f → %.0f (%+.1f), Weiß GoR: %.0f → %.0f (%+.1f).",
		black.Name, bn, white.Name, wn, board, gm.Handicap, gm.Komi, gm.Winner,
		gm.BlackGoRBefore, gm.BlackGoRAfter, gm.BlackGoRAfter-gm.BlackGoRBefore,
		gm.WhiteGoRBefore, gm.WhiteGoRAfter, gm.WhiteGoRAfter-gm.WhiteGoRBefore,
	)
	return textResult(out), nil
}

func (s *Server) toolListPlayers(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, _, err := s.resolveAdminGroup(ctx, args)
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
	g, _, err := s.resolveAdminGroup(ctx, args)
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
	g, err := s.Service.Store.GroupByID(ctx, sess.GroupID)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if _, _, err := s.resolveAdminGroup(ctx, map[string]any{"group": g.Slug}); err != nil {
		return errorResult(err.Error()), nil
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
	for _, gm := range games {
		fmt.Fprintf(&b, "  %s: %s (S) vs %s (W) auf %s — Vorgabe %d, Komi %.1f, Sieger %s\n",
			gm.PlayedAt.Format("15:04"),
			pn[gm.BlackPlayerID], pn[gm.WhitePlayerID], gm.BoardSize, gm.Handicap, gm.Komi, gm.Winner)
	}
	return textResult(b.String()), nil
}

func (s *Server) toolCreateSession(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, _, err := s.resolveAdminGroup(ctx, args)
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
