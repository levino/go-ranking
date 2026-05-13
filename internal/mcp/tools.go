package mcp

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/levino/go-ranking/internal/rating"
)

func toolDefs() []Tool {
	return []Tool{
		{
			Name:        "list_my_groups",
			Description: "Listet alle Gruppen, die der eingeloggte User administriert.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
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
					"group": map[string]any{"type": "string"},
					"email": map[string]any{"type": "string"},
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
					"group": map[string]any{"type": "string"},
					"email": map[string]any{"type": "string"},
				},
				"required": []string{"group", "email"},
			},
		},
		{
			Name:        "add_player",
			Description: "Fügt einen Spieler (nur Name, kein Account) zu einer Gruppe hinzu. Der Rang ist eine grobe Schätzung der Trainerin (z.B. '20k' für Anfänger) und wird in einen GoR-Startwert übersetzt.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group": map[string]any{"type": "string"},
					"name":  map[string]any{"type": "string"},
					"rank":  map[string]any{"type": "string", "description": "Optional, z.B. '15k' oder '3d'. Default '30k'."},
				},
				"required": []string{"group", "name"},
			},
		},
		{
			Name:        "update_player",
			Description: "Ändert einen Spieler — umbenennen oder aktiv/inaktiv setzen.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group":    map[string]any{"type": "string"},
					"name":     map[string]any{"type": "string"},
					"new_name": map[string]any{"type": "string"},
					"active":   map[string]any{"type": "boolean"},
				},
				"required": []string{"group", "name"},
			},
		},
		{
			Name:        "list_players",
			Description: "Aktuelle Spielerliste einer Gruppe mit GoR und Rang.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group": map[string]any{"type": "string"},
				},
				"required": []string{"group"},
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
			Name:        "recommend_handicap",
			Description: "Empfiehlt für ein Paar (zwei Spielernamen) und ein Brett die Vorgabe (Steine + Komi) und sagt, wer Schwarz spielen sollte.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group":    map[string]any{"type": "string"},
					"player_a": map[string]any{"type": "string", "description": "Name eines Spielers."},
					"player_b": map[string]any{"type": "string", "description": "Name des anderen Spielers."},
					"board":    map[string]any{"type": "string", "enum": []string{"9", "13", "19"}},
				},
				"required": []string{"group", "player_a", "player_b", "board"},
			},
		},
		{
			Name:        "record_game",
			Description: "Trägt eine Partie ein. Vorgabe (Steine) muss übergeben werden. Komi optional — wenn nicht angegeben, EGF-Standard (6.5 ohne Vorgabe, sonst 0.5). Negatives Komi (Rückkomi) ist erlaubt, vor allem auf 9x9 sinnvoll. Beide Spieler-Namen müssen in der Gruppe existieren.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group":    map[string]any{"type": "string"},
					"black":    map[string]any{"type": "string", "description": "Name des Schwarz-Spielers."},
					"white":    map[string]any{"type": "string", "description": "Name des Weiß-Spielers."},
					"board":    map[string]any{"type": "string", "enum": []string{"9", "13", "19"}},
					"handicap": map[string]any{"type": "integer", "description": "Anzahl Vorgabesteine (0 für ebenes Spiel)."},
					"komi":     map[string]any{"type": "number", "description": "Optional. Float, darf negativ sein (Rückkomi)."},
					"winner":   map[string]any{"type": "string", "enum": []string{"black", "white"}},
				},
				"required": []string{"group", "black", "white", "board", "handicap", "winner"},
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
	case "add_player":
		return s.toolAddPlayer(ctx, p.Arguments)
	case "update_player":
		return s.toolUpdatePlayer(ctx, p.Arguments)
	case "list_players":
		return s.toolListPlayers(ctx, p.Arguments)
	case "ranking":
		return s.toolRanking(ctx, p.Arguments)
	case "recommend_handicap":
		return s.toolRecommend(ctx, p.Arguments)
	case "record_game":
		return s.toolRecordGame(ctx, p.Arguments)
	}
	return nil, fmt.Errorf("unknown tool: %s", p.Name)
}

