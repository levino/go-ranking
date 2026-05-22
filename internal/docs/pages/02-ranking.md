# Spielstärke (OGS-Rating)

Go-Liga benutzt das **Rating-System von [Online-Go.com](https://online-go.com) (OGS)** — den größten Go-Server der Welt. Der Algorithmus ist quelloffen ([online-go/goratings](https://github.com/online-go/goratings), MIT-Lizenz) und hier 1:1 nach Go portiert. Kern ist **Glicko-2**.

## Warum OGS und nicht EGF?

Das EGF-System der European Go Federation wertet zwar Vorgabepartien, ist aber brettgrößen-blind und kennt nur Steine, kein Komi. Genau das ist auf 9×9 unbrauchbar. OGS rechnet Vorgabe **brettgrößen- und komi-bewusst** (siehe [Vorgabe und Komi](/docs/03-handicap)) und sieht ausdrücklich vor, dass man Spieler allein auf 9×9 bewertet — ideal für eine Kinder-AG.

## Die drei Glicko-2-Werte

Statt einer einzigen Zahl führt jeder Spieler **drei** Werte (Glicko-2-Paper, Glickman):

- **Rating** ($r$) — die geschätzte Spielstärke.
- **Deviation** ($\mathrm{RD}$) — die Unsicherheit dieser Schätzung. Neue Spieler starten mit hoher Deviation; sie sinkt mit jeder Partie.
- **Volatility** ($\sigma$) — wie sprunghaft sich die Stärke ändert.

Intern rechnet Glicko-2 auf einer transformierten Skala ($\mu$, $\phi$); die Anzeige nutzt $r = 173{,}7178\,\mu + 1500$.

## Die Rang-Skala

Rating und Rang hängen über das OGS-„log"-System zusammen (`a = 525`, `c = 23{,}15`):

$$
r = 525 \cdot e^{\,\text{Rang}/23{,}15}
\qquad
\text{Rang} = \ln(r/525)\cdot 23{,}15
$$

Die fortlaufende Rang-Nummer folgt der OGS-Konvention: **Rang 0 = 30 Kyu**, Rang 29 = 1 Kyu, Rang 30 = 1 Dan. Deshalb reicht die Skala bis **30 Kyu** hinunter — Rang 0 ist ihr natürlicher Boden. Für Anfänger-Kinder genau richtig.

Die Profilanzeige zeigt den Rang mit einer Nachkommastelle plus ein $\pm$, das die Deviation in Rangstufen ausdrückt — z.B. `11,0k ± 1,5`.

## Das Bewertungs-Raster

Wie auf OGS hat jeder Spieler **nicht eine, sondern mehrere** Bewertungen:

- **Gesamt** — eine Glicko-2-Bewertung über *alle* Partien.
- **9×9**, **13×13**, **19×19** — je eine eigene Glicko-2-Bewertung nur aus den Partien dieser Brettgröße.

Jede Partie aktualisiert ihre Brett-Kategorie *und* die Gesamt-Bewertung. Ein Kind, das nur 9×9 spielt, bekommt so trotzdem ein vollwertiges, aussagekräftiges Ranking. Wechselt es später auf 13×13, wächst eine zweite Kategorie heran, ohne dass das Gesamt-Rating bei null beginnt.

Nach der OGS-v5-Regel wird beim Aktualisieren einer Brett-Kategorie die *eigene* Seite aus dieser Kategorie genommen, der **Gegnerwert** aber aus dessen Gesamt-Bewertung.

## Wie eine Partie das Rating ändert

Pro Partie ist genau eine „Rating-Periode" mit einem Spiel. Für jeden Spieler wird die Glicko-2-Aktualisierung gerechnet:

- **Erwartetes Ergebnis** $E$ aus der Rating-Differenz (die Vorgabe verschiebt dabei den Gegner, siehe [Vorgabe und Komi](/docs/03-handicap)).
- **Neues Rating, neue Deviation, neue Volatility** nach dem Glicko-2-Verfahren (Schritt 3–8 des Glickman-Papers).

Sieg gegen einen stärkeren Gegner hebt das Rating stark, Niederlage gegen einen schwächeren senkt es; wie stark, hängt von beiden Deviations ab.

## GoR-Snapshot pro Partie

Bei jeder Eintragung wird die *Gesamt*-Bewertung beider Spieler vorher und nachher mit der Partie eingefroren. Die Spieler-Tabelle führt nur den *aktuellen* Stand.

## Bewertungen neu berechnen

Auf dem Gruppen-Dashboard gibt es **„Bewertungen neu berechnen"**. Das spielt die *komplette* Partie-Historie chronologisch erneut durch die Engine — von jedem Spieler-Startwert (Seed) an. Der Vorgang ist deterministisch (gleiche Partien → gleiches Ergebnis) und beliebig oft wiederholbar. Nützlich nach einer Korrektur an Altdaten oder zur Migration.
