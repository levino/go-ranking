package auth

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestHashAndVerifyRoundtrip(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("hunter2", hash)
	if err != nil || !ok {
		t.Fatalf("expected verify ok, got %v %v", ok, err)
	}
	ok, err = VerifyPassword("hunter3", hash)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected verify fail for wrong pw")
	}
}

func TestHashIsDifferentEachTime(t *testing.T) {
	a, _ := HashPassword("x")
	b, _ := HashPassword("x")
	if a == b {
		t.Fatal("hash must include random salt")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	if _, err := VerifyPassword("x", "garbage"); err == nil {
		t.Fatal("expected error for malformed hash")
	}
}

func TestSignerIssueAndRead(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	s := NewSigner(key)

	w := httptest.NewRecorder()
	if err := s.Issue(w, 42, time.Hour); err != nil {
		t.Fatal(err)
	}
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected cookie")
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(cookies[0])
	uid, err := s.Read(r)
	if err != nil {
		t.Fatal(err)
	}
	if uid != 42 {
		t.Fatalf("want 42, got %d", uid)
	}
}

func TestSignerRejectsTamperedCookie(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	s := NewSigner(key)

	w := httptest.NewRecorder()
	_ = s.Issue(w, 42, time.Hour)
	c := w.Result().Cookies()[0]
	// Flip a character
	c.Value = c.Value[:len(c.Value)-2] + "ZZ"

	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(c)
	if _, err := s.Read(r); err == nil {
		t.Fatal("expected error for tampered cookie")
	}
}

func TestSignerRejectsExpired(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	s := NewSigner(key)

	w := httptest.NewRecorder()
	_ = s.Issue(w, 42, -time.Hour) // already expired
	c := w.Result().Cookies()[0]

	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(c)
	if _, err := s.Read(r); err == nil {
		t.Fatal("expected error for expired cookie")
	}
}

func TestSignerRejectsForeignKey(t *testing.T) {
	a := NewSigner([]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	b := NewSigner([]byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

	w := httptest.NewRecorder()
	_ = a.Issue(w, 1, time.Hour)
	c := w.Result().Cookies()[0]

	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(c)
	if _, err := b.Read(r); err == nil {
		t.Fatal("foreign key must reject")
	}
}
