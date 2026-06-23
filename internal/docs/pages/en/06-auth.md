# Authentication

Go-Liga has two separate auth paths, which both point to the same identity.

## Web login (admin)

Anyone who wants to access `/g/{slug}` or `/g/{slug}/play` must be signed in. The flow is standard OpenID Connect with Authorization Code:

1. Browser calls `/` → no session → redirect `/login` → redirect `/auth/start`.
2. `/auth/start` generates a random state (cookie) and forwards to `id.levinkeller.de/oauth/v2/authorize?…`.
3. The person signs in at Zitadel.
4. Zitadel calls `https://ranking.go-ag.levinkeller.de/auth/callback?code=…`.
5. Go-Liga checks the state cookie, exchanges the code at Zitadel's token endpoint for an access token, calls userinfo, and creates a user entry (or updates it) via `oidc_subject`.
6. An HMAC-signed session is set as a cookie (TTL 30 days).

The OIDC application in Zitadel is called `go-liga-web`, is configured as a confidential client with client ID + secret, and is managed via Terraform in `server-config/tofu/zitadel/goliga.tf`.

## MCP auth (Claude.ai etc.)

The MCP endpoint is an **OAuth 2.1 resource** with its own built-in authorization server. This is the pattern the MCP specification explicitly recommends for smaller deployments:

1. Claude.ai POSTs to `/mcp` without a token → 401 with `WWW-Authenticate: Bearer resource_metadata="…"`.
2. Claude.ai follows the `resource_metadata` URL → gets JSON with the authorization server (= our app).
3. Claude.ai loads `/.well-known/oauth-authorization-server` from us → learns the endpoints.
4. Claude.ai does **Dynamic Client Registration** (RFC 7591) at our `/oauth/register` → gets a fresh `client_id`.
5. Claude.ai opens `/oauth/authorize?client_id=…&redirect_uri=…&code_challenge=…` in the browser.
6. If no web session exists, we bounce the browser through the OIDC flow against Zitadel (see above) and come back.
7. With a valid session we generate a one-time auth code and redirect back to Claude.ai with `?code=…`.
8. Claude.ai POSTs `/oauth/token` with code + PKCE verifier → we verify, generate a JWT (HS256, audience = MCP resource URL, subject = user ID) and respond with `{access_token, token_type, expires_in}`.
9. Subsequent `/mcp` calls send the JWT in the Bearer header. We verify locally — no round trip to Zitadel.

## Why our own AS, not a Zitadel proxy?

The first attempt was: Claude.ai does OAuth directly against Zitadel. Problem: Zitadel does not expose a `registration_endpoint`, but Claude.ai needs DCR. The fix would be a pre-registered app, which increases the setup overhead per client.

With our own AS:

- DCR works directly — every MCP client registers itself.
- The web session is the single source of identity — whoever goes through the OIDC flow then has the session in their browser, and `/oauth/authorize` immediately knows who it is.
- JWTs are validated locally, no performance issue on the hot path.

## Key management

All auth secrets live in a **SealedSecret** in the k3s namespace:

- `GO_LIGA_SIGNING_KEY` — 32-byte hex. With HMAC it signs both the web session cookies and the MCP JWTs. Rotation invalidates both.
- `GO_LIGA_OIDC_CLIENT_ID` / `GO_LIGA_OIDC_CLIENT_SECRET` — Zitadel app `go-liga-web`. Pulled from the Zitadel Tofu state via `tofu output -raw`.

Details and bootstrap commands in `server-config/services/go-liga/README.md`.
