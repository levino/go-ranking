// Package web is the HTML server-side rendered UI.
package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/levino/go-ranking/internal/auth"
	"github.com/levino/go-ranking/internal/docs"
	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

//go:embed templates/*.html
var tmplFS embed.FS

type Server struct {
	Service *service.Service
	Signer  *auth.Signer
	OIDC    *auth.OIDC
	tmpls   map[string]*template.Template

	// mcpURL is the public URL of the /mcp endpoint, derived from the
	// OIDC redirect URL on init. Shown on the index page so users can
	// copy it into Claude.ai without hunting for the host.
	mcpURL string
}

type ctxKey string

const (
	userKey         ctxKey = "user"
	oidcStateCookie        = "go_liga_oidc_state"
	returnToCookie         = "go_liga_return_to"
)

func New(s *service.Service, signer *auth.Signer, oidc *auth.OIDC) (*Server, error) {
	srv := &Server{Service: s, Signer: signer, OIDC: oidc}
	srv.mcpURL = deriveMCPURL(oidc.RedirectURL)
	if err := srv.loadTemplates(); err != nil {
		return nil, err
	}
	return srv, nil
}

// deriveMCPURL produces the public /mcp URL from the OIDC redirect URL
// (which is the only canonical public origin we have on hand).
func deriveMCPURL(redirectURL string) string {
	u, err := url.Parse(redirectURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/mcp"
}

func (s *Server) loadTemplates() error {
	funcs := template.FuncMap{
		"add":  func(a, b int) int { return a + b },
		"sub":  func(a, b float64) float64 { return a - b },
		"rank": rating.FormatRank,
		"playerName": func(m map[int64]string, id int64) string {
			if n, ok := m[id]; ok {
				return n
			}
			return fmt.Sprintf("#%d", id)
		},
		// Deterministic per-name colour for the player tiles. Eight
		// saturated, kid-friendly hues; the same name always renders the
		// same colour, so kids learn to recognise themselves visually.
		// Returns template.CSS so html/template doesn't replace it with
		// the ZgotmplZ safety placeholder in a style="" attribute.
		"tileColor": func(name string) template.CSS {
			var h uint32
			for _, r := range name {
				h = h*31 + uint32(r)
			}
			palette := []string{
				"#e63946", // crimson
				"#f4a261", // peach
				"#e9c46a", // mustard
				"#84cc16", // lime
				"#14b8a6", // teal
				"#0ea5e9", // sky
				"#a855f7", // violet
				"#fb7185", // rose
			}
			return template.CSS(palette[int(h%uint32(len(palette)))])
		},
		// Komi rendered with a non-confusing sign: positive shown as-is,
		// negative shown as e.g. "Rückkomi 7,5" so the kids see what
		// "negative" actually means in Go.
		"komiText": func(v float64) string {
			if v < 0 {
				return fmt.Sprintf("Rückkomi %.1f", -v)
			}
			return fmt.Sprintf("Komi %.1f", v)
		},
		"contains": func(list []store.Player, id int64) bool {
			for _, p := range list {
				if p.ID == id {
					return true
				}
			}
			return false
		},
	}
	pages := []string{
		"index", "dashboard", "players", "admin", "docs",
		"play_start", "play_pick_player", "play_pick_board",
		"play_result", "play_record_finish", "play_confirm",
	}
	s.tmpls = map[string]*template.Template{}
	for _, p := range pages {
		t, err := template.New(p).Funcs(funcs).ParseFS(tmplFS,
			"templates/layout.html",
			"templates/"+p+".html")
		if err != nil {
			return err
		}
		s.tmpls[p] = t
	}
	return nil
}

// Handler returns the root mux. The web UI serves both an admin view
// (rankings, history) and a tablet-friendly "play" page where the kids
// pick pairings and record results live during the session.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("GET /auth/start", s.handleAuthStart)
	mux.HandleFunc("GET /auth/callback", s.handleAuthCallback)
	mux.HandleFunc("POST /logout", s.handleLogout)

	mux.HandleFunc("GET /g/{slug}", s.requireGroupAdmin(s.handleDashboard))
	mux.HandleFunc("GET /g/{slug}/admins", s.requireGroupAdmin(s.handleAdminsGET))
	mux.HandleFunc("GET /g/{slug}/players", s.requireGroupAdmin(s.handlePlayersGET))

	// Tablet UI — a multi-step wizard the kids tap through.
	// /play landing has two entry buttons: Vorgabe-Rechner & Spiel-Eintrag.
	// Each wizard step is its own URL with the previous picks in the
	// query string, so the browser back button works as expected.
	mux.HandleFunc("GET /g/{slug}/play", s.requireGroupAdmin(s.handlePlayStart))

	// Recommend flow: pick p1 → pick p2 → pick board → see result.
	mux.HandleFunc("GET /g/{slug}/play/r/p1", s.requireGroupAdmin(s.handleRecP1))
	mux.HandleFunc("GET /g/{slug}/play/r/p2", s.requireGroupAdmin(s.handleRecP2))
	mux.HandleFunc("GET /g/{slug}/play/r/board", s.requireGroupAdmin(s.handleRecBoard))
	mux.HandleFunc("GET /g/{slug}/play/r/result", s.requireGroupAdmin(s.handleRecResult))

	// Record flow: pick p1 → pick p2 → pick board → adjust + winner → confirm → commit.
	mux.HandleFunc("GET /g/{slug}/play/g/p1", s.requireGroupAdmin(s.handleGameP1))
	mux.HandleFunc("GET /g/{slug}/play/g/p2", s.requireGroupAdmin(s.handleGameP2))
	mux.HandleFunc("GET /g/{slug}/play/g/board", s.requireGroupAdmin(s.handleGameBoard))
	mux.HandleFunc("GET /g/{slug}/play/g/finish", s.requireGroupAdmin(s.handleGameFinish))
	mux.HandleFunc("POST /g/{slug}/play/g/confirm", s.requireGroupAdmin(s.handleGameConfirm))
	mux.HandleFunc("POST /g/{slug}/play/g/commit", s.requireGroupAdmin(s.handleGameCommit))

	// PWA manifest and icon (served at the origin root so iOS finds them
	// without a per-group prefix). http.ServeMux's path patterns only
	// support whole-segment wildcards, so the sizes are wired explicitly.
	mux.HandleFunc("GET /manifest.webmanifest", s.handleManifest)
	mux.HandleFunc("GET /icon-192.png", s.iconHandler(192))
	mux.HandleFunc("GET /icon-512.png", s.iconHandler(512))

	// Documentation pages.
	mux.HandleFunc("GET /docs", s.handleDocs)
	mux.HandleFunc("GET /docs/{slug}", s.handleDocs)

	return s.withAuth(mux)
}

