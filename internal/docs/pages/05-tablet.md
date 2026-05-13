# Tablet-Workflow

Während der AG steht ein Tablet am Spielort. Die Trainerin loggt sich einmal vor der Stunde ein (über `id.levinkeller.de`); danach lockt sie das Gerät mit **iOS Guided Access** auf die App. Die Kinder können jetzt selbst Spiele eintragen, ohne die App verlassen oder Sachen einrichten zu können.

## Die Startseite

`/g/{slug}/play` zeigt zwei große Karten:

- **Vorgabe berechnen** — wenn zwei Kinder sich entscheiden müssen, mit welcher Vorgabe sie spielen wollen.
- **Spiel eintragen** — nach einer Partie das Ergebnis festhalten.

Unter den beiden Karten erscheint eine kleine Liste der zuletzt gespielten Partien — als Bestätigung, dass die Eingabe ankommt.

## Vorgabe berechnen (3 Schritte)

1. **Spieler 1 wählen.** Jedes Kind hat eine eigene Farbe, immer die gleiche — Maja ist immer in derselben Farb-Kachel, egal wann sie aufgerufen wird. Tippen.
2. **Spieler 2 wählen.** Die Kachel von Spieler 1 ist nicht mehr in der Liste, damit niemand versehentlich gegen sich selbst antritt.
3. **Brett wählen.** 9×9, 13×13, 19×19 — als drei große Kacheln mit echter Linien-Vorschau.

Danach kommt die Ergebnis­seite mit den großen Stein-Symbolen, wer Schwarz und wer Weiß spielt, und der Empfehlung in groß. **"Alles klar"** schließt den Wizard.

## Spiel eintragen (4 Schritte + Bestätigung)

1. Spieler 1
2. Spieler 2
3. Brett
4. **Anpassen + Sieger.** Vorgabe und Komi sind mit der Empfehlung vor­ausgefüllt. ±-Buttons (große Touch-Flächen) verändern die Werte schritt­weise. Komi darf negativ sein — der Hinweis am unteren Rand erinnert daran. Dann tippt eines der Kinder den großen **"Schwarz gewinnt"**- oder **"Weiß gewinnt"**-Knopf.
5. **Bestätigen.** Eine eigene Seite zeigt nochmal in groß: Schwarz, Weiß, Brett, Vorgabe, Komi, Sieger. **"Ja, eintragen"** schreibt die Partie in die Datenbank und führt zurück zur Start­seite. **"Abbrechen"** verwirft.

## Neuer Spieler in der AG?

Die Tablet-App kann **keine** neuen Spieler anlegen. Das ist Absicht: Spieler­verwaltung läuft über MCP, damit die Kinder am Tablet nicht versehentlich Profile durcheinander­bringen. Wenn jemand spontan dazu­kommt, sagt das Kind der Trainerin Bescheid, die das in 5 Sekunden via Claude.ai (`add_player`) erledigt.

## Add-to-Home-Screen

Die App hat ein Web-App-Manifest. Auf dem Tablet via Safari → **Teilen → Zum Home-Bildschirm**. Danach erscheint sie als eigene App mit Go-Stein-Icon, ohne URL-Leiste.

## Was die Kinder NICHT können

- Spieler löschen, hinzufügen, umbenennen
- Gruppen ändern
- Frühere Partien nachträglich bearbeiten
- Andere Gruppen sehen

Wenn eine Korrektur nötig ist (z.B. falsch eingetragene Partie), macht das die Trainerin nachträglich via MCP — `update_player` für Spieler, für Partien ist im aktuellen Stand bewusst keine Update-Funktion drin (statt­dessen: zweite Partie eintragen, die das ausgleicht).
