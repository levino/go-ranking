# Development

Technical reference for running, testing, and deploying go-liga.

The whole stack is one Go binary plus a SQLite file:

| Layer            | Endpoint                                   |
|------------------|--------------------------------------------|
| Web UI           | `/`, `/login`, `/g/{slug}/...`             |
| Per-session PDFs | `/g/{slug}/sessions/{pass}/matrix.pdf`     |
|                  | `/g/{slug}/sessions/{pass}/scorecard.pdf`  |
| MCP API          | `/mcp` (JSON-RPC 2.0; Accept SSE optional) |

## Local development

```bash
export GO_LIGA_SIGNING_KEY=$(head -c 32 /dev/urandom | xxd -p -c 64)
export GO_LIGA_OIDC_ISSUER=https://id.levinkeller.de
export GO_LIGA_OIDC_CLIENT_ID=...           # from Zitadel
export GO_LIGA_OIDC_CLIENT_SECRET=...        # from Zitadel
export GO_LIGA_OIDC_REDIRECT_URL=http://localhost:8080/auth/callback

go run ./cmd/server
```

Then open <http://localhost:8080/> and authenticate via id.levinkeller.de.

## Tests

```bash
go test ./... -race
```

End-to-end tests in `internal/e2e` spin up the full HTTP server against
a temporary SQLite database and exercise both the web UI and the MCP API.

## Layout

- `internal/rating` — EGF GoR formula, handicap tables, kyu/dan parsing.
- `internal/store` — SQLite-backed persistence; embeds the schema.
- `internal/service` — domain orchestration (sessions, recording games).
- `internal/auth` — argon2id password hashing, signed session cookies.
- `internal/passphrase` — adjective-noun session passphrase generator.
- `internal/pdfgen` — handicap matrix and score-card PDFs.
- `internal/web` — HTML UI (`html/template`), embedded templates.
- `internal/mcp` — MCP HTTP/SSE server (Claude.ai-compatible).
- `internal/e2e` — full-stack tests.
- `deploy/` — Kubernetes manifests for K3s.
- `.github/workflows/` — CI and CD pipelines.

## Deployment

The `deploy/` directory contains Kubernetes manifests for K3s.  After
filling in `deploy/secret.yaml` (signing key, MCP token, bootstrap
admin):

```bash
kubectl apply -k deploy/
```

`.github/workflows/deploy.yml` builds the container image, pushes it to
GHCR, and triggers `kubectl rollout` against the cluster using GitHub
OIDC for authentication (no static kubeconfig).

## Configuration

| Variable                       | Purpose                                     |
|--------------------------------|---------------------------------------------|
| `GO_LIGA_SIGNING_KEY`          | HMAC key for session cookies (32 bytes hex) |
| `GO_LIGA_DB`                   | SQLite path (default `go-liga.db`)          |
| `GO_LIGA_LISTEN`               | HTTP listen address (default `:8080`)       |
| `GO_LIGA_OIDC_ISSUER`          | OIDC issuer (e.g. `https://id.levinkeller.de`) |
| `GO_LIGA_OIDC_CLIENT_ID` / `_SECRET` | OIDC client credentials (web app) |
| `GO_LIGA_OIDC_REDIRECT_URL`    | Web app OAuth callback URL                  |