// ---- Middleware ----------------------------------------------------------

type pageContext struct {
	Title  string
	User   *store.User
	Group  *store.Group
	Groups []store.Group
	Admins []store.User
	MCPURL string
	// per-page extras
	Players     []store.Player
	RecentGames []store.Game
	PlayerNames map[int64]string
	GameCount   int

	// Docs page extras.
	DocsList   []docs.Page
	DocCurrent string
	DocBody    template.HTML

	// Play page extras.
	Recommendation *service.Recommendation
	Flash          string
	Selected       playSelection
	SelectedKomi   float64
	Wizard         WizardState
	// NextBoard is the board-step's per-size link builder. It lives on
	// pageContext rather than WizardState so play_pick_board.html can
	// call it without html/template seeing a dynamic query value.
	NextBoard func(board int) string
}

// WizardState carries the per-step navigation context for the play
// wizard templates: where to go next/back and what to ask.
//
// NextPathFor is a function the template calls per player to build the
// link target. Doing the URL construction in Go avoids html/template's
// "ambiguous context within a URL" complaint when a dynamic query key
// is interpolated into an href.
type WizardState struct {
	Flow        string // "r" (recommend) or "g" (record)
	Step        int
	Headline    string
	BackPath    string
	NextPathFor func(playerID int64) string
}

