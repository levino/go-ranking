# Vorgabe und Komi

Go-Spiele zwischen unterschiedlich starken Spielern brauchen einen Ausgleich, damit sie für beide Seiten spannend bleiben. Go nutzt dafür zwei Mechaniken: **Vorgabesteine** und **Komi**.

## Komi — kurz erklärt

Komi sind Punkte, die **Weiß** am Spielende dazubekommt (für den Nachteil, nicht als Erster zu ziehen). Komi gilt *immer nur für Weiß* — Schwarz bekommt nie Komi.

- Positives Komi (Standard, z.B. 6,5) = Weiß bekommt Punkte hinzugezählt.
- Negatives Komi (sogenanntes **Rückkomi**) = Weiß werden Punkte abgezogen.

Der halbe Punkt (`,5`) verhindert Unentschieden. Die [EGF-Turnierregeln](https://www.eurogofed.org/egf/tourrules.htm) setzen 6,5 als Default für ebene Partien.

## Vorgabesteine

Vor dem Spiel platziert Schwarz $N$ Vorgabesteine auf den Sternpunkten (Hoshi). **Danach** zieht Weiß den ersten regulären Zug. Dadurch ist Schwarz zu Beginn schon mit $N$ Steinen auf dem Brett, hat aber den Initiativ-Vorteil abgegeben (Weiß zieht jetzt zuerst).

### Warum es keine 1-Stein-Vorgabe gibt

Eine *einzelne* Vorgabe wäre konzeptuell sinnlos: Schwarz würde 1 Stein platzieren, dann Weiß ziehen — das ist effektiv dieselbe Situation wie ein ebenes Spiel, bei dem Schwarz einfach als erster Zug auf einen Sternpunkt setzt. Die Logik der Vorgabe — *Schwarz baut eine Anfangs-Stellung auf, bevor Weiß überhaupt einsteigt* — funktioniert erst ab zwei Steinen.

Die [British Go Association](https://www.britgo.org/about/rating) formuliert das so:

> *If the difference in grades is only one then usually the weaker player just takes the Black stones and doesn't give komi.*

Anders gesagt: Bei einem Rang Unterschied gibt es **keine platzierte Vorgabe**, sondern eine *Komi-Anpassung* — der schwächere Spieler nimmt Schwarz, das Komi für Weiß sinkt von 6,5 auf 0,5. Auch [Wikipedia: Handicapping in Go](https://en.wikipedia.org/wiki/Handicapping_in_Go) bestätigt diese Konvention.

## Die Vorgabe-Tabellen

### 19×19 — volles Brett

Auf dem vollen Brett wird **alles** über Steine ausgeglichen, nicht über Rückkomi. Rückkomi greift erst, wenn die 9 Steine ausgeschöpft sind — die [BGA-Praxis](https://www.britgo.org/about/rating) sagt dazu: *„Typically very few games are played with more than 9 handicap stones."*

<table>
<thead><tr><th>GoR-Differenz</th><th>Vorgabe</th><th>Komi</th></tr></thead>
<tbody>
<tr><td>0–100</td><td>0 Steine</td><td>6,5 (ebene Partie)</td></tr>
<tr><td>100–200</td><td>0 Steine</td><td>0,5 (1-Rang-Konvention)</td></tr>
<tr><td>200–300</td><td>2 Steine</td><td>0,5</td></tr>
<tr><td>300–400</td><td>3 Steine</td><td>0,5</td></tr>
<tr><td>400–500</td><td>4 Steine</td><td>0,5</td></tr>
<tr><td>500–600</td><td>5 Steine</td><td>0,5</td></tr>
<tr><td>600–700</td><td>6 Steine</td><td>0,5</td></tr>
<tr><td>700–800</td><td>7 Steine</td><td>0,5</td></tr>
<tr><td>800–900</td><td>8 Steine</td><td>0,5</td></tr>
<tr><td>900–1000</td><td>9 Steine</td><td>0,5</td></tr>
<tr><td>≥ 1000</td><td>9 Steine</td><td><strong>−5,5 (Rückkomi)</strong></td></tr>
</tbody>
</table>

### 13×13 — mittel

Eine *offizielle* EGF-Tabelle für 13×13 gibt es nicht ([EGF General Tournament Rules](https://www.eurogofed.org/egf/tourrules.htm) überlassen das den jeweiligen Turnier-Konventionen). [Wikipedia: Handicapping in Go](https://en.wikipedia.org/wiki/Handicapping_in_Go) zitiert die etablierte Skalierung: 13×13 ≈ 2,5–3 Ränge pro Stein. Wir spendieren Rückkomi als Zwischenstufe vor der ersten echten Vorgabe.

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

Auf 9×9 ist ein Stein ungleich mehr wert: [Wikipedia](https://en.wikipedia.org/wiki/Handicapping_in_Go) nennt ≈ 6 Ränge pro Stein. Die Tabelle bleibt deshalb lange bei 0 Steinen und nutzt Rückkomi als Zwischenstufe. Auch das Maximum ist niedriger — mehr als 4 Vorgabesteine machen auf 9×9 wenig Sinn.

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

Vor dem Eintragen können Vorgabe und Komi noch geändert werden, falls ihr während der Partie improvisiert habt (z.B. weil Schwarz die Vorgabe doch nicht alle plazieren wollte).

## Rating-Bonus aus der Vorgabe

Wenn Schwarz $h$ Vorgabesteine bekommt, behandelt das EGF-System Schwarz für die Erwartungs­berechnung so, als wäre seine GoR um

$$
100 \cdot (h - 0{,}5)\ \text{GoR}
$$

höher (siehe [Calculator.java in barcicki/GorCalculator](https://github.com/barcicki/GorCalculator/blob/master/app/src/main/java/com/barcicki/gorcalculator/core/Calculator.java) sowie [skillratings::egf](https://docs.rs/skillratings/latest/src/skillratings/egf.rs.html)). Auf 19×19 ist also 1 Stein immer 100 GoR wert.

Für kleinere Bretter macht das EGF-System keine Aussage; Go-Liga verwendet für die *Rating-Berechnung* (nicht für die Empfehlung!) deshalb absichtlich konservative Per-Stein-Werte:

- 9×9: 50 GoR pro Stein
- 13×13: 70 GoR pro Stein
- 19×19: 100 GoR pro Stein (EGF-Konvention)

Diese Werte spiegeln *wie viel Vertrauen* wir in den Vorgabe-Ausgleich haben — auf kleinen Brettern entscheidet ein einziger übersehener Zug das ganze Spiel, der Stein-Vorsprung gibt also weniger statistische Sicherheit.

**Wichtig**: Komi (auch Rückkomi) fließt *nicht* in den Rating-Bonus ein — nur die platzierten Steine. Komi-Anpassungen sind reine Spielausgleich-Sache und ändern die GoR nicht.

Der Bonus wird außerdem nur angewendet, wenn Schwarz tatsächlich der schwächere Spieler ist. Spielt versehentlich der Stärkere Schwarz mit Vorgabe, wird der Bonus auf 0 gesetzt — sonst würde der Stärkere doppelt belohnt.
