// go-liga is the Go-Liga management server.
//
// It bundles the web UI and the MCP API in a single binary backed by
// a SQLite database. Configuration is via environment variables:
//
//	GO_LIGA_DB                   Path to the SQLite database (default: go-liga.db)
//	GO_LIGA_LISTEN               HTTP listen address (default: :8080)
//	GO_LIGA_SIGNING_KEY          HMAC key for session cookies (required, >= 32 bytes hex)
//	GO_LIGA_OIDC_ISSUER          OIDC issuer URL (e.g. https://id.levinkeller.de)
//	GO_LIGA_OIDC_CLIENT_ID       OIDC client id (web app) from Zitadel
//	GO_LIGA_OIDC_CLIENT_SECRET   OIDC client secret (web app) from Zitadel
//	GO_LIGA_OIDC_REDIRECT_URL    e.g. https://ranking.go-ag.levinkeller.de/auth/callback
//
// With no arguments the binary starts the server. The single subcommand
//
//	go-liga recompute
//
// replays every group's game history through the rating engine and
// rewrites the stored ratings — run it on demand (e.g. via kubectl exec)
// after a rating-model change. Only GO_LIGA_DB is needed for it.
//
// The MCP endpoint is its own OAuth 2.1 authorization server (with
// Dynamic Client Registration) for MCP clients like Claude.ai. It
// rides on top of the OIDC session: the upstream IdP (Zitadel)
// authenticates the human, our AS issues short-lived JWTs scoped to
// the MCP resource.
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/levino/go-ranking/internal/auth"
	"github.com/levino/go-ranking/internal/mcp"
	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
	"github.com/levino/go-ranking/internal/web"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "recompute":
			if err := recompute(); err != nil {
				log.Fatalf("fatal: %v", err)
			}
			return
		default:
			log.Fatalf("unknown subcommand %q (known: recompute)", os.Args[1])
		}
	}
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

// recompute replays every group's full game history through the rating
// engine and rewrites the stored ratings. It is the manual replacement
// for the old run-once-at-startup repair: trigger it on demand with
//
//	kubectl -n go-liga exec deploy/go-liga -- /go-liga recompute
func recompute() error {
	st, err := store.Open(envOr("GO_LIGA_DB", "go-liga.db"))
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer st.Close()

	svc := service.New(st)
	ctx := context.Background()
	groups, err := svc.Store.ListGroups(ctx)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	for _, g := range groups {
		if err := svc.RecomputeGroup(ctx, g.ID); err != nil {
			return fmt.Errorf("recompute %s: %w", g.Slug, err)
		}
		log.Printf("recomputed group %q", g.Slug)
	}
	log.Printf("recompute done: %d group(s)", len(groups))
	return nil
}

func run() error {
	dbPath := envOr("GO_LIGA_DB", "go-liga.db")
	listen := envOr("GO_LIGA_LISTEN", ":8080")
	signingHex := os.Getenv("GO_LIGA_SIGNING_KEY")
	if signingHex == "" {
		return fmt.Errorf("GO_LIGA_SIGNING_KEY is required (>=32 bytes hex)")
	}
	key, err := hex.DecodeString(signingHex)
	if err != nil || len(key) < 32 {
		return fmt.Errorf("GO_LIGA_SIGNING_KEY must be a hex string of at least 32 bytes (64 hex chars)")
	}

	oidcCfg := auth.NewOIDC(
		os.Getenv("GO_LIGA_OIDC_ISSUER"),
		os.Getenv("GO_LIGA_OIDC_CLIENT_ID"),
		os.Getenv("GO_LIGA_OIDC_CLIENT_SECRET"),
		os.Getenv("GO_LIGA_OIDC_REDIRECT_URL"),
	)
	if oidcCfg.Issuer == "" || oidcCfg.ClientID == "" ||
		oidcCfg.ClientSecret == "" || oidcCfg.RedirectURL == "" {
		return fmt.Errorf("GO_LIGA_OIDC_{ISSUER,CLIENT_ID,CLIENT_SECRET,REDIRECT_URL} are all required")
	}

	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer st.Close()

	svc := service.New(st)
	signer := auth.NewSigner(key)

	webSrv, err := web.New(svc, signer, oidcCfg)
	if err != nil {
		return fmt.Errorf("web init: %w", err)
	}

	// MCP endpoint URL: derive from the OIDC redirect URL stem.
	mcpResource, err := deriveResourceURL(oidcCfg.RedirectURL)
	if err != nil {
		return fmt.Errorf("derive MCP resource URL: %w", err)
	}
	mcpSrv := &mcp.Server{
		Service:  svc,
		Signer:   signer,
		OIDC:     oidcCfg,
		Resource: mcpResource,
	}

	// Compose root mux. The MCP endpoint and all OAuth-facade endpoints
	// need CORS to be callable from a browser-based MCP client.
	root := http.NewServeMux()
	root.Handle("/mcp", mcp.CORS(mcpSrv.Handler()))
	root.Handle("/mcp/", mcp.CORS(mcpSrv.Handler()))
	root.Handle("/.well-known/oauth-protected-resource",
		mcp.CORS(http.HandlerFunc(mcpSrv.HandleProtectedResource)))
	root.Handle("/.well-known/oauth-authorization-server",
		mcp.CORS(http.HandlerFunc(mcpSrv.HandleAuthServerMetadata)))
	root.Handle("/oauth/register", mcp.CORS(http.HandlerFunc(mcpSrv.HandleRegister)))
	root.Handle("/oauth/authorize", mcp.CORS(http.HandlerFunc(mcpSrv.HandleAuthorize)))
	root.Handle("/oauth/token", mcp.CORS(http.HandlerFunc(mcpSrv.HandleToken)))
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	root.Handle("/", webSrv.Handler())

	srv := &http.Server{
		Addr:              listen,
		Handler:           logger(root),
		ReadHeaderTimeout: 10 * time.Second,
	}

	idle := make(chan struct{})
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		log.Println("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		close(idle)
	}()

	log.Printf("go-liga listening on %s (MCP at %s)", listen, mcpResource)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	<-idle
	return nil
}

// deriveResourceURL takes the configured OIDC redirect URL and returns
// the canonical /mcp endpoint URL on the same origin.
func deriveResourceURL(redirectURL string) (string, error) {
	u, err := url.Parse(redirectURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("cannot parse redirect URL %q", redirectURL)
	}
	return u.Scheme + "://" + u.Host + "/mcp", nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func logger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ua := r.Header.Get("User-Agent")
		if len(ua) > 50 {
			ua = ua[:50] + "..."
		}
		path := r.URL.Path
		h.ServeHTTP(w, r)
		log.Printf("%s %s %s [%s] %s",
			r.RemoteAddr, r.Method, path, time.Since(start), strings.TrimSpace(ua))
	})
}
