# Handicap and Komi

Go games between players of different strengths need a balancing mechanism. Go uses **handicap stones** and **komi** for this. Go-Liga computes both with the **OGS handicap model** — the same open-source formula the rating also uses (`get_handicap_rank_difference` from [online-go/goratings](https://github.com/online-go/goratings), `analysis/util/RatingMath.py`, MIT).

## Komi — briefly explained

Komi are points that **White** receives at the end of the game. Positive komi is the standard; negative komi is called **reverse komi** (White has points deducted — a balancing in Black's favor). The half point (`,5`) prevents draws.

## Handicap stones

Before the game Black places $N$ handicap stones, after which White moves first.

**A single handicap stone does not count.** The OGS model sets `num_extra_moves = stones − 1` (and never below 0): a 1-stone handicap is computationally identical to 0 stones — only the komi differs. A real placed handicap only takes effect from 2 stones on. This matches the Go convention; small gaps are balanced via komi.

## The OGS formula

Go-Liga plays with **Japanese rules** (territory scoring). Black's lead in points is

$$
\text{lead} = 6 - \text{komi} + 12 \cdot (\text{stones}-1)_{\ge 0}
$$

with a perfect komi of 6 and a stone value of 12 points. Divided by 12 and multiplied by a **board-size factor**, this gives the rank difference the handicap covers:

| Board | Factor | Meaning |
|---|---|---|
| 9×9   | 6 | one stone ≈ 6 ranks |
| 13×13 | 3 | one stone ≈ 3 ranks |
| 19×19 | 1 | one stone ≈ 1 rank |

This makes one stone on 9×9 worth six times as much as on 19×19 — which is exactly why a stone table from the big board is useless on 9×9.

## How handicap enters the rating

Unlike the old EGF system, **komi is fully included** — not just the stones. For the rating computation the opponent is shifted by the computed rank difference (`get_handicap_adjustment`): with a fair handicap the system sees the game as balanced, and a win by the weaker player is rated accordingly — no more bogus wins. Reverse komi and non-standard komi are cleanly covered in this.

## The recommendation

The wizard recommends a handicap by **reversing the same OGS formula**: it searches for the combination of stones and komi whose rank difference matches the strength gap between the two players.

- Small gaps are balanced **via komi alone** (down to reverse komi −6.5).
- A stone is only added once komi would leave the playable range.

On **9×9**, komi therefore carries roughly the first six ranks — handicap stones only appear for large gaps. On **19×19** there is practically one stone per rank, with komi only fine-tuning. This is not a separate system but the OGS rating formula read backwards; recommendation and rating come from *one* source.

## In the tablet wizard

1. **Calculate handicap** — player 1 → player 2 → board → finished handicap.
2. **Record game** — the same path, plus the result.

Handicap and komi can still be changed before recording, in case something was improvised during the game — the rating always evaluates the setup *actually played*.