func (s *Server) toolListMyGroups(ctx context.Context) (*ToolCallResult, error) {
	user, ok := userFromCtx(ctx)
	if !ok {
		return errorResult("no caller in context"), nil
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
	user, ok := userFromCtx(ctx)
	if !ok {
		return errorResult("no caller in context"), nil
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

func (s *Server) toolRecommend(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, _, err := s.resolveAdminGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	a, _ := args["player_a"].(string)
	bn, _ := args["player_b"].(string)
	if a == "" || bn == "" {
		return errorResult("player_a and player_b required"), nil
	}
	pa, err := s.Service.Store.PlayerByGroupAndName(ctx, g.ID, a)
	if err != nil {
		return errorResult(fmt.Sprintf("no player named %q", a)), nil
	}
	pb, err := s.Service.Store.PlayerByGroupAndName(ctx, g.ID, bn)
	if err != nil {
		return errorResult(fmt.Sprintf("no player named %q", bn)), nil
	}
	board, err := rating.ParseBoardSize(fmt.Sprintf("%v", args["board"]))
	if err != nil {
		return errorResult(err.Error()), nil
	}
	rec, err := s.Service.Recommend(ctx, pa.ID, pb.ID, board)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return textResult(fmt.Sprintf(
		"%s spielt Schwarz, %s spielt Weiß. Vorgabe: %d Steine, Komi %.1f (Brett %s).",
		rec.BlackPlayer.Name, rec.WhitePlayer.Name, rec.Stones, rec.Komi, rec.Board)), nil
}

func (s *Server) toolRecordGame(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	g, _, err := s.resolveAdminGroup(ctx, args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	blackName, _ := args["black"].(string)
	whiteName, _ := args["white"].(string)
	if blackName == "" || whiteName == "" {
		return errorResult("black and white names required"), nil
	}
	bp, err := s.Service.Store.PlayerByGroupAndName(ctx, g.ID, blackName)
	if err != nil {
		return errorResult(fmt.Sprintf("no player named %q", blackName)), nil
	}
	wp, err := s.Service.Store.PlayerByGroupAndName(ctx, g.ID, whiteName)
	if err != nil {
		return errorResult(fmt.Sprintf("no player named %q", whiteName)), nil
	}
	board, err := rating.ParseBoardSize(fmt.Sprintf("%v", args["board"]))
	if err != nil {
		return errorResult(err.Error()), nil
	}
	stones, ok := numArg(args["handicap"])
	if !ok {
		return errorResult("handicap must be an integer"), nil
	}
	komi := math.NaN()
	if raw, present := args["komi"]; present && raw != nil {
		if v, ok := floatArg(raw); ok {
			komi = v
		}
	}
	winner, _ := args["winner"].(string)
	winner = strings.ToLower(winner)
	if winner != "black" && winner != "white" {
		return errorResult("winner must be 'black' or 'white'"), nil
	}
	gm, err := s.Service.RecordGame(ctx, g.ID, bp.ID, wp.ID, board, stones, komi, winner == "black")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return textResult(fmt.Sprintf(
		"Eingetragen: %s (Schwarz) vs %s (Weiß) auf %s, Vorgabe %d, Komi %.1f, Sieger: %s.\n"+
			"GoR: %s %.0f → %.0f (%+.1f), %s %.0f → %.0f (%+.1f).",
		bp.Name, wp.Name, board, gm.Handicap, gm.Komi, gm.Winner,
		bp.Name, gm.BlackGoRBefore, gm.BlackGoRAfter, gm.BlackGoRAfter-gm.BlackGoRBefore,
		wp.Name, gm.WhiteGoRBefore, gm.WhiteGoRAfter, gm.WhiteGoRAfter-gm.WhiteGoRBefore)), nil
}

func floatArg(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		var n float64
		_, err := fmt.Sscanf(x, "%f", &n)
		return n, err == nil
	}
	return 0, false
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
