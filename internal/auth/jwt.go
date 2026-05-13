package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AccessClaims is the payload of an OAuth access token JWT issued by
// the go-liga MCP authorization server.
type AccessClaims struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"` // user_id as decimal string
	Audience  string `json:"aud"` // MCP resource URL
	ClientID  string `json:"client_id"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// SignAccess produces an HS256-signed JWT for the given claims, using
// the same HMAC key as the session signer. We use HS256 because the
// MCP endpoint is in the same binary as the token issuer — no need for
// asymmetric signing.
func (s *Signer) SignAccess(claims AccessClaims) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(pb)
	mac := hmac.New(sha256.New, s.Key)
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig, nil
}

// VerifyAccess parses and verifies a JWT issued by SignAccess. It
// checks the signature, issuer, audience and expiry.
func (s *Signer) VerifyAccess(token, issuer, audience string) (*AccessClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed JWT")
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, s.Key)
	mac.Write([]byte(signingInput))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return nil, errors.New("bad signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("bad payload encoding: %w", err)
	}
	var c AccessClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("bad payload json: %w", err)
	}
	if c.Issuer != issuer {
		return nil, fmt.Errorf("issuer mismatch: %q", c.Issuer)
	}
	if c.Audience != audience {
		return nil, fmt.Errorf("audience mismatch: %q", c.Audience)
	}
	if time.Now().Unix() > c.ExpiresAt {
		return nil, errors.New("expired")
	}
	return &c, nil
}

// VerifyPKCE checks that BASE64URL(SHA256(verifier)) == challenge,
// per RFC 7636 method "S256".
func VerifyPKCE(verifier, challenge string) bool {
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == challenge
}
