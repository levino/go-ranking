package mcp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/levino/go-ranking/internal/auth"
)

// This file implements a self-contained OAuth 2.1 authorization server
// just big enough for MCP clients (Claude.ai et al.). The flow is:
//
//   1. Client POSTs to /oauth/register (RFC 7591 DCR). We persist the
//      redirect_uris and return a fresh client_id.
//   2. Browser hits /oauth/authorize. If the user has a session cookie
//      (from the existing OIDC web login), we generate an auth code
//      and 302 back to the client's redirect_uri. If not, we kick off
//      the OIDC login first and resume after.
//   3. Client POSTs to /oauth/token with the code + PKCE verifier. We
//      verify and return our own HS256 JWT.
//   4. Client calls /mcp with Authorization: Bearer <JWT>. We verify
//      the signature, issuer, audience, and expiry locally.
//
// User identity ultimately comes from id.levinkeller.de (the web login
// is OIDC). MCP just rides on top of that session.

// HandleAuthServerMetadata serves /.well-known/oauth-authorization-server.
func (s *Server) HandleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	base := s.publicBase()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                base,
		"authorization_endpoint":                base + "/oauth/authorize",
		"token_endpoint":                        base + "/oauth/token",
		"registration_endpoint":                 base + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"mcp"},
	})
}

// HandleRegister implements RFC 7591 Dynamic Client Registration.
// Each call persists a new client (no deduplication — Claude.ai may
// register fresh each session) and returns its credentials.
func (s *Server) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return
	}
	if len(req.RedirectURIs) == 0 {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "redirect_uris required")
		return
	}
	clientID := "c-" + randomID(18)
	if err := s.Service.Store.CreateOAuthClient(r.Context(), clientID, req.ClientName, req.RedirectURIs); err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	resp := map[string]any{
		"client_id":                  clientID,
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":                req.ClientName,
		"redirect_uris":              req.RedirectURIs,
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleAuthorize is the user-facing authorization endpoint. If the
// user is logged in (via the existing OIDC session), we auto-approve
// and 302 back to the client with a fresh code. If not, we redirect
// to /auth/start with a return-to pointing back here so the OIDC
// flow runs first.
func (s *Server) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	codeChallenge := q.Get("code_challenge")
	method := q.Get("code_challenge_method")
	responseType := q.Get("response_type")

	if responseType != "code" {
		oauthError(w, http.StatusBadRequest, "unsupported_response_type", responseType)
		return
	}
	if method != "S256" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "code_challenge_method must be S256")
		return
	}
	if clientID == "" || redirectURI == "" || codeChallenge == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "missing required parameter")
		return
	}

	// Validate the registered client.
	client, err := s.Service.Store.OAuthClient(r.Context(), clientID)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}
	if !slicesContains(client.RedirectURIs, redirectURI) {
		oauthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is not registered for this client")
		return
	}

	// Resolve the user from the session cookie. If absent, kick off
	// the OIDC login and come back here.
	if s.Signer == nil {
		oauthError(w, http.StatusInternalServerError, "server_error", "no signer configured")
		return
	}
	uid, err := s.Signer.Read(r)
	if err != nil {
		dest := "/auth/start?return_to=" + url.QueryEscape(r.URL.RequestURI())
		http.Redirect(w, r, dest, http.StatusFound)
		return
	}
	if _, err := s.Service.Store.UserByID(r.Context(), uid); err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", "user lookup failed")
		return
	}

	// Mint a single-use auth code valid for 60s.
	code := "ac-" + randomID(24)
	if err := s.Service.Store.SaveAuthCode(r.Context(),
		code, clientID, uid, redirectURI, codeChallenge,
		time.Now().Add(60*time.Second)); err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	u, err := url.Parse(redirectURI)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri unparseable")
		return
	}
	qs := u.Query()
	qs.Set("code", code)
	if state != "" {
		qs.Set("state", state)
	}
	u.RawQuery = qs.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// HandleToken exchanges an authorization code for an access token JWT.
// PKCE (S256) is required.
func (s *Server) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code is supported")
		return
	}
	code := r.PostForm.Get("code")
	clientID := r.PostForm.Get("client_id")
	redirectURI := r.PostForm.Get("redirect_uri")
	codeVerifier := r.PostForm.Get("code_verifier")
	if code == "" || clientID == "" || redirectURI == "" || codeVerifier == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "missing required parameter")
		return
	}

	ac, err := s.Service.Store.ConsumeAuthCode(r.Context(), code)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "code is invalid, expired or already used")
		return
	}
	if ac.ClientID != clientID {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "code/client_id mismatch")
		return
	}
	if ac.RedirectURI != redirectURI {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}
	if !auth.VerifyPKCE(codeVerifier, ac.CodeChallenge) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}

	ttl := 24 * time.Hour
	now := time.Now()
	claims := auth.AccessClaims{
		Issuer:    s.publicBase(),
		Subject:   fmt.Sprintf("%d", ac.UserID),
		Audience:  s.Resource,
		ClientID:  ac.ClientID,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	}
	tok, err := s.Signer.SignAccess(claims)
	if err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": tok,
		"token_type":   "Bearer",
		"expires_in":   int(ttl.Seconds()),
	})
}

// publicBase is the canonical origin of this MCP server (e.g.
// "https://ranking.go-ag.levinkeller.de"). Derived from s.Resource by
// stripping the "/mcp" suffix.
func (s *Server) publicBase() string {
	return strings.TrimSuffix(s.Resource, "/mcp")
}

// CORS wraps a handler with the headers MCP clients need to call us
// from a browser context (claude.ai is a web app). Origins are echoed
// rather than whitelisted; the bearer-token auth is what actually
// protects the data.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers",
				"Authorization, Content-Type, MCP-Protocol-Version, Accept")
			w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- helpers -------------------------------------------------------------

func oauthError(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": desc,
	})
}

func randomID(byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		panic(err) // out of entropy — system is broken
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

