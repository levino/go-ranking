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
    language      TEXT NOT NULL DEFAULT 'de',
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

-- A player carries the "overall" Glicko-2 rating (gor/deviation/
-- volatility). seed_rating is the strength estimate the player started
-- from — used to re-seed a full history recompute. Per-board-size
-- ratings live in player_ratings.
CREATE TABLE IF NOT EXISTS players (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id    INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    gor         REAL NOT NULL DEFAULT 1500,
    deviation   REAL NOT NULL DEFAULT 350,
    volatility  REAL NOT NULL DEFAULT 0.06,
    seed_rating REAL NOT NULL DEFAULT 1500,
    active      INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_players_group ON players(group_id);

-- The OGS "ratings grid": one Glicko-2 rating per board-size category.
-- The overall rating is kept on the players row; this table holds the
-- 9x9 / 13x13 / 19x19 categories.
CREATE TABLE IF NOT EXISTS player_ratings (
    player_id   INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    category    TEXT NOT NULL,
    rating      REAL NOT NULL,
    deviation   REAL NOT NULL,
    volatility  REAL NOT NULL,
    games       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (player_id, category)
);

CREATE TABLE IF NOT EXISTS oauth_clients (
    client_id     TEXT PRIMARY KEY,
    client_name   TEXT,
    redirect_uris TEXT NOT NULL,
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

-- black_gor_* / white_gor_* hold each player's overall Glicko-2 rating
-- before and after the game (a per-game snapshot).
CREATE TABLE IF NOT EXISTS games (
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
);
CREATE INDEX IF NOT EXISTS idx_games_group ON games(group_id);
CREATE INDEX IF NOT EXISTS idx_games_black ON games(black_player_id);
CREATE INDEX IF NOT EXISTS idx_games_white ON games(white_player_id);

-- A tournament groups a set of players into a series of rounds. format is
-- 'round_robin' or 'mcmahon'; handicap=1 means the games carry the
-- recommended Vorgabe (so weaker players can win), 0 means even games.
-- rounds is the planned round count (McMahon; round robin derives it).
-- status walks 'setup' → 'running' → 'finished'.
CREATE TABLE IF NOT EXISTS tournaments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id    INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    format      TEXT NOT NULL CHECK (format IN ('round_robin','mcmahon')),
    handicap    INTEGER NOT NULL DEFAULT 1,
    board_size  INTEGER NOT NULL DEFAULT 9,
    rounds      INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL CHECK (status IN ('setup','running','finished')) DEFAULT 'setup',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tournaments_group ON tournaments(group_id);

-- The roster of a tournament. start_score is the McMahon starting score
-- (0 for round robin, where only wins count).
CREATE TABLE IF NOT EXISTS tournament_players (
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    player_id     INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    start_score   REAL NOT NULL DEFAULT 0,
    PRIMARY KEY (tournament_id, player_id)
);

-- One board of one round. white_player_id NULL means a bye for the black
-- player (who then scores a win). winner is NULL until the result is in
-- ('black' | 'white' | 'bye'). game_id links to the rating game that was
-- recorded, so a result also moves the players' ratings.
CREATE TABLE IF NOT EXISTS tournament_pairings (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id   INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    round_no        INTEGER NOT NULL,
    board_no        INTEGER NOT NULL,
    black_player_id INTEGER REFERENCES players(id),
    white_player_id INTEGER REFERENCES players(id),
    handicap        INTEGER NOT NULL DEFAULT 0,
    komi            REAL NOT NULL DEFAULT 6.5,
    winner          TEXT CHECK (winner IN ('black','white','bye')),
    game_id         INTEGER REFERENCES games(id),
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_pairings_tournament ON tournament_pairings(tournament_id, round_no);
