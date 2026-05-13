// Package web is the HTML server-side rendered UI.
package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/levino/go-ranking/internal/auth"
	"github.com/levino/go-ranking/internal/pdfgen"
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
	}
	pages := []string{"index", "dashboard", "players", "sessions", "session", "admin"}
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

// Handler returns the root mux. The web UI is intentionally read-only —
// every mutation goes through the MCP server (the only writer).
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
	mux.HandleFunc("GET /g/{slug}/sessions", s.requireGroupAdmin(s.handleSessionsGET))
	mux.HandleFunc("GET /g/{slug}/sessions/{pass}", s.requireGroupAdmin(s.handleSessionShow))
	mux.HandleFunc("GET /g/{slug}/sessions/{pass}/matrix.pdf", s.requireGroupAdmin(s.handleMatrixPDF))
	mux.HandleFunc("GET /g/{slug}/sessions/{pass}/scorecard.pdf", s.requireGroupAdmin(s.handleScoreCardPDF))

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
	Players      []store.Player
	Sessions     []store.Session
	Session      *store.Session
	GameCounts   map[int64]int
	RecentGames  []store.Game
	Games        []store.Game
	PlayerNames  map[int64]string
	SessionCount int
	GameCount    int
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
	sess, _ := s.Service.Store.ListSessions(r.Context(), g.ID)
	pn := map[int64]string{}
	for _, p := range players {
		pn[p.ID] = p.Name
	}
	gameCount := 0
	for _, sx := range sess {
		gs, _ := s.Service.Store.ListGamesBySession(r.Context(), sx.ID)
		gameCount += len(gs)
	}
	s.render(w, "dashboard", pageContext{
		Title:        g.Name,
		User:         userOf(r),
		Group:        g,
		Players:      players,
		RecentGames:  games,
		PlayerNames:  pn,
		SessionCount: len(sess),
		GameCount:    gameCount,
	})
}

func (s *Server) handlePlayersGET(w http.ResponseWriter, r *http.Request, g *store.Group) {
	players, _ := s.Service.Store.ListPlayers(r.Context(), g.ID, true)
	s.render(w, "players", pageContext{Title: "Spieler", User: userOf(r), Group: g, Players: players})
}

func (s *Server) handleSessionsGET(w http.ResponseWriter, r *http.Request, g *store.Group) {
	sess, _ := s.Service.Store.ListSessions(r.Context(), g.ID)
	counts := map[int64]int{}
	for _, sx := range sess {
		gs, _ := s.Service.Store.ListGamesBySession(r.Context(), sx.ID)
		counts[sx.ID] = len(gs)
	}
	s.render(w, "sessions", pageContext{Title: "Sessions", User: userOf(r), Group: g, Sessions: sess, GameCounts: counts})
}

func (s *Server) handleSessionShow(w http.ResponseWriter, r *http.Request, g *store.Group) {
	sess, err := s.lookupSession(r, g)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	games, _ := s.Service.Store.ListGamesBySession(r.Context(), sess.ID)
	pn := map[int64]string{}
	for _, e := range sess.Snapshot {
		pn[e.PlayerID] = e.Name
	}
	s.render(w, "session", pageContext{Title: sess.Passphrase, User: userOf(r), Group: g,
		Session: sess, Games: games, PlayerNames: pn})
}

func (s *Server) handleMatrixPDF(w http.ResponseWriter, r *http.Request, g *store.Group) {
	sess, err := s.lookupSession(r, g)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`inline; filename="matrix-%s.pdf"`, sess.Passphrase))
	err = pdfgen.Matrix(w, sess.Snapshot, pdfgen.MatrixOptions{
		GroupName:  g.Name,
		Passphrase: sess.Passphrase,
		Date:       sess.CreatedAt.Format("02.01.2006"),
		Boards:     []rating.BoardSize{rating.Board9, rating.Board13},
	})
	if err != nil {
		log.Printf("matrix pdf: %v", err)
	}
}

func (s *Server) handleScoreCardPDF(w http.ResponseWriter, r *http.Request, g *store.Group) {
	sess, err := s.lookupSession(r, g)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`inline; filename="scorecard-%s.pdf"`, sess.Passphrase))
	err = pdfgen.ScoreCard(w, sess.Snapshot, pdfgen.ScoreCardOptions{
		GroupName:  g.Name,
		Passphrase: sess.Passphrase,
		Date:       sess.CreatedAt.Format("02.01.2006"),
	})
	if err != nil {
		log.Printf("scorecard pdf: %v", err)
	}
}

func (s *Server) lookupSession(r *http.Request, g *store.Group) (*store.Session, error) {
	pass := r.PathValue("pass")
	sess, err := s.Service.Store.SessionByPassphrase(r.Context(), pass)
	if err != nil {
		return nil, err
	}
	if sess.GroupID != g.ID {
		return nil, errors.New("session not in this group")
	}
	return sess, nil
}
