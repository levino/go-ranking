# MCP-Server

Der **MCP-Server** ist die Admin-Schnittstelle von Go-Liga. Statt eine traditionelle Admin-UI zu bauen, sprechen Admins mit der App über einen KI-Chat (Claude.ai, Cursor, etc.), der MCP-Tools im Namen des Admins aufruft.

## Verbinden

In Claude.ai unter **Settings → Connectors → Add custom connector**:

```
https://ranking.go-ag.levinkeller.de/mcp
```

Auth-Modus: OAuth (automatisch). Beim ersten Verbinden öffnet sich ein Browser-Fenster, das zur Anmeldung an `id.levinkeller.de` führt — danach ist der Connector authentifiziert und bleibt es.

Es gibt keine API-Keys, keine Bearer-Tokens, die du irgendwo eintippen musst. Die Authentifizierung ist Code-Flow plus PKCE; Details unter [Authentifizierung](/docs/auth).

## Was kann der Server?

| Tool | Wozu |
|---|---|
| `list_my_groups` | Welche Gruppen administriere ich? |
| `create_group` | Neue Gruppe anlegen (Slug + Name). Der Aufrufer wird Admin. |
| `add_admin` | Weiteren Menschen per E-Mail zum Admin machen (muss sich vorher einmal eingeloggt haben). |
| `remove_admin` | Admin entfernen. Letzter Admin kann sich nicht selbst entfernen. |
| `add_player` | Spieler anlegen — Name + optional Start-Rang. |
| `update_player` | Spieler umbenennen oder aktiv/inaktiv schalten. |
| `list_players` | Aktuelle Spieler­liste mit GoR. |
| `ranking` | Sortierte Rangliste. |
| `recommend_handicap` | Für zwei Spieler + Brett: Empfehlung Steine/Komi und wer Schwarz spielt. |
| `record_game` | Partie eintragen. Vorgabe und Komi werden als Werte übergeben (Komi darf negativ sein). |

## Was passiert pro Tool-Call?

1. Claude.ai schickt einen JSON-RPC-Request an `/mcp` mit dem aktuellen Access-Token im Authorization-Header.
2. Der Server validiert das Token (signaturen-basiert, lokal — keine Round-Trips zu Zitadel auf dem Hot-Path).
3. Aus dem Token-Subject wird der zugeordnete Admin geladen.
4. Das Tool prüft, ob der Admin Zugriff auf die referenzierte Gruppe hat.
5. Bei Schreib­operationen wird die SQLite-Datenbank entsprechend angepasst.
6. Antwort als Plaintext (für die KI gut lesbar) zurück.

## Beispiele

Eine neue Gruppe anlegen und gleich Spieler dazu:

```
create_group   slug="go-ag-hannover"   name="Go-AG Hannover"
add_player     group="go-ag-hannover"  name="Anna"  rank="10k"
add_player     group="go-ag-hannover"  name="Ben"   rank="15k"
```

Eine Partie nachtragen:

```
record_game  group="go-ag-hannover"  black="Ben"  white="Anna"  board="13"  handicap="4"  winner="black"
```

Auf 9×9 mit Rückkomi statt Vorgabe:

```
record_game  group="go-ag-hannover"  black="Anna"  white="Ben"  board="9"  handicap="0"  komi="-3.5"  winner="white"
```

## Wenn etwas schief geht

- "you are not an admin of …" → Du administrierst die Gruppe nicht. Lass dich von einem bestehenden Admin per `add_admin` einladen (du musst dich vorher mindestens einmal über die Web-UI eingeloggt haben, damit dein User-Record existiert).
- "no player named …" → Spieler-Name passt nicht exakt. Mit `list_players` die korrekte Schreibweise prüfen.
- Token läuft 24 h, Refresh läuft transparent über den OAuth-Flow.
