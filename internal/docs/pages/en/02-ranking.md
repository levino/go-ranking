# Strength (OGS rating)

Go-Liga uses the **rating system from [Online-Go.com](https://online-go.com) (OGS)** — the largest Go server in the world. The algorithm is open source ([online-go/goratings](https://github.com/online-go/goratings), MIT license) and ported here to Go one-to-one. At its core is **Glicko-2**.

## Why OGS and not EGF?

The EGF system of the European Go Federation does rate handicap games, but it is board-size-blind and only knows stones, no komi. That is exactly what makes it useless on 9×9. OGS computes handicap **board-size- and komi-aware** (see [Handicap and Komi](/docs/handicap)) and explicitly allows rating players on 9×9 alone — ideal for a children's club.

## The three Glicko-2 values

Instead of a single number, each player carries **three** values (Glicko-2 paper, Glickman):

- **Rating** ($r$) — the estimated strength.
- **Deviation** ($\mathrm{RD}$) — the uncertainty of this estimate. New players start with high deviation; it drops with every game.
- **Volatility** ($\sigma$) — how erratically the strength changes.

Internally Glicko-2 computes on a transformed scale ($\mu$, $\phi$); the display uses $r = 173{,}7178\,\mu + 1500$.

## The rank scale

Rating and rank are linked via the OGS "log" system (`a = 525`, `c = 23{,}15`):

$$
r = 525 \cdot e^{\,\text{rank}/23{,}15}
\qquad
\text{rank} = \ln(r/525)\cdot 23{,}15
$$

The running rank number follows the OGS convention: **Rank 0 = 30 Kyu**, Rank 29 = 1 Kyu, Rank 30 = 1 Dan. That is why the scale reaches all the way down to **30 Kyu** — Rank 0 is its natural floor. Just right for beginner children.

The profile display shows the rank with one decimal place plus a $\pm$ that expresses the deviation in rank steps — e.g. `11,0k ± 1,5`.

## The rating grid

As on OGS, each player has **not one, but several** ratings:

- **Overall** — a single Glicko-2 rating over *all* games.
- **9×9**, **13×13**, **19×19** — one separate Glicko-2 rating each, drawn only from the games on that board size.

Every game updates its board category *and* the overall rating. A child who only plays 9×9 still gets a full, meaningful ranking this way. If it later switches to 13×13, a second category grows without the overall rating starting from zero.

Following the OGS v5 rule, when updating a board category the player's *own* side is taken from that category, but the **opponent's value** comes from their overall rating.

## How a game changes the rating

Each game is exactly one "rating period" with one game. For each player the Glicko-2 update is computed:

- **Expected result** $E$ from the rating difference (the handicap shifts the opponent here, see [Handicap and Komi](/docs/handicap)).
- **New rating, new deviation, new volatility** following the Glicko-2 procedure (steps 3–8 of the Glickman paper).

A win against a stronger opponent raises the rating sharply, a loss against a weaker one lowers it; how strongly depends on both deviations.

## GoR snapshot per game

On every entry the *overall* rating of both players is frozen with the game, before and after. The player table only carries the *current* state.

## Recomputing ratings

When needed, the *complete* game history can be replayed chronologically through the engine — from each player's starting value (seed). The process is deterministic (same games → same result) and can be repeated as often as desired. Useful after a correction to old data or an intervention in the starting values.

It is triggered via the `recompute` command of the server binary — so in the cluster:

```
kubectl -n go-liga exec deploy/go-liga -- /go-liga recompute
```
