package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/levino/go-ranking/internal/rating"
)

// Tournament is a series of rounds over a fixed roster of players.
type Tournament struct {
	ID        int64
	GroupID   int64
	Name      string
	Format    string // "round_robin" | "mcmahon"
	Handicap  bool   // true → games carry the recommended Vorgabe
	BoardSize rating.BoardSize
	Rounds    int    // planned round count
	Status    string // "setup" | "running" | "finished"
	CreatedAt time.Time
}

// TournamentPlayer is one roster entry with its McMahon starting score.
type TournamentPlayer struct {
	TournamentID int64
	PlayerID     int64
	StartScore   float64
}

// Pairing is one board of one round. WhitePlayerID == 0 marks a bye for
// the black player. Winner is "" until played, then "black"/"white"/"bye".
// GameID links to the recorded rating game (0 if none / bye).
type Pairing struct {
	ID            int64
	TournamentID  int64
	RoundNo       int
	BoardNo       int
	BlackPlayerID int64
	WhitePlayerID int64 // 0 → bye
	Handicap      int
	Komi          float64
	Winner        string // "", "black", "white", "bye"
	GameID        int64  // 0 → none
}

// IsBye reports whether the pairing is a bye rather than a real game.
func (p Pairing) IsBye() bool { return p.WhitePlayerID == 0 }

// Played reports whether a result has been recorded for the pairing.
func (p Pairing) Played() bool { return p.Winner != "" }

// ---- Tournaments ---------------------------------------------------------

func (s *Store) CreateTournament(ctx context.Context, t Tournament) (*Tournament, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO tournaments(group_id,name,format,handicap,board_size,rounds,status)
		 VALUES(?,?,?,?,?,?,?)`,
		t.GroupID, t.Name, t.Format, boolToInt(t.Handicap), int(t.BoardSize), t.Rounds, statusOr(t.Status))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	t.ID = id
	if t.Status == "" {
		t.Status = "setup"
	}
	t.CreatedAt = time.Now()
	return &t, nil
}

func statusOr(s string) string {
	if s == "" {
		return "setup"
	}
	return s
}

const tournamentCols = `id,group_id,name,format,handicap,board_size,rounds,status,created_at`

func scanTournament(sc interface{ Scan(...any) error }) (*Tournament, error) {
	t := &Tournament{}
	var hcap, bs int
	var created string
	if err := sc.Scan(&t.ID, &t.GroupID, &t.Name, &t.Format, &hcap, &bs, &t.Rounds, &t.Status, &created); err != nil {
		return nil, err
	}
	t.Handicap = hcap != 0
	t.BoardSize = rating.BoardSize(bs)
	t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return t, nil
}

func (s *Store) TournamentByID(ctx context.Context, id int64) (*Tournament, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+tournamentCols+` FROM tournaments WHERE id=?`, id)
	t, err := scanTournament(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Store) ListTournaments(ctx context.Context, groupID int64) ([]Tournament, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+tournamentCols+` FROM tournaments WHERE group_id=? ORDER BY created_at DESC, id DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tournament
	for rows.Next() {
		t, err := scanTournament(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *Store) UpdateTournamentStatus(ctx context.Context, id int64, status string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE tournaments SET status=? WHERE id=?`, status, id)
	return err
}

func (s *Store) UpdateTournamentRounds(ctx context.Context, id int64, rounds int) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE tournaments SET rounds=? WHERE id=?`, rounds, id)
	return err
}

// ---- Roster --------------------------------------------------------------

// SetTournamentRoster replaces the roster in one transaction: it clears
// any existing entries and inserts the given players with their starting
// scores. Used when a tournament starts.
func (s *Store) SetTournamentRoster(ctx context.Context, tournamentID int64, players []TournamentPlayer) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM tournament_players WHERE tournament_id=?`, tournamentID); err != nil {
		return err
	}
	for _, p := range players {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tournament_players(tournament_id,player_id,start_score) VALUES(?,?,?)`,
			tournamentID, p.PlayerID, p.StartScore); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListTournamentPlayers(ctx context.Context, tournamentID int64) ([]TournamentPlayer, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT tournament_id,player_id,start_score FROM tournament_players WHERE tournament_id=?`,
		tournamentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TournamentPlayer
	for rows.Next() {
		var tp TournamentPlayer
		if err := rows.Scan(&tp.TournamentID, &tp.PlayerID, &tp.StartScore); err != nil {
			return nil, err
		}
		out = append(out, tp)
	}
	return out, rows.Err()
}

// ---- Pairings ------------------------------------------------------------

// InsertPairings adds a round's pairings in one transaction.
func (s *Store) InsertPairings(ctx context.Context, pairings []Pairing) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range pairings {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tournament_pairings(tournament_id,round_no,board_no,black_player_id,white_player_id,handicap,komi,winner)
			 VALUES(?,?,?,?,?,?,?,?)`,
			p.TournamentID, p.RoundNo, p.BoardNo, nullID(p.BlackPlayerID), nullID(p.WhitePlayerID),
			p.Handicap, p.Komi, nullWinner(p.Winner)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListPairings(ctx context.Context, tournamentID int64) ([]Pairing, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id,tournament_id,round_no,board_no,black_player_id,white_player_id,handicap,komi,winner,game_id
		   FROM tournament_pairings WHERE tournament_id=? ORDER BY round_no, board_no`, tournamentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPairings(rows)
}

func (s *Store) PairingByID(ctx context.Context, id int64) (*Pairing, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id,tournament_id,round_no,board_no,black_player_id,white_player_id,handicap,komi,winner,game_id
		   FROM tournament_pairings WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ps, err := scanPairings(rows)
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, ErrNotFound
	}
	return &ps[0], nil
}

func scanPairings(rows *sql.Rows) ([]Pairing, error) {
	var out []Pairing
	for rows.Next() {
		var p Pairing
		var black, white, game sql.NullInt64
		var winner sql.NullString
		if err := rows.Scan(&p.ID, &p.TournamentID, &p.RoundNo, &p.BoardNo,
			&black, &white, &p.Handicap, &p.Komi, &winner, &game); err != nil {
			return nil, err
		}
		p.BlackPlayerID = black.Int64
		p.WhitePlayerID = white.Int64
		p.GameID = game.Int64
		p.Winner = winner.String
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetPairingResult records a pairing's outcome and links the rating game.
func (s *Store) SetPairingResult(ctx context.Context, pairingID int64, winner string, gameID int64) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE tournament_pairings SET winner=?, game_id=? WHERE id=?`,
		winner, nullID(gameID), pairingID)
	return err
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nullWinner(w string) any {
	if w == "" {
		return nil
	}
	return w
}
