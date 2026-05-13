// Package store is the SQLite-backed persistence layer for go-liga.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/levino/go-ranking/internal/rating"
)

//go:embed schema.sql
var schemaSQL string

// ErrNotFound is returned when a lookup yields no rows.
var ErrNotFound = errors.New("not found")

type Store struct {
	DB *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies the
// schema.  Foreign keys and WAL are enabled for concurrent reads.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite single-writer
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// ---- Domain types --------------------------------------------------------

type Group struct {
	ID        int64
	Slug      string
	Name      string
	CreatedAt time.Time
}

type Player struct {
	ID      int64
	GroupID int64
	Name    string
	GoR     float64
	Active  bool
}

type SnapshotEntry struct {
	PlayerID int64   `json:"player_id"`
	Number   int     `json:"number"`
	Name     string  `json:"name"`
	GoR      float64 `json:"gor"`
}

type Session struct {
	ID         int64
	GroupID    int64
	Passphrase string
	Snapshot   []SnapshotEntry
	CreatedAt  time.Time
}

type Game struct {
	ID             int64
	SessionID      int64
	BlackPlayerID  int64
	WhitePlayerID  int64
	BoardSize      rating.BoardSize
	Handicap       int
	Komi           float64
	Winner         string // "black" or "white"
	BlackGoRBefore float64
	WhiteGoRBefore float64
	BlackGoRAfter  float64
	WhiteGoRAfter  float64
	PlayedAt       time.Time
}

type User struct {
	ID          int64
	OIDCSubject string
	Email       string
	Name        string
	CreatedAt   time.Time
}

// ---- Groups --------------------------------------------------------------

func (s *Store) CreateGroup(ctx context.Context, slug, name string) (*Group, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO groups(slug,name) VALUES(?,?)`, slug, name)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Group{ID: id, Slug: slug, Name: name, CreatedAt: time.Now()}, nil
}

func (s *Store) GroupBySlug(ctx context.Context, slug string) (*Group, error) {
	g := &Group{}
	var created string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,slug,name,created_at FROM groups WHERE slug=?`, slug).
		Scan(&g.ID, &g.Slug, &g.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return g, nil
}

func (s *Store) GroupByID(ctx context.Context, id int64) (*Group, error) {
	g := &Group{}
	var created string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,slug,name,created_at FROM groups WHERE id=?`, id).
		Scan(&g.ID, &g.Slug, &g.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return g, nil
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,slug,name,created_at FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		var created string
		if err := rows.Scan(&g.ID, &g.Slug, &g.Name, &created); err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, g)
	}
	return out, rows.Err()
}

// ---- Players -------------------------------------------------------------

func (s *Store) CreatePlayer(ctx context.Context, groupID int64, name string, gor float64) (*Player, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO players(group_id,name,gor) VALUES(?,?,?)`, groupID, name, gor)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Player{ID: id, GroupID: groupID, Name: name, GoR: gor, Active: true}, nil
}

func (s *Store) UpdatePlayer(ctx context.Context, id int64, name string, active bool) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE players SET name=?, active=? WHERE id=?`, name, boolToInt(active), id)
	return err
}

// PlayerByGroupAndName looks up a player within a group by exact name.
// Used by the MCP add_player/update_player tools so callers can identify
// players without having to remember numeric IDs.
func (s *Store) PlayerByGroupAndName(ctx context.Context, groupID int64, name string) (*Player, error) {
	p := &Player{}
	var act int
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,group_id,name,gor,active FROM players WHERE group_id=? AND name=?`,
		groupID, name).
		Scan(&p.ID, &p.GroupID, &p.Name, &p.GoR, &act)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Active = act != 0
	return p, nil
}