// playSelection echoes the previous picks for templates that need to
// re-render with the same choices in place (e.g. the confirm page).
type playSelection struct {
	P1ID       int64
	P2ID       int64
	Board      string
	Handicap   int
	BlackID    int64
	WhiteID    int64
	BoardGame  string
	HandicapOK bool
}

// withAuth attaches the authenticated user, if any, to the request.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, err := s.Signer.Read(r)
		if err == nil {
			u, err := s.Service.Store.UserByID(r.Context(), uid)
			if err == nil {
				ctx := context.WithValue(r.Context(), userKey, u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func userOf(r *http.Request) *store.User {
	u, _ := r.Context().Value(userKey).(*store.User)
	return u
}

func (s *Server) requireGroupAdmin(h func(w http.ResponseWriter, r *http.Request, g *store.Group)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userOf(r)
		if u == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		slug := r.PathValue("slug")
		g, err := s.Service.Store.GroupBySlug(r.Context(), slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ok, err := s.Service.Store.IsGroupAdmin(r.Context(), u.ID, g.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r, g)
	}
}

// ---- Handlers ------------------------------------------------------------

func (s *Server) render(w http.ResponseWriter, name string, data pageContext) {
	t, ok := s.tmpls[name]
	if !ok {
		http.Error(w, "template missing: "+name, 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if u == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	groups, err := s.Service.Store.ListAdminGroups(r.Context(), u.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, "index", pageContext{Title: "Start", User: u, Groups: groups, MCPURL: s.mcpURL})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if userOf(r) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	// Single OIDC provider — skip the intermediate page.
	http.Redirect(w, r, "/auth/start", http.StatusFound)
}

func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	state := auth.RandomState()
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 min to complete the flow
	})
	// Allow a return_to so e.g. /oauth/authorize can bounce here and
	// come back after login. Path-only — never an absolute URL — to
	// stop us from doubling as an open redirector.
	if rt := r.URL.Query().Get("return_to"); strings.HasPrefix(rt, "/") {
		http.SetCookie(w, &http.Cookie{
			Name:     returnToCookie,
			Value:    rt,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   300,
		})
	}
	url, err := s.OIDC.AuthURL(r.Context(), state)
	if err != nil {
		http.Error(w, "OIDC discovery failed: "+err.Error(), 500)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if errStr := r.URL.Query().Get("error"); errStr != "" {
		http.Error(w, "OIDC error: "+errStr, 400)
		return
	}
	cookie, err := r.Cookie(oidcStateCookie)
	if err != nil {
		http.Error(w, "missing state cookie", 400)
		return
	}
	if r.URL.Query().Get("state") != cookie.Value {
		http.Error(w, "state mismatch", 400)
		return
	}
	// Clear the state cookie now that we've checked it.
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", 400)
		return
	}
	ui, err := s.OIDC.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), 500)
		return
	}
	user, err := s.Service.Store.UpsertUserByOIDC(r.Context(), ui.Subject, ui.Email, ui.DisplayName())
	if err != nil {
		http.Error(w, "user upsert failed: "+err.Error(), 500)
		return
	}
	if err := s.Signer.Issue(w, user.ID, 30*24*time.Hour); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	dest := "/"
	if c, err := r.Cookie(returnToCookie); err == nil && strings.HasPrefix(c.Value, "/") {
		dest = c.Value
		http.SetCookie(w, &http.Cookie{
			Name: returnToCookie, Path: "/", MaxAge: -1, HttpOnly: true,
			Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode,
		})
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.Signer.Clear(w)
	logoutURL, _ := s.OIDC.LogoutURL(r.Context(), "https://ranking.go-ag.levinkeller.de/login")
	if logoutURL != "" {
		http.Redirect(w, r, logoutURL, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleAdminsGET(w http.ResponseWriter, r *http.Request, g *store.Group) {
	admins, err := s.Service.Store.ListGroupAdmins(r.Context(), g.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, "admin", pageContext{Title: g.Name + " — Admins", User: userOf(r), Group: g, Admins: admins, MCPURL: s.mcpURL})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, g *store.Group) {
	players, _ := s.Service.Store.ListPlayers(r.Context(), g.ID, true)
	games, _ := s.Service.Store.ListRecentGames(r.Context(), g.ID, 25)
	pn := map[int64]string{}
	for _, p := range players {
		pn[p.ID] = p.Name
	}
	s.render(w, "dashboard", pageContext{
		Title:       g.Name,
		User:        userOf(r),
		Group:       g,
		Players:     players,
		RecentGames: games,
		PlayerNames: pn,
		GameCount:   len(games),
	})
}

func (s *Server) handlePlayersGET(w http.ResponseWriter, r *http.Request, g *store.Group) {
	players, _ := s.Service.Store.ListPlayers(r.Context(), g.ID, true)
	s.render(w, "players", pageContext{Title: "Spieler", User: userOf(r), Group: g, Players: players})
}

// handlePlayStart is the landing page: two big entry buttons.
func (s *Server) handlePlayStart(w http.ResponseWriter, r *http.Request, g *store.Group) {
	ctx := s.basePlayContext(r, g, "Spielen — "+g.Name)
	if f := r.URL.Query().Get("flash"); f != "" {
		ctx.Flash = f
	}
	s.render(w, "play_start", ctx)
}

// ---- Recommend wizard ----------------------------------------------------

func (s *Server) handleRecP1(w http.ResponseWriter, r *http.Request, g *store.Group) {
	ctx := s.basePlayContext(r, g, "Wer spielt?")
	ctx.Wizard = WizardState{
		Flow: "r", Step: 1,
		Headline: "Wer ist der erste Spieler?",
		BackPath: "/g/" + g.Slug + "/play",
		NextPathFor: func(id int64) string {
			return fmt.Sprintf("/g/%s/play/r/p2?p1=%d", g.Slug, id)
		},
	}
	s.render(w, "play_pick_player", ctx)
}

func (s *Server) handleRecP2(w http.ResponseWriter, r *http.Request, g *store.Group) {
	p1, _ := parseInt64(r.URL.Query().Get("p1"))
	if p1 == 0 {
		http.Redirect(w, r, "/g/"+g.Slug+"/play/r/p1", http.StatusFound)
		return
	}
	ctx := s.basePlayContext(r, g, "Und der zweite?")
	ctx.Players = filterPlayers(ctx.Players, p1)
	ctx.Wizard = WizardState{
		Flow: "r", Step: 2,
		Headline: "Und wer ist Spieler 2?",
		BackPath: "/g/" + g.Slug + "/play/r/p1",
		NextPathFor: func(id int64) string {
			return fmt.Sprintf("/g/%s/play/r/board?p1=%d&p2=%d", g.Slug, p1, id)
		},
	}
	s.render(w, "play_pick_player", ctx)
}

func (s *Server) handleRecBoard(w http.ResponseWriter, r *http.Request, g *store.Group) {
	p1, _ := parseInt64(r.URL.Query().Get("p1"))
	p2, _ := parseInt64(r.URL.Query().Get("p2"))
	if p1 == 0 || p2 == 0 || p1 == p2 {
		http.Redirect(w, r, "/g/"+g.Slug+"/play/r/p1", http.StatusFound)
		return
	}
	ctx := s.basePlayContext(r, g, "Wie groß ist das Brett?")
	ctx.Wizard = WizardState{
		Flow: "r", Step: 3,
		Headline: "Wie groß ist das Brett?",
		BackPath: fmt.Sprintf("/g/%s/play/r/p2?p1=%d", g.Slug, p1),
	}
	ctx.NextBoard = func(board int) string {
		return fmt.Sprintf("/g/%s/play/r/result?p1=%d&p2=%d&board=%d", g.Slug, p1, p2, board)
	}
	s.render(w, "play_pick_board", ctx)
}

func (s *Server) handleRecResult(w http.ResponseWriter, r *http.Request, g *store.Group) {
	p1, _ := parseInt64(r.URL.Query().Get("p1"))
	p2, _ := parseInt64(r.URL.Query().Get("p2"))
	board, err := rating.ParseBoardSize(r.URL.Query().Get("board"))
	if p1 == 0 || p2 == 0 || err != nil {
		http.Redirect(w, r, "/g/"+g.Slug+"/play/r/p1", http.StatusFound)
		return
	}
	rec, err := s.Service.Recommend(r.Context(), p1, p2, board)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx := s.basePlayContext(r, g, "Vorgabe")
	ctx.Recommendation = rec
	s.render(w, "play_result", ctx)
}

// ---- Record wizard -------------------------------------------------------

func (s *Server) handleGameP1(w http.ResponseWriter, r *http.Request, g *store.Group) {
	ctx := s.basePlayContext(r, g, "Spiel eintragen")
	ctx.Wizard = WizardState{
		Flow: "g", Step: 1,
		Headline: "Wer hat gespielt? Spieler 1.",
		BackPath: "/g/" + g.Slug + "/play",
		NextPathFor: func(id int64) string {
			return fmt.Sprintf("/g/%s/play/g/p2?p1=%d", g.Slug, id)
		},
	}
	s.render(w, "play_pick_player", ctx)
}

func (s *Server) handleGameP2(w http.ResponseWriter, r *http.Request, g *store.Group) {
	p1, _ := parseInt64(r.URL.Query().Get("p1"))
	if p1 == 0 {
		http.Redirect(w, r, "/g/"+g.Slug+"/play/g/p1", http.StatusFound)
		return
	}
	ctx := s.basePlayContext(r, g, "Spieler 2?")
	ctx.Players = filterPlayers(ctx.Players, p1)
	ctx.Wizard = WizardState{
		Flow: "g", Step: 2,
		Headline: "Und wer noch?",
		BackPath: "/g/" + g.Slug + "/play/g/p1",
		NextPathFor: func(id int64) string {
			return fmt.Sprintf("/g/%s/play/g/board?p1=%d&p2=%d", g.Slug, p1, id)
		},
	}
	s.render(w, "play_pick_player", ctx)
}

func (s *Server) handleGameBoard(w http.ResponseWriter, r *http.Request, g *store.Group) {
	p1, _ := parseInt64(r.URL.Query().Get("p1"))
	p2, _ := parseInt64(r.URL.Query().Get("p2"))
	if p1 == 0 || p2 == 0 || p1 == p2 {
		http.Redirect(w, r, "/g/"+g.Slug+"/play/g/p1", http.StatusFound)
		return
	}
	ctx := s.basePlayContext(r, g, "Brettgröße")
	ctx.Wizard = WizardState{
		Flow: "g", Step: 3,
		Headline: "Auf welchem Brett?",
		BackPath: fmt.Sprintf("/g/%s/play/g/p2?p1=%d", g.Slug, p1),
	}
	ctx.NextBoard = func(board int) string {
		return fmt.Sprintf("/g/%s/play/g/finish?p1=%d&p2=%d&board=%d", g.Slug, p1, p2, board)
	}
	s.render(w, "play_pick_board", ctx)
}

func (s *Server) handleGameFinish(w http.ResponseWriter, r *http.Request, g *store.Group) {
	p1, _ := parseInt64(r.URL.Query().Get("p1"))
	p2, _ := parseInt64(r.URL.Query().Get("p2"))
	board, berr := rating.ParseBoardSize(r.URL.Query().Get("board"))
	if p1 == 0 || p2 == 0 || berr != nil {
		http.Redirect(w, r, "/g/"+g.Slug+"/play/g/p1", http.StatusFound)
		return
	}
	rec, err := s.Service.Recommend(r.Context(), p1, p2, board)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx := s.basePlayContext(r, g, "Ergebnis")
	ctx.Recommendation = rec
	ctx.Wizard = WizardState{
		Flow: "g", Step: 4,
		BackPath: fmt.Sprintf("/g/%s/play/g/board?p1=%d&p2=%d", g.Slug, p1, p2),
		Headline: "Wer hat gewonnen?",
	}
	s.render(w, "play_record_finish", ctx)
}

// handleGameConfirm shows the preview/confirmation page after the
// kids have filled in stones/komi and tapped a winner.
func (s *Server) handleGameConfirm(w http.ResponseWriter, r *http.Request, g *store.Group) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	black, _ := parseInt64(r.FormValue("black"))
	white, _ := parseInt64(r.FormValue("white"))
	handicap, _ := parseInt64(r.FormValue("handicap"))
	komi, _ := parseFloat(r.FormValue("komi"))
	winner := r.FormValue("winner")
	board, err := rating.ParseBoardSize(r.FormValue("board"))
	if err != nil || black == 0 || white == 0 || (winner != "black" && winner != "white") {
		http.Error(w, "missing or invalid field", 400)
		return
	}
	bp, err := s.Service.Store.PlayerByID(r.Context(), black)
	if err != nil || bp.GroupID != g.ID {
		http.Error(w, "unknown black player", 400)
		return
	}
	wp, err := s.Service.Store.PlayerByID(r.Context(), white)
	if err != nil || wp.GroupID != g.ID {
		http.Error(w, "unknown white player", 400)
		return
	}
	pn := map[int64]string{bp.ID: bp.Name, wp.ID: wp.Name}
	ctx := s.basePlayContext(r, g, "Bestätigen")
	ctx.PlayerNames = pn
	ctx.Recommendation = &service.Recommendation{
		BlackPlayer: bp, WhitePlayer: wp, Board: board,
		Stones: int(handicap), Komi: komi,
	}
	ctx.Selected = playSelection{
		BlackID: bp.ID, WhiteID: wp.ID, BoardGame: fmt.Sprintf("%d", board),
		Handicap: int(handicap), HandicapOK: true,
	}
	ctx.SelectedKomi = komi
	ctx.Flash = winner
	s.render(w, "play_confirm", ctx)
}

// handleGameCommit writes the game to the store and redirects to /play
// with a flash message.
func (s *Server) handleGameCommit(w http.ResponseWriter, r *http.Request, g *store.Group) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	black, _ := parseInt64(r.FormValue("black"))
	white, _ := parseInt64(r.FormValue("white"))
	handicap, _ := parseInt64(r.FormValue("handicap"))
	komi, _ := parseFloat(r.FormValue("komi"))
	winner := r.FormValue("winner")
	board, err := rating.ParseBoardSize(r.FormValue("board"))
	if err != nil || black == 0 || white == 0 || (winner != "black" && winner != "white") {
		http.Error(w, "missing or invalid field", 400)
		return
	}
	gm, err := s.Service.RecordGame(r.Context(), g.ID, black, white, board, int(handicap), komi, winner == "black")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	bp, _ := s.Service.Store.PlayerByID(r.Context(), gm.BlackPlayerID)
	wp, _ := s.Service.Store.PlayerByID(r.Context(), gm.WhitePlayerID)
	flash := fmt.Sprintf("%s vs %s — %s hat gewonnen.", bp.Name, wp.Name, winnerName(winner, bp, wp))
	http.Redirect(w, r, "/g/"+g.Slug+"/play?flash="+url.QueryEscape(flash), http.StatusFound)
}

// handleDocs serves the embedded Markdown manual. The slug comes from
// the path; if empty (i.e. just `/docs`), redirect to the first page.
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	pages := docs.List()
	if len(pages) == 0 {
		http.NotFound(w, r)
		return
	}
	if slug == "" {
		http.Redirect(w, r, "/docs/"+pages[0].Slug, http.StatusFound)
		return
	}
	body := docs.Render(slug)
	if body == "" {
		http.NotFound(w, r)
		return
	}
	var title string
	for _, p := range pages {
		if p.Slug == slug {
			title = p.Title
		}
	}
	s.render(w, "docs", pageContext{
		Title:      title + " — Handbuch",
		User:       userOf(r),
		DocsList:   pages,
		DocCurrent: slug,
		DocBody:    template.HTML(body),
	})
}

