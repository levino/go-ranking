package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionIssueAndReadRoundtrip(t *testing.T) {
	s := newTestSigner(t)
	rec := httptest.NewRecorder()
	if err := s.Issue(rec, 123, time.Hour); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	uid, err := s.Read(req)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if uid != 123 {
		t.Fatalf("uid = %d, want 123", uid)
	}
}

func TestSessionClearRemovesCookie(t *testing.T) {
	s := newTestSigner(t)
	rec := httptest.NewRecorder()
	s.Clear(rec)
	resp := rec.Result()
	if len(resp.Cookies()) == 0 {
		t.Fatal("clear should emit a cookie with MaxAge<=0")
	}
	c := resp.Cookies()[0]
	if c.MaxAge > 0 {
		t.Errorf("MaxAge=%d, expected ≤ 0", c.MaxAge)
	}
}

func TestSessionReadRejectsMissingCookie(t *testing.T) {
	s := newTestSigner(t)
	req := httptest.NewRequest("GET", "/", nil)
	if _, err := s.Read(req); err == nil {
		t.Fatal("expected error when cookie is absent")
	}
}

func TestSessionReadRejectsForgedCookie(t *testing.T) {
	s := newTestSigner(t)
	req := httptest.NewRequest("GET", "/", nil)
	// Random base64 won't verify against the HMAC.
	req.AddCookie(&http.Cookie{Name: "go_liga_session", Value: "aGVsbG8.d29ybGQ"})
	if _, err := s.Read(req); err == nil {
		t.Fatal("expected error for forged cookie")
	}
}
