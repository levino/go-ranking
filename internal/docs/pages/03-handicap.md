# Vorgabe und Komi

Go-Spiele zwischen unterschiedlich starken Spielern brauchen einen Ausgleich. Go nutzt dafür **Vorgabesteine** und **Komi**. Go-Liga rechnet beides mit dem **OGS-Vorgabemodell** — derselben quelloffenen Formel, die auch das Rating verwendet (`get_handicap_rank_difference` aus [online-go/goratings](https://github.com/online-go/goratings), `analysis/util/RatingMath.py`, MIT).

## Komi — kurz erklärt

Komi sind Punkte, die **Weiß** am Spielende dazubekommt. Positives Komi ist der Standard; negatives Komi heißt **Rückkomi** (Weiß werden Punkte abgezogen — ein Ausgleich zugunsten von Schwarz). Der halbe Punkt (`,5`) verhindert Unentschieden.

## Vorgabesteine

Vor dem Spiel platziert Schwarz $N$ Vorgabesteine, danach zieht Weiß zuerst.

**Ein einzelner Vorgabestein zählt nicht.** Das OGS-Modell setzt `num_extra_moves = Steine − 1` (und nie unter 0): Eine 1-Stein-Vorgabe ist rechnerisch identisch mit 0 Steinen — nur das Komi unterscheidet sich. Echte platzierte Vorgabe greift erst ab 2 Steinen. Das entspricht der Go-Konvention; kleine Abstände werden über Komi ausgeglichen.

## Die OGS-Formel

Go-Liga spielt mit **japanischen Regeln** (Gebietswertung). Schwarz' Vorsprung in Punkten ist

$$
\text{Vorsprung} = 6 - \text{Komi} + 12 \cdot (\text{Steine}-1)_{\ge 0}
$$

mit perfektem Komi 6 und einem Steinwert von 12 Punkten. Geteilt durch 12 und multipliziert mit einem **Brettgrößen-Faktor** ergibt das die Rangdifferenz, die die Vorgabe abdeckt:

| Brett | Faktor | Bedeutung |
|---|---|---|
| 9×9   | 6 | ein Stein ≈ 6 Ränge |
| 13×13 | 3 | ein Stein ≈ 3 Ränge |
| 19×19 | 1 | ein Stein ≈ 1 Rang |

Dadurch ist ein Stein auf 9×9 sechsmal so viel wert wie auf 19×19 — genau deshalb ist eine Stein-Tabelle vom großen Brett auf 9×9 unbrauchbar.

## Wie Vorgabe ins Rating einfließt

Anders als beim alten EGF-System fließt **Komi voll mit ein** — nicht nur die Steine. Für die Rating-Berechnung wird der Gegner um die berechnete Rangdifferenz verschoben (`get_handicap_adjustment`): Bei einer fairen Vorgabe sieht das System die Partie als ausgeglichen, und ein Sieg des schwächeren Spielers wird entsprechend gewertet — kein Scheinsieg mehr. Rückkomi und nicht-Standard-Komi sind dabei sauber abgedeckt.

## Die Empfehlung

Der Wizard empfiehlt eine Vorgabe, indem er **dieselbe OGS-Formel umkehrt**: Er sucht die Kombination aus Steinen und Komi, deren Rangdifferenz die Stärkelücke der beiden Spieler trifft.

- Kleine Abstände werden **allein über Komi** ausgeglichen (bis hinunter zu Rückkomi −6,5).
- Ein Stein kommt erst dazu, wenn Komi den spielbaren Bereich verlassen würde.

Auf **9×9** trägt Komi damit rund die ersten sechs Ränge — Vorgabesteine erscheinen erst bei großen Lücken. Auf **19×19** liegt praktisch ein Stein pro Rang an, Komi feintunt nur. Das ist kein eigenes System, sondern die OGS-Bewertungsformel rückwärts gelesen; Empfehlung und Rating stammen aus *einer* Quelle.

## Im Tablet-Wizard

1. **Vorgabe berechnen** — Spieler 1 → Spieler 2 → Brett → fertige Vorgabe.
2. **Spiel eintragen** — derselbe Weg, plus Ergebnis.

Vorgabe und Komi lassen sich vor dem Eintragen noch ändern, falls während der Partie improvisiert wurde — das Rating wertet immer das *tatsächlich gespielte* Setup.