// basePlayContext gathers the bits every wizard step needs: the player
// list (active only) and recent games for the footer.
func (s *Server) basePlayContext(r *http.Request, g *store.Group, title string) pageContext {
	players, _ := s.Service.Store.ListPlayers(r.Context(), g.ID, false)
	games, _ := s.Service.Store.ListRecentGames(r.Context(), g.ID, 6)
	pn := map[int64]string{}
	for _, p := range players {
		pn[p.ID] = p.Name
	}
	return pageContext{
		Title:       title,
		User:        userOf(r),
		Group:       g,
		Players:     players,
		RecentGames: games,
		PlayerNames: pn,
	}
}

// filterPlayers returns a copy of the active list with the given id
// removed. Used by the p2 step so the template doesn't have to filter.
func filterPlayers(in []store.Player, exclude int64) []store.Player {
	out := make([]store.Player, 0, len(in))
	for _, p := range in {
		if p.ID != exclude {
			out = append(out, p)
		}
	}
	return out
}

func winnerName(winner string, b, w *store.Player) string {
	if winner == "black" {
		return b.Name
	}
	return w.Name
}

func parseInt64(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func parseFloat(s string) (float64, error) {
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	return v, err
}

// ---- PWA -----------------------------------------------------------------

// handleManifest serves the Web App Manifest. iOS reads name/short_name
// and the apple-touch-icon link (in layout.html) to render a tidy home
// screen icon when the user picks "Add to Home Screen".
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(`{
  "name": "Go-Liga",
  "short_name": "Go-Liga",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "orientation": "any",
  "background_color": "#fafafa",
  "theme_color": "#1c5d99",
  "icons": [
    {"src": "/icon-192.png", "sizes": "192x192", "type": "image/png"},
    {"src": "/icon-512.png", "sizes": "512x512", "type": "image/png"}
  ]
}`))
}

