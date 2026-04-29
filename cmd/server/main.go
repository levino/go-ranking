// go-liga is the Go-Liga management server.
//
// It bundles the web UI and the MCP API in a single binary backed by
// a SQLite database. Configuration is via environment variables:
//
//	GO_LIGA_DB         Path to the SQLite database (default: go-liga.db)
//	GO_LIGA_LISTEN     HTTP listen address (default: :8080)
//	GO_LIGA_SIGNING_KEY  HMAC key for session cookies (required, >= 32 bytes)
//	GO_LIGA_MCP_TOKEN  Bearer token gating /mcp (optional, recommended)
//	GO_LIGA_MCP_GROUP  Default group slug exposed via MCP (optional)
//	GO_LIGA_BOOTSTRAP_USER, GO_LIGA_BOOTSTRAP_PASSWORD
//	                   If set on first run (no users yet), create an admin.
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
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
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
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

	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer st.Close()

	if err := bootstrapAdmin(st); err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}

	svc := service.New(st)
	signer := auth.NewSigner(key)

	webSrv, err := web.New(svc, signer)
	if err != nil {
		return fmt.Errorf("web init: %w", err)
	}
	mcpSrv := &mcp.Server{
		Service:          svc,
		AuthToken:        os.Getenv("GO_LIGA_MCP_TOKEN"),
		DefaultGroupSlug: os.Getenv("GO_LIGA_MCP_GROUP"),
	}

	// Compose root mux: /mcp -> mcpSrv, everything else -> webSrv.
	root := http.NewServeMux()
	root.Handle("/mcp", mcpSrv.Handler())
	root.Handle("/mcp/", mcpSrv.Handler())
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

	log.Printf("go-liga listening on %s", listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	<-idle
	return nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func bootstrapAdmin(st *store.Store) error {
	user := os.Getenv("GO_LIGA_BOOTSTRAP_USER")
	pass := os.Getenv("GO_LIGA_BOOTSTRAP_PASSWORD")
	if user == "" || pass == "" {
		return nil
	}
	ctx := context.Background()
	any, err := st.HasAnyUsers(ctx)
	if err != nil {
		return err
	}
	if any {
		return nil
	}
	hash, err := auth.HashPassword(pass)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(ctx, user, hash, nil, true); err != nil {
		return err
	}
	log.Printf("bootstrapped admin user %q", user)
	return nil
}

func logger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Hide bearer tokens etc. from logs.
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