func (s *Store) PlayerByID(ctx context.Context, id int64) (*Player, error) {
	p := &Player{}
	var act int
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,group_id,name,gor,active FROM players WHERE id=?`, id).
		Scan(&p.ID, &p.GroupID, &p.Name, &p.GoR, &act)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Active = act != 0
	return p, nil
}

func (s *Store) ListPlayers(ctx context.Context, groupID int64, includeInactive bool) ([]Player, error) {
	q := `SELECT id,group_id,name,gor,active FROM players WHERE group_id=?`
	if !includeInactive {
		q += ` AND active=1`
	}
	q += ` ORDER BY gor DESC, name`
	rows, err := s.DB.QueryContext(ctx, q, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Player
	for rows.Next() {
		var p Player
		var act int
		if err := rows.Scan(&p.ID, &p.GroupID, &p.Name, &p.GoR, &act); err != nil {
			return nil, err
		}
		p.Active = act != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- Sessions ------------------------------------------------------------

func (s *Store) CreateSession(ctx context.Context, groupID int64, passphrase string, snapshot []SnapshotEntry) (*Session, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions(group_id,passphrase,snapshot) VALUES(?,?,?)`, groupID, passphrase, string(raw))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Session{ID: id, GroupID: groupID, Passphrase: passphrase, Snapshot: snapshot, CreatedAt: time.Now()}, nil
}

func (s *Store) SessionByPassphrase(ctx context.Context, p string) (*Session, error) {
	sess := &Session{}
	var snap, created string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,group_id,passphrase,snapshot,created_at FROM sessions WHERE passphrase=?`, p).
		Scan(&sess.ID, &sess.GroupID, &sess.Passphrase, &snap, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(snap), &sess.Snapshot); err != nil {
		return nil, fmt.Errorf("bad snapshot: %w", err)
	}
	sess.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return sess, nil
}

func (s *Store) SessionByID(ctx context.Context, id int64) (*Session, error) {
	sess := &Session{}
	var snap, created string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,group_id,passphrase,snapshot,created_at FROM sessions WHERE id=?`, id).
		Scan(&sess.ID, &sess.GroupID, &sess.Passphrase, &snap, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(snap), &sess.Snapshot); err != nil {
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return sess, nil
}

func (s *Store) ListSessions(ctx context.Context, groupID int64) ([]Session, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id,group_id,passphrase,snapshot,created_at FROM sessions WHERE group_id=? ORDER BY created_at DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		var snap, created string
		if err := rows.Scan(&sess.ID, &sess.GroupID, &sess.Passphrase, &snap, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(snap), &sess.Snapshot)
		sess.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, sess)
	}
	return out, rows.Err()
}

// ---- Games ---------------------------------------------------------------

// RecordGame inserts a new game and updates both players' GoR atomically.
func (s *Store) RecordGame(ctx context.Context, g Game) (*Game, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO games(session_id,black_player_id,white_player_id,board_size,handicap,komi,winner,
		    black_gor_before,white_gor_before,black_gor_after,white_gor_after)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		g.SessionID, g.BlackPlayerID, g.WhitePlayerID, int(g.BoardSize), g.Handicap, g.Komi, g.Winner,
		g.BlackGoRBefore, g.WhiteGoRBefore, g.BlackGoRAfter, g.WhiteGoRAfter)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE players SET gor=? WHERE id=?`, g.BlackGoRAfter, g.BlackPlayerID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE players SET gor=? WHERE id=?`, g.WhiteGoRAfter, g.WhitePlayerID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	g.ID = id
	g.PlayedAt = time.Now()
	return &g, nil
}

func (s *Store) ListGamesBySession(ctx context.Context, sessionID int64) ([]Game, error) {
	return s.queryGames(ctx,
		`SELECT id,session_id,black_player_id,white_player_id,board_size,handicap,komi,winner,
		        black_gor_before,white_gor_before,black_gor_after,white_gor_after,played_at
		 FROM games WHERE session_id=? ORDER BY played_at`, sessionID)
}

func (s *Store) ListRecentGames(ctx context.Context, groupID int64, limit int) ([]Game, error) {
	return s.queryGames(ctx,
		`SELECT g.id,g.session_id,g.black_player_id,g.white_player_id,g.board_size,g.handicap,g.komi,g.winner,
		        g.black_gor_before,g.white_gor_before,g.black_gor_after,g.white_gor_after,g.played_at
		   FROM games g JOIN sessions s ON g.session_id=s.id
		  WHERE s.group_id=? ORDER BY g.played_at DESC LIMIT ?`, groupID, limit)
}

