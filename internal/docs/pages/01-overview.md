# Überblick

Go-Liga ist eine kleine Management-App für Schul-Go-AGs. Sie hält Spielerinnen und Spieler einer Gruppe zusammen mit ihrer aktuellen Spielstärke (GoR) fest und schreibt die Ergebnisse der Partien fort, die in der wöchentlichen AG gespielt werden. Die Kinder tippen ihre Spiele live auf einem Tablet ein; die Trainerin verwaltet das Drumherum über die MCP-Schnittstelle aus einem KI-Chat heraus.

## Wer ist beteiligt?

- **Spieler** sind reine Namens­einträge mit aktueller GoR. Sie haben keinen Login und keine Sitzung — es sind nur Datensätze, denen Partien zugeordnet werden.
- **Admins** sind Menschen mit Konto bei [id.levinkeller.de](https://id.levinkeller.de). Sie können eine oder mehrere Gruppen administrieren, weitere Admins einladen und über das MCP-Tool sämtliche Verwaltungsaufgaben erledigen.
- **MCP-Clients** sind KI-Chats wie [Claude.ai](https://claude.ai). Sie sprechen das MCP-Protokoll, authentifizieren sich beim ersten Verbinden gegen unsere App, und können dann im Namen der angemeldeten Admins arbeiten.

## Drei Schnittstellen

- **Tablet-UI** unter `/g/{slug}/play` — bunte Kacheln, mehrstufiger Wizard, Vorgabe-Rechner und Spiel-Eintrag. Für die Kinder gedacht; iOS Guided Access lockt die App.
- **Admin-Übersicht** unter `/` und `/g/{slug}` — Rangliste, Spieler-Liste, Admin-Verwaltung. Read-only Anzeige; alle Änderungen laufen über MCP.
- **MCP-Endpoint** unter `/mcp` — JSON-RPC 2.0 mit OAuth-Authentifizierung. Hier hängt sich Claude.ai dran.

## Wie geht es weiter?

- **[Spielstärke](/docs/ranking)** — wie der GoR-Wert funktioniert und wie wir ihn fortschreiben (mit Formeln und Quellen).
- **[Vorgabe & Komi](/docs/handicap)** — wie wir aus der Spielstärken­differenz eine Steine-/Komi-Empfehlung machen.
- **[MCP-Server](/docs/mcp)** — wie der Admin-Chat angeschlossen wird und welche Tools es gibt.
- **[Tablet-Workflow](/docs/tablet)** — wie der Wizard durchgespielt wird.
- **[Authentifizierung](/docs/auth)** — wie OIDC und OAuth zusammenspielen.
- **[Deployment](/docs/deploy)** — wie der Code in den Cluster kommt.
