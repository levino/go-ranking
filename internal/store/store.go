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
//
// Order matters: the v1→v2 migration runs FIRST, before schema.sql,
// because schema.sql's CREATE INDEX idx_games_group references the new
// group_id column that the migration adds.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite single-writer
	s := &Store{DB: db}
	if err := s.migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := s.migrateRatings(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate ratings: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return s, nil
}

// migrateRatings handles the v2 → v3 transition: the single legacy
// EGF-GoR number per player is replaced by an OGS Glicko-2 triple
// (rating/deviation/volatility) plus a seed, and the per-board ratings
// grid (player_ratings) is added. Legacy GoR values are converted to
// the OGS rating scale. Runs before schema.sql; a no-op on a fresh DB
// (no players table yet) or once already migrated. Idempotent.
func (s *Store) migrateRatings(ctx context.Context) error {
	var playersExists, hasDeviation bool
	rows, err := s.DB.QueryContext(ctx, "PRAGMA table_info(players)")
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		playersExists = true
		if name == "deviation" {
			hasDeviation = true
		}
	}
	rows.Close()
	if !playersExists || hasDeviation {
		return nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`ALTER TABLE players ADD COLUMN deviation REAL NOT NULL DEFAULT 350`,
		`ALTER TABLE players ADD COLUMN volatility REAL NOT NULL DEFAULT 0.06`,
		`ALTER TABLE players ADD COLUMN seed_rating REAL NOT NULL DEFAULT 1500`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	// Convert legacy EGF-GoR player ratings to the OGS scale.
	type idVal struct {
		id  int64
		gor float64
	}
	var players []idVal
	prows, err := tx.QueryContext(ctx, `SELECT id, gor FROM players`)
	if err != nil {
		return err
	}
	for prows.Next() {
		var p idVal
		if err := prows.Scan(&p.id, &p.gor); err != nil {
			prows.Close()
			return err
		}
		players = append(players, p)
	}
	prows.Close()
	for _, p := range players {
		nr := rating.RatingFromLegacyGoR(p.gor)
		if _, err := tx.ExecContext(ctx,
			`UPDATE players SET gor=?, seed_rating=?, deviation=350, volatility=0.06 WHERE id=?`,
			nr, nr, p.id); err != nil {
			return err
		}
	}

	// Convert legacy game-snapshot columns to the OGS scale.
	type gameSnap struct {
		id                               int64
		bBefore, wBefore, bAfter, wAfter float64
	}
	var games []gameSnap
	grows, err := tx.QueryContext(ctx,
		`SELECT id, black_gor_before, white_gor_before, black_gor_after, white_gor_after FROM games`)
	if err != nil {
		return err
	}
	for grows.Next() {
		var g gameSnap
		if err := grows.Scan(&g.id, &g.bBefore, &g.wBefore, &g.bAfter, &g.wAfter); err != nil {
			grows.Close()
			return err
		}
		games = append(games, g)
	}
	grows.Close()
	conv := rating.RatingFromLegacyGoR
	for _, g := range games {
		if _, err := tx.ExecContext(ctx,
			`UPDATE games SET black_gor_before=?, white_gor_before=?, black_gor_after=?, white_gor_after=? WHERE id=?`,
			conv(g.bBefore), conv(g.wBefore), conv(g.bAfter), conv(g.wAfter), g.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// migrate handles the v1 → v2 transition: drops the legacy `sessions`
// table and rebuilds `games` without `session_id`, carrying the
// existing rows' group via the join. Idempotent.
func (s *Store) migrate(ctx context.Context) error {
	// Inspect the games table's columns.
	var hasSessionID, hasGroupID bool
	rows, err := s.DB.QueryContext(ctx, "PRAGMA table_info(games)")
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		switch name {
		case "session_id":
			hasSessionID = true
		case "group_id":
			hasGroupID = true
		}
	}
	rows.Close()
	if !hasSessionID || hasGroupID {
		return nil // already on the new schema
	}

	var sessExists int
	_ = s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sessions'`).
		Scan(&sessExists)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE games_new (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id         INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
			black_player_id  INTEGER NOT NULL REFERENCES players(id),
			white_player_id  INTEGER NOT NULL REFERENCES players(id),
			board_size       INTEGER NOT NULL,
			handicap         INTEGER NOT NULL,
			komi             REAL NOT NULL,
			winner           TEXT NOT NULL CHECK (winner IN ('black','white')),
			black_gor_before REAL NOT NULL,
			white_gor_before REAL NOT NULL,
			black_gor_after  REAL NOT NULL,
			white_gor_after  REAL NOT NULL,
			played_at        TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		return err
	}
	if sessExists == 1 {
		// Recover group_id via the session join. Existing rows survive.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO games_new (id, group_id, black_player_id, white_player_id,
			    board_size, handicap, komi, winner,
			    black_gor_before, white_gor_before, black_gor_after, white_gor_after, played_at)
			SELECT g.id, s.group_id, g.black_player_id, g.white_player_id,
			       g.board_size, g.handicap, g.komi, g.winner,
			       g.black_gor_before, g.white_gor_before, g.black_gor_after, g.white_gor_after, g.played_at
			  FROM games g JOIN sessions s ON g.session_id = s.id`); err != nil {
			return err
		}
	}
	// If sessions are gone we drop existing rows — the prod DB has none.
	if _, err := tx.ExecContext(ctx, `DROP TABLE games; ALTER TABLE games_new RENAME TO games; DROP TABLE IF EXISTS sessions`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX idx_games_group ON games(group_id);
	                                  CREATE INDEX idx_games_black ON games(black_player_id);
	                                  CREATE INDEX idx_games_white ON games(white_player_id);`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.DB.Close() }

// ---- Domain types --------------------------------------------------------

type Group struct {
	ID        int64
	Slug      string
	Name      string
	CreatedAt time.Time
}

// Player carries the "overall" Glicko-2 rating. GoR holds the overall
// rating value (the field name is kept for storage continuity);
// Deviation and Volatility are the other two Glicko-2 parameters.
// SeedRating is the strength the player was created at.
type Player struct {
	ID         int64
	GroupID    int64
	Name       string
	GoR        float64
	Deviation  float64
	Volatility float64
	SeedRating float64
	Active     bool
}

// RatingState is a Glicko-2 triple with a game counter.
type RatingState struct {
	Rating     float64
	Deviation  float64
	Volatility float64
	Games      int
}

// CategoryRating is a player's Glicko-2 rating in one board-size
// category ("9x9", "13x13", "19x19").
type CategoryRating struct {
	PlayerID int64
	Category string
	RatingState
}

type Game struct {
	ID             int64
	GroupID        int64
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

// CreatePlayer adds a player seeded at the given rating (the trainer's
// strength estimate). Deviation and volatility start at the Glicko-2
// defaults.
func (s *Store) CreatePlayer(ctx context.Context, groupID int64, name string, seed float64) (*Player, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO players(group_id,name,gor,deviation,volatility,seed_rating) VALUES(?,?,?,?,?,?)`,
		groupID, name, seed, rating.DefaultDeviation, rating.DefaultVolatility, seed)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Player{
		ID: id, GroupID: groupID, Name: name, GoR: seed,
		Deviation: rating.DefaultDeviation, Volatility: rating.DefaultVolatility,
		SeedRating: seed, Active: true,
	}, nil
}

const playerCols = `id,group_id,name,gor,deviation,volatility,seed_rating,active`

func scanPlayer(sc interface{ Scan(...any) error }) (*Player, error) {
	p := &Player{}
	var act int
	if err := sc.Scan(&p.ID, &p.GroupID, &p.Name, &p.GoR, &p.Deviation,
		&p.Volatility, &p.SeedRating, &act); err != nil {
		return nil, err
	}
	p.Active = act != 0
	return p, nil
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
	row := s.DB.QueryRowContext(ctx,
		`SELECT `+playerCols+` FROM players WHERE group_id=? AND name=?`, groupID, name)
	p, err := scanPlayer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) PlayerByID(ctx context.Context, id int64) (*Player, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+playerCols+` FROM players WHERE id=?`, id)
	p, err := scanPlayer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) ListPlayers(ctx context.Context, groupID int64, includeInactive bool) ([]Player, error) {
	q := `SELECT ` + playerCols + ` FROM players WHERE group_id=?`
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
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ---- Per-board ratings grid ----------------------------------------------

// CategoryRating returns a player's rating in one category. If the
// player has no games in that category yet, returns ErrNotFound.
func (s *Store) CategoryRating(ctx context.Context, playerID int64, category string) (CategoryRating, error) {
	cr := CategoryRating{PlayerID: playerID, Category: category}
	err := s.DB.QueryRowContext(ctx,
		`SELECT rating,deviation,volatility,games FROM player_ratings WHERE player_id=? AND category=?`,
		playerID, category).
		Scan(&cr.Rating, &cr.Deviation, &cr.Volatility, &cr.Games)
	if errors.Is(err, sql.ErrNoRows) {
		return cr, ErrNotFound
	}
	return cr, err
}

// ListCategoryRatings returns all per-board ratings for a player.
func (s *Store) ListCategoryRatings(ctx context.Context, playerID int64) ([]CategoryRating, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT category,rating,deviation,volatility,games FROM player_ratings WHERE player_id=?`,
		playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CategoryRating
	for rows.Next() {
		cr := CategoryRating{PlayerID: playerID}
		if err := rows.Scan(&cr.Category, &cr.Rating, &cr.Deviation, &cr.Volatility, &cr.Games); err != nil {
			return nil, err
		}
		out = append(out, cr)
	}
	return out, rows.Err()
}

// ---- Games ---------------------------------------------------------------

// RecordGame inserts a game and updates both players' ratings
// atomically: the overall rating on the players row, and the board-size
// category rating in player_ratings. The caller (service.RecordGame)
// has already computed all new values.
func (s *Store) RecordGame(ctx context.Context, g Game, bOverall, wOverall RatingState, bCat, wCat CategoryRating) (*Game, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO games(group_id,black_player_id,white_player_id,board_size,handicap,komi,winner,
		    black_gor_before,white_gor_before,black_gor_after,white_gor_after)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		g.GroupID, g.BlackPlayerID, g.WhitePlayerID, int(g.BoardSize), g.Handicap, g.Komi, g.Winner,
		g.BlackGoRBefore, g.WhiteGoRBefore, g.BlackGoRAfter, g.WhiteGoRAfter)
	if err != nil {
		return nil, err
	}
	if err := updatePlayerOverall(ctx, tx, g.BlackPlayerID, bOverall); err != nil {
		return nil, err
	}
	if err := updatePlayerOverall(ctx, tx, g.WhitePlayerID, wOverall); err != nil {
		return nil, err
	}
	if err := upsertCategory(ctx, tx, g.BlackPlayerID, bCat); err != nil {
		return nil, err
	}
	if err := upsertCategory(ctx, tx, g.WhitePlayerID, wCat); err != nil {
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

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func updatePlayerOverall(ctx context.Context, e execer, playerID int64, r RatingState) error {
	_, err := e.ExecContext(ctx,
		`UPDATE players SET gor=?, deviation=?, volatility=? WHERE id=?`,
		r.Rating, r.Deviation, r.Volatility, playerID)
	return err
}

func upsertCategory(ctx context.Context, e execer, playerID int64, cr CategoryRating) error {
	_, err := e.ExecContext(ctx,
		`INSERT INTO player_ratings(player_id,category,rating,deviation,volatility,games)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(player_id,category) DO UPDATE SET
		   rating=excluded.rating, deviation=excluded.deviation,
		   volatility=excluded.volatility, games=excluded.games`,
		playerID, cr.Category, cr.Rating, cr.Deviation, cr.Volatility, cr.Games)
	return err
}

// ListGamesByGroupAsc returns every game of a group, oldest first —
// the chronological order a full ratings recompute replays.
func (s *Store) ListGamesByGroupAsc(ctx context.Context, groupID int64) ([]Game, error) {
	return s.queryGames(ctx,
		`SELECT id,group_id,black_player_id,white_player_id,board_size,handicap,komi,winner,
		        black_gor_before,white_gor_before,black_gor_after,white_gor_after,played_at
		   FROM games WHERE group_id=? ORDER BY played_at, id`, groupID)
}

// SaveRecompute persists a full ratings recompute for a group in one
// transaction: every player's overall rating, the whole player_ratings
// grid (rebuilt from scratch), and every game's snapshot columns.
func (s *Store) SaveRecompute(ctx context.Context, groupID int64, players []Player, cats []CategoryRating, games []Game) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range players {
		if err := updatePlayerOverall(ctx, tx, p.ID,
			RatingState{Rating: p.GoR, Deviation: p.Deviation, Volatility: p.Volatility}); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM player_ratings WHERE player_id IN (SELECT id FROM players WHERE group_id=?)`,
		groupID); err != nil {
		return err
	}
	for _, cr := range cats {
		if err := upsertCategory(ctx, tx, cr.PlayerID, cr); err != nil {
			return err
		}
	}
	for _, g := range games {
		if _, err := tx.ExecContext(ctx,
			`UPDATE games SET black_gor_before=?, white_gor_before=?, black_gor_after=?, white_gor_after=? WHERE id=?`,
			g.BlackGoRBefore, g.WhiteGoRBefore, g.BlackGoRAfter, g.WhiteGoRAfter, g.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListRecentGames(ctx context.Context, groupID int64, limit int) ([]Game, error) {
	return s.queryGames(ctx,
		`SELECT id,group_id,black_player_id,white_player_id,board_size,handicap,komi,winner,
		        black_gor_before,white_gor_before,black_gor_after,white_gor_after,played_at
		   FROM games WHERE group_id=? ORDER BY played_at DESC LIMIT ?`, groupID, limit)
}

func (s *Store) ListGamesByPlayer(ctx context.Context, playerID int64) ([]Game, error) {
	return s.queryGames(ctx,
		`SELECT id,group_id,black_player_id,white_player_id,board_size,handicap,komi,winner,
		        black_gor_before,white_gor_before,black_gor_after,white_gor_after,played_at
		   FROM games WHERE black_player_id=? OR white_player_id=? ORDER BY played_at`,
		playerID, playerID)
}

// GameByID fetches a single game by its primary key.
func (s *Store) GameByID(ctx context.Context, id int64) (*Game, error) {
	games, err := s.queryGames(ctx,
		`SELECT id,group_id,black_player_id,white_player_id,board_size,handicap,komi,winner,
		        black_gor_before,white_gor_before,black_gor_after,white_gor_after,played_at
		   FROM games WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	if len(games) == 0 {
		return nil, sql.ErrNoRows
	}
	return &games[0], nil
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
		if err := rows.Scan(&g.ID, &g.GroupID, &g.BlackPlayerID, &g.WhitePlayerID, &bs,
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

// ---- OAuth (Authorization Server state) ----------------------------------

type OAuthClient struct {
	ClientID     string
	ClientName   string
	RedirectURIs []string
	CreatedAt    time.Time
}

func (s *Store) CreateOAuthClient(ctx context.Context, clientID, name string, redirectURIs []string) error {
	raw, err := json.Marshal(redirectURIs)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx,
		`INSERT INTO oauth_clients(client_id,client_name,redirect_uris) VALUES(?,?,?)`,
		clientID, name, string(raw))
	return err
}

func (s *Store) OAuthClient(ctx context.Context, clientID string) (*OAuthClient, error) {
	c := &OAuthClient{}
	var uris, created string
	err := s.DB.QueryRowContext(ctx,
		`SELECT client_id,client_name,redirect_uris,created_at FROM oauth_clients WHERE client_id=?`,
		clientID).Scan(&c.ClientID, &c.ClientName, &uris, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(uris), &c.RedirectURIs); err != nil {
		return nil, fmt.Errorf("bad redirect_uris: %w", err)
	}
	c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return c, nil
}

// SaveAuthCode persists a single-use authorization code with a PKCE
// challenge. expires_at is RFC 3339.
func (s *Store) SaveAuthCode(ctx context.Context, code, clientID string, userID int64, redirectURI, codeChallenge string, expiresAt time.Time) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO oauth_codes(code,client_id,user_id,redirect_uri,code_challenge,expires_at)
		 VALUES(?,?,?,?,?,?)`,
		code, clientID, userID, redirectURI, codeChallenge,
		expiresAt.UTC().Format("2006-01-02 15:04:05"))
	return err
}

type AuthCode struct {
	Code          string
	ClientID      string
	UserID        int64
	RedirectURI   string
	CodeChallenge string
	ExpiresAt     time.Time
}

// ConsumeAuthCode atomically marks a code as used and returns it. If
// the code is missing, already consumed, or expired, returns ErrNotFound.
func (s *Store) ConsumeAuthCode(ctx context.Context, code string) (*AuthCode, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ac := &AuthCode{}
	var expires string
	var consumed int
	err = tx.QueryRowContext(ctx,
		`SELECT code,client_id,user_id,redirect_uri,code_challenge,expires_at,consumed
		   FROM oauth_codes WHERE code=?`, code).
		Scan(&ac.Code, &ac.ClientID, &ac.UserID, &ac.RedirectURI, &ac.CodeChallenge, &expires, &consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if consumed != 0 {
		return nil, ErrNotFound
	}
	ac.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05", expires)
	if time.Now().UTC().After(ac.ExpiresAt) {
		return nil, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE oauth_codes SET consumed=1 WHERE code=?`, code); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ac, nil
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
