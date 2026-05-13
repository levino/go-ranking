package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OIDC implements a minimal OpenID Connect Authorization Code flow
// against a single issuer (Zitadel). It deliberately avoids ID-token
// signature verification: the access token is received over a TLS
// connection from the token endpoint and only used to call userinfo —
// no token ever crosses the trust boundary unsigned, and the user
// identity is taken from the userinfo response.
type OIDC struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Issuer       string
	HTTPClient   *http.Client

	mu       sync.Mutex
	cachedAt time.Time
	cfg      *providerConfig
}

type providerConfig struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// ProviderConfig is the public, immutable view of the OIDC provider's
// metadata document. Callers that need to know the upstream endpoints
// (e.g. an OAuth proxy) use Discover().
type ProviderConfig struct {
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserinfoEndpoint      string
	EndSessionEndpoint    string
	JWKSURI               string
}

// Discover returns the upstream OIDC provider metadata, cached for an
// hour. Used by the OAuth proxy in internal/mcp to point Claude.ai at
// Zitadel's authorization and token endpoints.
func (o *OIDC) Discover(ctx context.Context) (*ProviderConfig, error) {
	cfg, err := o.discover(ctx)
	if err != nil {
		return nil, err
	}
	return &ProviderConfig{
		Issuer:                cfg.Issuer,
		AuthorizationEndpoint: cfg.AuthorizationEndpoint,
		TokenEndpoint:         cfg.TokenEndpoint,
		UserinfoEndpoint:      cfg.UserinfoEndpoint,
		EndSessionEndpoint:    cfg.EndSessionEndpoint,
		JWKSURI:               cfg.JWKSURI,
	}, nil
}

// UserInfo is the subset of OIDC userinfo claims we care about.
type UserInfo struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	PreferredName string `json:"preferred_username"`
}

// DisplayName returns the most user-friendly name available.
func (u UserInfo) DisplayName() string {
	switch {
	case u.Name != "":
		return u.Name
	case u.PreferredName != "":
		return u.PreferredName
	default:
		return u.Email
	}
}

func NewOIDC(issuer, clientID, clientSecret, redirectURL string) *OIDC {
	return &OIDC{
		Issuer:       strings.TrimRight(issuer, "/"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (o *OIDC) discover(ctx context.Context) (*providerConfig, error) {
	o.mu.Lock()
	if o.cfg != nil && time.Since(o.cachedAt) < time.Hour {
		c := o.cfg
		o.mu.Unlock()
		return c, nil
	}
	o.mu.Unlock()

	req, _ := http.NewRequestWithContext(ctx, "GET",
		o.Issuer+"/.well-known/openid-configuration", nil)
	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("discover: HTTP %d", resp.StatusCode)
	}
	cfg := &providerConfig{}
	if err := json.NewDecoder(resp.Body).Decode(cfg); err != nil {
		return nil, fmt.Errorf("discover decode: %w", err)
	}

	o.mu.Lock()
	o.cfg = cfg
	o.cachedAt = time.Now()
	o.mu.Unlock()
	return cfg, nil
}

// AuthURL builds the URL to redirect the user to for login. The caller
// is responsible for storing `state` (typically in a signed cookie) and
// verifying it on callback.
func (o *OIDC) AuthURL(ctx context.Context, state string) (string, error) {
	cfg, err := o.discover(ctx)
	if err != nil {
		return "", err
	}
	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("client_id", o.ClientID)
	v.Set("redirect_uri", o.RedirectURL)
	v.Set("scope", "openid email profile")
	v.Set("state", state)
	return cfg.AuthorizationEndpoint + "?" + v.Encode(), nil
}

// Exchange swaps an authorization code for tokens and returns the
// resulting userinfo. The id_token is discarded — we trust the userinfo
// response from the same TLS-protected exchange.
func (o *OIDC) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	cfg, err := o.discover(ctx)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", o.RedirectURL)

	req, _ := http.NewRequestWithContext(ctx, "POST", cfg.TokenEndpoint,
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(o.ClientID), url.QueryEscape(o.ClientSecret))

	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("token decode: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, errors.New("token exchange: empty access_token")
	}
	return o.Userinfo(ctx, tok.AccessToken)
}

// Userinfo fetches the userinfo endpoint with the given bearer token.
// Used both at the end of the login flow and by the MCP server to
// validate access tokens issued by Zitadel.
func (o *OIDC) Userinfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	cfg, err := o.discover(ctx)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", cfg.UserinfoEndpoint, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("userinfo: HTTP %d: %s", resp.StatusCode, string(body))
	}
	ui := &UserInfo{}
	if err := json.Unmarshal(body, ui); err != nil {
		return nil, fmt.Errorf("userinfo decode: %w", err)
	}
	if ui.Subject == "" {
		return nil, errors.New("userinfo: empty subject")
	}
	return ui, nil
}

// LogoutURL returns Zitadel's RP-initiated logout URL with a post-logout
// redirect. Empty string if the issuer doesn't advertise an
// end_session_endpoint.
func (o *OIDC) LogoutURL(ctx context.Context, postLogoutRedirect string) (string, error) {
	cfg, err := o.discover(ctx)
	if err != nil {
		return "", err
	}
	if cfg.EndSessionEndpoint == "" {
		return "", nil
	}
	v := url.Values{}
	v.Set("client_id", o.ClientID)
	if postLogoutRedirect != "" {
		v.Set("post_logout_redirect_uri", postLogoutRedirect)
	}
	return cfg.EndSessionEndpoint + "?" + v.Encode(), nil
}

// RandomState returns a URL-safe random string suitable for OAuth state.
func RandomState() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
