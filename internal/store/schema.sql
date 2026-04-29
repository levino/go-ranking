CREATE TABLE IF NOT EXISTS groups (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS players (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id    INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    gor         REAL NOT NULL DEFAULT 100,
    active      INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_players_group ON players(group_id);

CREATE TABLE IF NOT EXISTS sessions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id    INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    passphrase  TEXT NOT NULL UNIQUE,
    snapshot    TEXT NOT NULL, -- JSON array of {player_id, name, gor, number}
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sessions_group ON sessions(group_id);

CREATE TABLE IF NOT EXISTS games (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id      INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    black_player_id INTEGER NOT NULL REFERENCES players(id),
    white_player_id INTEGER NOT NULL REFERENCES players(id),
    board_size      INTEGER NOT NULL,
    handicap        INTEGER NOT NULL,
    komi            REAL NOT NULL,
    winner          TEXT NOT NULL CHECK (winner IN ('black','white')),
    black_gor_before REAL NOT NULL,
    white_gor_before REAL NOT NULL,
    black_gor_after  REAL NOT NULL,
    white_gor_after  REAL NOT NULL,
    played_at       TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_games_session ON games(session_id);
CREATE INDEX IF NOT EXISTS idx_games_black   ON games(black_player_id);
CREATE INDEX IF NOT EXISTS idx_games_white   ON games(white_player_id);

CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    group_id      INTEGER REFERENCES groups(id) ON DELETE CASCADE,
    is_admin      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
