# MCP server

The **MCP server** is Go-Liga's admin interface. Instead of building a traditional admin UI, admins talk to the app via an AI chat (Claude.ai, Cursor, etc.) that calls MCP tools on the admin's behalf.

## Connecting

In Claude.ai under **Settings → Connectors → Add custom connector**:

```
https://ranking.go-ag.levinkeller.de/mcp
```

Auth mode: OAuth (automatic). On first connection a browser window opens that leads to sign-in at `id.levinkeller.de` — after that the connector is authenticated and stays that way.

There are no API keys, no bearer tokens you have to type in anywhere. The authentication is code flow plus PKCE; details under [Authentication](/docs/auth).

## What can the server do?

| Tool | What for |
|---|---|
| `list_my_groups` | Which groups do I administer? |
| `create_group` | Create a new group (slug + name). The caller becomes admin. |
| `add_admin` | Make another person admin by email (they must have signed in once before). |
| `remove_admin` | Remove an admin. The last admin cannot remove themselves. |
| `add_player` | Create a player — name + optional starting rank. |
| `update_player` | Rename a player or set them active/inactive. |
| `list_players` | Current player list with GoR. |
| `ranking` | Sorted ranking. |
| `recommend_handicap` | For two players + board: recommendation of stones/komi and who plays Black. |
| `record_game` | Record a game. Handicap and komi are passed as values (komi may be negative). |
| `list_games` | Recent games with their ID, optionally filtered by player; shows whether a game can still be deleted. |
| `delete_game` | **Destructive.** Delete a mis-entered game — only with explicit confirmation, see below. |

## What happens per tool call?

1. Claude.ai sends a JSON-RPC request to `/mcp` with the current access token in the Authorization header.
2. The server validates the token (signature-based, locally — no round trips to Zitadel on the hot path).
3. From the token subject the associated admin is loaded.
4. The tool checks whether the admin has access to the referenced group.
5. For write operations the SQLite database is adjusted accordingly.
6. Response as plaintext (easily readable for the AI) back.

## Examples

Create a new group and add players right away:

```
create_group   slug="go-ag-hannover"   name="Go-AG Hannover"
add_player     group="go-ag-hannover"  name="Anna"  rank="10k"
add_player     group="go-ag-hannover"  name="Ben"   rank="15k"
```

Record a game after the fact:

```
record_game  group="go-ag-hannover"  black="Ben"  white="Anna"  board="13"  handicap="4"  winner="black"
```

On 9×9 with reverse komi instead of a handicap:

```
record_game  group="go-ag-hannover"  black="Anna"  white="Ben"  board="9"  handicap="0"  komi="-3.5"  winner="white"
```


## Deleting a game

`delete_game` is meant for correcting mistakes (wrong winner, wrong players, entered twice). It is annotated as destructive (`destructiveHint`) so the AI client asks before calling it, and it works in two steps:

1. A call **without** `confirm` changes nothing and returns a preview: the game and how the affected players' ratings would change.
2. Only a second call with `confirm=true` deletes — and only after the human has explicitly agreed to the preview.

Rules:

- **14-day window** from when the game was recorded — enough to fix a typo at the next weekly session. Older games are settled history; deleting one would silently shift every rating built on top of it since.
- **Tournament games** cannot be deleted here, or pairings and standings would no longer add up.
- **What deleting does:** the game is removed for good and the group's entire game history is replayed without it (in one transaction). Afterwards every rating — including those of players who only played later — is exactly as if the game had never been recorded. There is no undo: re-recording puts the game at the end of the history, not back in its old place.

```
list_games   group="go-ag-hannover"  player="Ben"
delete_game  group="go-ag-hannover"  game_id=42                  # preview
delete_game  group="go-ag-hannover"  game_id=42  confirm=true    # after consent
```

## When something goes wrong

- "you are not an admin of …" → You do not administer the group. Have an existing admin invite you via `add_admin` (you must have signed in at least once via the web UI beforehand, so that your user record exists).
- "no player named …" → The player name doesn't match exactly. Check the correct spelling with `list_players`.
- The token lasts 24 h, refresh runs transparently via the OAuth flow.