func (s *Store) ListGamesByPlayer(ctx context.Context, playerID int64) ([]Game, error) {
	return s.queryGames(ctx,
		`SELECT id,session_id,black_player_id,white_player_id,board_size,handicap,komi,winner,
		        black_gor_before,white_gor_before,black_gor_after,white_gor_after,played_at
		   FROM games WHERE black_player_id=? OR white_player_id=? ORDER BY played_at`,
		playerID, playerID)
}

func (s *Store) queryGames(ctx context.Context, q string, args ...any) ([]Game, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Game
	for rows.Next() {
		var g Game
		var bs int
		var played string
		if err := rows.Scan(&g.ID, &g.SessionID, &g.BlackPlayerID, &g.WhitePlayerID, &bs,
			&g.Handicap, &g.Komi, &g.Winner,
			&g.BlackGoRBefore, &g.WhiteGoRBefore, &g.BlackGoRAfter, &g.WhiteGoRAfter, &played); err != nil {
			return nil, err
		}
		g.BoardSize = rating.BoardSize(bs)
		g.PlayedAt, _ = time.Parse("2006-01-02 15:04:05", played)
		out = append(out, g)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- Users ---------------------------------------------------------------

// UpsertUserByOIDC creates or updates a user identified by their OIDC
// subject. Email/name are refreshed on every login so a name change in
// Zitadel propagates here.
func (s *Store) UpsertUserByOIDC(ctx context.Context, subject, email, name string) (*User, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users(oidc_subject,email,name) VALUES(?,?,?)
		 ON CONFLICT(oidc_subject) DO UPDATE SET email=excluded.email, name=excluded.name`,
		subject, email, name); err != nil {
		return nil, err
	}
	u := &User{}
	var created string
	if err := tx.QueryRowContext(ctx,
		`SELECT id,oidc_subject,email,name,created_at FROM users WHERE oidc_subject=?`, subject).
		Scan(&u.ID, &u.OIDCSubject, &u.Email, &u.Name, &created); err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	u := &User{}
	var created string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,oidc_subject,email,name,created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.OIDCSubject, &u.Email, &u.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return u, nil
}

func (s *Store) UserByOIDC(ctx context.Context, subject string) (*User, error) {
	u := &User{}
	var created string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,oidc_subject,email,name,created_at FROM users WHERE oidc_subject=?`, subject).
		Scan(&u.ID, &u.OIDCSubject, &u.Email, &u.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return u, nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	var created string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,oidc_subject,email,name,created_at FROM users WHERE email=?`, email).
		Scan(&u.ID, &u.OIDCSubject, &u.Email, &u.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return u, nil
}

// ---- Group admins (M:N) --------------------------------------------------

func (s *Store) AddGroupAdmin(ctx context.Context, userID, groupID int64) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO group_admins(user_id,group_id) VALUES(?,?)
		 ON CONFLICT DO NOTHING`, userID, groupID)
	return err
}

func (s *Store) RemoveGroupAdmin(ctx context.Context, userID, groupID int64) error {
	_, err := s.DB.ExecContext(ctx,
		`DELETE FROM group_admins WHERE user_id=? AND group_id=?`, userID, groupID)
	return err
}

func (s *Store) IsGroupAdmin(ctx context.Context, userID, groupID int64) (bool, error) {
	var n int
	if err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM group_admins WHERE user_id=? AND group_id=?`,
		userID, groupID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListAdminGroups returns the groups the given user administers.
func (s *Store) ListAdminGroups(ctx context.Context, userID int64) ([]Group, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT g.id,g.slug,g.name,g.created_at
		   FROM groups g JOIN group_admins ga ON ga.group_id=g.id
		  WHERE ga.user_id=? ORDER BY g.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		var created string
		if err := rows.Scan(&g.ID, &g.Slug, &g.Name, &created); err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListGroupAdmins returns the users who administer the given group.
func (s *Store) ListGroupAdmins(ctx context.Context, groupID int64) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT u.id,u.oidc_subject,u.email,u.name,u.created_at
		   FROM users u JOIN group_admins ga ON ga.user_id=u.id
		  WHERE ga.group_id=? ORDER BY u.email`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var created string
		if err := rows.Scan(&u.ID, &u.OIDCSubject, &u.Email, &u.Name, &created); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, u)
	}
	return out, rows.Err()
}