// iconHandler returns a handler that synthesises a PWA icon at the
// given size: a black Go stone (with a small highlight) on a white
// background. Generated on the fly with image/png — no asset files.
func (s *Server) iconHandler(size int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		img := drawStoneIcon(size)
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_ = png.Encode(w, img)
	}
}

func drawStoneIcon(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	bg := color.RGBA{0xfa, 0xfa, 0xfa, 0xff}
	stone := color.RGBA{0x1e, 0x1e, 0x1e, 0xff}
	highlight := color.RGBA{0x70, 0x70, 0x70, 0xff}
	// Fill background.
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, bg)
		}
	}
	cx, cy := size/2, size/2
	r := size/2 - size/16 // small margin
	r2 := r * r
	// Stone — solid black circle.
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r2 {
				img.Set(x, y, stone)
			}
		}
	}
	// Highlight — a smaller grey disc offset top-left, giving the stone
	// a faint sense of volume on iOS dock backgrounds.
	hr := size / 6
	hcx, hcy := cx-r/3, cy-r/3
	hr2 := hr * hr
	for y := hcy - hr; y <= hcy+hr; y++ {
		for x := hcx - hr; x <= hcx+hr; x++ {
			dx, dy := x-hcx, y-hcy
			if dx*dx+dy*dy <= hr2 {
				img.Set(x, y, highlight)
			}
		}
	}
	return img
}

