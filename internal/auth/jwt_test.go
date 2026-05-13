package auth

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func newTestSigner(t *testing.T) *Signer {
	t.Helper()
	keyBytes, _ := hex.DecodeString(strings.Repeat("d", 64))
	return NewSigner(keyBytes)
}

func TestSignAndVerifyRoundtrip(t *testing.T) {
	s := newTestSigner(t)
	claims := AccessClaims{
		Issuer:    "https://example.test",
		Subject:   "42",
		Audience:  "https://example.test/mcp",
		ClientID:  "abc",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	tok, err := s.SignAccess(claims)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.VerifyAccess(tok, claims.Issuer, claims.Audience)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Subject != claims.Subject || got.ClientID != claims.ClientID {
		t.Errorf("claims mismatch: %+v vs %+v", got, claims)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	s := newTestSigner(t)
	tok, _ := s.SignAccess(AccessClaims{
		Issuer: "iss", Subject: "1", Audience: "aud",
		IssuedAt: time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	})
	if _, err := s.VerifyAccess(tok, "iss", "aud"); err == nil {
		t.Fatal("expected expired error")
	}
}

func TestVerifyRejectsWrongIssuer(t *testing.T) {
	s := newTestSigner(t)
	tok, _ := s.SignAccess(AccessClaims{
		Issuer: "iss-a", Audience: "aud",
		IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if _, err := s.VerifyAccess(tok, "iss-b", "aud"); err == nil {
		t.Fatal("expected issuer-mismatch error")
	}
}

func TestVerifyRejectsWrongAudience(t *testing.T) {
	s := newTestSigner(t)
	tok, _ := s.SignAccess(AccessClaims{
		Issuer: "iss", Audience: "aud-a",
		IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if _, err := s.VerifyAccess(tok, "iss", "aud-b"); err == nil {
		t.Fatal("expected audience-mismatch error")
	}
}

func TestVerifyRejectsBadSignature(t *testing.T) {
	s := newTestSigner(t)
	tok, _ := s.SignAccess(AccessClaims{
		Issuer: "iss", Audience: "aud",
		IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	// Flip a byte in the signature segment.
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatal("malformed JWT in fixture")
	}
	// Mangle by appending a char (still base64-url-safe).
	parts[2] += "A"
	if _, err := s.VerifyAccess(strings.Join(parts, "."), "iss", "aud"); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestVerifyRejectsMalformedJWT(t *testing.T) {
	s := newTestSigner(t)
	for _, tok := range []string{"", "a", "a.b", "a.b.c.d", "not.a.jwt"} {
		if _, err := s.VerifyAccess(tok, "iss", "aud"); err == nil {
			t.Errorf("expected error for %q", tok)
		}
	}
}

func TestVerifyPKCEMatchingChallenge(t *testing.T) {
	// Verifier from RFC 7636 example.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if !VerifyPKCE(verifier, challenge) {
		t.Fatal("standard RFC 7636 PKCE pair should verify")
	}
}

func TestVerifyPKCERejectsMismatch(t *testing.T) {
	if VerifyPKCE("anything", "wrong-challenge") {
		t.Fatal("mismatched PKCE pair should not verify")
	}
}
