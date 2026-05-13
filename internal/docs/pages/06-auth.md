# Authentifizierung

Go-Liga hat zwei getrennte Auth-Pfade, die aber auf dieselbe Identität zeigen.

## Web-Login (Admin)

Wer auf `/g/{slug}` oder `/g/{slug}/play` zugreifen will, muss eingeloggt sein. Der Flow ist Standard OpenID Connect mit Authorization Code:

1. Browser ruft `/` auf → keine Session → Redirect `/login` → Redirect `/auth/start`.
2. `/auth/start` erzeugt einen Zufalls-State (Cookie) und leitet zu `id.levinkeller.de/oauth/v2/authorize?…` weiter.
3. Bei Zitadel meldet sich der Mensch an.
4. Zitadel ruft `https://ranking.go-ag.levinkeller.de/auth/callback?code=…` auf.
5. Go-Liga prüft den State-Cookie, tauscht den Code an Zitadels Token-Endpoint gegen einen Access-Token, ruft userinfo auf und legt einen User-Eintrag (oder aktualisiert ihn) per `oidc_subject` an.
6. Eine HMAC-signierte Session wird als Cookie gesetzt (TTL 30 Tage).

Die OIDC-Anwendung in Zitadel heißt `go-liga-web`, ist als Confidential Client mit Client-ID + Secret konfiguriert und wird über Terraform in `server-config/tofu/zitadel/goliga.tf` verwaltet.

## MCP-Auth (Claude.ai etc.)

Der MCP-Endpoint ist eine **OAuth-2.1 Resource** mit eigenem, eingebautem Authorization Server. Das ist das Pattern, das die MCP-Spezifikation explizit empfiehlt für kleinere Deployments:

1. Claude.ai POSTet zu `/mcp` ohne Token → 401 mit `WWW-Authenticate: Bearer resource_metadata="…"`.
2. Claude.ai folgt der `resource_metadata`-URL → bekommt JSON mit dem Authorization-Server (= unsere App).
3. Claude.ai lädt `/.well-known/oauth-authorization-server` von uns → erfährt die Endpoints.
4. Claude.ai macht **Dynamic Client Registration** (RFC 7591) an unser `/oauth/register` → bekommt eine frische `client_id`.
5. Claude.ai öffnet im Browser `/oauth/authorize?client_id=…&redirect_uri=…&code_challenge=…`.
6. Wenn keine Web-Session existiert, bounce wir den Browser durch den OIDC-Flow gegen Zitadel (siehe oben) und kommen zurück.
7. Mit gültiger Session erzeugen wir einen einmaligen Auth-Code, redirecten zurück zu Claude.ai mit `?code=…`.
8. Claude.ai POSTet `/oauth/token` mit Code + PKCE-Verifier → wir verifizieren, erzeugen einen JWT (HS256, Audience = MCP-Resource-URL, Subject = User-ID) und antworten mit `{access_token, token_type, expires_in}`.
9. Folgende `/mcp`-Calls senden den JWT im Bearer-Header. Wir verifizieren lokal — kein Round-Trip zu Zitadel.

## Wieso eigener AS, kein Zitadel-Proxy?

Erster Versuch war: Claude.ai macht OAuth direkt gegen Zitadel. Problem: Zitadel exponiert kein `registration_endpoint`, Claude.ai braucht aber DCR. Lösung wäre eine pre-registrierte App, was den Setup-Overhead pro Client erhöht.

Mit eigenem AS:

- DCR funktioniert direkt — jeder MCP-Client registriert sich selbst.
- Die Web-Session ist die Single Source of Identity — wer den OIDC-Flow durchläuft, dessen Browser hat dann die Session und das `/oauth/authorize` weiß sofort, wer da ist.
- JWTs werden lokal validiert, kein Performance-Issue auf dem Hot-Path.

## Schlüssel-Management

Alle Auth-Geheimnisse leben in einem **SealedSecret** im k3s-Namespace:

- `GO_LIGA_SIGNING_KEY` — 32-Byte-Hex. Signiert HMAC sowohl die Web-Session-Cookies als auch die MCP-JWTs. Rotation invalidiert beide.
- `GO_LIGA_OIDC_CLIENT_ID` / `GO_LIGA_OIDC_CLIENT_SECRET` — Zitadel-App `go-liga-web`. Wird per `tofu output -raw` aus dem Zitadel-Tofu-State gezogen.

Details und Bootstrap-Befehle in `server-config/services/go-liga/README.md`.
