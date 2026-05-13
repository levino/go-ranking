# Vorgabe und Komi

Go-Spiele zwischen unterschiedlich starken Spielern brauchen einen Ausgleich, damit sie für beide Seiten spannend bleiben. In Go-Liga wird das auf zwei Arten geregelt: **Vorgabe­steine** und **Komi**.

## Vorgabe­steine

Schwarz darf zu Beginn eine bestimmte Anzahl Steine auf den Sternpunkten platzieren, bevor Weiß seinen ersten regulären Zug macht. Je größer der Spielstärke­abstand, desto mehr Steine.

Die Empfehlung kommt aus einer Tabelle in `internal/rating/handicap.go`, die nach EGF-Konvention pro Brettgröße den Steine-Wert liefert. Hier die wichtigsten Schwellen:

<table>
<thead><tr><th>GoR-Differenz</th><th>9×9</th><th>13×13</th><th>19×19</th></tr></thead>
<tbody>
<tr><td>0–100</td><td>0 (ebenes Spiel)</td><td>0</td><td>0</td></tr>
<tr><td>100–200</td><td>0, Komi anpassen</td><td>0, Komi anpassen</td><td>1 Stein</td></tr>
<tr><td>200–300</td><td>1 Stein</td><td>1 Stein</td><td>2 Steine</td></tr>
<tr><td>300–500</td><td>2 Steine</td><td>2–3 Steine</td><td>3–4 Steine</td></tr>
<tr><td>500–800</td><td>3 Steine</td><td>3–5 Steine</td><td>5–7 Steine</td></tr>
<tr><td>900+</td><td>4 Steine</td><td>6 Steine</td><td>9 Steine</td></tr>
</tbody>
</table>

Auf 9×9 sind selten mehr als 4 Vorgabe­steine sinnvoll — das Brett ist zu klein für mehr. Auf 19×19 sind bis zu 9 möglich.

## Komi

Komi sind Punkte, die Weiß als Ausgleich für das Nicht-Anfangen am Ende der Partie auf sein Konto bekommen. In Go-Liga:

- **Ebene Partie** (keine Vorgabe): Komi = 6,5. Die halbe Punktzahl verhindert Unentschieden.
- **Mit Vorgabe**: Komi = 0,5. Die halbe Punktzahl bleibt aus dem gleichen Grund.

Auf dem Tablet-Wizard wird das automatisch vor­ausgefüllt; die Trainerin oder das Kind kann's vor dem Eintragen noch anpassen.

## Rückkomi (negatives Komi)

Auf kleinen Brettern, vor allem 9×9, gibt es oft Konstellationen, wo eine ganze Vorgabe­steine zu viel und keine Vorgabe zu wenig ist. Lösung: **negatives Komi** — Schwarz spielt normal, bekommt aber am Ende Punkte abgezogen. Effektiv eine "halbe" Vorgabe.

Go-Liga akzeptiert beliebige Komi-Werte (positiv wie negativ) beim Spiel­eintrag. Der **Rating-Bonus** wird allerdings unverändert nur aus der Steine-Zahl berechnet — Komi ist ein Spiel-Detail, kein Rating-Detail.

## Wie der Rating-Bonus aus der Vorgabe entsteht

Wenn Schwarz `h` Vorgabe­steine bekommt, wirkt das im Rating-System so, als wäre der Schwarz-Spieler um `(h − 0,5) × StoneValue(board)` GoR stärker, als er tatsächlich ist. `StoneValue` ist brett­abhängig:

- 9×9: ca. 65 GoR pro Stein
- 13×13: ca. 130 GoR pro Stein
- 19×19: ca. 200 GoR pro Stein

Das `−0,5` rechnet die "fehlende halbe Komi" mit ein. Die genaue Formel und Tabelle stehen in `rating.Handicap.HandicapBonus`.

**Wichtig**: Der Bonus wird nur angewendet, wenn Schwarz tatsächlich der schwächere Spieler ist. Spielt mal versehentlich der Stärkere Schwarz mit Vorgabe, wird der Bonus auf 0 gesetzt — sonst würde der Stärkere doppelt belohnt.
