# Tablet workflow

During the club session a tablet sits at the playing location. The coach signs in once before the session (via `id.levinkeller.de`); after that she locks the device onto the app with **iOS Guided Access**. The children can now record games themselves without being able to leave the app or change settings.

## The start page

`/g/{slug}/play` shows two large cards:

- **Calculate handicap** — when two children need to decide which handicap they want to play with.
- **Record game** — to capture the result after a game.

Below the two cards a small list of the most recently played games appears — as confirmation that the input is coming through.

## Calculate handicap (3 steps)

1. **Choose player 1.** Each child has its own color, always the same — Maja is always in the same color tile, no matter when she is called up. Tap.
2. **Choose player 2.** Player 1's tile is no longer in the list, so no one accidentally plays against themselves.
3. **Choose board.** 9×9, 13×13, 19×19 — as three large tiles with a real line preview.

After that comes the result page with the large stone symbols showing who plays Black and who plays White, and the recommendation in large print. **"Got it"** closes the wizard.

## Record game (4 steps + confirmation + result)

1. Player 1
2. Player 2
3. Board
4. **Adjust + winner.** Handicap and komi are pre-filled with the recommendation. ±-buttons (large touch areas) change the values step by step. Komi may be negative — the note at the bottom edge is a reminder. Then one of the children taps the large **"Black wins"** or **"White wins"** button.
5. **Confirm.** A dedicated page shows once more in large print: Black, White, board, handicap, komi, winner. **"Yes, record it"** writes the game to the database. **"Cancel"** discards it.
6. **Result.** A result page shows how many points both players gained or lost and whether their rank changed. From there a large button leads straight to the **ranking** — or right on to the next game entry.

## New player in the club?

The tablet app **cannot** create new players. This is by design: player management runs through MCP, so the children at the tablet don't accidentally mix up profiles. If someone joins spontaneously, the child tells the coach, who handles it in 5 seconds via Claude.ai (`add_player`).

## Add-to-home-screen

The app has a web app manifest. On the tablet via Safari → **Share → Add to Home Screen**. After that it appears as its own app with a Go-stone icon, without a URL bar.

## What the children CANNOT do

- Delete, add, or rename players
- Change groups
- Edit earlier games after the fact
- See other groups

If a correction is needed (e.g. an incorrectly recorded game), the coach does it afterwards via MCP — `update_player` for players; for games there is deliberately no update function in the current state (instead: record a second game that balances it out).
