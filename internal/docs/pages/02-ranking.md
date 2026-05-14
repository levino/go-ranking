# Spielstärke (GoR)

Go-Liga benutzt das **EGF-Rating-System** der [European Go Federation](https://www.europeangodatabase.eu/EGD/EGF_rating_system.php) — das offizielle Punktesystem aller europäischen Go-Verbände.

## Die Skala

Die EGF-GoR ist eine Zahl, die ungefähr die Spielstärke ausdrückt. Höhere Werte sind stärker. Pro Rangstufe liegen genau 100 GoR-Punkte. Verankert wird die Skala bei **1 Dan = 2100 GoR**:

| GoR | Rang |
|---|---|
| 2700+ | 1 Pro (≈ 7 Dan Amateur) |
| 2600 | 6 Dan |
| 2100 | 1 Dan |
| 2000 | 1 Kyu |
| 1500 | 6 Kyu |
| 1000 | 11 Kyu |
| 500 | 16 Kyu |
| 100 | 20 Kyu |

Quelle: [Wikipedia — Go ranks and ratings](https://en.wikipedia.org/wiki/Go_ranks_and_ratings), zitiert aus der EGF-Spec. Theoretisch geht die GoR bis -900 herunter (siehe [GorCalculator-Konstanten](https://github.com/barcicki/GorCalculator/blob/master/app/src/main/java/com/barcicki/gorcalculator/core/Calculator.java)), praktisch werden Werte unter 100 (= 20 Kyu) aber nicht differenziert — die EGF zeigt das als Untergrenze.

Wenn die Trainerin einen neuen Spieler mit Rang „15k" anlegt, übersetzt Go-Liga das in eine Start-GoR von 550. Das ist nur eine Schätzung — nach den ersten Partien justiert sich der Wert.

## Wie wird die GoR nach einer Partie angepasst?

Die EGF-Formel (Stand seit April 2021, [skillratings::egf](https://docs.rs/skillratings/latest/src/skillratings/egf.rs.html) bzw. [barcicki/GorCalculator/Calculator.java](https://github.com/barcicki/GorCalculator/blob/master/app/src/main/java/com/barcicki/gorcalculator/core/Calculator.java)) hat drei Komponenten.

**Erwartetes Ergebnis** für Spieler A gegen B:

$$
\mathrm{SE}(A, B) = \frac{1}{1 + \exp(\beta(R_B) - \beta(R_A))}
\quad\text{mit}\quad
\beta(R) = -7 \cdot \ln(3300 - R)
$$

Bei einer Vorgabe von $h$ Steinen für Schwarz wird vor der Rechnung Schwarz' GoR temporär um $100 \cdot (h - 0.5)$ erhöht — die EGF rechnet Vorgabe-Steine als Rating-Bonus.

**Volatilitäts­koeffizient** (auch *con* genannt):

$$
\mathrm{con}(R) = \left(\frac{3300 - R}{200}\right)^{1.6}
$$

`con` ist groß bei niedrigen Ratings (Anfänger bewegen sich schnell) und klein bei Dan-Spielern (deren GoR ist stabil).

**Anti-Deflations-Bonus**:

$$
\mathrm{bonus}(R) = \frac{\ln\bigl(1 + \exp\bigl(\tfrac{2300 - R}{80}\bigr)\bigr)}{5}
$$

Dieser kleine Bonus kompensiert die Tendenz des Systems, in Anfänger-Pools insgesamt Rating zu verlieren.

**Update-Formel**:

$$
R_\text{neu} = R + \mathrm{con}(R) \cdot \bigl(S - \mathrm{SE}\bigr) + \mathrm{bonus}(R)
$$

Wobei $S = 1$ bei Sieg, $S = 0$ bei Niederlage. Beide Spieler werden in derselben Partie aktualisiert, jeder mit seinem eigenen `con`.

## GoR-Snapshot pro Partie

**Wichtig**: Wenn eine Partie eingetragen wird, werden die GoR-Werte beider Spieler *zum Zeitpunkt der Eintragung* zusammen mit der Partie eingefroren (Felder `black_gor_before`, `black_gor_after`, `white_gor_before`, `white_gor_after` in der Datenbank). Das hat zwei Konsequenzen:

- Die Spieler-Tabelle führt nur die *aktuelle* GoR; in den Partien steht der historische Wert.
- Eine nachträgliche Änderung der Spieler-GoR (z.B. durch externe Kalibrierung, siehe unten) ändert frühere Partien *nicht*.

Eine vollständige Neuberechnung der Historie wäre möglich (chronologisch alle Partien mit neuen Startwerten durchrechnen), ist aktuell aber kein Knopf in der App.

## Externe Rating-Quellen abgleichen

Falls in Zukunft Spieler auch auf [OGS](https://online-go.com), [KGS](https://www.gokgs.com), oder einem offiziellen EGF-Turnier spielen, wäre es sinnvoll, deren externes Rating als Anker zu nutzen — gerade weil unser Pool klein ist und das interne Rating dadurch zu langsam justieren könnte.

Geplanter Ansatz (noch nicht implementiert): die Spieler-Tabelle bekommt optionale Felder `external_source` (z.B. `"ogs"`, `"egd"`) und `external_id`. Ein Sync-Job zieht regelmäßig deren aktuelle Werte; bei großer Abweichung kann die interne GoR sanft an den externen Wert herangezogen werden (kein harter Reset). Das ist aber Roadmap-Material, kein Code heute.
