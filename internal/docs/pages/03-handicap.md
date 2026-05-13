# Vorgabe und Komi

Go-Spiele zwischen unterschiedlich starken Spielern brauchen einen Ausgleich, damit sie für beide Seiten spannend bleiben. In Go-Liga wird das auf zwei Arten geregelt: **Vorgabe­steine** und **Komi**.

## Grundprinzipien

- **Schwarz** ist immer der schwächere Spieler — sonst gäbe es keinen Sinn für eine Vorgabe.
- **Komi** sind Punkte, die Weiß am Ende der Partie auf sein Konto bekommt (Ausgleich dafür, dass Weiß nicht anfängt). Standard ist 6,5.
- **Vorgabe­steine** sind Steine, die Schwarz vor dem ersten Zug auf den Sternpunkten platziert. Pro Stein bekommt Schwarz im Schnitt einen ordentlichen Vorteil.
- **Rück­komi** ist negatives Komi: Schwarz bekommt am Ende Punkte abgezogen. Damit lässt sich ein Vorteil unterhalb von "einem ganzen Vorgabe­stein" abbilden.

## Warum kein 1 Vorgabe­stein?

Ein **einzelner Vorgabe­stein** ist im Go praktisch sinnlos:

- Er ist *zu wenig*, um den Abstand zwischen zwei Spielern auszugleichen, sobald die Lücke groß genug ist, um überhaupt eine Vorgabe zu rechtfertigen.
- Er ist *zu viel*, um als "ebene Partie mit kleinem Ausgleich" durchzugehen.

Deshalb gibt der Vorgabe-Rechner **niemals 1 Stein** aus. Stattdessen werden kleine Spielstärke­unterschiede über Komi geregelt:

- Sehr klein (ein paar Steine Spielstärke­unterschied): **Ebenes Spiel, Komi 0,5** statt der vollen 6,5.
- Etwas größer (aber noch unter 2 Vorgabe­steinen sinnvoll): **Rückkomi**, also negatives Komi (typisch −5,5). Schwarz spielt ohne Vorgabe­steine, bekommt aber am Ende Punkte abgezogen.
- Ab dem nächsten Sprung dann **2 Vorgabe­steine** und Komi 0,5.

So bleibt die Vorgabe immer ein sinnvoller Hebel: **0 Steine → 2 Steine → 3 Steine → …**, nie 1 Stein.

## Die Vorgabe-Tabelle

Die Empfehlung kommt aus einer Tabelle in `internal/rating/handicap.go`, die nach Brettgröße den passenden Wert liefert. Hier die Schwellen für die GoR-Differenz (Stärker minus Schwächer):

### 19×19 — volles Brett

<table>
<thead><tr><th>GoR-Differenz</th><th>Vorgabe</th><th>Komi</th></tr></thead>
<tbody>
<tr><td>0–100</td><td>0 Steine</td><td>6,5</td></tr>
<tr><td>100–200</td><td>0 Steine</td><td>0,5</td></tr>
<tr><td>200–300</td><td>0 Steine</td><td><strong>−5,5 (Rückkomi)</strong></td></tr>
<tr><td>300–400</td><td>2 Steine</td><td>0,5</td></tr>
<tr><td>400–500</td><td>3 Steine</td><td>0,5</td></tr>
<tr><td>500–600</td><td>4 Steine</td><td>0,5</td></tr>
<tr><td>600–700</td><td>5 Steine</td><td>0,5</td></tr>
<tr><td>700–800</td><td>6 Steine</td><td>0,5</td></tr>
<tr><td>800–900</td><td>7 Steine</td><td>0,5</td></tr>
<tr><td>900–1000</td><td>8 Steine</td><td>0,5</td></tr>
<tr><td>≥ 1000</td><td>9 Steine</td><td>0,5</td></tr>
</tbody>
</table>

### 13×13 — mittel

