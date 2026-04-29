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
	"strconv"
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
	tmpls   map[string]*template.Template
}

type ctxKey string

const userKey ctxKey = "user"

func New(s *service.Service, signer *auth.Signer) (*Server, error) {
	srv := &Server{Service: s, Signer: signer}
	if err := srv.loadTemplates(); err != nil {
		return nil, err
	}
	return srv, nil
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
	pages := []string{"login", "index", "dashboard", "players", "sessions", "session", "admin"}
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

// Handler returns the root mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /login", s.handleLoginGET)
	mux.HandleFunc("POST /login", s.handleLoginPOST)
	mux.HandleFunc("POST /logout", s.handleLogout)

	mux.HandleFunc("GET /admin", s.requireAdmin(s.handleAdmin))
	mux.HandleFunc("POST /admin/groups", s.requireAdmin(s.handleAdminCreateGroup))
	mux.HandleFunc("POST /admin/users", s.requireAdmin(s.handleAdminCreateUser))

	mux.HandleFunc("GET /g/{slug}", s.requireGroup(s.handleDashboard))
	mux.HandleFunc("GET /g/{slug}/players", s.requireGroup(s.handlePlayersGET))
	mux.HandleFunc("POST /g/{slug}/players", s.requireGroup(s.handlePlayersPOST))
	mux.HandleFunc("POST /g/{slug}/players/{id}", s.requireGroup(s.handlePlayerUpdate))
	mux.HandleFunc("GET /g/{slug}/sessions", s.requireGroup(s.handleSessionsGET))
	mux.HandleFunc("POST /g/{slug}/sessions", s.requireGroup(s.handleSessionsPOST))
	mux.HandleFunc("GET /g/{slug}/sessions/{pass}", s.requireGroup(s.handleSessionShow))
	mux.HandleFunc("GET /g/{slug}/sessions/{pass}/matrix.pdf", s.requireGroup(s.handleMatrixPDF))
	mux.HandleFunc("GET /g/{slug}/sessions/{pass}/scorecard.pdf", s.requireGroup(s.handleScoreCardPDF))

	return s.withAuth(mux)
}

// ---- Middleware ----------------------------------------------------------

type pageContext struct {
	Title  string
	User   *store.User
	Group  *store.Group
	Flash  string
	Error  string
	Groups []store.Group
	// per-page extras filled in via embedding
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

func (s *Server) requireAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userOf(r)
		if u == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if !u.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func (s *Server) requireGroup(h func(w http.ResponseWriter, r *http.Request, g *store.Group)) http.HandlerFunc {
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
		if !u.IsAdmin && (!u.GroupID.Valid || u.GroupID.Int64 != g.ID) {
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
	if !u.IsAdmin && u.GroupID.Valid {
		// Per-group user goes straight to their dashboard.
		g, err := s.Service.Store.GroupByID(r.Context(), u.GroupID.Int64)
		if err == nil {
			http.Redirect(w, r, "/g/"+g.Slug, http.StatusFound)
			return
		}
	}
	groups, _ := s.Service.Store.ListGroups(r.Context())
	s.render(w, "index", pageContext{Title: "Start", User: u, Groups: groups})
}

func (s *Server) handleLoginGET(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login", pageContext{Title: "Anmelden"})
}

func (s *Server) handleLoginPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	u, err := s.Service.Store.UserByUsername(r.Context(), r.FormValue("username"))
	if err != nil {
		s.render(w, "login", pageContext{Title: "Anmelden", Error: "Ungültige Anmeldedaten."})
		return
	}
	ok, err := auth.VerifyPassword(r.FormValue("password"), u.PasswordHash)
	if err != nil || !ok {
		s.render(w, "login", pageContext{Title: "Anmelden", Error: "Ungültige Anmeldedaten."})
		return
	}
	if err := s.Signer.Issue(w, u.ID, 30*24*time.Hour); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.Signer.Clear(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	groups, _ := s.Service.Store.ListGroups(r.Context())
	s.render(w, "admin", pageContext{Title: "Admin", User: u, Groups: groups})
}

func (s *Server) handleAdminCreateGroup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", 400)
		return
	}
	if _, err := s.Service.CreateGroup(r.Context(), name); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	gidStr := r.FormValue("group_id")
	if username == "" || password == "" {
		http.Error(w, "username and password required", 400)
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var gid *int64
	isAdmin := true
	if gidStr != "" {
		v, err := strconv.ParseInt(gidStr, 10, 64)
		if err != nil {
			http.Error(w, "bad group_id", 400)
			return
		}
		gid = &v
		isAdmin = false
	}
	if _, err := s.Service.Store.CreateUser(r.Context(), username, hash, gid, isAdmin); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
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

func (s *Server) handlePlayersPOST(w http.ResponseWriter, r *http.Request, g *store.Group) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", 400)
		return
	}
	gor := 100.0
	if rk := r.FormValue("rank"); rk != "" {
		v, err := rating.FromRank(rk)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		gor = v
	}
	if _, err := s.Service.Store.CreatePlayer(r.Context(), g.ID, name, gor); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/g/"+g.Slug+"/players", http.StatusFound)
}

func (s *Server) handlePlayerUpdate(w http.ResponseWriter, r *http.Request, g *store.Group) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	p, err := s.Service.Store.PlayerByID(r.Context(), id)
	if err != nil || p.GroupID != g.ID {
		http.Error(w, "not found", 404)
		return
	}
	active := r.FormValue("active") == "1"
	name := r.FormValue("name")
	if name == "" {
		name = p.Name
	}
	if err := s.Service.Store.UpdatePlayer(r.Context(), id, name, active); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/g/"+g.Slug+"/players", http.StatusFound)
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

func (s *Server) handleSessionsPOST(w http.ResponseWriter, r *http.Request, g *store.Group) {
	sess, err := s.Service.CreateSession(r.Context(), g.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/g/"+g.Slug+"/sessions/"+sess.Passphrase, http.StatusFound)
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

