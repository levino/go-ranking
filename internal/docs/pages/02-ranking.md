# Spielstärke (GoR)

Go-Liga verwendet das **EGF-GoR-System** der European Go Federation. Es ist das offizielle Rating der europäischen Go-Verbände und für Schul-AGs eine gute Wahl: stabil, robust gegen Ausreißer, gut dokumentiert.

Quelle und Spezifikation: [europeangodatabase.eu/EGD/EGF_rating_system.php](https://www.europeangodatabase.eu/EGD/EGF_rating_system.php).

## Was ist die GoR?

Die **GoR (Go Rating)** ist eine Zahl, die ungefähr die Spielstärke ausdrückt. Höhere Werte sind stärker. Faustregel:

- 100 ≈ Anfänger (30 Kyu)
- 1000 ≈ 11 Kyu
- 2050 ≈ 1 Dan
- 3000 ≈ 10 Dan (theoretischer Wert)

Wir rechnen intern in dieser Skala — sie hat keine Lücken, ist linear, und damit gut geeignet für Statistik.

## Kyu/Dan ↔ GoR

Die EGF-Konvention nutzt 100-Punkt-Schritte pro Rang:

```
1 Dan      = 2050
1 Kyu      = 1950
5 Kyu      = 1550
10 Kyu     = 1050
15 Kyu     =  550
20 Kyu     =   50
```

Wenn die Trainerin einen neuen Spieler mit "20k" anlegt, übersetzt Go-Liga das in eine Start-GoR von 50. Das ist nur eine Schätzung — nach den ersten Partien justiert sich der Wert.

## Wie wird die GoR nach einer Partie angepasst?

Die zentrale Idee: jeder Spieler hat eine "erwartete Gewinn­wahrscheinlichkeit" basierend auf der GoR-Differenz. Wer mehr gewinnt als erwartet, steigt; wer weniger gewinnt als erwartet, fällt.

Konkret:

```
exp_score = 1 / (1 + 10^((opp_gor - own_gor + bonus) / 200))
new_gor   = own_gor + K * (actual_score - exp_score)
```

Dabei ist:

- `actual_score` = 1 bei Sieg, 0 bei Niederlage.
- `opp_gor` = GoR des Gegners.
- `bonus` = Vorgabe­bonus (in GoR-Punkten ausgedrückt). Wenn ich mit Vorgabe spiele, wird mein effektiver Gegner schwächer berechnet.
- `K` = Volatilitäts­faktor. EGF skaliert K mit der eigenen Stärke: schwache Spieler bewegen sich schneller (K bis 116), Dan-Spieler langsamer (K ab 10). Genaue Tabelle in `internal/rating/egf.go`.

Beide Spieler werden in derselben Partie angepasst — der eine gewinnt das, was der andere verliert, allerdings mit unterschiedlichen K-Faktoren.

## Warum gerade dieses System?

- **Symmetrisch**: Vorgabe-Spiele werden fair gerechnet, weil der Bonus auf die Erwartung wirkt, nicht auf den absoluten Wert.
- **Lokal anpassbar**: K-Faktor sorgt dafür, dass Anfänger nicht ewig im Rauschen hängen, sondern schnell ihre Niveau-Stufe finden.
- **Bekannt**: Wenn ein Kind auch im Verein spielt, läuft dort dasselbe System. Die Werte sind übertragbar.