<table>
<thead><tr><th>GoR-Differenz</th><th>Vorgabe</th><th>Komi</th></tr></thead>
<tbody>
<tr><td>0–100</td><td>0 Steine</td><td>6,5</td></tr>
<tr><td>100–200</td><td>0 Steine</td><td>0,5</td></tr>
<tr><td>200–300</td><td>0 Steine</td><td><strong>−5,5 (Rückkomi)</strong></td></tr>
<tr><td>300–450</td><td>2 Steine</td><td>0,5</td></tr>
<tr><td>450–600</td><td>3 Steine</td><td>0,5</td></tr>
<tr><td>600–800</td><td>4 Steine</td><td>0,5</td></tr>
<tr><td>800–1000</td><td>5 Steine</td><td>0,5</td></tr>
<tr><td>≥ 1000</td><td>6 Steine</td><td>0,5</td></tr>
</tbody>
</table>

### 9×9 — schnelle Partie

Auf 9×9 sind mehr als 4 Vorgabe­steine nicht sinnvoll — das Brett ist zu klein. Bei riesigen Lücken wird ab da nur noch das Rück­komi verschärft.

<table>
<thead><tr><th>GoR-Differenz</th><th>Vorgabe</th><th>Komi</th></tr></thead>
<tbody>
<tr><td>0–70</td><td>0 Steine</td><td>6,5</td></tr>
<tr><td>70–140</td><td>0 Steine</td><td>0,5</td></tr>
<tr><td>140–210</td><td>0 Steine</td><td><strong>−5,5 (Rückkomi)</strong></td></tr>
<tr><td>210–280</td><td>2 Steine</td><td>0,5</td></tr>
<tr><td>280–350</td><td>3 Steine</td><td>0,5</td></tr>
<tr><td>350–420</td><td>4 Steine</td><td>0,5</td></tr>
<tr><td>≥ 420</td><td>4 Steine</td><td><strong>−5,5 (Rückkomi)</strong></td></tr>
</tbody>
</table>

## Im Tablet-Wizard

Der Wizard kennt zwei Wege:

1. **Vorgabe berechnen** — Spieler 1 → Spieler 2 → Brett → fertige Vorgabe wird angezeigt.
2. **Spiel eintragen** — derselbe Weg, plus Ergebnis und Bestätigung.

Wenn du eine Vorgabe berechnest und am Ende auf **„Ergebnis eintragen"** tippst, springt der Wizard direkt zum Ergebnis-Schritt — Spieler, Brett, Vorgabe und Komi sind dann schon vor­ausgefüllt. So musst du die Auswahl nicht doppelt machen.

Vor dem Eintragen kannst du Vorgabe und Komi noch ändern, falls ihr während der Partie improvisiert habt.

## Rückkomi (negatives Komi)

Schwarz spielt normal ohne Vorgabe­steine, bekommt aber am Ende der Partie Punkte abgezogen. Effektiv eine "halbe" Vorgabe — perfekt für den Bereich, in dem ein voller Vorgabe­stein zu viel und keine Vorgabe zu wenig wäre.

Go-Liga akzeptiert beliebige Komi-Werte (positiv wie negativ) beim Spiel­eintrag. Der **Rating-Bonus** wird allerdings unverändert nur aus der Steine-Zahl berechnet — Komi ist ein Spiel-Detail, kein Rating-Detail.

## Wie der Rating-Bonus aus der Vorgabe entsteht

Wenn Schwarz `h` Vorgabe­steine bekommt, wirkt das im Rating-System so, als wäre der Schwarz-Spieler um `(h − 0,5) × StoneValue(board)` GoR stärker, als er tatsächlich ist. `StoneValue` ist brett­abhängig:

- 9×9: 50 GoR pro Stein
- 13×13: 70 GoR pro Stein
- 19×19: 100 GoR pro Stein

Das `−0,5` rechnet die "fehlende halbe Komi" mit ein. Die genaue Formel und Tabelle stehen in `rating.Handicap.HandicapBonus`.

**Wichtig**: Der Bonus wird nur angewendet, wenn Schwarz tatsächlich der schwächere Spieler ist. Spielt mal versehentlich der Stärkere Schwarz mit Vorgabe, wird der Bonus auf 0 gesetzt — sonst würde der Stärkere doppelt belohnt.
