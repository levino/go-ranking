CREATE TABLE IF NOT EXISTS groups (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    oidc_subject  TEXT NOT NULL UNIQUE,
    email         TEXT,
    name          TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

CREATE TABLE IF NOT EXISTS group_admins (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id    INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, group_id)
);
CREATE INDEX IF NOT EXISTS idx_group_admins_group ON group_admins(group_id);

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

CREATE TABLE IF NOT EXISTS oauth_clients (
    client_id     TEXT PRIMARY KEY,
    client_name   TEXT,
    redirect_uris TEXT NOT NULL, -- JSON array of strings
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS oauth_codes (
    code           TEXT PRIMARY KEY,
    client_id      TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    redirect_uri   TEXT NOT NULL,
    code_challenge TEXT NOT NULL,
    expires_at     TEXT NOT NULL,
    consumed       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_oauth_codes_expires ON oauth_codes(expires_at);

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
